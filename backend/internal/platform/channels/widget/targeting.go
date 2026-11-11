package widget

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
)

// PageContext is what is known of the page and visitor when the widget
// loads. The SDK evaluates the same rules in the browser on every SPA
// navigation (frontend/widget-sdk/src/shared/targeting.ts); both sides are
// tested against testdata/targeting_cases.json.
type PageContext struct {
	URL        string `json:"url"`
	Device     string `json:"device"`
	Country    string `json:"country"`
	Language   string `json:"language"`
	Returning  bool   `json:"returning"`
	Identified bool   `json:"identified"`
}

var mobileUARe = regexp.MustCompile(`(?i)Mobi|Android|iPhone|iPod|Windows Phone|BlackBerry|Opera Mini`)

// DeviceOf classifies a User-Agent as "mobile" or "desktop" (tablets count
// as desktop: the widget has room there).
func DeviceOf(userAgent string) string {
	if mobileUARe.MatchString(userAgent) && !strings.Contains(strings.ToLower(userAgent), "ipad") {
		return "mobile"
	}
	return "desktop"
}

// MatchURL reports whether a page URL matches one rule. Matching ignores
// case and the fragment; a value starting with "/" is compared with the
// path and query only, anything else with the whole URL.
func MatchURL(r URLRule, page string) bool {
	target := strings.ToLower(page)
	if i := strings.IndexByte(target, '#'); i >= 0 {
		target = target[:i]
	}
	value := strings.ToLower(strings.TrimSpace(r.Value))
	if strings.HasPrefix(value, "/") {
		if u, err := url.Parse(target); err == nil {
			target = u.EscapedPath()
			if target == "" {
				target = "/"
			}
			if u.RawQuery != "" {
				target += "?" + u.RawQuery
			}
		}
	}
	switch r.Op {
	case "equals":
		return strings.TrimSuffix(target, "/") == strings.TrimSuffix(value, "/")
	case "starts_with":
		return strings.HasPrefix(target, value)
	case "ends_with":
		return strings.HasSuffix(target, value)
	case "glob":
		return globMatch(value, target)
	case "not_contains":
		return !strings.Contains(target, value)
	default:
		return strings.Contains(target, value)
	}
}

// globMatch matches "*" as any run of characters, anchored at both ends.
func globMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(s, p)
		if i < 0 {
			return false
		}
		s = s[i+len(p):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// MatchRules applies a rule list: when there are positive rules at least
// one must match; every "not_contains" rule must hold. No rules match all.
func MatchRules(rules []URLRule, page string) bool {
	positive, hit := false, false
	for _, r := range rules {
		if r.Op == "not_contains" {
			if !MatchURL(r, page) {
				return false
			}
			continue
		}
		positive = true
		if MatchURL(r, page) {
			hit = true
		}
	}
	return !positive || hit
}

// Visible reports whether the widget loads on this page and, when not, why
// ("device", "country", "hidden_page", "not_listed_page").
func (t Targeting) Visible(ctx PageContext) (bool, string) {
	if t.Devices != "" && t.Devices != "all" && ctx.Device != "" && t.Devices != ctx.Device {
		return false, "device"
	}
	if cc := strings.ToUpper(ctx.Country); cc != "" {
		in := slices.Contains(t.Countries.List, cc)
		if (t.Countries.Mode == "allow" && !in) || (t.Countries.Mode == "block" && in) {
			return false, "country"
		}
	}
	if ctx.URL != "" {
		for _, r := range t.HideOn {
			if MatchURL(r, ctx.URL) {
				return false, "hidden_page"
			}
		}
		if len(t.ShowOn) > 0 && !MatchRules(t.ShowOn, ctx.URL) {
			return false, "not_listed_page"
		}
	}
	return true, ""
}

// Eligible reports whether the page-level conditions of a trigger hold.
// Time on page, scroll depth and exit intent are left to the browser.
func (t Trigger) Eligible(ctx PageContext) bool {
	c := t.Conditions
	if !t.Enabled {
		return false
	}
	if ctx.URL != "" && !MatchRules(c.URL, ctx.URL) {
		return false
	}
	switch {
	case c.Visitor == "new" && ctx.Returning, c.Visitor == "returning" && !ctx.Returning:
		return false
	case c.Device != "" && c.Device != "any" && ctx.Device != "" && c.Device != ctx.Device:
		return false
	case c.Identified == "yes" && !ctx.Identified, c.Identified == "no" && ctx.Identified:
		return false
	}
	if len(c.Languages) > 0 && ctx.Language != "" && !slices.Contains(c.Languages, strings.ToLower(ctx.Language)) {
		return false
	}
	if len(c.Countries) > 0 && ctx.Country != "" && !slices.Contains(c.Countries, strings.ToUpper(ctx.Country)) {
		return false
	}
	if c.UTMSource != "" || c.UTMMedium != "" || c.UTMCampaign != "" {
		q := url.Values{}
		if u, err := url.Parse(ctx.URL); err == nil {
			q = u.Query()
		}
		for key, want := range map[string]string{"utm_source": c.UTMSource, "utm_medium": c.UTMMedium, "utm_campaign": c.UTMCampaign} {
			if want != "" && !strings.EqualFold(q.Get(key), want) {
				return false
			}
		}
	}
	return true
}

// EligibleTriggers returns the ids of the triggers whose page-level
// conditions hold.
func (s Settings) EligibleTriggers(ctx PageContext) []string {
	out := []string{}
	for _, t := range s.Triggers {
		if t.Eligible(ctx) {
			out = append(out, t.ID)
		}
	}
	return out
}

// ReplyEstimate is the "we usually reply …" line of the widget.
type ReplyEstimate struct {
	// Value: "minutes", "hour", "hours", "day" or "" (hidden).
	Value string `json:"value"`
	// NextOpenAt is the next opening time while outside business hours.
	NextOpenAt *time.Time `json:"next_open_at"`
}

// EstimateReply picks the reply time line: hidden, the manual value, or
// (auto) the bucket of the recent median first response; nil median falls
// back to "minutes" while online and "day" otherwise. hours is the
// organization's working-hours state (CF-198).
func (s Settings) EstimateReply(hours businesshours.State, median *time.Duration) ReplyEstimate {
	out := ReplyEstimate{NextOpenAt: hours.NextOpenAt}
	switch s.ReplyTime.Mode {
	case "hidden":
		return out
	case "manual":
		out.Value = s.ReplyTime.Value
		return out
	}
	switch {
	case median == nil && s.Online(hours.Open):
		out.Value = "minutes"
	case median == nil:
		out.Value = "day"
	case *median <= 10*time.Minute:
		out.Value = "minutes"
	case *median <= time.Hour:
		out.Value = "hour"
	case *median <= 8*time.Hour:
		out.Value = "hours"
	default:
		out.Value = "day"
	}
	return out
}
