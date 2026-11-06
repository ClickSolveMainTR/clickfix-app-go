package format

import (
	"html"
	"strconv"
	"strings"
	"unicode/utf8"
)

// style is how one text channel writes the source format.
type style struct {
	// strong and em are the markers written on both sides ("" = none).
	strong, em string
	// boundary: the channel only formats a marker that ends at a word
	// boundary (WhatsApp, Slack). "**Ayarlar**dan" is then written without
	// markers instead of leaving a stray "*" on the customer's screen.
	boundary bool
	// escape rewrites literal text (nil = as is).
	escape func(string) string
	// code and pre write inline code and a code block (nil = bare text).
	code, pre func(string) string
	// link writes a link; bare means the label is the address itself.
	link func(href, label string, bare bool) string
	// bullet replaces unordered list markers ("" = as written); renumber
	// writes ordered markers as "N." instead of as written.
	bullet   string
	renumber bool
	// br separates lines of a paragraph, blockSep blocks ("" = one or two
	// newlines, as in the source).
	br, blockSep string
	// lineStart fixes the beginning of a paragraph line (nil = as is).
	lineStart func(string) string
	// speech drops code blocks and list bullets (text read aloud).
	speech bool
}

// address is a link target as a person reads it.
func address(href string) string { return strings.TrimPrefix(href, "mailto:") }

func plainLink(href, label string, bare bool) string {
	if bare || label == href || label == address(href) {
		return address(href)
	}
	return label + " (" + address(href) + ")"
}

// firstRune is the first character nodes show (-1 = nothing).
func firstRune(nodes []Inline) rune {
	for _, n := range nodes {
		switch n.T {
		case Text, Code:
			if n.V != "" {
				r, _ := utf8.DecodeRuneInString(n.V)
				return r
			}
		case Break:
			return '\n'
		default:
			if r := firstRune(n.C); r != -1 {
				return r
			}
		}
	}
	return -1
}

// inline writes nodes; after is the character that follows them and
// strong/em tell which markers are already open.
func (st *style) inline(nodes []Inline, after rune, strong, em bool) string {
	var b strings.Builder
	for i, n := range nodes {
		next := firstRune(nodes[i+1:])
		if next == -1 {
			next = after
		}
		mark := func(m string, open bool) string {
			if m == "" || open || (st.boundary && isWord(next)) {
				return ""
			}
			return m
		}
		switch n.T {
		case Text:
			if st.escape != nil {
				b.WriteString(st.escape(n.V))
			} else {
				b.WriteString(n.V)
			}
		case Break:
			b.WriteString(st.br)
		case Strong:
			m := mark(st.strong, strong)
			b.WriteString(m + st.inline(n.C, next, true, em) + m)
		case Em:
			m := mark(st.em, em)
			b.WriteString(m + st.inline(n.C, next, strong, true) + m)
		case Code:
			if st.code != nil {
				b.WriteString(st.code(n.V))
			} else {
				b.WriteString(n.V)
			}
		case Link:
			bare := len(n.C) == 1 && n.C[0].T == Text && n.C[0].V == n.Href
			label := st.inline(n.C, next, strong, em)
			if bare {
				label = n.Href
			}
			b.WriteString(st.link(n.Href, label, bare))
		}
	}
	return b.String()
}

func (st *style) render(src string) string {
	var b strings.Builder
	for _, blk := range Parse(src) {
		var s string
		switch blk.T {
		case Paragraph:
			s = st.inline(blk.C, -1, false, false)
			if st.lineStart != nil {
				lines := strings.Split(s, st.br)
				for i := range lines {
					lines[i] = st.lineStart(lines[i])
				}
				s = strings.Join(lines, st.br)
			}
		case List:
			lines := make([]string, 0, len(blk.Items))
			for i, item := range blk.Items {
				mark := blk.marks[i]
				switch {
				case blk.Ordered && st.renumber:
					mark = strings.TrimRight(mark, ".)") + "."
				case !blk.Ordered && st.speech:
					mark = ""
				case !blk.Ordered && st.bullet != "":
					mark = st.bullet
				}
				if mark != "" {
					mark += " "
				}
				lines = append(lines, mark+st.inline(item, -1, false, false))
			}
			s = strings.Join(lines, "\n")
		case Pre:
			switch {
			case st.speech:
				continue
			case st.pre != nil:
				s = st.pre(blk.V)
			default:
				s = blk.V
			}
		}
		if b.Len() > 0 {
			switch {
			case st.blockSep != "":
				b.WriteString(st.blockSep)
			case blk.gap:
				b.WriteString("\n\n")
			default:
				b.WriteString("\n")
			}
		}
		b.WriteString(s)
	}
	return b.String()
}

func backticks(v string) string { return "`" + v + "`" }
func fence(v string) string     { return "```\n" + v + "\n```" }

// Plain is the message without formatting marks, for channels that show
// text as typed (SMS, Instagram, Messenger): a named link becomes
// "label (address)", lists and line breaks stay.
func Plain(src string) string {
	st := style{link: plainLink, br: "\n"}
	return st.render(src)
}

// Strip is Plain for excerpts: a named link keeps only its label.
func Strip(src string) string {
	st := style{br: "\n", link: func(href, label string, bare bool) string {
		if bare {
			return href
		}
		return label
	}}
	return st.render(src)
}

// Speech is the message as text to read aloud (voice): no marks, no code
// blocks, no bullets, and a named link is only its label. Inline code is
// kept: it is usually an order number or a name the caller needs. A
// message that is nothing but a code block falls back to Plain so the call
// is not empty.
func Speech(src string) string {
	st := style{speech: true, br: "\n", link: func(href, label string, bare bool) string {
		if bare {
			return address(href)
		}
		return label
	}}
	if out := strings.TrimSpace(st.render(src)); out != "" {
		return out
	}
	return Plain(src)
}

// WhatsApp writes WhatsApp's own marks: *bold*, _italic_, `code`,
// ```block```, "- " and "1. " lists. WhatsApp has no named links.
func WhatsApp(src string) string {
	st := style{strong: "*", em: "_", boundary: true, code: backticks, pre: fence, link: plainLink, bullet: "-", renumber: true, br: "\n"}
	return st.render(src)
}

var (
	slackText = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	slackHref = strings.NewReplacer("&", "&amp;", "<", "%3C", ">", "%3E", "|", "%7C")
)

// Slack writes mrkdwn: *bold*, _italic_, <url|label>, with the control
// characters & < > escaped everywhere (Slack decodes exactly these three).
func Slack(src string) string {
	st := style{
		strong: "*", em: "_", boundary: true, escape: slackText.Replace, bullet: "•", renumber: true, br: "\n",
		code: func(v string) string { return backticks(slackText.Replace(v)) },
		pre:  func(v string) string { return fence(slackText.Replace(v)) },
		link: func(href, label string, bare bool) string {
			if bare {
				return "<" + slackHref.Replace(href) + ">"
			}
			return "<" + slackHref.Replace(href) + "|" + label + ">"
		},
	}
	return st.render(src)
}

func markdownLink(href, label string, bare bool) string {
	if bare {
		return href
	}
	return "[" + label + "](" + href + ")"
}

// GoogleChat writes the Markdown syntax of the Chat API (the request must
// carry markupSyntax MARKUP_SYNTAX_MARKDOWN): **bold**, *italic*, `code`,
// fenced blocks, [label](url), "- " and "1. " lists. Text is not escaped:
// Chat documents no escape character.
func GoogleChat(src string) string {
	st := style{strong: "**", em: "*", code: backticks, pre: fence, link: markdownLink, bullet: "-", renumber: true, br: "\n"}
	return st.render(src)
}

// commonMarkText escapes what CommonMark would read as markup in literal
// text: "*" and "_" that touch a word, backticks, "[", "\\" and the HTML
// characters. "5 * 3" and snake_case are left as typed.
func commonMarkText(s string) string {
	var b strings.Builder
	for i, r := range s {
		prev, next := before(s, i), at(s, i+utf8.RuneLen(r))
		switch r {
		case '\\', '`', '[':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '*':
			if (prev == -1 || isSpace(prev)) && (next == -1 || isSpace(next)) {
				b.WriteRune(r)
			} else {
				b.WriteString(`\*`)
			}
		case '_':
			if isWord(prev) && isWord(next) || (prev == -1 || isSpace(prev)) && (next == -1 || isSpace(next)) {
				b.WriteRune(r)
			} else {
				b.WriteString(`\_`)
			}
		case '<':
			b.WriteString("&lt;")
		case '&':
			b.WriteString("&amp;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Teams writes CommonMark for textFormat "extendedmarkdown": blocks are
// separated by a blank line, a line break inside a paragraph is a hard
// break (two spaces before the newline) and literal markup is escaped.
func Teams(src string) string {
	st := style{
		strong: "**", em: "*", escape: commonMarkText, code: backticks, pre: fence, link: markdownLink,
		bullet: "-", renumber: true, br: "  \n", blockSep: "\n\n",
		lineStart: func(l string) string {
			if strings.HasPrefix(l, "#") || strings.HasPrefix(l, ">") {
				return `\` + l
			}
			return l
		},
	}
	return st.render(src)
}

// HTML renders the message as e-mail body markup: <p>, <br>, <strong>,
// <em>, <code>, <pre>, <ul>/<ol> and <a>. All text is escaped; only http,
// https and mailto addresses become links (the parser accepts no others).
func HTML(src string) string {
	var b strings.Builder
	for i, blk := range Parse(src) {
		if i > 0 {
			b.WriteString("\n")
		}
		switch blk.T {
		case Paragraph:
			b.WriteString("<p>" + htmlInline(blk.C) + "</p>")
		case Pre:
			b.WriteString(`<pre style="white-space:pre-wrap"><code>` + html.EscapeString(blk.V) + "</code></pre>")
		case List:
			tag := "<ul>"
			if blk.Ordered {
				tag = "<ol>"
				if n, err := strconv.Atoi(strings.TrimRight(blk.marks[0], ".)")); err == nil && n != 1 {
					tag = `<ol start="` + strconv.Itoa(n) + `">`
				}
			}
			b.WriteString(tag)
			for _, item := range blk.Items {
				b.WriteString("<li>" + htmlInline(item) + "</li>")
			}
			b.WriteString("</" + tag[1:3] + ">")
		}
	}
	return b.String()
}

func htmlInline(nodes []Inline) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n.T {
		case Text:
			b.WriteString(html.EscapeString(n.V))
		case Break:
			b.WriteString("<br>")
		case Strong:
			b.WriteString("<strong>" + htmlInline(n.C) + "</strong>")
		case Em:
			b.WriteString("<em>" + htmlInline(n.C) + "</em>")
		case Code:
			b.WriteString("<code>" + html.EscapeString(n.V) + "</code>")
		case Link:
			b.WriteString(`<a href="` + html.EscapeString(n.Href) + `">` + htmlInline(n.C) + "</a>")
		}
	}
	return b.String()
}
