package widget

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
)

func TestNormalizeV1KeepsLegacyValues(t *testing.T) {
	s := Normalize(map[string]any{
		"color": "#FF0000", "position": "left", "locale": "en", "greeting_title": "Hi",
		"prechat": map[string]any{"enabled": false, "name": "required"}, "csat": false, // "name" is the pre-CF-198 shape
	})
	if s.SchemaVersion != SchemaVersion || s.Color != "#ff0000" || s.Position != "left" || s.Locale != "en" {
		t.Fatalf("legacy values lost: %+v", s)
	}
	if s.PreChat.Enabled || s.PreChat.FormUUID != "" {
		t.Fatalf("prechat: %+v", s.PreChat)
	}
	if s.CSAT || s.Appearance.Launcher.Size != 56 || len(s.Home.Cards) == 0 || s.Privacy.Persistence != "local" {
		t.Fatalf("v2 defaults missing: %+v", s)
	}
	if got := s.Text("tr", "greeting_title"); got != "Hi" {
		t.Fatalf("legacy greeting fallback: %q", got)
	}
}

func TestValidateReportsPaths(t *testing.T) {
	raw := map[string]any{
		"color":      "red",
		"appearance": map[string]any{"radius": float64(99), "launcher": map[string]any{"icon": "custom"}, "logo_url": "javascript:alert(1)", "bogus": true},
		"triggers": []any{
			map[string]any{"id": "t1", "message": map[string]any{"tr": "Merhaba"}},
			map[string]any{"id": "t1", "message": "dup"},
			map[string]any{"id": "t3"},
		},
		"prechat":           map[string]any{"form_uuid": "not-a-uuid"},
		"channels":          map[string]any{"whatsapp": "abc"},
		"attribute_mapping": []any{map[string]any{"key": "plan", "target": "custom:Plan"}},
		"texts":             map[string]any{"de": map[string]any{}},
		// Pre-CF-198 widgets stored their own hours; the key is ignored.
		"business_hours": map[string]any{"days": map[string]any{"mon": []any{map[string]any{"start": "18:00", "end": "09:00"}}}},
	}
	got := map[string]string{}
	for _, i := range Validate(raw) {
		got[i.Path] = i.Code
	}
	want := map[string]string{
		"color":                         IssueInvalid,
		"appearance.radius":             IssueRange,
		"appearance.launcher.image_url": IssueRequired,
		"appearance.logo_url":           IssueUnsafe,
		"appearance.bogus":              IssueUnknownKey,
		"triggers.1.id":                 IssueDuplicate,
		"triggers.2.message":            IssueRequired,
		"prechat.form_uuid":             IssueInvalid,
		"channels.whatsapp":             IssueInvalid,
		"attribute_mapping.0.target":    IssueInvalid,
		"texts.de":                      IssueUnknownKey,
	}
	for path, code := range want {
		if got[path] != code {
			t.Errorf("%s: got %q want %q (all: %v)", path, got[path], code, got)
		}
	}
	for path := range got {
		if strings.HasPrefix(path, "business_hours") {
			t.Errorf("legacy business_hours must be ignored, got issue at %s", path)
		}
	}
	if issues := Validate(map[string]any{}); len(issues) != 0 {
		t.Fatalf("empty settings must be valid: %v", issues)
	}
}

func TestDefaultsRoundTripValid(t *testing.T) {
	raw, err := json.Marshal(Defaults())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if issues := Validate(m); len(issues) != 0 {
		t.Fatalf("defaults must validate: %v", issues)
	}
	again, _ := json.Marshal(Normalize(m))
	if string(again) != string(raw) {
		t.Fatalf("normalize is not idempotent:\n%s\n%s", raw, again)
	}
}

func TestCookielessForcesMemory(t *testing.T) {
	s := Normalize(map[string]any{"privacy": map[string]any{"cookieless": true, "persistence": "local"}})
	if s.Privacy.Persistence != "memory" {
		t.Fatalf("persistence: %s", s.Privacy.Persistence)
	}
}

func TestOnlineFollowsOrganizationHours(t *testing.T) {
	// The widget has no hours of its own (CF-198): online is the
	// organization's working-hours state unless away mode is on.
	s := Normalize(map[string]any{"business_hours": map[string]any{"enabled": true, "days": map[string]any{}}})
	if !s.Online(true) || s.Online(false) {
		t.Fatal("online must follow the organization's hours")
	}
	s.AwayMode = true
	if s.Online(true) {
		t.Fatal("away mode is offline")
	}
	next := time.Date(2026, 11, 2, 6, 0, 0, 0, time.UTC)
	est := Defaults().EstimateReply(businesshours.State{Open: false, NextOpenAt: &next, Scheduled: true}, nil)
	if est.NextOpenAt == nil || !est.NextOpenAt.Equal(next) || est.Value != "day" {
		t.Fatalf("closed estimate: %+v", est)
	}
}

func TestEstimateReply(t *testing.T) {
	now := businesshours.State{Open: true}
	s := Defaults()
	d := 30 * time.Minute
	if v := s.EstimateReply(now, &d).Value; v != "hour" {
		t.Fatalf("auto 30m: %s", v)
	}
	if v := s.EstimateReply(now, nil).Value; v != "minutes" {
		t.Fatalf("auto online: %s", v)
	}
	s.ReplyTime = ReplyTimeSetting{Mode: "manual", Value: "day"}
	if v := s.EstimateReply(now, &d).Value; v != "day" {
		t.Fatalf("manual: %s", v)
	}
	s.ReplyTime.Mode = "hidden"
	if v := s.EstimateReply(now, &d).Value; v != "" {
		t.Fatalf("hidden: %s", v)
	}
}

func TestDeviceOf(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Mobile/15E148":           "mobile",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) Mobile Safari/537.36":                  "mobile",
		"Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) Mobile/15E148":                    "desktop",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130 Safari": "desktop",
	}
	for ua, want := range cases {
		if got := DeviceOf(ua); got != want {
			t.Errorf("%s: %s", ua, got)
		}
	}
}

// targetingCases is the fixture shared with the SDK tests.
type targetingCases struct {
	URL []struct {
		Rule  URLRule `json:"rule"`
		URL   string  `json:"url"`
		Match bool    `json:"match"`
	} `json:"url"`
	Visible []struct {
		Targeting Targeting   `json:"targeting"`
		Ctx       PageContext `json:"ctx"`
		Visible   bool        `json:"visible"`
		Reason    string      `json:"reason"`
	} `json:"visible"`
	Triggers []struct {
		Trigger  Trigger     `json:"trigger"`
		Ctx      PageContext `json:"ctx"`
		Eligible bool        `json:"eligible"`
	} `json:"triggers"`
	CSS []struct {
		In       string `json:"in"`
		Out      string `json:"out"`
		Problems int    `json:"problems"`
	} `json:"css"`
}

func loadCases(t *testing.T) targetingCases {
	t.Helper()
	raw, err := os.ReadFile("testdata/targeting_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c targetingCases
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSharedTargetingCases(t *testing.T) {
	c := loadCases(t)
	for _, tc := range c.URL {
		if got := MatchURL(tc.Rule, tc.URL); got != tc.Match {
			t.Errorf("MatchURL(%+v, %s) = %v", tc.Rule, tc.URL, got)
		}
	}
	for i, tc := range c.Visible {
		ok, reason := tc.Targeting.Visible(tc.Ctx)
		if ok != tc.Visible || reason != tc.Reason {
			t.Errorf("visible[%d] = %v %q, want %v %q", i, ok, reason, tc.Visible, tc.Reason)
		}
	}
	for _, tc := range c.Triggers {
		if got := tc.Trigger.Eligible(tc.Ctx); got != tc.Eligible {
			t.Errorf("trigger %s eligible = %v", tc.Trigger.ID, got)
		}
	}
	for _, tc := range c.CSS {
		out, problems := SanitizeCSS(tc.In)
		if out != tc.Out || len(problems) != tc.Problems {
			t.Errorf("SanitizeCSS(%q) = %q %v", tc.In, out, problems)
		}
	}
}

func TestSanitizeCSSNeverKeepsDangerousTokens(t *testing.T) {
	out, _ := SanitizeCSS(`@IMPORT "x"; a{b:url( "http://x" )} c{d:JavaScript:e} f{g:-moz-binding:url(x)}`)
	low := strings.ToLower(out)
	for _, bad := range []string{"@import", "http://", "javascript:", "-moz-binding:"} {
		if strings.Contains(low, bad) {
			t.Fatalf("%q kept in %q", bad, out)
		}
	}
}

func TestAttachmentKinds(t *testing.T) {
	s := Defaults()
	cases := []struct {
		ct, name, kind string
		ok             bool
	}{
		{"image/png", "a.png", "image", true},
		{"application/octet-stream", "report.PDF", "pdf", true},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "x", "spreadsheet", true},
		{"video/mp4", "clip.mp4", "video", false},
		{"application/x-msdownload", "setup.exe", "", false},
	}
	for _, c := range cases {
		if got := AttachmentKind(c.ct, c.name); got != c.kind {
			t.Errorf("%s %s: %q", c.ct, c.name, got)
		}
		if got := s.AttachmentAllowed(c.ct, c.name); got != c.ok {
			t.Errorf("%s allowed = %v", c.name, got)
		}
	}
}

// TestDefaultsFixture keeps testdata/defaults.json (read by the SDK tests,
// frontend/widget-sdk/tests) equal to Defaults(): the TypeScript defaults
// must not drift from the API's.
func TestDefaultsFixture(t *testing.T) {
	want, err := json.MarshalIndent(Defaults(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if os.Getenv("UPDATE_FIXTURES") == "1" {
		if err := os.WriteFile("testdata/defaults.json", want, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile("testdata/defaults.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("testdata/defaults.json is stale: UPDATE_FIXTURES=1 go test ./internal/platform/channels/widget/")
	}
}
