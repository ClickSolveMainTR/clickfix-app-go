package capability

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Compaction limits of a read result sent to the model (CF-211).
const (
	// compactStringRunes clips long free text (message bodies, articles).
	compactStringRunes = 400
	// compactMinStringRunes is the clip used when the result still does
	// not fit after the lists were shortened.
	compactMinStringRunes = 120
)

// droppedKeys never help the model and cost tokens.
var droppedKeys = map[string]bool{
	"avatar_url": true, "avatar": true, "logo_url": true, "icon_url": true, "thumbnail_url": true,
	"etag": true, "version_hash": true, "search_vector": true, "embedding": true,
}

// TruncatedMarker ends every text the compaction clipped. A write
// capability refuses params that carry it (Check), so a clipped article or
// message is never written back over the full one.
const TruncatedMarker = "[TRUNCATED"

func clipText(r []rune, n, total int) string {
	return string(r[:n]) + fmt.Sprintf("…%s: %d of %d chars shown; read-only, never send this text to a write tool]", TruncatedMarker, n, total)
}

// clipString clips s to n runes; a string clipped before is clipped again
// from its kept text, keeping the original length in the marker.
func clipString(s string, n int) string {
	if i := strings.Index(s, "…"+TruncatedMarker); i >= 0 {
		var shown, total int
		if _, err := fmt.Sscanf(s[i+len("…"+TruncatedMarker):], ": %d of %d", &shown, &total); err != nil {
			total = utf8.RuneCountInString(s)
		}
		kept := []rune(s[:i])
		if len(kept) <= n {
			return s
		}
		return clipText(kept, n, total)
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return clipText(r, n, len(r))
}

// HasTruncated reports whether a param value (at any depth) holds clipped text.
func HasTruncated(v any) bool {
	switch x := v.(type) {
	case string:
		return strings.Contains(x, TruncatedMarker)
	case map[string]any:
		for _, e := range x {
			if HasTruncated(e) {
				return true
			}
		}
	case Params:
		return HasTruncated(map[string]any(x))
	case []any:
		for _, e := range x {
			if HasTruncated(e) {
				return true
			}
		}
	}
	return false
}

// Compacted is a read result shrunk to fit the model's budget.
type Compacted struct {
	Data any `json:"data"`
	// Note says which lists were shortened and how to get the rest.
	Note string `json:"note,omitempty"`
}

// Compact shrinks data to at most budget bytes of JSON: it drops empty
// values and noisy keys, clips long text, then keeps the first items of
// the longest lists and says so (the model can ask for the next page with
// limit/offset or narrower filters). ok is false when even that does not
// fit.
func Compact(data any, budget int) (Compacted, bool) {
	return compact(data, budget, compactStringRunes)
}

// CompactFull is Compact for single-record reads (an article, a
// conversation's messages): text is kept whole and only clipped, with
// TruncatedMarker, when the record does not fit the budget otherwise.
func CompactFull(data any, budget int) (Compacted, bool) {
	return compact(data, budget, 0)
}

func compact(data any, budget, clip int) (Compacted, bool) {
	var v any
	switch x := data.(type) {
	case json.RawMessage:
		if json.Unmarshal(x, &v) != nil {
			return Compacted{}, false
		}
	case []byte:
		if json.Unmarshal(x, &v) != nil {
			return Compacted{}, false
		}
	default:
		raw, err := json.Marshal(x)
		if err != nil || json.Unmarshal(raw, &v) != nil {
			return Compacted{}, false
		}
	}
	v = clean(v, clip)
	out := Compacted{Data: v}
	if size(out) <= budget {
		return out, true
	}
	shortened := map[string][2]int{}
	for size(out) > budget {
		path, arr := longestList(out.Data, "")
		if len(arr) <= 1 {
			break
		}
		keep := (len(arr) + 1) / 2
		if keep >= len(arr) {
			keep = len(arr) - 1
		}
		total := len(arr)
		if prev, ok := shortened[path]; ok {
			total = prev[1]
		}
		shortened[path] = [2]int{keep, total}
		out.Data = shortenList(out.Data, path, "", keep)
		out.Note = listNote(shortened)
	}
	for _, n := range []int{2000, 1000, compactStringRunes, compactMinStringRunes} {
		if size(out) <= budget {
			break
		}
		if clip == 0 || n < clip {
			out.Data = clean(out.Data, n)
		}
	}
	return out, size(out) <= budget
}

func size(v any) int {
	raw, _ := json.Marshal(v)
	return len(raw)
}

func listNote(s map[string][2]int) string {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		name := k
		if name == "" {
			name = "list"
		}
		parts = append(parts, fmt.Sprintf("%s: first %d of %d", name, s[k][0], s[k][1]))
	}
	return "Shortened (" + strings.Join(parts, "; ") + "). Ask for the next page (offset/limit) or narrow the filters; never guess the rest."
}

func clean(v any, maxRunes int) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if droppedKeys[k] {
				continue
			}
			e = clean(e, maxRunes)
			if empty(e) {
				continue
			}
			out[k] = e
		}
		return out
	case []any:
		out := make([]any, 0, len(x))
		for _, e := range x {
			e = clean(e, maxRunes)
			if e != nil {
				out = append(out, e)
			}
		}
		return out
	case string:
		if maxRunes > 0 {
			return clipString(x, maxRunes)
		}
		return x
	}
	return v
}

func empty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

// longestList finds the list with the most JSON bytes.
func longestList(v any, path string) (string, []any) {
	bestPath, best, bestSize := "", []any(nil), -1
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				p := k
				if path != "" {
					p = path + "." + k
				}
				walk(e, p)
			}
		case []any:
			if n := size(x); len(x) > 1 && n > bestSize {
				bestPath, best, bestSize = path, x, n
			}
			for _, e := range x {
				walk(e, path+"[]")
			}
		}
	}
	walk(v, path)
	return bestPath, best
}

// shortenList keeps the first keep items of every list at path (a path
// inside a list, "items[].messages", addresses that list in each item).
func shortenList(v any, path, cur string, keep int) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			p := k
			if cur != "" {
				p = cur + "." + k
			}
			x[k] = shortenList(e, path, p, keep)
		}
		return x
	case []any:
		if cur == path && len(x) > keep {
			x = x[:keep]
		}
		for i, e := range x {
			x[i] = shortenList(e, path, cur+"[]", keep)
		}
		return x
	}
	return v
}
