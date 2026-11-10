// Package businesshours does working-time arithmetic for the SLA engine
// (CF-46): "add 4 business hours to this instant" and "how much working time
// passed between these two instants" on a weekly schedule with dated
// exceptions (public and organization holidays), in the schedule's time zone.
//
// The weekly shape follows takwim-app-go branches.working_hours
// ({"mon": {"closed": false, "open": "09:00", "close": "18:00"}, ...}).
// A nil *Calendar means "always open" (24/7).
package businesshours

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// DayKeys are the weekly schedule keys, indexed by time.Weekday.
var DayKeys = [7]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// Day is one weekday of the weekly schedule. Open / Close are "HH:MM" in the
// calendar's zone; Close may be "24:00".
type Day struct {
	Closed bool   `json:"closed"`
	Open   string `json:"open"`
	Close  string `json:"close"`
}

// Week maps DayKeys to their hours. Missing days are closed.
type Week map[string]Day

// Exception overrides one calendar date: closed all day, or open only
// between Open and Close (a half-day eve closes at 13:00).
type Exception struct {
	Date   string // YYYY-MM-DD
	Name   string
	Closed bool
	Open   string
	Close  string
}

// HalfDayClose is when a half-day public holiday eve (arife) closes.
const HalfDayClose = "13:00"

// ErrInvalid is returned for malformed schedules.
var ErrInvalid = errors.New("businesshours: invalid schedule")

// maxScanDays bounds the walk over the calendar (a schedule with exceptions
// covering every day for years would otherwise loop forever).
const maxScanDays = 3 * 366

type window struct{ from, to int } // minutes from local midnight, [from, to)

// Calendar is a compiled schedule.
type Calendar struct {
	loc        *time.Location
	week       [7][]window
	exceptions map[string][]window // date → windows (empty = closed)
}

// ParseClock parses "HH:MM" (00:00 – 24:00) into minutes from midnight.
func ParseClock(s string) (int, error) {
	s = strings.TrimSpace(s)
	if len(s) == 8 && s[5] == ':' { // HH:MM:SS from a TIME column
		s = s[:5]
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		if s == "24:00" {
			return 24 * 60, nil
		}
		return 0, fmt.Errorf("%w: bad time %q", ErrInvalid, s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func dayWindow(open, close string) ([]window, error) {
	from, err := ParseClock(open)
	if err != nil {
		return nil, err
	}
	to, err := ParseClock(close)
	if err != nil {
		return nil, err
	}
	if to <= from {
		return nil, fmt.Errorf("%w: close %s must be after open %s", ErrInvalid, close, open)
	}
	return []window{{from, to}}, nil
}

// ValidateWeek checks a weekly schedule: known day keys, valid hours and at
// least one open day.
func ValidateWeek(w Week) error {
	open := false
	for key, d := range w {
		if !validKey(key) {
			return fmt.Errorf("%w: unknown day %q", ErrInvalid, key)
		}
		if d.Closed {
			continue
		}
		if _, err := dayWindow(d.Open, d.Close); err != nil {
			return err
		}
		open = true
	}
	if !open {
		return fmt.Errorf("%w: at least one day must be open", ErrInvalid)
	}
	return nil
}

func validKey(k string) bool {
	for _, d := range DayKeys {
		if d == k {
			return true
		}
	}
	return false
}

// New compiles a calendar. tz is an IANA zone ("Europe/Istanbul").
func New(tz string, week Week, exceptions []Exception) (*Calendar, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("%w: time zone %q", ErrInvalid, tz)
	}
	if err := ValidateWeek(week); err != nil {
		return nil, err
	}
	c := &Calendar{loc: loc, exceptions: map[string][]window{}}
	for i, key := range DayKeys {
		d, ok := week[key]
		if !ok || d.Closed {
			continue
		}
		c.week[i], _ = dayWindow(d.Open, d.Close)
	}
	// Exceptions on the same date intersect (a holiday eve on a custom short
	// day keeps the earlier close).
	for _, ex := range exceptions {
		day, err := time.ParseInLocation("2006-01-02", ex.Date, loc)
		if err != nil {
			return nil, fmt.Errorf("%w: date %q", ErrInvalid, ex.Date)
		}
		var ws []window
		if !ex.Closed {
			if ws, err = dayWindow(ex.Open, ex.Close); err != nil {
				return nil, err
			}
		}
		if prev, ok := c.exceptions[ex.Date]; ok {
			ws = intersect(prev, ws)
		} else if !ex.Closed {
			// An exception never opens a day wider than the weekly hours.
			ws = intersect(c.week[day.Weekday()], ws)
		}
		c.exceptions[ex.Date] = ws
	}
	return c, nil
}

func intersect(a, b []window) []window {
	out := []window{}
	for _, x := range a {
		for _, y := range b {
			from, to := max(x.from, y.from), min(x.to, y.to)
			if to > from {
				out = append(out, window{from, to})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].from < out[j].from })
	return out
}

// Location is the calendar's zone (UTC for a nil calendar).
func (c *Calendar) Location() *time.Location {
	if c == nil {
		return time.UTC
	}
	return c.loc
}

// windows returns the open intervals of the local date that contains day.
func (c *Calendar) windows(day time.Time) []window {
	if ws, ok := c.exceptions[day.Format("2006-01-02")]; ok {
		return ws
	}
	return c.week[day.Weekday()]
}

func at(day time.Time, minute int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, minute, 0, 0, day.Location())
}

// IsOpen tells whether t falls in working time.
func (c *Calendar) IsOpen(t time.Time) bool {
	if c == nil {
		return true
	}
	local := t.In(c.loc)
	m := local.Hour()*60 + local.Minute()
	for _, w := range c.windows(local) {
		if m >= w.from && m < w.to {
			return true
		}
	}
	return false
}

// Add returns the instant at which d of working time has passed since start.
// d <= 0 returns start moved forward to the next working instant (or start
// itself when it is already working time).
func (c *Calendar) Add(start time.Time, d time.Duration) time.Time {
	if c == nil {
		if d < 0 {
			d = 0
		}
		return start.Add(d)
	}
	cur := start.In(c.loc)
	day := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, c.loc)
	for i := 0; i < maxScanDays; i++ {
		for _, w := range c.windows(day) {
			from, to := at(day, w.from), at(day, w.to)
			if !to.After(cur) {
				continue
			}
			if from.Before(cur) {
				from = cur
			}
			span := to.Sub(from)
			if d <= span {
				return from.Add(max(d, 0)).UTC()
			}
			d -= span
		}
		day = day.AddDate(0, 0, 1)
	}
	// No working time within the scan window: fall back to wall clock.
	return start.Add(max(d, 0)).UTC()
}

// Between returns the working time in [from, to). It is 0 when to <= from.
func (c *Calendar) Between(from, to time.Time) time.Duration {
	if !to.After(from) {
		return 0
	}
	if c == nil {
		return to.Sub(from)
	}
	cur, end := from.In(c.loc), to.In(c.loc)
	day := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, c.loc)
	var total time.Duration
	for i := 0; i < maxScanDays*4 && day.Before(end); i++ {
		for _, w := range c.windows(day) {
			a, b := at(day, w.from), at(day, w.to)
			if a.Before(cur) {
				a = cur
			}
			if b.After(end) {
				b = end
			}
			if b.After(a) {
				total += b.Sub(a)
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return total
}

// Span is a half-open interval [Start, End) of working time.
type Span struct {
	Start, End time.Time
}

// Spans returns the working intervals that overlap [from, to), clipped to
// it, in order (UTC). A nil calendar is open around the clock.
func (c *Calendar) Spans(from, to time.Time) []Span {
	if !to.After(from) {
		return nil
	}
	if c == nil {
		return []Span{{Start: from.UTC(), End: to.UTC()}}
	}
	cur, end := from.In(c.loc), to.In(c.loc)
	day := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, c.loc)
	var out []Span
	for i := 0; i < maxScanDays && day.Before(end); i++ {
		for _, w := range c.windows(day) {
			a, b := at(day, w.from), at(day, w.to)
			if a.Before(cur) {
				a = cur
			}
			if b.After(end) {
				b = end
			}
			if !b.After(a) {
				continue
			}
			// A day closing at 24:00 continues into a day opening at 00:00.
			if n := len(out); n > 0 && out[n-1].End.Equal(a.UTC()) {
				out[n-1].End = b.UTC()
				continue
			}
			out = append(out, Span{Start: a.UTC(), End: b.UTC()})
		}
		day = day.AddDate(0, 0, 1)
	}
	return out
}

// State is whether a schedule is open at one instant (CF-198: the widget,
// the AI agent and the automatic replies all read the same answer).
type State struct {
	// Open: the instant is working time (always true without a schedule).
	Open bool `json:"open"`
	// NextOpenAt is when the schedule next opens while closed; nil when
	// open or without a schedule.
	NextOpenAt *time.Time `json:"next_open_at"`
	// Scheduled: a schedule exists (false: open around the clock).
	Scheduled bool `json:"scheduled"`
}

// State evaluates the calendar at t. A nil calendar is always open.
func (c *Calendar) State(t time.Time) State {
	if c == nil {
		return State{Open: true}
	}
	st := State{Open: c.IsOpen(t), Scheduled: true}
	if !st.Open {
		next := c.Add(t, 0)
		if next.After(t) {
			st.NextOpenAt = &next
		}
	}
	return st
}

// Source answers whether an organization is inside its working hours. The
// SLA module implements it from the organization's default schedule; other
// modules depend on this interface instead of each other.
type Source interface {
	HoursState(ctx context.Context, orgID int64, at time.Time) (State, error)
}
