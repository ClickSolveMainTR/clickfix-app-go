package widget

import (
	"fmt"
	"github.com/google/uuid"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Issue is one rejected settings value. Path is the dotted JSON path
// ("appearance.launcher.size", "triggers.2.message"), so the editor can put
// the message next to the field.
type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Issue codes.
const (
	IssueInvalid    = "invalid"
	IssueTooLong    = "too_long"
	IssueRange      = "out_of_range"
	IssueDuplicate  = "duplicate"
	IssueLimit      = "limit"
	IssueRequired   = "required"
	IssueUnsafe     = "unsafe"
	IssueUnknownKey = "unknown_key"
)

// Normalize reads stored settings with fallbacks for every missing or
// invalid value.
func Normalize(raw map[string]any) Settings {
	s, _ := normalize(raw)
	return s
}

// Validate normalizes raw strictly: every value that Normalize would drop
// or replace is reported. Publishing requires an empty result; drafts are
// saved with their issues so the editor can show them inline.
func Validate(raw map[string]any) []Issue {
	_, issues := normalize(raw)
	return issues
}

// topKeys are the settings keys this package (or replay.go) reads.
// "business_hours" is accepted and ignored: widgets stored before CF-198
// carry it; the hours now come from the SLA module's schedule.
var topKeys = []string{
	"schema_version", "color", "position", "locale", "greeting_title", "greeting_text", "show_team",
	"team_member_uuids", "prechat", "offline_message", "allowed_domains", "require_identity",
	"file_upload", "csat", "replay", "appearance", "texts", "reply_time", "home", "quick_replies", "csat_options",
	"offline", "away_mode", "channels", "ai", "attachments", "features", "triggers", "targeting", "privacy",
	"rate_limit", "mobile", "attribute_mapping", "business_hours",
}

type norm struct{ issues []Issue }

func (n *norm) add(path, code, format string, args ...any) {
	n.issues = append(n.issues, Issue{Path: path, Code: code, Message: fmt.Sprintf(format, args...)})
}

func join(path string, key any) string {
	if path == "" {
		return fmt.Sprint(key)
	}
	return path + "." + fmt.Sprint(key)
}

// obj reads a nested object; a non-object value is an issue.
func (n *norm) obj(path string, v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}
	m, ok := v.(map[string]any)
	if !ok {
		n.add(path, IssueInvalid, "must be an object")
	}
	return m, ok
}

func (n *norm) list(path string, v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	l, ok := v.([]any)
	if !ok {
		n.add(path, IssueInvalid, "must be a list")
	}
	return l, ok
}

func (n *norm) str(path string, v any, max int) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		n.add(path, IssueInvalid, "must be text")
		return ""
	}
	s = strings.TrimSpace(s)
	if len([]rune(s)) > max {
		n.add(path, IssueTooLong, "at most %d characters", max)
		return clip(s, max)
	}
	return s
}

func (n *norm) bool(path string, v any, def bool) bool {
	if v == nil {
		return def
	}
	b, ok := v.(bool)
	if !ok {
		n.add(path, IssueInvalid, "must be true or false")
		return def
	}
	return b
}

func (n *norm) enum(path string, v any, allowed []string, def string) string {
	if v == nil {
		return def
	}
	if s, ok := v.(string); ok && slices.Contains(allowed, s) {
		return s
	}
	n.add(path, IssueInvalid, "must be one of %s", strings.Join(allowed, ", "))
	return def
}

func (n *norm) color(path string, v any, def string) string {
	s := n.str(path, v, 7)
	if s == "" {
		return def
	}
	if !colorRe.MatchString(s) {
		n.add(path, IssueInvalid, "must be a #rrggbb color")
		return def
	}
	return strings.ToLower(s)
}

// int reads a whole JSON number within [lo, hi].
func (n *norm) int(path string, v any, lo, hi, def int) int {
	if v == nil {
		return def
	}
	var x int
	switch f := v.(type) {
	case float64:
		if f != float64(int(f)) {
			n.add(path, IssueInvalid, "must be a whole number")
			return def
		}
		x = int(f)
	case int:
		x = f
	default:
		n.add(path, IssueInvalid, "must be a number")
		return def
	}
	if x < lo || x > hi {
		n.add(path, IssueRange, "must be between %d and %d", lo, hi)
		return def
	}
	return x
}

// url keeps absolute https URLs (http only for localhost), so stored links
// never carry javascript: or data: URLs.
func (n *norm) url(path string, v any) string {
	s := n.str(path, v, 1000)
	if s == "" {
		return ""
	}
	if out := safeURL(s); out != "" {
		return out
	}
	n.add(path, IssueUnsafe, "must be an https:// address")
	return ""
}

func (n *norm) localized(path string, v any, max int) Localized {
	out := Localized{}
	switch x := v.(type) {
	case nil:
	case string:
		// A plain string is the text of every locale.
		if s := n.str(path, x, max); s != "" {
			for _, loc := range Locales {
				out[loc] = s
			}
		}
	case map[string]any:
		for k, val := range x {
			if !slices.Contains(Locales, k) {
				n.add(join(path, k), IssueUnknownKey, "unsupported language")
				continue
			}
			if s := n.str(join(path, k), val, max); s != "" {
				out[k] = s
			}
		}
	default:
		n.add(path, IssueInvalid, "must be text per language")
	}
	return out
}

func (n *norm) strs(path string, v any, max, maxLen int) []string {
	out := []string{}
	list, ok := n.list(path, v)
	if !ok {
		return out
	}
	for i, item := range list {
		s := n.str(join(path, i), item, maxLen)
		if s == "" {
			continue
		}
		if len(out) >= max {
			n.add(path, IssueLimit, "at most %d items", max)
			break
		}
		out = append(out, s)
	}
	return out
}

func (n *norm) keys(path string, m map[string]any, allowed ...string) {
	for k := range m {
		if !slices.Contains(allowed, k) {
			n.add(join(path, k), IssueUnknownKey, "unknown setting")
		}
	}
}

func normalize(raw map[string]any) (Settings, []Issue) {
	s := Defaults()
	n := &norm{}
	if raw == nil {
		return s, nil
	}
	n.keys("", raw, topKeys...)
	if v, ok := raw["schema_version"]; ok {
		n.int("schema_version", v, 1, SchemaVersion, SchemaVersion)
	}
	s.Color = n.color("color", raw["color"], s.Color)
	s.Position = n.enum("position", raw["position"], []string{"right", "left"}, s.Position)
	s.Locale = n.enum("locale", raw["locale"], append([]string{"auto"}, Locales...), s.Locale)
	s.GreetingTitle = n.str("greeting_title", raw["greeting_title"], 80)
	s.GreetingText = n.str("greeting_text", raw["greeting_text"], 300)
	s.OfflineMessage = n.str("offline_message", raw["offline_message"], 300)
	s.ShowTeam = n.bool("show_team", raw["show_team"], s.ShowTeam)
	s.RequireIdentity = n.bool("require_identity", raw["require_identity"], false)
	s.FileUpload = n.bool("file_upload", raw["file_upload"], s.FileUpload)
	s.CSAT = n.bool("csat", raw["csat"], s.CSAT)
	s.AwayMode = n.bool("away_mode", raw["away_mode"], false)
	s.TeamMemberUUIDs = n.strs("team_member_uuids", raw["team_member_uuids"], 8, 36)
	for i, d := range n.strs("allowed_domains", raw["allowed_domains"], 50, 255) {
		dom := normalizeDomain(d)
		switch {
		case dom == "":
			n.add(join("allowed_domains", i), IssueInvalid, "%q is not a domain", d)
		case !slices.Contains(s.AllowedDomains, dom):
			s.AllowedDomains = append(s.AllowedDomains, dom)
		}
	}
	if m, ok := n.obj("prechat", raw["prechat"]); ok {
		n.preChat(&s.PreChat, m)
	}
	if m, ok := n.obj("appearance", raw["appearance"]); ok {
		n.appearance(&s.Appearance, m)
	}
	if m, ok := n.obj("texts", raw["texts"]); ok {
		for k, v := range m {
			if !slices.Contains(Locales, k) {
				n.add(join("texts", k), IssueUnknownKey, "unsupported language")
				continue
			}
			t, _ := n.obj(join("texts", k), v)
			p := join("texts", k)
			n.keys(p, t, "greeting_title", "greeting_text", "team_intro", "offline_message", "away_message", "placeholder")
			s.Texts[k] = Texts{
				GreetingTitle: n.str(join(p, "greeting_title"), t["greeting_title"], 80), GreetingText: n.str(join(p, "greeting_text"), t["greeting_text"], 300),
				TeamIntro: n.str(join(p, "team_intro"), t["team_intro"], 200), OfflineMessage: n.str(join(p, "offline_message"), t["offline_message"], 300),
				AwayMessage: n.str(join(p, "away_message"), t["away_message"], 300), Placeholder: n.str(join(p, "placeholder"), t["placeholder"], 80),
			}
		}
	}
	if m, ok := n.obj("reply_time", raw["reply_time"]); ok {
		n.keys("reply_time", m, "mode", "value")
		s.ReplyTime.Mode = n.enum("reply_time.mode", m["mode"], ReplyModes, s.ReplyTime.Mode)
		s.ReplyTime.Value = n.enum("reply_time.value", m["value"], ReplyValues, s.ReplyTime.Value)
	}
	if m, ok := n.obj("home", raw["home"]); ok {
		n.keys("home", m, "cards")
		if list, ok := n.list("home.cards", m["cards"]); ok {
			s.Home.Cards = n.cards(list)
		}
	}
	if list, ok := n.list("quick_replies", raw["quick_replies"]); ok {
		for i, item := range list {
			l := n.localized(join("quick_replies", i), item, 120)
			if len(l) == 0 {
				continue
			}
			if len(s.QuickReplies) >= MaxQuickReplies {
				n.add("quick_replies", IssueLimit, "at most %d quick replies", MaxQuickReplies)
				break
			}
			s.QuickReplies = append(s.QuickReplies, l)
		}
	}
	if m, ok := n.obj("csat_options", raw["csat_options"]); ok {
		n.keys("csat_options", m, "scale", "comment", "question")
		s.CSATOptions.Scale = n.enum("csat_options.scale", m["scale"], CSATScales, s.CSATOptions.Scale)
		s.CSATOptions.Comment = n.bool("csat_options.comment", m["comment"], s.CSATOptions.Comment)
		s.CSATOptions.Question = n.localized("csat_options.question", m["question"], 160)
	}
	if m, ok := n.obj("offline", raw["offline"]); ok {
		n.keys("offline", m, "mode", "collect_email")
		s.Offline.Mode = n.enum("offline.mode", m["mode"], OfflineModes, s.Offline.Mode)
		s.Offline.CollectEmail = n.bool("offline.collect_email", m["collect_email"], s.Offline.CollectEmail)
	}
	if m, ok := n.obj("channels", raw["channels"]); ok {
		n.channels(&s.Channels, m)
	}
	if m, ok := n.obj("ai", raw["ai"]); ok {
		n.keys("ai", m, "enabled", "human_button", "handoff_keywords")
		s.AI.Enabled = n.bool("ai.enabled", m["enabled"], s.AI.Enabled)
		s.AI.HumanButton = n.bool("ai.human_button", m["human_button"], s.AI.HumanButton)
		for _, k := range n.strs("ai.handoff_keywords", m["handoff_keywords"], 30, 60) {
			s.AI.HandoffKeywords = append(s.AI.HandoffKeywords, strings.ToLower(k))
		}
	}
	if m, ok := n.obj("attachments", raw["attachments"]); ok {
		n.keys("attachments", m, "max_mb", "types")
		s.Attachments.MaxMB = n.int("attachments.max_mb", m["max_mb"], 1, 10, s.Attachments.MaxMB)
		if _, ok := m["types"]; ok {
			s.Attachments.Types = []string{}
			for i, t := range n.strs("attachments.types", m["types"], len(AttachmentTypes), 20) {
				switch {
				case !slices.Contains(AttachmentTypes, t):
					n.add(join("attachments.types", i), IssueInvalid, "must be one of %s", strings.Join(AttachmentTypes, ", "))
				case !slices.Contains(s.Attachments.Types, t):
					s.Attachments.Types = append(s.Attachments.Types, t)
				}
			}
		}
	}
	if m, ok := n.obj("features", raw["features"]); ok {
		n.keys("features", m, "emoji", "typing", "read_receipts", "sound", "notifications", "markdown", "history")
		f := &s.Features
		f.Emoji = n.bool("features.emoji", m["emoji"], f.Emoji)
		f.Typing = n.bool("features.typing", m["typing"], f.Typing)
		f.ReadReceipts = n.bool("features.read_receipts", m["read_receipts"], f.ReadReceipts)
		f.Sound = n.bool("features.sound", m["sound"], f.Sound)
		f.Notifications = n.bool("features.notifications", m["notifications"], f.Notifications)
		f.Markdown = n.bool("features.markdown", m["markdown"], f.Markdown)
		f.History = n.bool("features.history", m["history"], f.History)
	}
	if list, ok := n.list("triggers", raw["triggers"]); ok {
		seen := map[string]bool{}
		for i, item := range list {
			p := join("triggers", i)
			m, ok := n.obj(p, item)
			if !ok {
				continue
			}
			t, ok := n.trigger(p, m)
			if !ok {
				continue
			}
			if seen[t.ID] {
				n.add(join(p, "id"), IssueDuplicate, "trigger ids must be unique")
				continue
			}
			if len(s.Triggers) >= MaxTriggers {
				n.add("triggers", IssueLimit, "at most %d triggers", MaxTriggers)
				break
			}
			seen[t.ID] = true
			s.Triggers = append(s.Triggers, t)
		}
	}
	if m, ok := n.obj("targeting", raw["targeting"]); ok {
		n.keys("targeting", m, "show_on", "hide_on", "devices", "countries")
		s.Targeting.ShowOn = n.urlRules("targeting.show_on", m["show_on"])
		s.Targeting.HideOn = n.urlRules("targeting.hide_on", m["hide_on"])
		s.Targeting.Devices = n.enum("targeting.devices", m["devices"], TargetDevices, s.Targeting.Devices)
		if c, ok := n.obj("targeting.countries", m["countries"]); ok {
			n.keys("targeting.countries", c, "mode", "list")
			s.Targeting.Countries.Mode = n.enum("targeting.countries.mode", c["mode"], CountryModes, "all")
			s.Targeting.Countries.List = n.countries("targeting.countries.list", c["list"])
		}
	}
	if m, ok := n.obj("privacy", raw["privacy"]); ok {
		n.keys("privacy", m, "consent_required", "cookieless", "consent_text", "privacy_url", "persistence")
		s.Privacy.ConsentRequired = n.bool("privacy.consent_required", m["consent_required"], false)
		s.Privacy.Cookieless = n.bool("privacy.cookieless", m["cookieless"], false)
		s.Privacy.ConsentText = n.localized("privacy.consent_text", m["consent_text"], 500)
		s.Privacy.PrivacyURL = n.url("privacy.privacy_url", m["privacy_url"])
		s.Privacy.Persistence = n.enum("privacy.persistence", m["persistence"], Persistences, s.Privacy.Persistence)
	}
	if s.Privacy.Cookieless {
		s.Privacy.Persistence = "memory"
	}
	if m, ok := n.obj("rate_limit", raw["rate_limit"]); ok {
		n.keys("rate_limit", m, "messages_per_minute")
		s.RateLimit.MessagesPerMinute = n.int("rate_limit.messages_per_minute", m["messages_per_minute"], 5, 120, s.RateLimit.MessagesPerMinute)
	}
	if m, ok := n.obj("mobile", raw["mobile"]); ok {
		n.keys("mobile", m, "fullscreen")
		s.Mobile.Fullscreen = n.bool("mobile.fullscreen", m["fullscreen"], s.Mobile.Fullscreen)
	}
	if list, ok := n.list("attribute_mapping", raw["attribute_mapping"]); ok {
		seen := map[string]bool{}
		for i, item := range list {
			p := join("attribute_mapping", i)
			m, ok := n.obj(p, item)
			if !ok {
				continue
			}
			n.keys(p, m, "key", "target", "anonymous")
			e := AttributeMapEntry{Key: n.str(join(p, "key"), m["key"], 40), Target: n.str(join(p, "target"), m["target"], 50), Anonymous: n.bool(join(p, "anonymous"), m["anonymous"], false)}
			switch {
			case !keyRe.MatchString(e.Key):
				n.add(join(p, "key"), IssueInvalid, "use lowercase letters, digits and _")
			case !validTarget(e.Target):
				n.add(join(p, "target"), IssueInvalid, "must be name, email, phone or custom:<field key>")
			case seen[e.Key]:
				n.add(join(p, "key"), IssueDuplicate, "attribute keys must be unique")
			case len(s.AttributeMapping) >= MaxMappings:
				n.add("attribute_mapping", IssueLimit, "at most %d mappings", MaxMappings)
			default:
				seen[e.Key] = true
				s.AttributeMapping = append(s.AttributeMapping, e)
			}
		}
	}
	// Cross-field rules.
	if s.PreChat.Consent.Enabled && len(s.PreChat.Consent.Text) == 0 {
		n.add("prechat.consent.text", IssueRequired, "write the consent text")
	}
	if s.Targeting.Countries.Mode != "all" && len(s.Targeting.Countries.List) == 0 {
		n.add("targeting.countries.list", IssueRequired, "pick at least one country")
	}
	return s, n.issues
}

func (s Settings) hasMapping(key string) bool {
	for _, m := range s.AttributeMapping {
		if m.Key == key {
			return true
		}
	}
	return false
}

// preChat reads the pre-chat switch, the request form choice and the
// consent box. "name", "email", "phone" and "fields" are the pre-CF-198
// shape (the fields now live on the request form); they are accepted and
// ignored so stored settings keep normalizing.
func (n *norm) preChat(p *PreChat, m map[string]any) {
	n.keys("prechat", m, "enabled", "form_uuid", "consent", "name", "email", "phone", "fields")
	p.Enabled = n.bool("prechat.enabled", m["enabled"], p.Enabled)
	if v := n.str("prechat.form_uuid", m["form_uuid"], 36); v != "" {
		if _, err := uuid.Parse(v); err != nil {
			n.add("prechat.form_uuid", IssueInvalid, "not a form id")
		} else {
			p.FormUUID = v
		}
	}
	if c, ok := n.obj("prechat.consent", m["consent"]); ok {
		n.keys("prechat.consent", c, "enabled", "required", "text", "url")
		p.Consent.Enabled = n.bool("prechat.consent.enabled", c["enabled"], false)
		p.Consent.Required = n.bool("prechat.consent.required", c["required"], p.Consent.Required)
		p.Consent.Text = n.localized("prechat.consent.text", c["text"], 500)
		p.Consent.URL = n.url("prechat.consent.url", c["url"])
	}
}

func (n *norm) appearance(a *Appearance, m map[string]any) {
	n.keys("appearance", m, "theme", "launcher_color", "header_color", "bubble_color", "font", "radius", "launcher",
		"logo_url", "pattern", "z_index", "custom_css", "show_branding")
	a.Theme = n.enum("appearance.theme", m["theme"], Themes, a.Theme)
	a.LauncherColor = n.color("appearance.launcher_color", m["launcher_color"], "")
	a.HeaderColor = n.color("appearance.header_color", m["header_color"], "")
	a.BubbleColor = n.color("appearance.bubble_color", m["bubble_color"], "")
	a.Font = n.enum("appearance.font", m["font"], Fonts, a.Font)
	a.Radius = n.int("appearance.radius", m["radius"], 0, 28, a.Radius)
	a.LogoURL = n.url("appearance.logo_url", m["logo_url"])
	a.Pattern = n.enum("appearance.pattern", m["pattern"], Patterns, a.Pattern)
	a.ZIndex = n.int("appearance.z_index", m["z_index"], 1, DefaultZIndex+1000, a.ZIndex)
	css, problems := SanitizeCSS(n.str("appearance.custom_css", m["custom_css"], MaxCustomCSS))
	for _, p := range problems {
		n.add("appearance.custom_css", IssueUnsafe, "%s", p)
	}
	a.CustomCSS = css
	a.ShowBranding = n.bool("appearance.show_branding", m["show_branding"], a.ShowBranding)
	l, ok := n.obj("appearance.launcher", m["launcher"])
	if !ok {
		return
	}
	n.keys("appearance.launcher", l, "icon", "image_url", "label", "size", "shape", "offset_x", "offset_y")
	a.Launcher.Icon = n.enum("appearance.launcher.icon", l["icon"], LauncherIcons, a.Launcher.Icon)
	a.Launcher.ImageURL = n.url("appearance.launcher.image_url", l["image_url"])
	if a.Launcher.Icon == "custom" && a.Launcher.ImageURL == "" {
		n.add("appearance.launcher.image_url", IssueRequired, "a custom icon needs an image address")
		a.Launcher.Icon = "chat"
	}
	a.Launcher.Label = n.localized("appearance.launcher.label", l["label"], 40)
	a.Launcher.Size = n.int("appearance.launcher.size", l["size"], 40, 80, a.Launcher.Size)
	a.Launcher.Shape = n.enum("appearance.launcher.shape", l["shape"], LauncherShapes, a.Launcher.Shape)
	a.Launcher.OffsetX = n.int("appearance.launcher.offset_x", l["offset_x"], 0, 200, a.Launcher.OffsetX)
	a.Launcher.OffsetY = n.int("appearance.launcher.offset_y", l["offset_y"], 0, 200, a.Launcher.OffsetY)
}

func (n *norm) cards(list []any) []HomeCard {
	out := []HomeCard{}
	seen := map[string]bool{}
	for i, item := range list {
		p := join("home.cards", i)
		m, ok := n.obj(p, item)
		if !ok {
			continue
		}
		n.keys(p, m, "id", "type", "enabled", "title", "description", "url")
		c := HomeCard{
			ID: n.str(join(p, "id"), m["id"], 40), Type: n.enum(join(p, "type"), m["type"], CardTypes, ""),
			Enabled: n.bool(join(p, "enabled"), m["enabled"], true), Title: n.localized(join(p, "title"), m["title"], 80),
			Description: n.localized(join(p, "description"), m["description"], 200), URL: n.url(join(p, "url"), m["url"]),
		}
		if c.Type == "" {
			continue
		}
		if c.ID == "" {
			c.ID = c.Type
		}
		switch {
		case seen[c.ID]:
			n.add(join(p, "id"), IssueDuplicate, "card ids must be unique")
		case (c.Type == "link" || c.Type == "status") && c.URL == "":
			n.add(join(p, "url"), IssueRequired, "add the address this card opens")
		case c.Type == "link" && len(c.Title) == 0:
			n.add(join(p, "title"), IssueRequired, "write a title")
		case len(out) >= MaxHomeCards:
			n.add("home.cards", IssueLimit, "at most %d cards", MaxHomeCards)
		default:
			seen[c.ID] = true
			out = append(out, c)
		}
	}
	return out
}

func (n *norm) channels(c *ChannelLinks, m map[string]any) {
	n.keys("channels", m, "chat", "email", "whatsapp", "instagram", "telegram", "messenger", "phone", "callback", "video")
	c.Chat = n.bool("channels.chat", m["chat"], c.Chat)
	if v := n.str("channels.email", m["email"], 254); v != "" {
		if strings.Count(v, "@") == 1 && !strings.ContainsAny(v, " <>,;\"") && strings.Contains(v[strings.Index(v, "@"):], ".") {
			c.Email = strings.ToLower(v)
		} else {
			n.add("channels.email", IssueInvalid, "not an e-mail address")
		}
	}
	if v := n.str("channels.whatsapp", m["whatsapp"], 30); v != "" {
		if c.WhatsApp = digits(v); c.WhatsApp == "" {
			n.add("channels.whatsapp", IssueInvalid, "use the number with country code")
		}
	}
	if v := n.str("channels.phone", m["phone"], 30); v != "" {
		if c.Phone = phone(v); c.Phone == "" {
			n.add("channels.phone", IssueInvalid, "not a phone number")
		}
	}
	for key, dst := range map[string]*string{"instagram": &c.Instagram, "telegram": &c.Telegram, "messenger": &c.Messenger} {
		if v := n.str(join("channels", key), m[key], 65); v != "" {
			if *dst = handle(v); *dst == "" {
				n.add(join("channels", key), IssueInvalid, "not a user name")
			}
		}
	}
	c.Callback = n.bool("channels.callback", m["callback"], false)
	c.Video = n.bool("channels.video", m["video"], false)
}

func (n *norm) trigger(p string, m map[string]any) (Trigger, bool) {
	n.keys(p, m, "id", "name", "enabled", "message", "delay_s", "frequency", "open", "conditions")
	t := Trigger{
		ID: n.str(join(p, "id"), m["id"], 40), Name: n.str(join(p, "name"), m["name"], 80), Enabled: n.bool(join(p, "enabled"), m["enabled"], true),
		Message: n.localized(join(p, "message"), m["message"], 500), DelaySec: n.int(join(p, "delay_s"), m["delay_s"], 0, 3600, 0),
		Frequency: n.enum(join(p, "frequency"), m["frequency"], TriggerFreqs, "once"), Open: n.bool(join(p, "open"), m["open"], false),
	}
	if t.ID == "" {
		n.add(join(p, "id"), IssueRequired, "missing id")
		return t, false
	}
	if len(t.Message) == 0 {
		n.add(join(p, "message"), IssueRequired, "write the message")
		return t, false
	}
	cp := join(p, "conditions")
	c, _ := n.obj(cp, m["conditions"])
	n.keys(cp, c, "url", "time_on_page_s", "scroll_pct", "exit_intent", "visitor", "device", "utm_source", "utm_medium",
		"utm_campaign", "identified", "languages", "countries")
	t.Conditions = TriggerConditions{
		URL: n.urlRules(join(cp, "url"), c["url"]), TimeOnPage: n.int(join(cp, "time_on_page_s"), c["time_on_page_s"], 0, 3600, 0),
		ScrollPct: n.int(join(cp, "scroll_pct"), c["scroll_pct"], 0, 100, 0), ExitIntent: n.bool(join(cp, "exit_intent"), c["exit_intent"], false),
		Visitor: n.enum(join(cp, "visitor"), c["visitor"], VisitorKinds, "any"), Device: n.enum(join(cp, "device"), c["device"], DeviceKinds, "any"),
		UTMSource: n.str(join(cp, "utm_source"), c["utm_source"], 100), UTMMedium: n.str(join(cp, "utm_medium"), c["utm_medium"], 100),
		UTMCampaign: n.str(join(cp, "utm_campaign"), c["utm_campaign"], 100), Identified: n.enum(join(cp, "identified"), c["identified"], IdentifiedKinds, "any"),
		Languages: []string{}, Countries: n.countries(join(cp, "countries"), c["countries"]),
	}
	for i, l := range n.strs(join(cp, "languages"), c["languages"], 20, 2) {
		if len(l) == 2 {
			t.Conditions.Languages = append(t.Conditions.Languages, strings.ToLower(l))
		} else {
			n.add(join(join(cp, "languages"), i), IssueInvalid, "use two-letter language codes")
		}
	}
	return t, true
}

func (n *norm) urlRules(p string, v any) []URLRule {
	out := []URLRule{}
	list, _ := n.list(p, v)
	for i, item := range list {
		ip := join(p, i)
		m, ok := n.obj(ip, item)
		if !ok {
			continue
		}
		n.keys(ip, m, "op", "value")
		r := URLRule{Op: n.enum(join(ip, "op"), m["op"], URLOps, "contains"), Value: n.str(join(ip, "value"), m["value"], 300)}
		switch {
		case r.Value == "":
		case len(out) >= MaxURLRules:
			n.add(p, IssueLimit, "at most %d rules", MaxURLRules)
		default:
			out = append(out, r)
		}
	}
	return out
}

func (n *norm) countries(p string, v any) []string {
	out := []string{}
	for i, c := range n.strs(p, v, 250, 2) {
		c = strings.ToUpper(c)
		switch {
		case !ccRe.MatchString(c):
			n.add(join(p, i), IssueInvalid, "use two-letter country codes")
		case !slices.Contains(out, c):
			out = append(out, c)
		}
	}
	return out
}

func validTarget(t string) bool {
	switch t {
	case "name", "email", "phone":
		return true
	}
	key, ok := strings.CutPrefix(t, "custom:")
	return ok && keyRe.MatchString(key)
}

// safeURL keeps absolute https URLs (http only for localhost).
func safeURL(v string) string {
	if v == "" || len(v) > 1000 {
		return ""
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" || u.User != nil {
		return ""
	}
	if u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")) {
		return u.String()
	}
	return ""
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func digits(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if !strings.ContainsRune(" +-().", r) {
			return ""
		}
	}
	if b.Len() < 6 || b.Len() > 15 {
		return ""
	}
	return b.String()
}

func phone(v string) string {
	d := digits(v)
	if d == "" {
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(v), "+") {
		return "+" + d
	}
	return d
}

var handleRe = regexp.MustCompile(`^[A-Za-z0-9_.]{2,64}$`)

func handle(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "@")
	if handleRe.MatchString(v) {
		return v
	}
	return ""
}
