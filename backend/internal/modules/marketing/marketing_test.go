package marketing

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSanitizeSnippet(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"keeps vendor script", `<script async src="https://www.googletagmanager.com/gtag/js?id=G-1"></script>`,
			`<script async="" src="https://www.googletagmanager.com/gtag/js?id=G-1"></script>`},
		{"keeps inline body", `<script>window.x = 1 < 2;</script>`, `<script>window.x = 1 < 2;</script>`},
		{"drops event handlers and other tags", `<div onclick="x()"><img src="https://a.example/p.gif" onerror="alert(1)"></div><b>hi</b>`,
			`<img src="https://a.example/p.gif">`},
		{"drops javascript urls", `<iframe src="javascript:alert(1)"></iframe>`, `<iframe></iframe>`},
		{"drops http urls", `<script src="http://evil.example/x.js"></script>`, `<script></script>`},
		{"noscript pixel", `<noscript><img height="1" width="1" src="https://px.example/tr?id=1"/></noscript>`,
			`<noscript><img height="1" width="1" src="https://px.example/tr?id=1"></noscript>`},
		{"closes unclosed", `<noscript><iframe src="https://x.example">`, `<noscript><iframe src="https://x.example"></iframe></noscript>`},
		{"drops comments", `<!-- hi --><meta name="a" content="b">`, `<meta name="a" content="b">`},
		{"empty", "  ", ""},
	}
	for _, c := range cases {
		if got := SanitizeSnippet(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	ok := Config{GTMID: " gtm-abc123 ", GA4ID: "g-XYZ1234", MetaPixelID: "123456789", GoogleAdsID: "aw-123456789", GoogleAdsSignupLabel: "AbC_123", ClarityID: "AbCdEf12", GoogleVerification: "tok_en-123"}.Normalize()
	if d := ok.Validate(); len(d) != 0 {
		t.Fatalf("unexpected errors: %+v", d)
	}
	if ok.GTMID != "GTM-ABC123" || ok.ClarityID != "abcdef12" {
		t.Fatalf("not normalized: %+v", ok)
	}
	bad := Config{GTMID: "UA-1", GoogleAdsSignupLabel: "label123", GoogleVerification: `<meta name="x">`, SiteTitle: strings.Repeat("ş", 71)}.Normalize()
	fields := map[string]bool{}
	for _, d := range bad.Validate() {
		fields[d.Field] = true
	}
	for _, f := range []string{"gtm_id", "google_ads_id", "google_verification", "site_title"} {
		if !fields[f] {
			t.Errorf("expected error on %s, got %v", f, fields)
		}
	}
}

func TestParseAttribution(t *testing.T) {
	raw := url.QueryEscape(`{"ft":{"source":"Google","medium":"cpc","campaign":"Lansman Ekim","landing":"/features","at":"2026-10-01T10:00:00Z"},"lt":{"referrer":"https://www.linkedin.com/feed/","landing":"https://evil.example/"}}`)
	a, ok := ParseAttribution(raw)
	if !ok {
		t.Fatal("expected attribution")
	}
	if a.FirstTouch.Source != "google" || a.FirstTouch.Campaign != "Lansman Ekim" || a.FirstTouch.Landing != "/features" {
		t.Errorf("first touch: %+v", a.FirstTouch)
	}
	if a.LastTouch.Source != "linkedin.com" || a.LastTouch.Medium != "referral" || a.LastTouch.Landing != "" {
		t.Errorf("last touch: %+v", a.LastTouch)
	}
	if src, med, camp := a.summary(); src != "google" || med != "cpc" || camp != "Lansman Ekim" {
		t.Errorf("summary: %s %s %s", src, med, camp)
	}

	only, ok := ParseAttribution(url.QueryEscape(`{"lt":{"gclid":"abc"}}`))
	if !ok || only.FirstTouch.Source != "google" || only.FirstTouch.Medium != "cpc" {
		t.Errorf("click id fallback: %+v %v", only, ok)
	}
	for _, in := range []string{"", "%zz", url.QueryEscape(`{"ft":{}}`), url.QueryEscape(`{"ft":{"referrer":"javascript:alert(1)"}}`), strings.Repeat("a", maxAttributionHeader+1)} {
		if _, ok := ParseAttribution(in); ok {
			t.Errorf("expected rejection of %q", in)
		}
	}
}

func TestReportRange(t *testing.T) {
	now := time.Date(2026, 10, 29, 15, 0, 0, 0, time.UTC)
	from, to, ok := reportRange(httptest.NewRequest("GET", "/x", nil), now)
	if !ok || !from.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("default range: %v %v %v", from, to, ok)
	}
	if _, _, ok := reportRange(httptest.NewRequest("GET", "/x?from=2026-10-10&to=2026-10-01", nil), now); ok {
		t.Error("reversed range accepted")
	}
	if _, _, ok := reportRange(httptest.NewRequest("GET", "/x?from=2020-01-01&to=2026-10-01", nil), now); ok {
		t.Error("too long range accepted")
	}
}
