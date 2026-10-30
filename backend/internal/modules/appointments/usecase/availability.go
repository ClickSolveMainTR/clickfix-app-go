package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Availability: a host can be booked inside working time, outside approved
// time off and outside other bookings (each widened by its own buffers).
//
// Working time is the host's agent shifts (CF-105) when the host has shifts
// in the requested range; otherwise the business hours (CF-46) of the type's
// team or the organization default, with public and organization holidays.
// Without any business hours the default is Monday–Friday 09:00–18:00 in the
// organization's appointment time zone.
//
// Slots start on a grid of slot_interval minutes counted from local midnight
// in the organization's time zone, so DST days keep "09:00, 09:30, …" wall
// clock starts.

type span struct{ start, end time.Time }

func (a span) overlaps(b span) bool { return a.start.Before(b.end) && b.start.Before(a.end) }

// slotRule is one slot search.
type slotRule struct {
	duration, interval, before, after time.Duration
	notBefore, notAfter               time.Time
	loc                               *time.Location
}

// hostSlots returns the slot starts of one host: inside a working span, not
// before notBefore (minimum notice), not after notAfter (booking window),
// and with [start-before, end+after) clear of busy.
func hostSlots(work, busy []span, r slotRule) []time.Time {
	var out []time.Time
	for _, w := range work {
		lo := w.start
		if lo.Before(r.notBefore) {
			lo = r.notBefore
		}
		t := align(lo, r.interval, r.loc)
		for ; !t.Add(r.duration).After(w.end); t = t.Add(r.interval) {
			if t.After(r.notAfter) {
				break
			}
			block := span{t.Add(-r.before), t.Add(r.duration + r.after)}
			free := true
			for _, b := range busy {
				if block.overlaps(b) {
					free = false
					break
				}
			}
			if free {
				out = append(out, t)
			}
		}
	}
	return out
}

// align returns the first grid instant >= t; the grid starts at local
// midnight of t's day and steps by interval.
func align(t time.Time, interval time.Duration, loc *time.Location) time.Time {
	local := t.In(loc)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	since := t.Sub(midnight)
	steps := since / interval
	at := midnight.Add(steps * interval)
	if at.Before(t) {
		at = at.Add(interval)
	}
	return at.UTC()
}

// subtract removes cut from spans.
func subtract(spans, cut []span) []span {
	out := spans
	for _, c := range cut {
		next := make([]span, 0, len(out))
		for _, s := range out {
			if !s.overlaps(c) {
				next = append(next, s)
				continue
			}
			if s.start.Before(c.start) {
				next = append(next, span{s.start, c.start})
			}
			if c.end.Before(s.end) {
				next = append(next, span{c.end, s.end})
			}
		}
		out = next
	}
	return out
}

// defaultWeek is used when an organization has no business hours.
var defaultWeek = businesshours.Week{
	"mon": {Open: "09:00", Close: "18:00"}, "tue": {Open: "09:00", Close: "18:00"},
	"wed": {Open: "09:00", Close: "18:00"}, "thu": {Open: "09:00", Close: "18:00"},
	"fri": {Open: "09:00", Close: "18:00"},
}

// Slot is a bookable start with the hosts free at that time.
type Slot struct {
	StartsAt time.Time   `json:"starts_at"`
	EndsAt   time.Time   `json:"ends_at"`
	Hosts    []uuid.UUID `json:"host_uuids,omitempty"`
}

type host struct {
	id           int64
	uuid         uuid.UUID
	name, email  string
	lastAssigned time.Time
}

// hostsOf returns the hosts of a type in round-robin order (least recently
// assigned first).
func (s *Service) hostsOf(ctx context.Context, q *db.Queries, org int64, t db.AppointmentType) ([]host, error) {
	var out []host
	if t.HostMode == HostTeam {
		if !t.TeamID.Valid {
			return nil, nil
		}
		rows, err := q.AppointmentTeamHosts(ctx, db.AppointmentTeamHostsParams{OrganizationID: org, TeamIds: []int64{t.TeamID.Int64}})
		if err != nil {
			return nil, err
		}
		listed, err := q.AppointmentTypeHosts(ctx, db.AppointmentTypeHostsParams{OrganizationID: org, TypeIds: []int64{t.ID}})
		if err != nil {
			return nil, err
		}
		last := map[int64]time.Time{}
		for _, l := range listed {
			last[l.UserID] = l.LastAssignedAt.Time
		}
		for _, r := range rows {
			out = append(out, host{id: r.ID, uuid: r.Uuid, name: fullName(r.Name, r.Surname), email: r.Email, lastAssigned: last[r.ID]})
		}
	} else {
		rows, err := q.AppointmentTypeHosts(ctx, db.AppointmentTypeHostsParams{OrganizationID: org, TypeIds: []int64{t.ID}})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, host{id: r.UserID, uuid: r.UserUuid, name: fullName(r.Name, r.Surname), email: r.Email, lastAssigned: r.LastAssignedAt.Time})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].lastAssigned.Equal(out[j].lastAssigned) {
			return out[i].lastAssigned.Before(out[j].lastAssigned)
		}
		return out[i].id < out[j].id
	})
	return out, nil
}

// calendar compiles the business hours that apply to a type.
func (s *Service) calendar(ctx context.Context, q *db.Queries, org int64, t db.AppointmentType, loc *time.Location, from, to time.Time) (*businesshours.Calendar, error) {
	var bh db.BusinessHour
	var err error
	found := false
	if t.TeamID.Valid {
		bh, err = q.GetTeamBusinessHours(ctx, db.GetTeamBusinessHoursParams{OrganizationID: org, TeamID: t.TeamID})
		if err == nil {
			found = true
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	if !found {
		bh, err = q.GetDefaultBusinessHours(ctx, org)
		if err == nil {
			found = true
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	if !found {
		return businesshours.New(loc.String(), defaultWeek, nil)
	}
	week := businesshours.Week{}
	if err := json.Unmarshal(bh.WorkingHours, &week); err != nil {
		return businesshours.New(loc.String(), defaultWeek, nil)
	}
	fromDate := pgtype.Date{Time: from.AddDate(0, 0, -1), Valid: true}
	toDate := pgtype.Date{Time: to.AddDate(0, 0, 1), Valid: true}
	var ex []businesshours.Exception
	if bh.UsePublicHolidays {
		rows, err := q.ListPublicHolidays(ctx, db.ListPublicHolidaysParams{CountryCode: bh.HolidayCountry, FromDate: fromDate, ToDate: toDate})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			e := businesshours.Exception{Date: r.Date.Time.Format(time.DateOnly), Name: r.Name, Closed: !r.HalfDay}
			if r.HalfDay {
				e.Open, e.Close = "00:00", businesshours.HalfDayClose
			}
			ex = append(ex, e)
		}
	}
	rows, err := q.ListBusinessHolidays(ctx, db.ListBusinessHolidaysParams{
		OrganizationID: org, BusinessHoursID: pgtype.Int8{Int64: bh.ID, Valid: true}, FromDate: fromDate, ToDate: toDate,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		e := businesshours.Exception{Date: r.Date.Time.Format(time.DateOnly), Name: r.Name, Closed: r.Closed}
		if !r.Closed {
			e.Open, e.Close = clock(r.OpenTime), clock(r.CloseTime)
		}
		ex = append(ex, e)
	}
	cal, err := businesshours.New(bh.Timezone, week, ex)
	if err != nil {
		// An invalid stored schedule falls back to the default week.
		return businesshours.New(loc.String(), defaultWeek, nil)
	}
	return cal, nil
}

func clock(t pgtype.Time) string {
	if !t.Valid {
		return "00:00"
	}
	m := t.Microseconds / int64(time.Minute/time.Microsecond)
	return time.Date(2000, 1, 1, int(m/60), int(m%60), 0, 0, time.UTC).Format("15:04")
}

// availability is what slot search loads for a set of hosts and a range.
type availability struct {
	work map[int64][]span
	busy map[int64][]span
	// perDay counts active bookings of the type per host and local day.
	perDay map[int64]map[string]int
}

// load builds working time and busy time of hosts over [from, to).
// exceptID skips one booking (the one being rescheduled).
func (s *Service) load(ctx context.Context, q *db.Queries, org int64, t db.AppointmentType, hosts []host, loc *time.Location, from, to time.Time, exceptID int64, ignoreWorkingHours bool) (availability, error) {
	a := availability{work: map[int64][]span{}, busy: map[int64][]span{}, perDay: map[int64]map[string]int{}}
	ids := make([]int64, 0, len(hosts))
	for _, h := range hosts {
		ids = append(ids, h.id)
	}
	if len(ids) == 0 {
		return a, nil
	}
	// Widen by a day so buffers and overnight shifts at the edges count.
	wFrom, wTo := from.Add(-24*time.Hour), to.Add(24*time.Hour)
	if ignoreWorkingHours {
		for _, id := range ids {
			a.work[id] = []span{{from, to}}
		}
	} else {
		shifts, err := q.AppointmentHostShifts(ctx, db.AppointmentHostShiftsParams{OrganizationID: org, UserIds: ids, RangeFrom: ts(wFrom), RangeTo: ts(wTo)})
		if err != nil {
			return a, err
		}
		for _, sh := range shifts {
			a.work[sh.UserID] = append(a.work[sh.UserID], span{sh.StartsAt.Time.UTC(), sh.EndsAt.Time.UTC()})
		}
		var cal []span
		calLoaded := false
		for _, id := range ids {
			if len(a.work[id]) > 0 {
				a.work[id] = clip(a.work[id], from, to)
				continue
			}
			if !calLoaded {
				c, err := s.calendar(ctx, q, org, t, loc, from, to)
				if err != nil {
					return a, err
				}
				for _, sp := range c.Spans(from, to) {
					cal = append(cal, span{sp.Start, sp.End})
				}
				calLoaded = true
			}
			a.work[id] = cal
		}
	}
	off, err := q.AppointmentHostTimeOff(ctx, db.AppointmentHostTimeOffParams{OrganizationID: org, UserIds: ids, RangeFrom: ts(wFrom), RangeTo: ts(wTo)})
	if err != nil {
		return a, err
	}
	for _, o := range off {
		a.busy[o.UserID] = append(a.busy[o.UserID], span{o.StartsAt.Time.UTC(), o.EndsAt.Time.UTC()})
	}
	busy, err := q.AppointmentHostBusy(ctx, db.AppointmentHostBusyParams{OrganizationID: org, UserIds: ids, ExceptID: exceptID, RangeFrom: ts(wFrom), RangeTo: ts(wTo)})
	if err != nil {
		return a, err
	}
	for _, b := range busy {
		if !b.HostUserID.Valid {
			continue
		}
		id := b.HostUserID.Int64
		before := time.Duration(b.BufferBefore) * time.Minute
		after := time.Duration(b.BufferAfter) * time.Minute
		a.busy[id] = append(a.busy[id], span{b.StartsAt.Time.UTC().Add(-before), b.EndsAt.Time.UTC().Add(after)})
		if b.TypeID == t.ID {
			if a.perDay[id] == nil {
				a.perDay[id] = map[string]int{}
			}
			a.perDay[id][b.StartsAt.Time.In(loc).Format(time.DateOnly)]++
		}
	}
	return a, nil
}

func clip(spans []span, from, to time.Time) []span {
	out := make([]span, 0, len(spans))
	for _, s := range spans {
		if s.start.Before(from) {
			s.start = from
		}
		if s.end.After(to) {
			s.end = to
		}
		if s.end.After(s.start) {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b span) int { return a.start.Compare(b.start) })
	return out
}

// SlotQuery asks for the slots of a type.
type SlotQuery struct {
	From, To  time.Time
	Duration  int32
	HostUUID  *uuid.UUID
	ExceptID  int64
	Internal  bool
	SkipHours bool
}

func (s *Service) durationOf(t db.AppointmentType, d int32) (int32, error) {
	if d == 0 {
		return t.DefaultDuration, nil
	}
	if !slices.Contains(t.DurationOptions, d) {
		return 0, invalidf("duration must be one of %v", t.DurationOptions)
	}
	return d, nil
}

// slots computes the free slots of a type over [From, To).
func (s *Service) slots(ctx context.Context, q *db.Queries, org int64, t db.AppointmentType, st orgSettings, in SlotQuery) ([]Slot, []host, error) {
	dur, err := s.durationOf(t, in.Duration)
	if err != nil {
		return nil, nil, err
	}
	hosts, err := s.hostsOf(ctx, q, org, t)
	if err != nil {
		return nil, nil, err
	}
	if in.HostUUID != nil {
		i := slices.IndexFunc(hosts, func(h host) bool { return h.uuid == *in.HostUUID })
		if i < 0 {
			return nil, nil, invalidf("host is not a host of this type")
		}
		hosts = []host{hosts[i]}
	}
	now := s.now().UTC()
	rule := slotRule{
		duration: time.Duration(dur) * time.Minute, interval: time.Duration(t.SlotInterval) * time.Minute,
		before: time.Duration(t.BufferBefore) * time.Minute, after: time.Duration(t.BufferAfter) * time.Minute,
		notBefore: now.Add(time.Duration(t.MinNoticeMinutes) * time.Minute),
		notAfter:  now.Add(time.Duration(t.BookingWindowDays) * 24 * time.Hour),
		loc:       st.loc,
	}
	if in.Internal {
		// Agents book for customers: no minimum notice or booking window,
		// only "not in the past".
		rule.notBefore, rule.notAfter = now, in.To
	}
	av, err := s.load(ctx, q, org, t, hosts, st.loc, in.From, in.To, in.ExceptID, in.SkipHours)
	if err != nil {
		return nil, nil, err
	}
	byStart := map[time.Time][]uuid.UUID{}
	for _, h := range hosts {
		for _, start := range hostSlots(av.work[h.id], av.busy[h.id], rule) {
			if t.MaxPerDay > 0 && !in.Internal && av.perDay[h.id][start.In(st.loc).Format(time.DateOnly)] >= int(t.MaxPerDay) {
				continue
			}
			byStart[start] = append(byStart[start], h.uuid)
		}
	}
	out := make([]Slot, 0, len(byStart))
	for start, hs := range byStart {
		out = append(out, Slot{StartsAt: start, EndsAt: start.Add(rule.duration), Hosts: hs})
	}
	slices.SortFunc(out, func(a, b Slot) int { return a.StartsAt.Compare(b.StartsAt) })
	return out, hosts, nil
}

// maxSlotDays caps one slot request.
const maxSlotDays = 42

// SlotRange parses a [from, to) date range in the organization's time zone.
func SlotRange(loc *time.Location, fromStr, toStr string) (time.Time, time.Time, error) {
	from, err := time.ParseInLocation(time.DateOnly, fromStr, loc)
	if err != nil {
		return from, from, invalidf("from must be YYYY-MM-DD")
	}
	to, err := time.ParseInLocation(time.DateOnly, toStr, loc)
	if err != nil {
		return from, to, invalidf("to must be YYYY-MM-DD")
	}
	if !to.After(from) {
		return from, to, invalidf("to must be after from")
	}
	if to.Sub(from) > maxSlotDays*24*time.Hour+2*time.Hour {
		return from, to, invalidf("range is limited to %d days", maxSlotDays)
	}
	return from.UTC(), to.UTC(), nil
}

// Slots returns the free slots of a type for agents (with hosts).
func (s *Service) Slots(ctx context.Context, org int64, typeID uuid.UUID, fromStr, toStr string, duration int32, hostUUID *uuid.UUID) ([]Slot, error) {
	t, err := s.q.AppointmentTypeByUUID(ctx, db.AppointmentTypeByUUIDParams{OrganizationID: org, Uuid: typeID})
	if err != nil {
		return nil, notFound(err)
	}
	st, err := s.settings(ctx, org)
	if err != nil {
		return nil, err
	}
	from, to, err := SlotRange(st.loc, fromStr, toStr)
	if err != nil {
		return nil, err
	}
	out, _, err := s.slots(ctx, s.q, org, t, st, SlotQuery{From: from, To: to, Duration: duration, HostUUID: hostUUID, Internal: true})
	return out, err
}
