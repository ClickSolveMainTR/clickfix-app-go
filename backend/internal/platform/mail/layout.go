package mail

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"unicode/utf8"
)

// LogoCID is the Content-ID of the embedded ClickFix tile in branded mail.
const LogoCID = "logo@getclickfix.com"

// SiteURL is the public marketing site linked from every footer.
const SiteURL = "https://getclickfix.com"

//go:embed layout.html.tmpl
var layoutSource string

// logoPNG is frontend/src/assets/brand/clickfix-tile-green.svg rendered to
// 80x80 (shown at 40px). Email clients do not render SVG.
//
//go:embed assets/logo.png
var logoPNG []byte

var layoutTmpl = template.Must(template.New("layout").Parse(layoutSource))

// Layout is the content of one branded email. Every outgoing message uses
// the same layout (CF-155): Title becomes the heading, Body is plain text
// whose paragraphs, "Label: value" rows, codes and links are laid out.
type Layout struct {
	// Lang is "tr" (default) or "en"; it picks the fixed footer copy.
	Lang string
	// Eyebrow is the small category label above the heading (optional).
	Eyebrow string
	Title   string
	Body    string
	// ActionURL renders a full-width button (absolute http(s) only).
	ActionURL   string
	ActionLabel string
	// PreferencesURL adds a notification preferences link to the footer.
	PreferencesURL string
}

type layoutView struct {
	Lang, Title, Preheader, Eyebrow string
	LogoSrc                         template.URL
	Blocks                          []block
	ActionURL, ActionLabel          string
	FallbackHint                    string
	SentBy, SiteURL                 string
	PreferencesURL                  string
	PreferencesLabel                string
	MsoOpen, MsoClose               template.HTML
}

type block struct {
	Kind  string // text | details | code | links
	Lines [][]textPart
	Rows  []detailRow
	Links []textPart
	Label string
	Code  string
}

type textPart struct{ Text, URL string }

type detailRow struct{ Key, Value string }

type footerCopy struct{ sentBy, prefs, action, fallback string }

var footers = map[string]footerCopy{
	"tr": {"Bu e-posta ClickFix tarafından gönderildi.", "Bildirim tercihleri", "Görüntüle", "Düğme çalışmazsa bu bağlantıyı tarayıcınıza yapıştırın:"},
	"en": {"This email was sent by ClickFix.", "Notification preferences", "Open", "If the button does not work, paste this link into your browser:"},
}

// Outlook desktop ignores max-width; these ghost tables hold it at 600px.
// html/template drops comments, so they are passed as trusted constants.
const (
	msoOpen  = `<!--[if mso]><table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" align="center"><tr><td><![endif]-->`
	msoClose = `<!--[if mso]></td></tr></table><![endif]-->`
)

// Branded builds a multipart message (plain text + HTML + inline logo) from
// l. Subject is l.Title; the plain-text part keeps the body as written.
func Branded(to []string, l Layout) (Message, error) {
	text, html, err := RenderLayout(l)
	if err != nil {
		return Message{}, err
	}
	return Message{
		To: to, Subject: l.Title, Body: text, HTML: html,
		Inline: []Inline{{CID: LogoCID, Filename: "clickfix.png", ContentType: "image/png", Data: logoPNG}},
	}, nil
}

// RenderLayout returns the plain-text and HTML bodies of l. All values are
// escaped by html/template; only http(s) links become anchors.
func RenderLayout(l Layout) (string, string, error) {
	fc, ok := footers[l.Lang]
	if !ok {
		l.Lang = "tr"
		fc = footers["tr"]
	}
	action := safeURL(l.ActionURL)
	label := strings.TrimSpace(l.ActionLabel)
	if label == "" {
		label = fc.action
	}
	prefs := safeURL(l.PreferencesURL)
	view := layoutView{
		Lang: l.Lang, Title: l.Title, Eyebrow: l.Eyebrow,
		Preheader: preheader(l.Body),
		LogoSrc:   template.URL("cid:" + LogoCID),
		Blocks:    parseBody(l.Body),
		ActionURL: action, ActionLabel: label, FallbackHint: fc.fallback,
		SentBy: fc.sentBy, SiteURL: SiteURL,
		PreferencesURL: prefs, PreferencesLabel: fc.prefs,
		MsoOpen: msoOpen, MsoClose: msoClose,
	}
	var buf bytes.Buffer
	if err := layoutTmpl.Execute(&buf, view); err != nil {
		return "", "", fmt.Errorf("mail: render layout: %w", err)
	}
	return plainText(l.Body, action, label, prefs, fc), buf.String(), nil
}

func plainText(body, action, label, prefs string, fc footerCopy) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(normalizeNewlines(body)))
	if action != "" && !strings.Contains(body, action) {
		b.WriteString("\n\n")
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(action)
	}
	b.WriteString("\n\n--\n")
	b.WriteString(fc.sentBy)
	b.WriteString("\n")
	b.WriteString(SiteURL)
	if prefs != "" {
		b.WriteString("\n")
		b.WriteString(fc.prefs)
		b.WriteString(": ")
		b.WriteString(prefs)
	}
	b.WriteString("\n")
	return b.String()
}

var (
	urlRe  = regexp.MustCompile(`https?://[^\s<>"']+`)
	codeRe = regexp.MustCompile(`^[A-Z0-9-]{4,12}$`)
)

// parseBody turns template text into layout blocks. Paragraphs are split on
// blank lines; inside one, consecutive lines are grouped by shape:
//   - "Label: https://…"  → a link whose text is the label
//   - "Label: 123456"     → a code box (sign-in and confirmation codes)
//   - two or more "Label: value" rows → the tinted details table
//   - anything else       → text, with bare http(s) URLs linked
func parseBody(body string) []block {
	var out []block
	for _, para := range strings.Split(normalizeNewlines(body), "\n\n") {
		lines := nonEmptyLines(para)
		if len(lines) == 0 {
			continue
		}
		out = append(out, groupLines(lines)...)
	}
	return out
}

func groupLines(lines []string) []block {
	type item struct {
		kind       string
		key, value string
		line       string
	}
	items := make([]item, 0, len(lines))
	for _, ln := range lines {
		key, value, ok := labelValue(ln)
		switch {
		case ok && isOnlyURL(value):
			items = append(items, item{kind: "link", key: key, value: value})
		case ok && isCode(value):
			items = append(items, item{kind: "code", key: key, value: value})
		case ok:
			items = append(items, item{kind: "row", key: key, value: value, line: ln})
		default:
			items = append(items, item{kind: "text", line: ln})
		}
	}
	// A lone "Label: value" row reads better as text ("Not: …").
	for i := range items {
		if items[i].kind != "row" {
			continue
		}
		prev := i > 0 && items[i-1].kind == "row"
		next := i+1 < len(items) && items[i+1].kind == "row"
		if !prev && !next {
			items[i].kind = "text"
		}
	}
	var out []block
	for i := 0; i < len(items); {
		j := i
		for j < len(items) && items[j].kind == items[i].kind && items[i].kind != "code" {
			j++
		}
		if items[i].kind == "code" {
			j = i + 1
		}
		group := items[i:j]
		switch items[i].kind {
		case "link":
			b := block{Kind: "links"}
			for _, it := range group {
				b.Links = append(b.Links, textPart{Text: it.key, URL: it.value})
			}
			out = append(out, b)
		case "code":
			out = append(out, block{Kind: "code", Label: group[0].key, Code: group[0].value})
		case "row":
			b := block{Kind: "details"}
			for _, it := range group {
				b.Rows = append(b.Rows, detailRow{Key: it.key, Value: it.value})
			}
			out = append(out, b)
		default:
			b := block{Kind: "text"}
			for _, it := range group {
				b.Lines = append(b.Lines, linkify(it.line))
			}
			out = append(out, b)
		}
		i = j
	}
	return out
}

// labelValue splits "Label: value" when the label is short and plain.
func labelValue(line string) (string, string, bool) {
	idx := strings.Index(line, ": ")
	if idx <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+2:])
	if value == "" || utf8.RuneCountInString(key) > 48 || strings.Contains(key, "://") ||
		strings.ContainsAny(key, ".!?") {
		return "", "", false
	}
	return key, value, true
}

func isOnlyURL(v string) bool {
	return !strings.ContainsAny(v, " \t") && safeURL(v) == v
}

func isCode(v string) bool {
	return codeRe.MatchString(v) && strings.ContainsAny(v, "0123456789")
}

func linkify(line string) []textPart {
	var parts []textPart
	last := 0
	for _, m := range urlRe.FindAllStringIndex(line, -1) {
		start, end := m[0], m[1]
		// Sentence punctuation after a URL is not part of it.
		for end > start && strings.ContainsRune(".,;:!?)", rune(line[end-1])) {
			end--
		}
		if start > last {
			parts = append(parts, textPart{Text: line[last:start]})
		}
		u := line[start:end]
		parts = append(parts, textPart{Text: u, URL: u})
		last = end
	}
	if last < len(line) {
		parts = append(parts, textPart{Text: line[last:]})
	}
	return parts
}

// safeURL keeps absolute http(s) URLs without whitespace; anything else is
// dropped so a template variable can never become a javascript: link.
func safeURL(raw string) string {
	u := strings.TrimSpace(raw)
	lower := strings.ToLower(u)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		return ""
	}
	if strings.ContainsAny(u, " \t\r\n<>\"") {
		return ""
	}
	return u
}

func preheader(body string) string {
	for _, ln := range nonEmptyLines(normalizeNewlines(body)) {
		if strings.HasSuffix(ln, ",") {
			continue // "Merhaba Ayşe," says nothing in an inbox preview
		}
		if r := []rune(ln); len(r) > 110 {
			return string(r[:110]) + "…"
		}
		return ln
	}
	return ""
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}
