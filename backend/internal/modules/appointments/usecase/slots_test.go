package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestHostSlotsRespectBuffersNoticeAndBusy(t *testing.T) {
	loc := mustLoc(t, "Europe/Istanbul")
	day := func(h, m int) time.Time { return time.Date(2026, 11, 2, h, m, 0, 0, loc).UTC() }
	work := []span{{day(9, 0), day(12, 0)}}
	busy := []span{{day(10, 0), day(10, 30)}}
	r := slotRule{
		duration: 30 * time.Minute, interval: 30 * time.Minute, before: 0, after: 15 * time.Minute,
		notBefore: day(9, 10), notAfter: day(23, 0), loc: loc,
	}
	got := hostSlots(work, busy, r)
	var clocks []string
	for _, s := range got {
		clocks = append(clocks, s.In(loc).Format("15:04"))
	}
	// 09:00 is before the notice; 09:30 ends 10:00 + 15m after buffer and
	// would touch the 10:00 booking; 10:00 is busy; 11:30 ends at closing.
	want := "10:30,11:00,11:30"
	if strings.Join(clocks, ",") != want {
		t.Fatalf("slots = %v, want %s", clocks, want)
	}
}

func TestAlignKeepsWallClockGridAcrossDST(t *testing.T) {
	loc := mustLoc(t, "Europe/Berlin")
	// 29 March 2026: clocks jump from 02:00 to 03:00 in Berlin.
	at := time.Date(2026, 3, 29, 9, 5, 0, 0, loc)
	got := align(at, 30*time.Minute, loc).In(loc)
	if got.Format("15:04") != "09:30" {
		t.Fatalf("aligned %v from %v", got, at)
	}
}

func TestSubtractSplitsSpans(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return base.Add(time.Duration(n) * time.Hour) }
	got := subtract([]span{{h(9), h(17)}}, []span{{h(12), h(13)}, {h(16), h(18)}})
	if len(got) != 2 || !got[0].end.Equal(h(12)) || !got[1].start.Equal(h(13)) || !got[1].end.Equal(h(16)) {
		t.Fatalf("subtract = %+v", got)
	}
}

func TestFitsNeedsWorkingTimeAndFreeBufferedBlock(t *testing.T) {
	base := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	h := func(hh, mm int) time.Time {
		return base.Add(time.Duration(hh)*time.Hour + time.Duration(mm)*time.Minute)
	}
	av := availability{work: map[int64][]span{1: {{h(9, 0), h(17, 0)}}}, busy: map[int64][]span{1: {{h(11, 0), h(12, 0)}}}}
	if !fits(av, 1, h(9, 0), h(10, 0), 0, 0) {
		t.Fatal("09:00-10:00 should fit")
	}
	if fits(av, 1, h(10, 0), h(11, 0), 0, 10*time.Minute) {
		t.Fatal("after buffer touches the 11:00 booking")
	}
	if fits(av, 1, h(16, 30), h(17, 30), 0, 0) {
		t.Fatal("past closing time must not fit")
	}
	if fits(av, 2, h(9, 0), h(10, 0), 0, 0) {
		t.Fatal("unknown host has no working time")
	}
}

func TestManageTokenRoundTripAndTamper(t *testing.T) {
	s := &Service{secret: "test-secret"}
	id := uuid.New()
	tok := s.Token(42, id, 3)
	c, err := s.parseToken(tok)
	if err != nil || c.Org != 42 || c.ID != id || c.Version != 3 {
		t.Fatalf("claims %+v err=%v", c, err)
	}
	if _, err := s.parseToken(tok[:len(tok)-2] + "xx"); err == nil {
		t.Fatal("tampered token accepted")
	}
	other := &Service{secret: "other"}
	if _, err := other.parseToken(tok); err == nil {
		t.Fatal("token signed with another key accepted")
	}
}

func TestICSEscapesAndFolds(t *testing.T) {
	a := Appointment{
		UUID: uuid.MustParse("6f1c1f0e-8a62-4f0b-9b8f-6e1c3b7a2d10"), Status: StatusConfirmed,
		Title:    "Kurulum, eğitim; demo",
		StartsAt: time.Date(2026, 11, 2, 7, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Notes: strings.Repeat("uzun not ", 20),
		LocationValue: "https://meet.example.com/abc",
	}
	out := string(ICS(a, 2, "Acme"))
	for _, want := range []string{
		"UID:6f1c1f0e-8a62-4f0b-9b8f-6e1c3b7a2d10@clickfix", "SEQUENCE:2", "DTSTART:20261102T070000Z",
		`SUMMARY:Kurulum\, eğitim\; demo`, "STATUS:CONFIRMED", "METHOD:PUBLISH",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ICS missing %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\r\n") {
		if len(line) > 75 {
			t.Fatalf("unfolded line (%d): %q", len(line), line)
		}
	}
	a.Status = StatusCancelled
	if out := string(ICS(a, 3, "")); !strings.Contains(out, "STATUS:CANCELLED") || !strings.Contains(out, "METHOD:CANCEL") {
		t.Fatalf("cancelled ICS:\n%s", out)
	}
}

func TestRenderAndSlugify(t *testing.T) {
	got := Render("Merhaba {{name}}, {{ type }} {{unknown}}", map[string]string{"name": "Ayşe", "type": "Demo"})
	if got != "Merhaba Ayşe, Demo {{unknown}}" {
		t.Fatalf("render = %q", got)
	}
	if s := Slugify("  Kurulum Görüşmesi (Onboarding) "); s != "kurulum-gorusmesi-onboarding" {
		t.Fatalf("slug = %q", s)
	}
}

func TestFormatWhenTurkish(t *testing.T) {
	loc := mustLoc(t, "Europe/Istanbul")
	got := formatWhen(time.Date(2026, 11, 2, 7, 0, 0, 0, time.UTC), loc, "tr")
	if got != "2 Kasım 2026 Pazartesi, 10:00" {
		t.Fatalf("formatWhen = %q", got)
	}
}
