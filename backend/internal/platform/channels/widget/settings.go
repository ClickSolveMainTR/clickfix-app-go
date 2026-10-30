package widget

import (
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// SchemaVersion is the current settings schema (CF-150). Version 1 is the
// flat CF-55 shape; version 2 keeps every v1 key and adds the grouped
// appearance, content, behaviour, targeting and privacy objects. Stored v1
// JSON normalizes to a complete v2 value, so old widgets keep working.
const SchemaVersion = 2

// Locales are the languages the widget ships texts for.
var Locales = []string{"tr", "en"}

// Settings is the normalized design and behaviour configuration of a widget
// instance (channel_instances.settings). Normalize is the single place that
// turns stored, possibly partial or stale JSON into complete values with
// deterministic fallbacks (docs/widget/03), so the API, the SDK and the
// admin preview never disagree. The SDK and the editor share the TypeScript
// mirror in frontend/widget-sdk/src/shared/schema.ts.
type Settings struct {
	SchemaVersion   int           `json:"schema_version"`
	Color           string        `json:"color"`
	Position        string        `json:"position"`
	Locale          string        `json:"locale"`
	GreetingTitle   string        `json:"greeting_title"`
	GreetingText    string        `json:"greeting_text"`
	ShowTeam        bool          `json:"show_team"`
	TeamMemberUUIDs []string      `json:"team_member_uuids"`
	PreChat         PreChat       `json:"prechat"`
	BusinessHours   BusinessHours `json:"business_hours"`
	OfflineMessage  string        `json:"offline_message"`
	AllowedDomains  []string      `json:"allowed_domains"`
	RequireIdentity bool          `json:"require_identity"`
	FileUpload      bool          `json:"file_upload"`
	CSAT            bool          `json:"csat"`

	Appearance       Appearance          `json:"appearance"`
	Texts            map[string]Texts    `json:"texts"`
	ReplyTime        ReplyTimeSetting    `json:"reply_time"`
	Home             Home                `json:"home"`
	QuickReplies     []Localized         `json:"quick_replies"`
	CSATOptions      CSATOptions         `json:"csat_options"`
	Offline          Offline             `json:"offline"`
	AwayMode         bool                `json:"away_mode"`
	Channels         ChannelLinks        `json:"channels"`
	AI               AISettings          `json:"ai"`
	Attachments      Attachments         `json:"attachments"`
	Features         Features            `json:"features"`
	Triggers         []Trigger           `json:"triggers"`
	Targeting        Targeting           `json:"targeting"`
	Privacy          Privacy             `json:"privacy"`
	RateLimit        RateLimit           `json:"rate_limit"`
	Mobile           Mobile              `json:"mobile"`
	AttributeMapping []AttributeMapEntry `json:"attribute_mapping"`
}

// Localized is a text per locale ("tr", "en").
type Localized map[string]string

// Get returns the text for locale, falling back to the other locales.
func (l Localized) Get(locale string) string {
	if v := l[locale]; v != "" {
		return v
	}
	for _, loc := range Locales {
		if v := l[loc]; v != "" {
			return v
		}
	}
	return ""
}

// PreChat is the form shown before the first message.
type PreChat struct {
	Enabled bool `json:"enabled"`
	// Name / Email / Phone: "required", "optional" or "hidden".
	Name    string         `json:"name"`
	Email   string         `json:"email"`
	Phone   string         `json:"phone"`
	Fields  []PreChatField `json:"fields"`
	Consent Consent        `json:"consent"`
}

// PreChatField is a custom question of the pre-chat form; Attribute maps
// the answer to a contact attribute (AttributeMapping key).
type PreChatField struct {
	Key       string    `json:"key"`
	Label     Localized `json:"label"`
	Type      string    `json:"type"`
	Required  bool      `json:"required"`
	Options   []string  `json:"options"`
	Attribute string    `json:"attribute"`
}

// Consent is the GDPR/KVKK checkbox of the pre-chat form.
type Consent struct {
	Enabled  bool      `json:"enabled"`
	Required bool      `json:"required"`
	Text     Localized `json:"text"`
	URL      string    `json:"url"`
}

// BusinessHours limits when the widget shows as online. Outside them the
// widget shows the offline form (e-mail required) instead of live chat.
type BusinessHours struct {
	Enabled  bool                `json:"enabled"`
	Timezone string              `json:"timezone"`
	Days     map[string][]Period `json:"days"`
}

// Period is one opening span of a day ("09:00"–"18:00").
type Period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Appearance is the visual design.
type Appearance struct {
	Theme         string   `json:"theme"`
	LauncherColor string   `json:"launcher_color"`
	HeaderColor   string   `json:"header_color"`
	BubbleColor   string   `json:"bubble_color"`
	Font          string   `json:"font"`
	Radius        int      `json:"radius"`
	Launcher      Launcher `json:"launcher"`
	LogoURL       string   `json:"logo_url"`
	Pattern       string   `json:"pattern"`
	ZIndex        int      `json:"z_index"`
	CustomCSS     string   `json:"custom_css"`
	ShowBranding  bool     `json:"show_branding"`
}

// Launcher is the floating button.
type Launcher struct {
	Icon     string    `json:"icon"`
	ImageURL string    `json:"image_url"`
	Label    Localized `json:"label"`
	Size     int       `json:"size"`
	Shape    string    `json:"shape"`
	OffsetX  int       `json:"offset_x"`
	OffsetY  int       `json:"offset_y"`
}

// Texts are per-locale overrides of the widget copy.
type Texts struct {
	GreetingTitle  string `json:"greeting_title"`
	GreetingText   string `json:"greeting_text"`
	TeamIntro      string `json:"team_intro"`
	OfflineMessage string `json:"offline_message"`
	AwayMessage    string `json:"away_message"`
	Placeholder    string `json:"placeholder"`
}

// ReplyTimeSetting is the "we usually reply in …" line.
type ReplyTimeSetting struct {
	// Mode: "auto" (from first-response history and business hours),
	// "manual" or "hidden".
	Mode  string `json:"mode"`
	Value string `json:"value"`
}

// Home lists the cards of the home screen, in order.
type Home struct {
	Cards []HomeCard `json:"cards"`
}

// HomeCard is one home screen card.
type HomeCard struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Enabled     bool      `json:"enabled"`
	Title       Localized `json:"title"`
	Description Localized `json:"description"`
	URL         string    `json:"url"`
}

// CSATOptions shapes the post-chat survey.
type CSATOptions struct {
	Scale    string    `json:"scale"`
	Comment  bool      `json:"comment"`
	Question Localized `json:"question"`
}

// Offline is the out-of-hours behaviour.
type Offline struct {
	// Mode: "form" (leave a message, e-mail required), "message" (show the
	// offline text only) or "hide" (hide the launcher).
	Mode         string `json:"mode"`
	CollectEmail bool   `json:"collect_email"`
}

// ChannelLinks are the other ways to reach the team (deep links).
type ChannelLinks struct {
	Chat      bool   `json:"chat"`
	Email     string `json:"email"`
	WhatsApp  string `json:"whatsapp"`
	Instagram string `json:"instagram"`
	Telegram  string `json:"telegram"`
	Messenger string `json:"messenger"`
	Phone     string `json:"phone"`
	Callback  bool   `json:"callback"`
	Video     bool   `json:"video"`
}

// AISettings turns the organization's AI agent on or off for this widget.
type AISettings struct {
	Enabled bool `json:"enabled"`
	// HumanButton shows "talk to a person" while the AI agent answers.
	HumanButton     bool     `json:"human_button"`
	HandoffKeywords []string `json:"handoff_keywords"`
}

// Attachments limits visitor uploads.
type Attachments struct {
	MaxMB int      `json:"max_mb"`
	Types []string `json:"types"`
}

// Features toggles conversation UI details.
type Features struct {
	Emoji         bool `json:"emoji"`
	Typing        bool `json:"typing"`
	ReadReceipts  bool `json:"read_receipts"`
	Sound         bool `json:"sound"`
	Notifications bool `json:"notifications"`
	Markdown      bool `json:"markdown"`
	History       bool `json:"history"`
}

// Trigger is a proactive message.
type Trigger struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Enabled    bool              `json:"enabled"`
	Message    Localized         `json:"message"`
	DelaySec   int               `json:"delay_s"`
	Frequency  string            `json:"frequency"`
	Open       bool              `json:"open"`
	Conditions TriggerConditions `json:"conditions"`
}

// TriggerConditions must all hold for a trigger to fire. The server checks
// the page-independent ones at bootstrap; time on page, scroll and exit
// intent are checked by the SDK in the browser.
type TriggerConditions struct {
	URL         []URLRule `json:"url"`
	TimeOnPage  int       `json:"time_on_page_s"`
	ScrollPct   int       `json:"scroll_pct"`
	ExitIntent  bool      `json:"exit_intent"`
	Visitor     string    `json:"visitor"`
	Device      string    `json:"device"`
	UTMSource   string    `json:"utm_source"`
	UTMMedium   string    `json:"utm_medium"`
	UTMCampaign string    `json:"utm_campaign"`
	Identified  string    `json:"identified"`
	Languages   []string  `json:"languages"`
	Countries   []string  `json:"countries"`
}

// URLRule matches a page URL.
type URLRule struct {
	Op    string `json:"op"`
	Value string `json:"value"`
}

// Targeting decides on which pages, devices and countries the widget loads.
type Targeting struct {
	ShowOn    []URLRule `json:"show_on"`
	HideOn    []URLRule `json:"hide_on"`
	Devices   string    `json:"devices"`
	Countries Countries `json:"countries"`
}

// Countries allow- or block-lists ISO 3166 alpha-2 codes.
type Countries struct {
	Mode string   `json:"mode"`
	List []string `json:"list"`
}

// Privacy is the GDPR/KVKK behaviour.
type Privacy struct {
	// ConsentRequired asks the visitor before anything is stored.
	ConsentRequired bool `json:"consent_required"`
	// Cookieless keeps the session in memory only (no browser storage).
	Cookieless  bool      `json:"cookieless"`
	ConsentText Localized `json:"consent_text"`
	PrivacyURL  string    `json:"privacy_url"`
	// Persistence: "local" (survives restarts), "session" or "memory".
	Persistence string `json:"persistence"`
}

// RateLimit bounds visitor messages.
type RateLimit struct {
	MessagesPerMinute int `json:"messages_per_minute"`
}

// Mobile is the small-screen behaviour.
type Mobile struct {
	Fullscreen bool `json:"fullscreen"`
}

// AttributeMapEntry maps an SDK attribute (setAttributes / pre-chat field)
// to a contact field: "name", "email", "phone" or "custom:<field key>".
// Anonymous lets unverified visitors set it (verified users always can).
type AttributeMapEntry struct {
	Key       string `json:"key"`
	Target    string `json:"target"`
	Anonymous bool   `json:"anonymous"`
}

// Field modes of the pre-chat form.
const (
	FieldRequired = "required"
	FieldOptional = "optional"
	FieldHidden   = "hidden"
)

// Defaults.
const (
	DefaultColor    = "#4f46e5"
	DefaultTimezone = "Europe/Istanbul"
	DefaultZIndex   = 2147483000
)

// Enumerations (first value is the default).
var (
	Themes           = []string{"light", "dark", "auto"}
	Fonts            = []string{"system", "inherit", "rounded", "serif", "mono"}
	LauncherIcons    = []string{"chat", "help", "headset", "sparkles", "wave", "custom"}
	LauncherShapes   = []string{"circle", "rounded", "pill"}
	Patterns         = []string{"none", "dots", "grid", "waves", "diagonal"}
	ReplyModes       = []string{"auto", "manual", "hidden"}
	ReplyValues      = []string{"minutes", "hour", "hours", "day"}
	CardTypes        = []string{"conversation", "help_search", "articles", "status", "link", "booking", "channels"}
	CSATScales       = []string{"stars", "emoji", "thumbs"}
	OfflineModes     = []string{"form", "message", "hide"}
	AttachmentTypes  = []string{"image", "pdf", "document", "spreadsheet", "video", "audio", "archive", "text"}
	TriggerFreqs     = []string{"once", "session", "always"}
	VisitorKinds     = []string{"any", "new", "returning"}
	DeviceKinds      = []string{"any", "desktop", "mobile"}
	TargetDevices    = []string{"all", "desktop", "mobile"}
	IdentifiedKinds  = []string{"any", "yes", "no"}
	URLOps           = []string{"contains", "equals", "starts_with", "ends_with", "glob", "not_contains"}
	CountryModes     = []string{"all", "allow", "block"}
	Persistences     = []string{"local", "session", "memory"}
	PreChatTypes     = []string{"text", "textarea", "email", "phone", "number", "select", "checkbox"}
	defaultCardTypes = []string{"conversation", "help_search", "articles", "booking", "channels"}
)

// Bounds.
const (
	MaxTriggers      = 20
	MaxQuickReplies  = 8
	MaxHomeCards     = 12
	MaxPreChatFields = 10
	MaxMappings      = 50
	MaxURLRules      = 20
	MaxCustomCSS     = 8000
)

var (
	colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	clockRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	keyRe   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	ccRe    = regexp.MustCompile(`^[A-Z]{2}$`)
	dayKeys = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
)

// DefaultCards is the home screen of a new widget.
func DefaultCards() []HomeCard {
	out := make([]HomeCard, 0, len(defaultCardTypes))
	for _, t := range defaultCardTypes {
		out = append(out, HomeCard{ID: t, Type: t, Enabled: true, Title: Localized{}, Description: Localized{}})
	}
	return out
}

// Defaults returns the settings of a new widget.
func Defaults() Settings {
	return Settings{
		SchemaVersion: SchemaVersion,
		Color:         DefaultColor, Position: "right", Locale: "auto", ShowTeam: true,
		PreChat: PreChat{
			Enabled: true, Name: FieldOptional, Email: FieldOptional, Phone: FieldHidden,
			Fields: []PreChatField{}, Consent: Consent{Required: true, Text: Localized{}},
		},
		BusinessHours:   BusinessHours{Timezone: DefaultTimezone, Days: map[string][]Period{}},
		FileUpload:      true,
		CSAT:            true,
		TeamMemberUUIDs: []string{},
		AllowedDomains:  []string{},
		Appearance: Appearance{
			Theme: "light", Font: "system", Radius: 16, Pattern: "none", ZIndex: DefaultZIndex, ShowBranding: true,
			Launcher: Launcher{Icon: "chat", Label: Localized{}, Size: 56, Shape: "circle", OffsetX: 20, OffsetY: 20},
		},
		Texts:            map[string]Texts{"tr": {}, "en": {}},
		ReplyTime:        ReplyTimeSetting{Mode: "auto", Value: "minutes"},
		Home:             Home{Cards: DefaultCards()},
		QuickReplies:     []Localized{},
		CSATOptions:      CSATOptions{Scale: "stars", Comment: true, Question: Localized{}},
		Offline:          Offline{Mode: "form", CollectEmail: true},
		Channels:         ChannelLinks{Chat: true},
		AI:               AISettings{Enabled: true, HumanButton: true, HandoffKeywords: []string{}},
		Attachments:      Attachments{MaxMB: 10, Types: []string{"image", "pdf", "document", "spreadsheet", "text"}},
		Features:         Features{Emoji: true, Typing: true, ReadReceipts: true, Sound: true, Markdown: true, History: true},
		Triggers:         []Trigger{},
		Targeting:        Targeting{ShowOn: []URLRule{}, HideOn: []URLRule{}, Devices: "all", Countries: Countries{Mode: "all", List: []string{}}},
		Privacy:          Privacy{ConsentText: Localized{}, Persistence: "local"},
		RateLimit:        RateLimit{MessagesPerMinute: 30},
		Mobile:           Mobile{Fullscreen: true},
		AttributeMapping: []AttributeMapEntry{},
	}
}

// Text returns the widget copy for a locale: the per-locale override, else
// the legacy top-level value.
func (s Settings) Text(locale, field string) string {
	t := s.Texts[locale]
	var v, legacy string
	switch field {
	case "greeting_title":
		v, legacy = t.GreetingTitle, s.GreetingTitle
	case "greeting_text":
		v, legacy = t.GreetingText, s.GreetingText
	case "offline_message":
		v, legacy = t.OfflineMessage, s.OfflineMessage
	case "team_intro":
		v = t.TeamIntro
	case "away_message":
		v = t.AwayMessage
	case "placeholder":
		v = t.Placeholder
	}
	if v != "" {
		return v
	}
	return legacy
}

// Online reports whether the widget is inside its business hours at now
// (always true when business hours are off) and not in away mode.
func (s Settings) Online(now time.Time) bool {
	if s.AwayMode {
		return false
	}
	return s.BusinessHours.Open(now)
}

// Open reports whether now is inside the opening hours (always true when
// business hours are off).
func (h BusinessHours) Open(now time.Time) bool {
	if !h.Enabled {
		return true
	}
	local := now.In(h.location())
	clock := local.Format("15:04")
	for _, p := range h.Days[dayKeys[local.Weekday()]] {
		if clock >= p.Start && clock < p.End {
			return true
		}
	}
	return false
}

func (h BusinessHours) location() *time.Location {
	loc, err := time.LoadLocation(h.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// NextOpening returns the start of the next opening period after now (nil
// when business hours are off, open now or have no periods).
func (h BusinessHours) NextOpening(now time.Time) *time.Time {
	if !h.Enabled || h.Open(now) {
		return nil
	}
	loc := h.location()
	local := now.In(loc)
	for d := 0; d < 8; d++ {
		day := local.AddDate(0, 0, d)
		periods := slices.Clone(h.Days[dayKeys[day.Weekday()]])
		slices.SortFunc(periods, func(a, b Period) int { return strings.Compare(a.Start, b.Start) })
		for _, p := range periods {
			clock, err := time.Parse("15:04", p.Start)
			if err != nil {
				continue
			}
			at := time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
			if at.After(now) {
				utc := at.UTC()
				return &utc
			}
		}
	}
	return nil
}

// OriginAllowed reports whether a page origin ("https://shop.example.com")
// may use the widget. An empty allowlist allows every origin; "example.com"
// matches the domain itself, "*.example.com" any of its subdomains.
func (s Settings) OriginAllowed(origin string) bool {
	if len(s.AllowedDomains) == 0 {
		return true
	}
	host := hostOf(origin)
	if host == "" {
		return false
	}
	for _, d := range s.AllowedDomains {
		if rest, ok := strings.CutPrefix(d, "*."); ok {
			if host == rest || strings.HasSuffix(host, "."+rest) {
				return true
			}
			continue
		}
		if host == d {
			return true
		}
	}
	return false
}

// MaxUploadBytes is the per-file limit of the settings.
func (s Settings) MaxUploadBytes() int64 {
	return int64(s.Attachments.MaxMB) << 20
}

func hostOf(origin string) string {
	origin = strings.TrimSpace(origin)
	if origin == "" || origin == "null" {
		return ""
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// normalizeDomain accepts "example.com", "*.example.com" or a full origin
// and returns the lowercase host pattern ("" when invalid).
func normalizeDomain(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	wild := strings.HasPrefix(v, "*.")
	v = strings.TrimPrefix(v, "*.")
	if strings.Contains(v, "://") {
		v = hostOf(v)
	} else if i := strings.IndexAny(v, "/:"); i >= 0 {
		v = v[:i]
	}
	if v == "" || strings.ContainsAny(v, " *") || !strings.Contains(v, ".") && v != "localhost" {
		return ""
	}
	if wild {
		return "*." + v
	}
	return v
}

// Stored returns the normalized settings as the JSON object kept on the
// instance. The session replay block is copied as is (usecase/replay.go
// reads and bounds it itself).
func Stored(raw map[string]any) (map[string]any, error) {
	b, err := json.Marshal(Normalize(raw))
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if r, ok := raw["replay"]; ok {
		out["replay"] = r
	}
	return out, nil
}
