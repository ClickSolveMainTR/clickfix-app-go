// Package content builds the text of a notification once, for every
// channel (CF-157). A Message answers who, what, which customer or
// conversation, when and what to do next; Enqueue turns it into the in-app
// row, the e-mail and the lock-screen-safe push of the same event.
//
// E-mail is plain text shaped for the shared branded layout
// (platform/mail, CF-155): a lead paragraph, "Label: value" detail rows and
// "Label: https://…" links. The layout reads those lines as a details table
// and a link list; without it the same text reads well as plain text.
package content

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
)

// Push categories: what a push can do besides opening (web notification
// actions, mobile category buttons).
const (
	// CategoryConversation: reply, mark read, open.
	CategoryConversation = "conversation"
	// CategoryConversationAssign: as conversation, plus "assign to me" (the
	// conversation has no assignee).
	CategoryConversationAssign = "conversation_assign"
	// CategoryGeneric: mark read, open.
	CategoryGeneric = "generic"
)

// Action ids. The first action of a Message is the primary call to action.
const (
	ActionOpen = "open"
	ActionJoin = "join"
	// ActionSecondary is any further link (calendar, settings).
	ActionSecondary = "link"
	// ActionDesktop opens the primary action in ClickFix Masaüstü (CF-236).
	// E-mail only, and only for users with an active desktop session.
	ActionDesktop = "desktop"
)

// DesktopOpenPath is the web page behind "Masaüstünde aç": it hands the
// path to the desktop app (clickfix://) and falls back to the browser. An
// https link, because many mail clients drop custom-scheme links.
const DesktopOpenPath = "/desktop/open"

// Detail is one "Label: value" row.
type Detail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Action is a call to action. URL is an app path ("/inbox?c=…") or an
// absolute URL; it only ever opens a page (state changes need a click on
// that page: no GET side effects from e-mail links).
type Action struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Push is the lock-screen text: no customer names, message text or
// amounts, only the kind of event and harmless references (conversation
// number, channel, time).
type Push struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Message is one notification in one language.
type Message struct {
	Event string
	Lang  string
	// Title is the e-mail subject and the in-app title.
	Title string
	// Lead is the first paragraph: who did what. It doubles as the e-mail
	// preheader (inbox preview line).
	Lead string
	// Quote is user content shown as a quotation (message preview, note,
	// survey comment); already stripped of HTML and shortened.
	Quote      string
	QuoteLabel string
	Paragraphs []string
	Details    []Detail
	Actions    []Action
	Push       Push
	Category   string
}

// Primary returns the main call to action, if any.
func (m Message) Primary() (Action, bool) {
	for _, a := range m.Actions {
		if a.URL != "" {
			return a, true
		}
	}
	return Action{}, false
}

// WithDesktopAction adds "Masaüstünde aç" after the actions when the
// primary action is an app path; otherwise m is returned unchanged.
func (m Message) WithDesktopAction() Message {
	primary, ok := m.Primary()
	if !ok || !strings.HasPrefix(primary.URL, "/") || strings.HasPrefix(primary.URL, "//") {
		return m
	}
	actions := make([]Action, 0, len(m.Actions)+1)
	actions = append(actions, m.Actions...)
	m.Actions = append(actions, Action{
		ID:    ActionDesktop,
		Label: t(m.Lang, "Masaüstünde aç", "Open in ClickFix Desktop"),
		URL:   DesktopOpenPath + "?path=" + url.QueryEscape(primary.URL),
	})
	return m
}

// Summary is the in-app body: the lead, the quote and up to three details
// on one line.
func (m Message) Summary() string {
	parts := []string{m.Lead}
	if m.Quote != "" {
		parts = append(parts, "“"+m.Quote+"”")
	}
	var rows []string
	for _, d := range m.Details {
		if len(rows) == 3 {
			break
		}
		rows = append(rows, d.Label+": "+d.Value)
	}
	if len(rows) > 0 {
		parts = append(parts, strings.Join(rows, " · "))
	}
	return strings.TrimSpace(strings.Join(nonEmpty(parts), "\n"))
}

// EmailText is the plain-text e-mail body. Relative action URLs are made
// absolute with appURL (the public web origin).
func (m Message) EmailText(appURL string) string {
	return m.emailText(appURL, true)
}

// EmailBody is EmailText without the primary call to action: the branded
// layout (platform/mail, CF-155) renders that one as the button from the
// row's action_url and adds it to the plain-text part itself.
func (m Message) EmailBody(appURL string) string {
	return m.emailText(appURL, false)
}

func (m Message) emailText(appURL string, withPrimary bool) string {
	var blocks []string
	blocks = append(blocks, m.Lead)
	if m.Quote != "" {
		label := m.QuoteLabel
		if label == "" {
			label = t(m.Lang, "Mesaj", "Message")
		}
		blocks = append(blocks, label+":\n“"+m.Quote+"”")
	}
	blocks = append(blocks, m.Paragraphs...)
	if len(m.Details) > 0 {
		rows := make([]string, 0, len(m.Details))
		for _, d := range m.Details {
			rows = append(rows, d.Label+": "+d.Value)
		}
		blocks = append(blocks, strings.Join(rows, "\n"))
	}
	var links []string
	primary, hasPrimary := m.Primary()
	for _, a := range m.Actions {
		if !withPrimary && hasPrimary && a == primary {
			continue
		}
		if u := Absolute(appURL, a.URL); u != "" {
			links = append(links, a.Label+": "+u)
		}
	}
	if len(links) > 0 {
		blocks = append(blocks, strings.Join(links, "\n"))
	}
	return strings.Join(nonEmpty(blocks), "\n\n")
}

// PayloadValue is the structured form stored on the notification row
// (payload.content) for clients that lay it out themselves.
func (m Message) PayloadValue() map[string]any {
	details := m.Details
	if details == nil {
		details = []Detail{}
	}
	actions := m.Actions
	if actions == nil {
		actions = []Action{}
	}
	out := map[string]any{"preheader": m.Lead, "details": details, "actions": actions}
	if m.Quote != "" {
		out["quote"] = m.Quote
	}
	return out
}

// Absolute resolves an app path against appURL; absolute http(s) URLs are
// kept, anything else is dropped.
func Absolute(appURL, link string) string {
	link = strings.TrimSpace(link)
	switch {
	case link == "":
		return ""
	case strings.HasPrefix(link, "https://"), strings.HasPrefix(link, "http://"):
		return link
	case strings.HasPrefix(link, "/") && !strings.HasPrefix(link, "//"):
		if appURL == "" {
			return ""
		}
		return strings.TrimRight(appURL, "/") + link
	}
	return ""
}

// Lang maps a locale ("tr-TR", "EN", "") to tr | en (default tr).
func Lang(v string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(v)), "en") {
		return "en"
	}
	return "tr"
}

func t(lang, tr, en string) string {
	if lang == "en" {
		return en
	}
	return tr
}

var (
	tagRE   = regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</(script|style)>|<[^>]*>`)
	spaceRE = regexp.MustCompile(`[\s\p{Z}]+`)
)

// Preview turns message content (plain text, Markdown-ish or HTML) into a
// one-line excerpt of at most n characters: tags and entities removed,
// whitespace collapsed, cut on a word with "…".
func Preview(s string, n int) string {
	s = tagRE.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = spaceRE.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n]
	cut := string(r)
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:-") + "…"
}

// Duration is a short human duration: "45 dk", "2 sa 5 dk", "3 gün"
// (en "45 min", "2 h 5 min", "3 days"). Negative durations read as zero.
func Duration(d time.Duration, lang string) string {
	if d < 0 {
		d = 0
	}
	mins := int(d.Round(time.Minute) / time.Minute)
	days, hours, m := mins/1440, (mins%1440)/60, mins%60
	switch {
	case days > 0 && hours > 0:
		return strconv.Itoa(days) + t(lang, " gün ", plural(days, " day ", " days ")) + strconv.Itoa(hours) + t(lang, " sa", " h")
	case days > 0:
		return strconv.Itoa(days) + t(lang, " gün", plural(days, " day", " days"))
	case hours > 0 && m > 0:
		return strconv.Itoa(hours) + t(lang, " sa ", " h ") + strconv.Itoa(m) + t(lang, " dk", " min")
	case hours > 0:
		return strconv.Itoa(hours) + t(lang, " sa", " h")
	}
	return strconv.Itoa(m) + t(lang, " dk", " min")
}

// When formats an instant in the organization's time zone with the zone
// name: "2 Kasım 2026 Pazartesi, 10:00 (Europe/Istanbul)".
func When(at time.Time, timezone, lang string) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" {
		loc, _ = time.LoadLocation("Europe/Istanbul")
	}
	if loc == nil {
		loc = time.UTC
	}
	return i18n.FormatDateTime(at, loc, i18n.Normalize(lang)) + " (" + loc.String() + ")"
}

// Clock is the time of day in the organization's time zone ("14:30").
func Clock(at time.Time, timezone, lang string) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" {
		loc, _ = time.LoadLocation("Europe/Istanbul")
	}
	return i18n.FormatClock(at, loc, i18n.Normalize(lang))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func nonEmpty(in []string) []string {
	out := in[:0:0]
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
