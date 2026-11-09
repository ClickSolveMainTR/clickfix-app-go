package usecase

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/model"
)

// Field picking from a real answer ("alanlar cevaptan tıklanarak seçilir").
// Paths are dotted ("customer.name", "lines.0.amount"); the items path
// points at the array (or object) that holds the concept's records.

const (
	maxItems    = 20
	maxPaths    = 200
	maxDepth    = 6
	maxFields   = 30
	sampleChars = 120
)

var pathRxp = regexp.MustCompile(`^[A-Za-z0-9_\-]{1,64}(\.[A-Za-z0-9_\-]{1,64}){0,7}$`)

// ValidPath checks a dotted path ("" is the root).
func ValidPath(p string) bool { return p == "" || pathRxp.MatchString(p) }

// Get returns the value at a dotted path.
func Get(v any, path string) (any, bool) {
	if path == "" {
		return v, true
	}
	cur := v
	for _, part := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[part]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			cur = node[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// Items returns the records under itemsPath: an array's elements, or a
// single object.
func Items(body any, itemsPath string) []any {
	v, ok := Get(body, itemsPath)
	if !ok || v == nil {
		return nil
	}
	switch node := v.(type) {
	case []any:
		if len(node) > maxItems {
			node = node[:maxItems]
		}
		return node
	case map[string]any:
		return []any{node}
	default:
		return nil
	}
}

// Pick keeps the chosen fields of each item, keyed by label (or path).
// With no fields chosen, scalar top-level values are kept.
func Pick(items []any, fields []model.Field) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row := map[string]any{}
		if len(fields) == 0 {
			if m, ok := item.(map[string]any); ok {
				for k, v := range m {
					if scalar(v) && len(row) < maxFields {
						row[k] = v
					}
				}
			}
		}
		for _, f := range fields {
			if v, ok := Get(item, f.Path); ok {
				key := strings.TrimSpace(f.Label)
				if key == "" {
					key = f.Path
				}
				row[key] = v
			}
		}
		out = append(out, row)
	}
	return out
}

func scalar(v any) bool {
	switch v.(type) {
	case nil, string, float64, bool:
		return true
	}
	return false
}

// Paths lists the leaves of v (arrays are walked through their first
// element) with a short sample of each value.
func Paths(v any) []model.PathOption {
	out := []model.PathOption{}
	var walk func(node any, prefix string, depth int)
	walk = func(node any, prefix string, depth int) {
		if len(out) >= maxPaths || depth > maxDepth {
			return
		}
		switch n := node.(type) {
		case map[string]any:
			keys := make([]string, 0, len(n))
			for k := range n {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(n[k], join(prefix, k), depth+1)
			}
		case []any:
			if len(n) > 0 {
				walk(n[0], join(prefix, "0"), depth+1)
			}
		default:
			if prefix != "" && ValidPath(prefix) {
				out = append(out, model.PathOption{Path: prefix, Type: typeOf(n), Sample: sample(n)})
			}
		}
	}
	walk(v, "", 0)
	return out
}

// ArrayPaths lists the paths of arrays of objects: candidates for the
// items path.
func ArrayPaths(v any) []string {
	out := []string{}
	var walk func(node any, prefix string, depth int)
	walk = func(node any, prefix string, depth int) {
		if len(out) >= 20 || depth > maxDepth {
			return
		}
		switch n := node.(type) {
		case map[string]any:
			keys := make([]string, 0, len(n))
			for k := range n {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(n[k], join(prefix, k), depth+1)
			}
		case []any:
			if len(n) > 0 {
				if _, ok := n[0].(map[string]any); ok && ValidPath(prefix) {
					out = append(out, prefix)
				}
			}
		}
	}
	walk(v, "", 0)
	return out
}

func join(prefix, k string) string {
	if prefix == "" {
		return k
	}
	return prefix + "." + k
}

func typeOf(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	}
	return "object"
}

func sample(v any) any {
	if s, ok := v.(string); ok {
		return clipText(s, sampleChars)
	}
	if v == nil {
		return nil
	}
	if f, ok := v.(float64); ok {
		return f
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return fmt.Sprint(v)
}
