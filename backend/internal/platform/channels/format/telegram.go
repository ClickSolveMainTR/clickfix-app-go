package format

import (
	"strings"
	"unicode/utf16"
)

// Entity is a Telegram MessageEntity: a formatted range of the text, in
// UTF-16 code units.
type Entity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	URL    string `json:"url,omitempty"`
}

// Rich is plain text plus the entities that format it. The text alone is
// a readable message, so a send that is refused with entities can be
// repeated without them.
type Rich struct {
	Text     string
	Entities []Entity
}

// Len is the text length in UTF-16 code units (Telegram's limits).
func (r Rich) Len() int { return len(utf16.Encode([]rune(r.Text))) }

type richBuilder struct {
	units []uint16
	ents  []Entity
}

func (b *richBuilder) write(s string) { b.units = append(b.units, utf16.Encode([]rune(s))...) }

// span marks what fn writes as one entity; a range that directly continues
// the previous entity of the same kind extends it.
func (b *richBuilder) span(kind, url string, fn func()) {
	start := len(b.units)
	fn()
	n := len(b.units) - start
	if kind == "" || n == 0 {
		return
	}
	if l := len(b.ents) - 1; l >= 0 && b.ents[l].Type == kind && b.ents[l].URL == url && b.ents[l].Offset+b.ents[l].Length == start {
		b.ents[l].Length += n
		return
	}
	b.ents = append(b.ents, Entity{Type: kind, Offset: start, Length: n, URL: url})
}

// inline writes nodes. Bold and italic are applied to each piece of text
// rather than around the whole node, because Telegram refuses code inside
// another entity.
func (b *richBuilder) inline(nodes []Inline, strong, em bool) {
	for _, n := range nodes {
		switch n.T {
		case Text:
			kind := ""
			if strong {
				kind = "bold"
			}
			b.span(kind, "", func() {
				kind := ""
				if em {
					kind = "italic"
				}
				b.span(kind, "", func() { b.write(n.V) })
			})
		case Break:
			b.write("\n")
		case Strong:
			b.inline(n.C, true, em)
		case Em:
			b.inline(n.C, strong, true)
		case Code:
			b.span("code", "", func() { b.write(n.V) })
		case Link:
			bare := len(n.C) == 1 && n.C[0].T == Text && n.C[0].V == n.Href
			switch {
			case bare:
				b.write(n.Href) // Telegram links bare addresses itself
			case strings.HasPrefix(n.Href, "mailto:"):
				b.inline(n.C, strong, em)
				b.write(" (" + address(n.Href) + ")")
			default:
				b.span("text_link", n.Href, func() { b.inline(n.C, strong, em) })
			}
		}
	}
}

// Telegram renders the message as text with entities: bold, italic, code,
// pre and text_link. Nothing is escaped, so no text can break the markup.
func Telegram(src string) Rich {
	var b richBuilder
	for _, blk := range Parse(src) {
		if len(b.units) > 0 {
			if blk.gap {
				b.write("\n\n")
			} else {
				b.write("\n")
			}
		}
		switch blk.T {
		case Paragraph:
			b.inline(blk.C, false, false)
		case List:
			for i, item := range blk.Items {
				if i > 0 {
					b.write("\n")
				}
				mark := "•"
				if blk.Ordered {
					mark = strings.TrimRight(blk.marks[i], ".)") + "."
				}
				b.write(mark + " ")
				b.inline(item, false, false)
			}
		case Pre:
			b.span("pre", "", func() { b.write(blk.V) })
		}
	}
	return Rich{Text: string(utf16.Decode(b.units)), Entities: b.ents}
}

func spaceUnit(u uint16) bool { return u == ' ' || u == '\n' || u == '\t' || u == '\r' }

// Split cuts r into parts of at most limit UTF-16 units, preferring a
// newline, then a space, in the second half of a part. Entities are cut at
// the same places and re-based, so every part keeps its formatting and no
// markup is ever broken. Parts are trimmed; blank parts are dropped.
func (r Rich) Split(limit int) []Rich {
	units := utf16.Encode([]rune(r.Text))
	var out []Rich
	for start := 0; start < len(units); {
		end := min(start+limit, len(units))
		if end < len(units) {
			if utf16.IsSurrogate(rune(units[end-1])) && units[end-1] < 0xDC00 {
				end-- // never between the halves of a surrogate pair
			}
			half := start + (end-start)/2
			if cut := lastUnit(units[start:end], func(u uint16) bool { return u == '\n' }); start+cut > half {
				end = start + cut
			} else if cut := lastUnit(units[start:end], spaceUnit); start+cut > half {
				end = start + cut
			}
		}
		from, to := start, end
		for from < to && spaceUnit(units[from]) {
			from++
		}
		for to > from && spaceUnit(units[to-1]) {
			to--
		}
		if from < to {
			part := Rich{Text: string(utf16.Decode(units[from:to]))}
			for _, e := range r.Entities {
				a, z := max(e.Offset, from), min(e.Offset+e.Length, to)
				if a < z {
					part.Entities = append(part.Entities, Entity{Type: e.Type, Offset: a - from, Length: z - a, URL: e.URL})
				}
			}
			out = append(out, part)
		}
		start = end
	}
	return out
}

// lastUnit returns the index just after the last unit matching f (0 = none).
func lastUnit(us []uint16, f func(uint16) bool) int {
	for i := len(us) - 1; i >= 0; i-- {
		if f(us[i]) {
			return i + 1
		}
	}
	return 0
}
