package usecase

// The organization calendar, ported from otopoly-go
// backend/internal/modules/calendar (feed.go, move.go, service.go): a feed of
// layers the caller may see for a date range, with per-layer permissions,
// and drag-and-drop moves. ClickFix layers are appointments plus the agent
// shifts and approved time off of the schedules module (CF-105); the feed is
// filtered by host, team, type and status.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

// Layers the feed can return; each can be toggled in the UI.
const (
	LayerAppointments = "appointments"
	LayerShifts       = "shifts"
	LayerTimeOff      = "time_off"
)

// Layers in display order.
var Layers = []string{LayerAppointments, LayerShifts, LayerTimeOff}

// layerAccess is what a member needs to see (read) or move (write) a layer.
type layerAccess struct {
	read, write, refKind string
}

var access = map[string]layerAccess{
	LayerAppointments: {read: rbac.PermTenantAppointmentsRead, write: rbac.PermTenantAppointmentsWrite, refKind: "appointment"},
	LayerShifts:       {read: rbac.PermTenantSchedulesRead, refKind: "shift"},
	LayerTimeOff:      {read: rbac.PermTenantSchedulesRead, refKind: "time_off"},
}

// maxRangeDays caps one feed request (a 6-week month grid plus slack).
const maxRangeDays = 62

// FeedItem is one calendar entry. RefKind + RefUUID name the record the UI
// opens. Movable items can be rescheduled with POST /calendar/items/move.
type FeedItem struct {
	ID          string       `json:"id"`
	Layer       string       `json:"layer"`
	Kind        string       `json:"kind"`
	Title       string       `json:"title"`
	Subtitle    string       `json:"subtitle,omitempty"`
	StartsAt    time.Time    `json:"starts_at"`
	EndsAt      time.Time    `json:"ends_at"`
	AllDay      bool         `json:"all_day"`
	Status      string       `json:"status,omitempty"`
	Color       string       `json:"color,omitempty"`
	HostUUID    *uuid.UUID   `json:"host_uuid,omitempty"`
	RefKind     string       `json:"ref_kind,omitempty"`
	RefUUID     *uuid.UUID   `json:"ref_uuid,omitempty"`
	Movable     bool         `json:"movable"`
	Appointment *Appointment `json:"appointment,omitempty"`
}

// LayerInfo tells the client which layers this member may see and move.
type LayerInfo struct {
	Key     string `json:"key"`
	Movable bool   `json:"movable"`
}

// Feed is the calendar for [From, To) (dates, To exclusive).
type Feed struct {
	From     string           `json:"from"`
	To       string           `json:"to"`
	Timezone string           `json:"timezone"`
	Layers   []LayerInfo      `json:"layers"`
	Items    []FeedItem       `json:"items"`
	Counts   map[string]int64 `json:"counts"`
}

// Meta is calendar metadata for clients.
type Meta struct {
	Layers       []LayerInfo `json:"layers"`
	Colors       []string    `json:"colors"`
	Statuses     []string    `json:"statuses"`
	Timezone     string      `json:"timezone"`
	MaxRangeDays int         `json:"max_range_days"`
	Types        []TypeRef   `json:"types"`
}

// ParseLayers validates a comma-separated list; empty means every layer.
func ParseLayers(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Layers, nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, l := range strings.Split(raw, ",") {
		l = strings.TrimSpace(l)
		if l == "" || seen[l] {
			continue
		}
		if _, ok := access[l]; !ok {
			return nil, invalidf("unknown layer %q", l)
		}
		seen[l] = true
		out = append(out, l)
	}
	return out, nil
}

// Available lists the layers the caller may read, in display order.
func Available(ctx context.Context) []LayerInfo {
	sc, _ := orgctx.From(ctx)
	out := make([]LayerInfo, 0, len(Layers))
	for _, l := range Layers {
		a := access[l]
		if !sc.Has(a.read) {
			continue
		}
		out = append(out, LayerInfo{Key: l, Movable: a.write != "" && sc.Has(a.write)})
	}
	return out
}

// GetMeta returns the calendar metadata for the caller.
func (s *Service) GetMeta(ctx context.Context, org int64) (Meta, error) {
	st, err := s.settings(ctx, org)
	if err != nil {
		return Meta{}, err
	}
	rows, err := s.q.AppointmentTypes(ctx, db.AppointmentTypesParams{OrganizationID: org})
	if err != nil {
		return Meta{}, err
	}
	types := make([]TypeRef, 0, len(rows))
	for _, r := range rows {
		types = append(types, TypeRef{UUID: r.Uuid, Name: r.Name, Color: r.Color})
	}
	return Meta{Layers: Available(ctx), Colors: Colors, Statuses: Statuses, Timezone: st.Timezone, MaxRangeDays: maxRangeDays, Types: types}, nil
}

// ParseRange parses the [from, to) dates of a feed request.
func ParseRange(loc *time.Location, fromStr, toStr string) (time.Time, time.Time, error) {
	from, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(fromStr), loc)
	if err != nil {
		return from, from, invalidf("from must be YYYY-MM-DD")
	}
	to, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(toStr), loc)
	if err != nil {
		return from, to, invalidf("to must be YYYY-MM-DD")
	}
	if !to.After(from) {
		return from, to, invalidf("to must be after from")
	}
	if days := int(to.Sub(from).Hours()/24 + 0.5); days > maxRangeDays {
		return from, to, invalidf("range is limited to %d days", maxRangeDays)
	}
	return from, to, nil
}

// FeedFilter narrows the feed.
type FeedFilter struct {
	Layers    []string
	HostUUIDs []uuid.UUID
	TeamUUID  *uuid.UUID
	TypeUUIDs []uuid.UUID
	Statuses  []string
}

// GetFeed collects the requested layers the caller may see, concurrently.
// Layers outside the caller's permissions are silently left out.
func (s *Service) GetFeed(ctx context.Context, org int64, fromStr, toStr string, f FeedFilter) (Feed, error) {
	st, err := s.settings(ctx, org)
	if err != nil {
		return Feed{}, err
	}
	from, to, err := ParseRange(st.loc, fromStr, toStr)
	if err != nil {
		return Feed{}, err
	}
	avail := Available(ctx)
	movable := map[string]bool{}
	allowed := map[string]bool{}
	for _, l := range avail {
		allowed[l.Key] = true
		movable[l.Key] = l.Movable
	}
	var load []string
	for _, l := range f.Layers {
		if allowed[l] {
			load = append(load, l)
		}
	}
	hostIDs, err := s.feedHosts(ctx, org, f)
	if err != nil {
		return Feed{}, err
	}
	results := make([][]FeedItem, len(load))
	g, gctx := errgroup.WithContext(ctx)
	for i, layer := range load {
		g.Go(func() error {
			items, err := s.loadLayer(gctx, org, layer, from, to, hostIDs, f)
			results[i] = items
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return Feed{}, err
	}
	feed := Feed{
		From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Timezone: st.Timezone,
		Layers: avail, Items: []FeedItem{}, Counts: map[string]int64{},
	}
	for i, layer := range load {
		feed.Counts[layer] = int64(len(results[i]))
		for _, it := range results[i] {
			it.RefKind = access[layer].refKind
			it.Movable = movable[layer] && it.Movable
			feed.Items = append(feed.Items, it)
		}
	}
	return feed, nil
}

// feedHosts resolves the host / team filter to user ids (nil = everyone).
func (s *Service) feedHosts(ctx context.Context, org int64, f FeedFilter) ([]int64, error) {
	if f.TeamUUID == nil && len(f.HostUUIDs) == 0 {
		return nil, nil
	}
	ids := []int64{0}
	if f.TeamUUID != nil {
		t, err := s.q.AppointmentTeamByUUID(ctx, db.AppointmentTeamByUUIDParams{OrganizationID: org, Uuid: *f.TeamUUID})
		if err != nil {
			return nil, invalidf("unknown team")
		}
		members, err := s.q.AppointmentTeamHosts(ctx, db.AppointmentTeamHostsParams{OrganizationID: org, TeamIds: []int64{t.ID}})
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			if len(f.HostUUIDs) == 0 || slices.Contains(f.HostUUIDs, m.Uuid) {
				ids = append(ids, m.ID)
			}
		}
		return ids, nil
	}
	rows, err := s.q.AppointmentMembers(ctx, db.AppointmentMembersParams{OrganizationID: org, UserUuids: f.HostUUIDs})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids, nil
}

func (s *Service) loadLayer(ctx context.Context, org int64, layer string, from, to time.Time, hostIDs []int64, f FeedFilter) ([]FeedItem, error) {
	switch layer {
	case LayerAppointments:
		lf := ListFilter{From: &from, To: &to, Statuses: f.Statuses, TypeUUIDs: f.TypeUUIDs, Limit: 1000}
		if hostIDs != nil {
			lf.HostUUIDs = nil
		}
		rows, err := s.listByIDs(ctx, org, lf, hostIDs)
		if err != nil {
			return nil, err
		}
		out := make([]FeedItem, 0, len(rows))
		for _, a := range rows {
			out = append(out, appointmentItem(a))
		}
		return out, nil
	case LayerShifts, LayerTimeOff:
		members, err := s.memberIndex(ctx, org, hostIDs)
		if err != nil {
			return nil, err
		}
		ids := hostIDs
		if ids == nil {
			for id := range members {
				ids = append(ids, id)
			}
		}
		if layer == LayerShifts {
			rows, err := s.q.AppointmentHostShifts(ctx, db.AppointmentHostShiftsParams{OrganizationID: org, UserIds: ids, RangeFrom: ts(from), RangeTo: ts(to)})
			if err != nil {
				return nil, err
			}
			out := make([]FeedItem, 0, len(rows))
			for i, r := range rows {
				m := members[r.UserID]
				out = append(out, FeedItem{
					ID: fmt.Sprintf("shifts:%d:%d:%d", r.UserID, r.StartsAt.Time.Unix(), i), Layer: LayerShifts, Kind: "shift",
					Title: m.Name, StartsAt: r.StartsAt.Time.UTC(), EndsAt: r.EndsAt.Time.UTC(), Color: "slate", HostUUID: ref(m.UUID),
				})
			}
			return out, nil
		}
		rows, err := s.q.AppointmentHostTimeOff(ctx, db.AppointmentHostTimeOffParams{OrganizationID: org, UserIds: ids, RangeFrom: ts(from), RangeTo: ts(to)})
		if err != nil {
			return nil, err
		}
		out := make([]FeedItem, 0, len(rows))
		for i, r := range rows {
			m := members[r.UserID]
			start, end := r.StartsAt.Time.UTC(), r.EndsAt.Time.UTC()
			out = append(out, FeedItem{
				ID: fmt.Sprintf("time_off:%d:%d:%d", r.UserID, start.Unix(), i), Layer: LayerTimeOff, Kind: "time_off",
				Title: m.Name, StartsAt: start, EndsAt: end, AllDay: end.Sub(start) >= 24*time.Hour, Color: "rose", HostUUID: ref(m.UUID),
			})
		}
		return out, nil
	}
	return nil, invalidf("unknown layer %q", layer)
}

func ref(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// memberIndex maps member ids to names (all members when ids is nil).
func (s *Service) memberIndex(ctx context.Context, org int64, ids []int64) (map[int64]Member, error) {
	out := map[int64]Member{}
	if ids == nil {
		rows, err := s.q.ListOrganizationMembersByOrg(ctx, db.ListOrganizationMembersByOrgParams{OrganizationID: org, LimitCount: 1000})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.UserID] = Member{UUID: r.UserUuid, Name: fullName(r.Name, r.Surname), Email: r.Email}
		}
		return out, nil
	}
	rows, err := s.q.AppointmentMembersByID(ctx, db.AppointmentMembersByIDParams{OrganizationID: org, UserIds: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = Member{UUID: r.Uuid, Name: fullName(r.Name, r.Surname), Email: r.Email}
	}
	return out, nil
}

func (s *Service) listByIDs(ctx context.Context, org int64, f ListFilter, hostIDs []int64) ([]Appointment, error) {
	p := db.AppointmentListParams{OrganizationID: org, RangeFrom: ts(*f.From), RangeTo: ts(*f.To), RowLimit: f.Limit, HostIds: hostIDs}
	for _, st := range f.Statuses {
		if !slices.Contains(Statuses, st) {
			return nil, invalidf("unknown status %q", st)
		}
	}
	if len(f.Statuses) > 0 {
		p.Statuses = f.Statuses
	}
	if len(f.TypeUUIDs) > 0 {
		ids := []int64{0}
		for _, id := range f.TypeUUIDs {
			if t, err := s.q.AppointmentTypeByUUID(ctx, db.AppointmentTypeByUUIDParams{OrganizationID: org, Uuid: id}); err == nil {
				ids = append(ids, t.ID)
			}
		}
		p.TypeIds = ids
	}
	rows, err := s.q.AppointmentList(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]Appointment, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.mapRow(ctx, org, db.AppointmentDetailRow(r)))
	}
	return out, nil
}

func appointmentItem(a Appointment) FeedItem {
	id := a.UUID
	sub := a.Type.Name
	if a.Host != nil {
		sub += " · " + a.Host.Name
	}
	it := FeedItem{
		ID: "appointments:" + a.UUID.String(), Layer: LayerAppointments, Kind: "appointment", Title: a.Title,
		Subtitle: sub, StartsAt: a.StartsAt, EndsAt: a.EndsAt, Status: a.Status, Color: a.Type.Color,
		RefUUID: &id, Movable: active(a.Status), Appointment: &a,
	}
	if a.Host != nil {
		it.HostUUID = &a.Host.UUID
	}
	return it
}

// MoveInput reschedules one feed item (drag and drop / resize).
type MoveInput struct {
	Layer    string     `json:"layer"`
	RefUUID  uuid.UUID  `json:"ref_uuid"`
	StartsAt time.Time  `json:"starts_at"`
	EndsAt   time.Time  `json:"ends_at"`
	HostUUID *uuid.UUID `json:"host_uuid"`
}

// Move reschedules an appointment from the calendar. Agents may place it
// outside working hours (they see the calendar), never on top of the host's
// other bookings or time off.
func (s *Service) Move(ctx context.Context, org, actorID int64, in MoveInput) (FeedItem, error) {
	if in.RefUUID == uuid.Nil {
		return FeedItem{}, invalidf("ref_uuid is required")
	}
	if _, ok := access[in.Layer]; !ok {
		return FeedItem{}, invalidf("unknown layer %q", in.Layer)
	}
	allowed := false
	for _, l := range Available(ctx) {
		if l.Key == in.Layer {
			allowed = l.Movable
		}
	}
	if !allowed {
		return FeedItem{}, fmt.Errorf("%w: items of layer %q cannot be moved", ErrForbidden, in.Layer)
	}
	if !in.EndsAt.After(in.StartsAt) {
		return FeedItem{}, invalidf("ends_at must be after starts_at")
	}
	dur := in.EndsAt.Sub(in.StartsAt)
	if dur < 5*time.Minute || dur > 8*time.Hour {
		return FeedItem{}, invalidf("an appointment lasts between 5 minutes and 8 hours")
	}
	a, t, st, err := s.loadForChange(ctx, org, in.RefUUID)
	if err != nil {
		return FeedItem{}, err
	}
	if !active(a.Status) {
		return FeedItem{}, invalidf("only scheduled or confirmed appointments can be moved")
	}
	hostUUID := in.HostUUID
	if hostUUID == nil {
		hostUUID = s.hostUUIDOf(ctx, org, a)
	}
	moved, err := s.move(ctx, org, actorID, a, booking{
		t: t, st: st, start: in.StartsAt.UTC(), duration: dur, hostUUID: hostUUID,
		internal: true, skipWork: true, exceptID: a.ID,
	}, "")
	if err != nil {
		return FeedItem{}, err
	}
	it := appointmentItem(moved)
	it.RefKind = access[LayerAppointments].refKind
	return it, nil
}
