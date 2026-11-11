package autoreply

import (
	"strings"
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	recent := now.Add(-time.Hour)
	old := now.Add(-OfflineRepeat - time.Minute)
	cases := []struct {
		name        string
		first, open bool
		last        *time.Time
		want        Decision
	}{
		{"first message inside hours is acknowledged", true, true, nil, KindAck},
		{"later message inside hours is quiet", false, true, nil, ""},
		{"first message outside hours gets the offline notice", true, false, nil, KindOffline},
		{"later message outside hours gets it once per window", false, false, &recent, ""},
		{"a new closed window gets it again", false, false, &old, KindOffline},
	}
	for _, c := range cases {
		if got := Decide(c.first, c.open, c.last, now); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestRenderAndDefaults(t *testing.T) {
	s := Defaults()
	two := 2 * time.Hour
	got := s.Render(KindAck, "tr", &two)
	if !strings.Contains(got, "Tahmini yanıt süresi: 2 saat.") || strings.Contains(got, "{eta}") {
		t.Fatalf("ack tr: %q", got)
	}
	if got := s.Render(KindAck, "en", nil); strings.Contains(got, "{eta}") || strings.HasSuffix(got, " ") {
		t.Fatalf("ack without eta: %q", got)
	}
	if got := s.Render(KindOffline, "de", nil); got != s.Offline.Text["tr"] {
		t.Fatalf("unknown locale falls back to tr: %q", got)
	}
	s.Handoff.Enabled = false
	if s.Render(KindHandoff, "tr", nil) != "" {
		t.Fatal("disabled kind renders nothing")
	}
	for _, c := range []struct {
		d    time.Duration
		want string
	}{{30 * time.Minute, "30 dakika"}, {90 * time.Minute, "1 buçuk saat"}, {8 * time.Hour, "8 saat"}, {48 * time.Hour, "2 iş günü"}} {
		if got := ETAText("tr", &c.d); !strings.Contains(got, c.want) {
			t.Errorf("%v: %q", c.d, got)
		}
	}
	// Normalize: empty texts fall back, too long is refused, unknown
	// locales are dropped.
	in := Settings{Ack: Reply{Enabled: true, Text: Localized{"tr": "", "de": "x"}}}
	out, err := Normalize(in)
	if err != nil || out.Ack.Text["tr"] != Defaults().Ack.Text["tr"] || out.Ack.Text["de"] != "" || out.Offline.Text["en"] == "" {
		t.Fatalf("normalize: %+v err=%v", out, err)
	}
	in.Offline.Text = Localized{"tr": strings.Repeat("a", MaxTextLength+1)}
	if _, err := Normalize(in); err == nil {
		t.Fatal("too long text accepted")
	}
}
