// Package format translates a message from ClickFix's source format (the
// widget's small Markdown subset) into what each channel renders (CF-177).
//
// The source format is what agents and AI write and what the database
// keeps: **bold**, *italic* / _italic_, `code`, ``` code blocks,
// [label](https://…), bare links, "- " and "1. " lists. Adapters call one
// of the renderers inside Send, so the stored content never changes and a
// retried send is never translated twice.
//
// Parse mirrors frontend/widget-sdk/src/shared/markdown.ts; both read
// testdata/markdown_cases.json, so the widget, the inbox and the channels
// agree on what is bold. The package is pure: no I/O, no dependencies.
package format

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Inline node types.
const (
	Text   = "text"
	Strong = "strong"
	Em     = "em"
	Code   = "code"
	Link   = "link"
	Break  = "br"
)

// Block node types.
const (
	Paragraph = "p"
	List      = "list"
	Pre       = "pre"
)

// Inline is one inline node: text and code carry V, strong, em and link
// carry children, link also Href.
type Inline struct {
	T    string
	V    string
	Href string
	C    []Inline
}

// Block is one block: a paragraph (C), a list (Items) or a code block (V).
type Block struct {
	T       string
	C       []Inline
	Ordered bool
	Items   [][]Inline
	V       string

	// marks are the list markers as written ("-", "•", "3.", "2)"), so plain
	// text keeps "24. madde" as it was. Not part of the shared token tree.
	marks []string
	// gap: a blank line separated this block from the previous one.
	gap bool
}

var (
	linkRE = regexp.MustCompile(`^\[([^\]\n]{1,300})\]\((https?://[^\s)]+|mailto:[^\s)]+)\)`)
	autoRE = regexp.MustCompile(`^https?://[^\s<>"']+[^\s<>"'.,;:!?)]`)
	itemRE = regexp.MustCompile(`^[ \t]*(?:([-*•])|(\d{1,3})([.)]))[ \t]+(.*)$`)
)

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == ' ' }

// isWord: a letter or digit of the Basic Multilingual Plane (the widget
// tests one UTF-16 unit, so astral letters are not word characters there
// either).
func isWord(r rune) bool {
	return r < 0x10000 && (unicode.IsLetter(r) || unicode.IsNumber(r))
}

// before and at return the rune ending before / starting at byte i (-1 at
// the edges).
func before(s string, i int) rune {
	if i <= 0 {
		return -1
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return r
}

func at(s string, i int) rune {
	if i >= len(s) {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r
}

// closeStrong finds the "**" that closes a strong opened at rest[0:2]: the
// first one after at least one character that does not follow a space.
func closeStrong(rest string) int {
	from := 3
	for from < len(rest) {
		e := strings.Index(rest[from:], "**")
		if e < 0 {
			return -1
		}
		e += from
		if !isSpace(before(rest, e)) {
			return e
		}
		from = e + 1
	}
	return -1
}

// closeEm finds the closing "*" or "_" of an emphasis opened at rest[0]:
// not after a space, not part of a doubled marker and, for "_", not inside
// a word (snake_case stays text).
func closeEm(rest string) int {
	d := rest[0]
	for e := 2; e < len(rest); e++ {
		if rest[e] != d || rest[e-1] == d || isSpace(before(rest, e)) {
			continue
		}
		if e+1 < len(rest) && rest[e+1] == d {
			continue
		}
		if d == '_' && isWord(at(rest, e+1)) {
			continue
		}
		return e
	}
	return -1
}

// ParseInline tokenizes one line.
func ParseInline(src string) []Inline {
	var out []Inline
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			out = append(out, Inline{T: Text, V: text.String()})
			text.Reset()
		}
	}
	for i := 0; i < len(src); {
		rest := src[i:]
		if rest[0] == '`' {
			if end := strings.IndexByte(rest[1:], '`'); end >= 1 {
				flush()
				out = append(out, Inline{T: Code, V: rest[1 : 1+end]})
				i += end + 2
				continue
			}
		}
		// A marker opens only at a word boundary and before a non-space,
		// so 2*3*4, snake_case and a lone * stay text.
		opens := !isWord(before(src, i))
		if opens && strings.HasPrefix(rest, "**") && len(rest) > 2 && !isSpace(at(rest, 2)) {
			if end := closeStrong(rest); end > 0 {
				flush()
				out = append(out, Inline{T: Strong, C: ParseInline(rest[2:end])})
				i += end + 2
				continue
			}
		}
		if opens && (rest[0] == '*' || rest[0] == '_') && len(rest) > 1 && rest[1] != rest[0] && !isSpace(at(rest, 1)) {
			if end := closeEm(rest); end > 0 {
				flush()
				out = append(out, Inline{T: Em, C: ParseInline(rest[1:end])})
				i += end + 1
				continue
			}
		}
		if m := linkRE.FindStringSubmatch(rest); m != nil {
			flush()
			out = append(out, Inline{T: Link, Href: m[2], C: ParseInline(m[1])})
			i += len(m[0])
			continue
		}
		if p := before(src, i); p == -1 || isSpace(p) || p == '(' {
			if m := autoRE.FindString(rest); m != "" {
				flush()
				out = append(out, Inline{T: Link, Href: m, C: []Inline{{T: Text, V: m}}})
				i += len(m)
				continue
			}
		}
		_, n := utf8.DecodeRuneInString(rest)
		text.WriteString(rest[:n])
		i += n
	}
	flush()
	return out
}

// Parse tokenizes a message into blocks.
func Parse(src string) []Block {
	src = strings.ReplaceAll(strings.ReplaceAll(src, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(src, "\n")
	var blocks []Block
	var para []Inline
	gap, paraGap := false, false
	push := func(b Block) {
		b.gap = gap && len(blocks) > 0
		gap = false
		blocks = append(blocks, b)
	}
	endPara := func() {
		if len(para) > 0 {
			blocks = append(blocks, Block{T: Paragraph, C: para, gap: paraGap && len(blocks) > 0})
		}
		para = nil
	}
	for n := 0; n < len(lines); n++ {
		line := lines[n]
		if strings.HasPrefix(line, "```") {
			endPara()
			var body []string
			for n++; n < len(lines) && !strings.HasPrefix(lines[n], "```"); n++ {
				body = append(body, lines[n])
			}
			push(Block{T: Pre, V: strings.Join(body, "\n")})
			continue
		}
		if m := itemRE.FindStringSubmatch(line); m != nil {
			endPara()
			ordered := m[2] != ""
			mark := m[1]
			if ordered {
				mark = m[2] + m[3]
			}
			content := ParseInline(m[4])
			// Like the widget, items join the previous list of the same kind
			// even across a blank line.
			if last := len(blocks) - 1; last >= 0 && blocks[last].T == List && blocks[last].Ordered == ordered {
				gap = false
				blocks[last].Items = append(blocks[last].Items, content)
				blocks[last].marks = append(blocks[last].marks, mark)
			} else {
				push(Block{T: List, Ordered: ordered, Items: [][]Inline{content}, marks: []string{mark}})
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			endPara()
			gap = true
			continue
		}
		if len(para) > 0 {
			para = append(para, Inline{T: Break})
		} else {
			paraGap = gap
			gap = false
		}
		para = append(para, ParseInline(line)...)
	}
	endPara()
	return blocks
}
