package email

import (
	"html"
	"net/textproto"
	"regexp"
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
)

// Quote markers that start the quoted history of a reply (English and
// Turkish clients: Gmail, Outlook, Apple Mail, Thunderbird).
var quoteStartRE = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*On .{3,200} wrote:\s*$`),
	regexp.MustCompile(`(?i)^\s*.{3,200}\s(tarihinde|zamanında)\s.{0,200}yazdı:\s*$`),
	regexp.MustCompile(`(?i)^\s*.{3,200}\sşunu yazdı:\s*$`),
	regexp.MustCompile(`(?i)^\s*Le .{3,200} a écrit\s*:\s*$`),
	regexp.MustCompile(`(?i)^\s*Am .{3,200} schrieb .{0,200}:\s*$`),
	regexp.MustCompile(`(?i)^\s*-{2,}\s*(Original Message|Orijinal (İleti|Mesaj)|Özgün İleti|Forwarded message|İletilen ileti)\s*-{2,}\s*$`),
	regexp.MustCompile(`^\s*_{20,}\s*$`),
	regexp.MustCompile(`(?i)^\s*(From|Kimden|Gönderen)\s*:\s*.+$`),
}

// Lines that only advertise the client.
var mobileFooterRE = regexp.MustCompile(`(?i)^\s*(Sent from my .{1,40}|iPhone'umdan gönderildi|iPad'imden gönderildi|Android için Outlook'u edinin|Get Outlook for .{1,20}|Samsung Galaxy .{0,30}ile gönderildi)\s*$`)

// outlookHeaderRE matches the second line of an Outlook quote header block
// ("From: …" followed by "Sent: …"/"Gönderildi: …").
var outlookHeaderRE = regexp.MustCompile(`(?i)^\s*(Sent|Date|Gönderildi|Tarih|To|Kime)\s*:`)

// VisibleReply returns the new part of a reply: the text above the quoted
// history, without the "-- " signature and mobile footers. When stripping
// leaves nothing (a forward, a fully quoted mail) the whole text is kept.
func VisibleReply(text string) string {
	lines := strings.Split(normalizeNewlines(text), "\n")
	cut := len(lines)
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), ">") && quotedTail(lines[i:]) {
			cut = i
			break
		}
		if line == "-- " || strings.TrimSpace(line) == "--" {
			cut = i
			break
		}
		if startsQuote(lines, i) {
			cut = i
			break
		}
		// "On Tue, … <a@b.c>\nwrote:" wrapped over two lines.
		if i+1 < len(lines) && quoteStartRE[0].MatchString(line+" "+lines[i+1]) && !quoteStartRE[0].MatchString(lines[i+1]) {
			cut = i
			break
		}
	}
	kept := make([]string, 0, cut)
	for _, line := range lines[:cut] {
		if mobileFooterRE.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	if out == "" {
		return strings.TrimSpace(text)
	}
	return out
}

func startsQuote(lines []string, i int) bool {
	line := lines[i]
	for k, re := range quoteStartRE {
		if !re.MatchString(line) {
			continue
		}
		if k == len(quoteStartRE)-1 {
			// "From:" alone is only a quote header when the next lines are
			// the rest of an Outlook header block.
			for j := i + 1; j < len(lines) && j <= i+3; j++ {
				if outlookHeaderRE.MatchString(lines[j]) {
					return true
				}
			}
			return false
		}
		return true
	}
	return false
}

// quotedTail reports whether the remaining lines are quoted ("> …") or
// blank, i.e. the quote is the bottom of the mail, not inline answers.
func quotedTail(lines []string) bool {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, ">") || mobileFooterRE.MatchString(t) {
			continue
		}
		return false
	}
	return true
}

var (
	policyOnce sync.Once
	policy     *bluemonday.Policy
)

// SanitizeHTML makes an inbound HTML body safe to show: scripts, event
// handlers, forms, iframes and remote-loading tricks are dropped, links
// open in a new tab with rel=noopener. The inbox renders it in a sandboxed
// iframe as a second line of defence.
func SanitizeHTML(in string) string {
	policyOnce.Do(func() {
		p := bluemonday.UGCPolicy()
		p.AllowStyling()
		p.AllowAttrs("style").Globally()
		p.AllowElements("center", "font", "span", "div", "table", "thead", "tbody", "tfoot", "tr", "td", "th", "colgroup", "col")
		p.AllowAttrs("color", "face", "size").OnElements("font")
		p.AllowAttrs("align", "valign", "bgcolor", "width", "height", "border", "cellpadding", "cellspacing").Globally()
		p.AllowURLSchemes("http", "https", "mailto", "tel", "cid")
		p.RequireNoReferrerOnLinks(true)
		p.AddTargetBlankToFullyQualifiedLinks(true)
		policy = p
	})
	return strings.TrimSpace(policy.Sanitize(in))
}

var (
	blockTagRE  = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/tr|/li|/h[1-6]|/blockquote)\s*/?>`)
	styleRE     = regexp.MustCompile(`(?is)<(style|script|head)[^>]*>.*?</(style|script|head)>`)
	anyTagRE    = regexp.MustCompile(`(?s)<[^>]*>`)
	blankRunsRE = regexp.MustCompile(`\n{3,}`)
)

// HTMLToText renders an HTML body as plain text (for HTML-only mails).
func HTMLToText(in string) string {
	s := styleRE.ReplaceAllString(in, "")
	s = blockTagRE.ReplaceAllString(s, "\n")
	s = anyTagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(normalizeNewlines(s), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(strings.ReplaceAll(l, " ", " "), " \t")
	}
	return strings.TrimSpace(blankRunsRE.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// TextToHTML renders an agent's plain text reply as simple HTML.
func TextToHTML(text string) string {
	var b strings.Builder
	b.WriteString(`<div style="font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;font-size:14px;line-height:1.5">`)
	for i, para := range strings.Split(strings.TrimSpace(normalizeNewlines(text)), "\n\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(html.EscapeString(para), "\n", "<br>"))
		b.WriteString("</p>")
	}
	b.WriteString("</div>")
	return b.String()
}

// AutoReason reports why a message must not open or continue a
// conversation: an automatic reply (RFC 3834), a bulk or list mail, or a
// message ClickFix sent itself (loop). "" means a human wrote it.
func AutoReason(h textproto.MIMEHeader, ownMarker string) string {
	if ownMarker != "" && strings.EqualFold(strings.TrimSpace(h.Get(HeaderInstance)), ownMarker) {
		return "loop"
	}
	if v := strings.ToLower(strings.TrimSpace(h.Get("Auto-Submitted"))); v != "" && v != "no" {
		return "auto_submitted"
	}
	switch strings.ToLower(strings.TrimSpace(h.Get("Precedence"))) {
	case "bulk", "junk", "list", "auto_reply":
		return "precedence"
	}
	if h.Get("X-Autoreply") != "" || h.Get("X-Autorespond") != "" || strings.EqualFold(h.Get("X-Auto-Response-Suppress"), "All") {
		return "auto_reply"
	}
	if h.Get("List-Id") != "" || h.Get("List-Unsubscribe") != "" {
		return "list"
	}
	return ""
}
