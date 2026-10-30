package businesshours

import (
	"testing"
	"time"
)

var weekdays = Week{
	"mon": {Open: "09:00", Close: "18:00"},
	"tue": {Open: "09:00", Close: "18:00"},
	"wed": {Open: "09:00", Close: "18:00"},
	"thu": {Open: "09:00", Close: "18:00"},
	"fri": {Open: "09:00", Close: "18:00"},
	"sat": {Closed: true},
}

func ist(t *testing.T, s string) time.Time {
	t.Helper()
	loc, _ := time.LoadLocation("Europe/Istanbul")
	v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustCal(t *testing.T, ex ...Exception) *Calendar {
	t.Helper()
	c, err := New("Europe/Istanbul", weekdays, ex)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAddWithinDay(t *testing.T) {
	c := mustCal(t)
	// Wednesday 10:00 + 2h = 12:00.
	got := c.Add(ist(t, "2026-10-28 10:00"), 2*time.Hour)
	if want := ist(t, "2026-10-28 12:00"); !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestAddCrossesNightAndWeekend(t *testing.T) {
	c := mustCal(t)
	// Friday 17:00 + 2h: 1h on Friday, then Monday 09:00 + 1h.
	got := c.Add(ist(t, "2026-10-30 17:00"), 2*time.Hour)
	if want := ist(t, "2026-11-02 10:00"); !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
	// Started on Saturday: counts from Monday 09:00.
	got = c.Add(ist(t, "2026-10-31 11:00"), 30*time.Minute)
	if want := ist(t, "2026-11-02 09:30"); !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestHolidaysAndHalfDays(t *testing.T) {
	// 29 Ekim closed, 28 Ekim (arife) closes at 13:00.
	c := mustCal(t,
		Exception{Date: "2026-10-28", Open: "00:00", Close: HalfDayClose},
		Exception{Date: "2026-10-29", Closed: true},
	)
	// Wednesday 12:00 + 3h: 1h until 13:00, Thursday closed, Friday 09:00 + 2h.
	got := c.Add(ist(t, "2026-10-28 12:00"), 3*time.Hour)
	if want := ist(t, "2026-10-30 11:00"); !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
	if c.IsOpen(ist(t, "2026-10-28 14:00")) || !c.IsOpen(ist(t, "2026-10-28 09:30")) {
		t.Fatal("half day hours wrong")
	}
	if b := c.Between(ist(t, "2026-10-28 12:00"), ist(t, "2026-10-30 11:00")); b != 3*time.Hour {
		t.Fatalf("between = %s", b)
	}
}

func TestBetweenIsInverseOfAdd(t *testing.T) {
	c := mustCal(t, Exception{Date: "2026-11-04", Closed: true})
	start := ist(t, "2026-10-29 16:45")
	for _, d := range []time.Duration{0, time.Minute, 75 * time.Minute, 9 * time.Hour, 40 * time.Hour} {
		end := c.Add(start, d)
		if got := c.Between(start, end); got != d {
			t.Fatalf("d=%s: between(add) = %s", d, got)
		}
	}
}

func TestNilCalendarIsAlwaysOpen(t *testing.T) {
	var c *Calendar
	start := time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC)
	if got := c.Add(start, 2*time.Hour); !got.Equal(start.Add(2 * time.Hour)) {
		t.Fatal(got)
	}
	if !c.IsOpen(start) || c.Between(start, start.Add(time.Hour)) != time.Hour {
		t.Fatal("nil calendar must be 24/7")
	}
}

func TestValidate(t *testing.T) {
	cases := []Week{
		{},
		{"mon": {Closed: true}},
		{"mon": {Open: "18:00", Close: "09:00"}},
		{"xyz": {Open: "09:00", Close: "18:00"}},
		{"mon": {Open: "9am", Close: "18:00"}},
	}
	for i, w := range cases {
		if err := ValidateWeek(w); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
	if err := ValidateWeek(Week{"sun": {Open: "00:00", Close: "24:00"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := New("Mars/Olympus", weekdays, nil); err == nil {
		t.Fatal("bad zone accepted")
	}
}

func TestSpansFollowLocalDaysAndExceptions(t *testing.T) {
	week := Week{"mon": {Open: "09:00", Close: "12:00"}, "tue": {Open: "09:00", Close: "18:00"}}
	cal, err := New("Europe/Istanbul", week, []Exception{{Date: "2026-11-03", Closed: false, Open: "09:00", Close: "13:00"}})
	if err != nil {
		t.Fatal(err)
	}
	loc := cal.Location()
	from := time.Date(2026, 11, 2, 10, 0, 0, 0, loc) // Monday
	to := time.Date(2026, 11, 4, 0, 0, 0, 0, loc)
	got := cal.Spans(from, to)
	if len(got) != 2 {
		t.Fatalf("spans = %v", got)
	}
	if !got[0].Start.Equal(from) || !got[0].End.Equal(time.Date(2026, 11, 2, 12, 0, 0, 0, loc)) {
		t.Fatalf("monday span = %v", got[0])
	}
	if !got[1].End.Equal(time.Date(2026, 11, 3, 13, 0, 0, 0, loc)) {
		t.Fatalf("tuesday exception span = %v", got[1])
	}
	var none *Calendar
	if s := none.Spans(from, to); len(s) != 1 || !s[0].End.Equal(to) {
		t.Fatalf("nil calendar spans = %v", s)
	}
}
