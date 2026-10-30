package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/engine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Ref names a linked record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// ConversationRef names a linked conversation.
type ConversationRef struct {
	UUID   uuid.UUID `json:"uuid"`
	Number int64     `json:"number"`
}

// TypeRef names the type of a booking.
type TypeRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

// Invitee is who the appointment is with.
type Invitee struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// Links are the add-to-calendar and self-service links of a booking.
type Links struct {
	Manage  string `json:"manage"`
	ICS     string `json:"ics"`
	Google  string `json:"google"`
	Outlook string `json:"outlook"`
}

// Appointment is a booking.
type Appointment struct {
	Video         *Video           `json:"video,omitempty"`
	VideoWarning  string           `json:"video_warning,omitempty"`
	UUID          uuid.UUID        `json:"uuid"`
	Status        string           `json:"status"`
	Title         string           `json:"title"`
	Notes         string           `json:"notes"`
	StartsAt      time.Time        `json:"starts_at"`
	EndsAt        time.Time        `json:"ends_at"`
	Timezone      string           `json:"timezone"`
	BufferBefore  int32            `json:"buffer_before"`
	BufferAfter   int32            `json:"buffer_after"`
	LocationKind  string           `json:"location_kind"`
	LocationValue string           `json:"location_value"`
	VideoRoomUUID *uuid.UUID       `json:"video_room_uuid"`
	Type          TypeRef          `json:"type"`
	Host          *Ref             `json:"host"`
	Invitee       Invitee          `json:"invitee"`
	IntakeValues  map[string]any   `json:"intake_values"`
	Contact       *Ref             `json:"contact"`
	Company       *Ref             `json:"company"`
	Conversation  *ConversationRef `json:"conversation"`
	Source        string           `json:"source"`
	CancelReason  string           `json:"cancel_reason"`
	CancelledBy   *string          `json:"cancelled_by"`
	CancelledAt   *time.Time       `json:"cancelled_at"`
	ConfirmedAt   *time.Time       `json:"confirmed_at"`
	CompletedAt   *time.Time       `json:"completed_at"`
	Links         Links            `json:"links"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func (s *Service) mapRow(ctx context.Context, org int64, r db.AppointmentDetailRow) Appointment {
	a := Appointment{
		UUID: r.Uuid, Status: r.Status, Title: r.Title, Notes: r.Notes, StartsAt: r.StartsAt.Time.UTC(),
		EndsAt: r.EndsAt.Time.UTC(), Timezone: r.Timezone, BufferBefore: r.BufferBefore, BufferAfter: r.BufferAfter,
		LocationKind: r.LocationKind, LocationValue: r.LocationValue, VideoRoomUUID: pgUUID(r.VideoRoomUuid),
		Type:    TypeRef{UUID: r.TypeUuid, Name: r.TypeName, Color: r.TypeColor},
		Invitee: Invitee{Name: r.InviteeName, Email: r.InviteeEmail, Phone: r.InviteePhone},
		Source:  r.Source, CancelReason: r.CancelReason, CancelledAt: tsPtr(r.CancelledAt),
		ConfirmedAt: tsPtr(r.ConfirmedAt), CompletedAt: tsPtr(r.CompletedAt),
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(), IntakeValues: map[string]any{},
	}
	_ = json.Unmarshal(r.IntakeValues, &a.IntakeValues)
	if r.CancelledBy.Valid {
		v := r.CancelledBy.String
		a.CancelledBy = &v
	}
	if id := pgUUID(r.HostUuid); id != nil {
		a.Host = &Ref{UUID: *id, Name: r.HostName}
	}
	if id := pgUUID(r.ContactUuid); id != nil {
		a.Contact = &Ref{UUID: *id, Name: r.ContactName}
	}
	if id := pgUUID(r.CompanyUuid); id != nil {
		a.Company = &Ref{UUID: *id, Name: r.CompanyName}
	}
	if id := pgUUID(r.ConversationUuid); id != nil {
		a.Conversation = &ConversationRef{UUID: *id, Number: r.ConversationNumber.Int64}
	}
	s.videoView(ctx, org, &a)
	a.Links = s.links(org, r.Uuid, r.TokenVersion, a)
	return a
}

// Get returns one booking.
func (s *Service) Get(ctx context.Context, org int64, id uuid.UUID) (Appointment, error) {
	row, err := s.q.AppointmentDetail(ctx, db.AppointmentDetailParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return Appointment{}, notFound(err)
	}
	return s.mapRow(ctx, org, row), nil
}

// ListFilter narrows a booking list.
type ListFilter struct {
	From, To         *time.Time
	Statuses         []string
	HostUUIDs        []uuid.UUID
	TeamUUID         *uuid.UUID
	TypeUUIDs        []uuid.UUID
	ContactUUID      *uuid.UUID
	CompanyUUID      *uuid.UUID
	ConversationUUID *uuid.UUID
	NewestFirst      bool
	Limit            int32
}

// List returns bookings.
func (s *Service) List(ctx context.Context, org int64, f ListFilter) ([]Appointment, error) {
	p := db.AppointmentListParams{OrganizationID: org, NewestFirst: f.NewestFirst, RowLimit: f.Limit}
	if p.RowLimit <= 0 || p.RowLimit > 1000 {
		p.RowLimit = 500
	}
	if f.From != nil {
		p.RangeFrom = ts(*f.From)
	}
	if f.To != nil {
		p.RangeTo = ts(*f.To)
	}
	for _, st := range f.Statuses {
		if !slices.Contains(Statuses, st) {
			return nil, invalidf("unknown status %q", st)
		}
	}
	if len(f.Statuses) > 0 {
		p.Statuses = f.Statuses
	}
	hostUUIDs := f.HostUUIDs
	if f.TeamUUID != nil {
		t, err := s.q.AppointmentTeamByUUID(ctx, db.AppointmentTeamByUUIDParams{OrganizationID: org, Uuid: *f.TeamUUID})
		if err != nil {
			return nil, invalidf("unknown team")
		}
		members, err := s.q.AppointmentTeamHosts(ctx, db.AppointmentTeamHostsParams{OrganizationID: org, TeamIds: []int64{t.ID}})
		if err != nil {
			return nil, err
		}
		ids := []int64{0}
		for _, m := range members {
			if len(hostUUIDs) == 0 || slices.Contains(hostUUIDs, m.Uuid) {
				ids = append(ids, m.ID)
			}
		}
		p.HostIds = ids
	} else if len(hostUUIDs) > 0 {
		rows, err := s.q.AppointmentMembers(ctx, db.AppointmentMembersParams{OrganizationID: org, UserUuids: hostUUIDs})
		if err != nil {
			return nil, err
		}
		ids := []int64{0}
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		p.HostIds = ids
	}
	if len(f.TypeUUIDs) > 0 {
		ids := []int64{0}
		for _, id := range f.TypeUUIDs {
			t, err := s.q.AppointmentTypeByUUID(ctx, db.AppointmentTypeByUUIDParams{OrganizationID: org, Uuid: id})
			if err == nil {
				ids = append(ids, t.ID)
			}
		}
		p.TypeIds = ids
	}
	if f.ContactUUID != nil {
		c, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: org, Uuid: *f.ContactUUID})
		if err != nil {
			return nil, notFound(err)
		}
		p.ContactID = i8(c.ID)
	}
	if f.CompanyUUID != nil {
		c, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: org, Uuid: *f.CompanyUUID})
		if err != nil {
			return nil, notFound(err)
		}
		p.CompanyID = i8(c.ID)
	}
	if f.ConversationUUID != nil {
		c, err := s.q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: *f.ConversationUUID})
		if err != nil {
			return nil, notFound(err)
		}
		p.ConversationID = i8(c.ID)
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

// Upcoming returns the next active bookings (mine: hosted by userID).
func (s *Service) Upcoming(ctx context.Context, org, userID int64, mine bool, limit int32) ([]Appointment, error) {
	now := s.now().UTC()
	f := ListFilter{From: &now, Statuses: []string{StatusScheduled, StatusConfirmed}, Limit: limit}
	if limit <= 0 || limit > 50 {
		f.Limit = 10
	}
	p := db.AppointmentListParams{OrganizationID: org, RangeFrom: ts(now), Statuses: f.Statuses, RowLimit: f.Limit}
	if mine {
		p.HostIds = []int64{userID}
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

// BookInput is an agent booking for a customer.
type BookInput struct {
	TypeUUID         uuid.UUID      `json:"type_uuid"`
	StartsAt         time.Time      `json:"starts_at"`
	Duration         int32          `json:"duration"`
	HostUUID         *uuid.UUID     `json:"host_uuid"`
	Timezone         string         `json:"timezone"`
	Title            string         `json:"title"`
	Notes            string         `json:"notes"`
	ContactUUID      *uuid.UUID     `json:"contact_uuid"`
	CompanyUUID      *uuid.UUID     `json:"company_uuid"`
	ConversationUUID *uuid.UUID     `json:"conversation_uuid"`
	InviteeName      string         `json:"invitee_name"`
	InviteeEmail     string         `json:"invitee_email"`
	InviteePhone     string         `json:"invitee_phone"`
	IntakeValues     map[string]any `json:"intake_values"`
	// OverrideAvailability books outside working hours (still never on top
	// of another booking or time off). Needs tenant.appointments.manage.
	OverrideAvailability bool `json:"override_availability"`
	// Notify sends the invitee the confirmation e-mail (default true).
	Notify *bool `json:"notify"`
}

// booking is a validated booking request.
type booking struct {
	t        db.AppointmentType
	st       orgSettings
	start    time.Time
	duration time.Duration
	hostUUID *uuid.UUID
	internal bool
	skipWork bool
	exceptID int64
	insert   db.AppointmentCreateParams
}

// ConversationAccess checks that the caller may see a conversation.
type ConversationAccess func(ctx context.Context, org int64, id uuid.UUID) (db.Conversation, error)

// Book creates a booking for an agent. canSee checks conversation visibility
// (team rules) when the booking starts from a conversation.
func (s *Service) Book(ctx context.Context, org, actorID int64, canManage bool, in BookInput, canSee ConversationAccess) (Appointment, error) {
	t, err := s.q.AppointmentTypeByUUID(ctx, db.AppointmentTypeByUUIDParams{OrganizationID: org, Uuid: in.TypeUUID})
	if err != nil {
		return Appointment{}, notFound(err)
	}
	if !t.IsActive {
		return Appointment{}, invalidf("appointment type is inactive")
	}
	if in.OverrideAvailability && !canManage {
		return Appointment{}, ErrForbidden
	}
	st, err := s.settings(ctx, org)
	if err != nil {
		return Appointment{}, err
	}
	dur, err := s.durationOf(t, in.Duration)
	if err != nil {
		return Appointment{}, err
	}
	p := db.AppointmentCreateParams{
		OrganizationID: org, TypeID: t.ID, Source: SourceInternal, BookedByUserID: i8(actorID),
		Notes: strings.TrimSpace(in.Notes), Title: strings.TrimSpace(in.Title),
	}
	if in.ConversationUUID != nil {
		conv, err := canSee(ctx, org, *in.ConversationUUID)
		if err != nil {
			return Appointment{}, err
		}
		p.ConversationID = i8(conv.ID)
		if conv.ContactID != 0 && in.ContactUUID == nil {
			c, err := s.q.GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: org, ID: conv.ContactID})
			if err == nil {
				in.ContactUUID = &c.Uuid
			}
		}
		if conv.CompanyID.Valid {
			p.CompanyID = conv.CompanyID
		}
	}
	if in.ContactUUID != nil {
		c, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: org, Uuid: *in.ContactUUID})
		if err != nil {
			return Appointment{}, invalidf("unknown contact")
		}
		p.ContactID = i8(c.ID)
		if !p.CompanyID.Valid {
			p.CompanyID = c.CompanyID
		}
		if in.InviteeName == "" {
			in.InviteeName = c.Name
		}
		if in.InviteeEmail == "" {
			in.InviteeEmail = c.Email.String
		}
		if in.InviteePhone == "" {
			in.InviteePhone = c.Phone.String
		}
	}
	if in.CompanyUUID != nil {
		c, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: org, Uuid: *in.CompanyUUID})
		if err != nil {
			return Appointment{}, invalidf("unknown company")
		}
		p.CompanyID = i8(c.ID)
		if in.InviteeName == "" {
			in.InviteeName = c.Name
		}
	}
	inv, err := normalizeInvitee(in.InviteeName, in.InviteeEmail, in.InviteePhone, false)
	if err != nil {
		return Appointment{}, err
	}
	values, err := s.checkIntake(ctx, org, t, in.IntakeValues, true)
	if err != nil {
		return Appointment{}, err
	}
	tz, err := zoneOr(in.Timezone, st.Timezone)
	if err != nil {
		return Appointment{}, err
	}
	if len([]rune(p.Notes)) > 5000 || len([]rune(p.Title)) > 200 {
		return Appointment{}, invalidf("notes or title too long")
	}
	p.InviteeName, p.InviteeEmail, p.InviteePhone, p.IntakeValues, p.Timezone = inv.Name, inv.Email, inv.Phone, values, tz
	b := booking{
		t: t, st: st, start: in.StartsAt.UTC(), duration: time.Duration(dur) * time.Minute, hostUUID: in.HostUUID,
		internal: true, skipWork: in.OverrideAvailability, insert: p,
	}
	row, err := s.book(ctx, org, b)
	if err != nil {
		return Appointment{}, err
	}
	notify := in.Notify == nil || *in.Notify
	s.afterChange(ctx, org, row.ID, change{event: "booked", actorID: actorID, notifyInvitee: notify})
	return s.Get(ctx, org, row.Uuid)
}

func zoneOr(tz, fallback string) (string, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return fallback, nil
	}
	if _, err := time.LoadLocation(tz); err != nil || tz == "Local" {
		return "", invalidf("invalid timezone")
	}
	return tz, nil
}

func normalizeInvitee(name, email, phone string, requireEmail bool) (Invitee, error) {
	inv := Invitee{Name: strings.TrimSpace(name), Email: strings.ToLower(strings.TrimSpace(email)), Phone: strings.TrimSpace(phone)}
	if len([]rune(inv.Name)) > 120 || len(inv.Email) > 254 || len(inv.Phone) > 40 {
		return inv, invalidf("invitee details are too long")
	}
	if inv.Email != "" {
		addr, err := mail.ParseAddress(inv.Email)
		if err != nil || addr.Address != inv.Email {
			return inv, invalidf("invalid e-mail")
		}
	}
	if requireEmail && (inv.Email == "" || inv.Name == "") {
		return inv, invalidf("name and e-mail are required")
	}
	if inv.Name == "" {
		inv.Name = inv.Email
	}
	return inv, nil
}

// checkIntake validates intake answers against the type's form and keeps
// only answers to its fields.
func (s *Service) checkIntake(ctx context.Context, org int64, t db.AppointmentType, values map[string]any, internal bool) ([]byte, error) {
	resolved, _, err := s.publicIntake(ctx, org, t)
	if err != nil {
		return nil, err
	}
	if values == nil {
		values = map[string]any{}
	}
	if len(resolved) > 0 {
		var optional []string
		if internal {
			// Agents may book before the customer answered.
			for _, f := range resolved {
				optional = append(optional, f.Key)
			}
		}
		if _, err := engine.Check(resolved, values, optional...); err != nil {
			var ve *customfields.ValidationError
			if errors.As(err, &ve) {
				return nil, invalidf("intake form: %s", ve.Error())
			}
			return nil, err
		}
	}
	kept := map[string]any{}
	for _, f := range resolved {
		if v, ok := values[f.Key]; ok {
			kept[f.Key] = v
		}
	}
	return json.Marshal(kept)
}

// book checks the slot under a host lock and inserts the booking.
func (s *Service) book(ctx context.Context, org int64, b booking) (db.Appointment, error) {
	t := b.t
	end := b.start.Add(b.duration)
	if !b.start.After(s.now()) {
		return db.Appointment{}, invalidf("starts_at must be in the future")
	}
	if !b.internal {
		if err := s.onGrid(ctx, org, b); err != nil {
			return db.Appointment{}, err
		}
	}
	video, err := s.preloadVideo(ctx, org, t)
	if err != nil {
		return db.Appointment{}, err
	}
	var out db.Appointment
	err = s.inTx(ctx, func(q *db.Queries) error {
		if b.exceptID != 0 {
			current, err := q.AppointmentLock(ctx, db.AppointmentLockParams{OrganizationID: org, ID: b.exceptID})
			if err != nil {
				return err
			}
			if !active(current.Status) {
				return invalidf("only active appointments can be rescheduled")
			}
		}
		hosts, err := s.hostsOf(ctx, q, org, t)
		if err != nil {
			return err
		}
		if b.hostUUID != nil {
			i := slices.IndexFunc(hosts, func(h host) bool { return h.uuid == *b.hostUUID })
			if i < 0 {
				return invalidf("host is not a host of this type")
			}
			hosts = []host{hosts[i]}
		}
		if len(hosts) == 0 {
			return ErrSlotTaken
		}
		// Lock every candidate in id order (deadlock-free), then pick by
		// round-robin order.
		ids := make([]int64, 0, len(hosts))
		for _, h := range hosts {
			ids = append(ids, h.id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			if err := q.AppointmentLockHost(ctx, db.AppointmentLockHostParams{OrganizationID: org, UserID: id}); err != nil {
				return err
			}
		}
		dayFrom, dayTo := localDay(b.start, b.st.loc)
		av, err := s.load(ctx, q, org, t, hosts, b.st.loc, minTime(dayFrom, b.start), maxTime(dayTo, end), b.exceptID, b.skipWork)
		if err != nil {
			return err
		}
		before := time.Duration(t.BufferBefore) * time.Minute
		after := time.Duration(t.BufferAfter) * time.Minute
		var chosen *host
		for i := range hosts {
			h := hosts[i]
			if !fits(av, h.id, b.start, end, before, after) {
				continue
			}
			if t.MaxPerDay > 0 && !b.internal && av.perDay[h.id][b.start.In(b.st.loc).Format(time.DateOnly)] >= int(t.MaxPerDay) {
				continue
			}
			chosen = &h
			break
		}
		if chosen == nil {
			return ErrSlotTaken
		}
		p := b.insert
		p.HostUserID = i8(chosen.id)
		p.StartsAt, p.EndsAt = ts(b.start), ts(end)
		p.BufferBefore, p.BufferAfter = t.BufferBefore, t.BufferAfter
		p.LocationKind, p.LocationValue = t.LocationKind, t.LocationValue
		if p.Title == "" {
			p.Title = t.Name
			if p.InviteeName != "" {
				p.Title = t.Name + " · " + p.InviteeName
			}
			if r := []rune(p.Title); len(r) > 200 {
				p.Title = string(r[:200])
			}
		}
		p.Status = StatusConfirmed
		if t.RequiresConfirmation {
			p.Status = StatusScheduled
		}
		if b.exceptID != 0 {
			out, err = q.AppointmentReschedule(ctx, db.AppointmentRescheduleParams{
				OrganizationID: org, ID: b.exceptID, StartsAt: p.StartsAt, EndsAt: p.EndsAt, HostUserID: p.HostUserID,
			})
		} else {
			out, err = q.AppointmentCreate(ctx, p)
		}
		if err != nil {
			return err
		}
		if err := s.syncVideo(ctx, q, video, out, chosen.uuid); err != nil {
			return err
		}
		return q.AppointmentTouchHost(ctx, db.AppointmentTouchHostParams{OrganizationID: org, TypeID: t.ID, UserID: chosen.id})
	})
	return out, err
}

// onGrid checks a public start against the computed slots.
func (s *Service) onGrid(ctx context.Context, org int64, b booking) error {
	from, to := localDay(b.start, b.st.loc)
	slots, _, err := s.slots(ctx, s.q, org, b.t, b.st, SlotQuery{
		From: from, To: to, Duration: minutes(b.duration), HostUUID: b.hostUUID, ExceptID: b.exceptID,
	})
	if err != nil {
		return err
	}
	for _, sl := range slots {
		if sl.StartsAt.Equal(b.start) {
			return nil
		}
	}
	return ErrSlotTaken
}

// fits: [start, end) is inside working time and, widened by the buffers,
// clear of the host's busy time.
func fits(av availability, hostID int64, start, end time.Time, before, after time.Duration) bool {
	inside := false
	for _, w := range av.work[hostID] {
		if !start.Before(w.start) && !end.After(w.end) {
			inside = true
			break
		}
	}
	if !inside {
		return false
	}
	block := span{start.Add(-before), end.Add(after)}
	for _, b := range av.busy[hostID] {
		if block.overlaps(b) {
			return false
		}
	}
	return true
}

func localDay(t time.Time, loc *time.Location) (time.Time, time.Time) {
	l := t.In(loc)
	from := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
	return from.UTC(), from.AddDate(0, 0, 1).UTC()
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// RescheduleInput moves a booking.
type RescheduleInput struct {
	StartsAt             time.Time  `json:"starts_at"`
	Duration             int32      `json:"duration"`
	HostUUID             *uuid.UUID `json:"host_uuid"`
	OverrideAvailability bool       `json:"override_availability"`
}

func (s *Service) loadForChange(ctx context.Context, org int64, id uuid.UUID) (db.Appointment, db.AppointmentType, orgSettings, error) {
	row, err := s.q.AppointmentDetail(ctx, db.AppointmentDetailParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return db.Appointment{}, db.AppointmentType{}, orgSettings{}, notFound(err)
	}
	a, err := s.q.AppointmentByID(ctx, db.AppointmentByIDParams{OrganizationID: org, ID: row.ID})
	if err != nil {
		return a, db.AppointmentType{}, orgSettings{}, notFound(err)
	}
	t, err := s.q.AppointmentTypeByID(ctx, db.AppointmentTypeByIDParams{OrganizationID: org, ID: a.TypeID})
	if err != nil {
		return a, t, orgSettings{}, err
	}
	st, err := s.settings(ctx, org)
	return a, t, st, err
}

func active(status string) bool { return status == StatusScheduled || status == StatusConfirmed }

// Reschedule moves a booking (agent). Any duration of the type may be used;
// a host change is allowed when the new host is a host of the type.
func (s *Service) Reschedule(ctx context.Context, org, actorID int64, canManage bool, id uuid.UUID, in RescheduleInput) (Appointment, error) {
	a, t, st, err := s.loadForChange(ctx, org, id)
	if err != nil {
		return Appointment{}, err
	}
	if !active(a.Status) {
		return Appointment{}, invalidf("only scheduled or confirmed appointments can be rescheduled")
	}
	if in.OverrideAvailability && !canManage {
		return Appointment{}, ErrForbidden
	}
	dur := minutes(a.EndsAt.Time.Sub(a.StartsAt.Time))
	if in.Duration != 0 {
		if in.Duration < 5 || in.Duration > 480 {
			return Appointment{}, invalidf("duration must be between 5 and 480 minutes")
		}
		dur = in.Duration
	}
	hostUUID := in.HostUUID
	if hostUUID == nil {
		hostUUID = s.hostUUIDOf(ctx, org, a)
	}
	return s.move(ctx, org, actorID, a, booking{
		t: t, st: st, start: in.StartsAt.UTC(), duration: time.Duration(dur) * time.Minute, hostUUID: hostUUID,
		internal: true, skipWork: in.OverrideAvailability, exceptID: a.ID,
	}, "")
}

func (s *Service) hostUUIDOf(ctx context.Context, org int64, a db.Appointment) *uuid.UUID {
	if !a.HostUserID.Valid {
		return nil
	}
	rows, err := s.q.AppointmentMembersByID(ctx, db.AppointmentMembersByIDParams{OrganizationID: org, UserIds: []int64{a.HostUserID.Int64}})
	if err != nil || len(rows) == 0 {
		return nil
	}
	return &rows[0].Uuid
}

func (s *Service) move(ctx context.Context, org, actorID int64, a db.Appointment, b booking, by string) (Appointment, error) {
	prev := a.StartsAt.Time.UTC()
	row, err := s.book(ctx, org, b)
	if errors.Is(err, ErrSlotTaken) && b.hostUUID != nil && !b.internal {
		// The invitee's host is busy then: any host of the type may take it.
		b.hostUUID = nil
		row, err = s.book(ctx, org, b)
	}
	if err != nil {
		return Appointment{}, err
	}
	s.afterChange(ctx, org, row.ID, change{event: "rescheduled", actorID: actorID, by: by, previous: &prev, notifyInvitee: true})
	return s.Get(ctx, org, row.Uuid)
}

// StatusInput changes the lifecycle status.
type StatusInput struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

var transitions = map[string][]string{
	StatusScheduled: {StatusConfirmed, StatusCompleted, StatusNoShow, StatusCancelled},
	StatusConfirmed: {StatusCompleted, StatusNoShow, StatusCancelled},
	StatusCompleted: {StatusNoShow},
	StatusNoShow:    {StatusCompleted},
}

// SetStatus moves a booking through its lifecycle (agent).
func (s *Service) SetStatus(ctx context.Context, org, actorID int64, id uuid.UUID, in StatusInput) (Appointment, error) {
	a, _, _, err := s.loadForChange(ctx, org, id)
	if err != nil {
		return Appointment{}, err
	}
	if !slices.Contains(transitions[a.Status], in.Status) {
		return Appointment{}, invalidf("cannot change status from %s to %s", a.Status, in.Status)
	}
	if (in.Status == StatusCompleted || in.Status == StatusNoShow) && a.StartsAt.Time.After(s.now()) {
		return Appointment{}, invalidf("an appointment can be completed or marked no-show only after it starts")
	}
	if a.VideoRoomUuid.Valid && (in.Status == StatusCompleted || in.Status == StatusNoShow) {
		attendance, err := s.q.VideoAttendance(ctx, db.VideoAttendanceParams{OrganizationID: org, Uuid: uuid.UUID(a.VideoRoomUuid.Bytes)})
		if err != nil {
			return Appointment{}, err
		}
		want := StatusNoShow
		if attendance.HostJoined && attendance.GuestJoined {
			want = StatusCompleted
		}
		if in.Status != want {
			return Appointment{}, invalidf("video outcome must match host and guest participation")
		}
		if want == StatusNoShow {
			room, err := s.q.VideoRoom(ctx, db.VideoRoomParams{OrganizationID: org, Uuid: uuid.UUID(a.VideoRoomUuid.Bytes)})
			if err != nil {
				return Appointment{}, err
			}
			if room.ScheduledEndAt.Valid && s.now().Before(room.ScheduledEndAt.Time) {
				return Appointment{}, invalidf("video no-show can be determined only after the room window ends")
			}
		}
	}
	reason := strings.TrimSpace(in.Reason)
	if len([]rune(reason)) > 1000 {
		return Appointment{}, invalidf("reason is limited to 1000 characters")
	}
	return s.setStatus(ctx, org, actorID, a, in.Status, "host", reason)
}

func (s *Service) setStatus(ctx context.Context, org, actorID int64, a db.Appointment, status, by, reason string) (Appointment, error) {
	var row db.Appointment
	err := s.inTx(ctx, func(q *db.Queries) error {
		current, err := q.AppointmentLock(ctx, db.AppointmentLockParams{OrganizationID: org, ID: a.ID})
		if err != nil {
			return err
		}
		if current.Sequence != a.Sequence {
			return invalidf("appointment has changed; reload before updating its status")
		}
		if !slices.Contains(transitions[current.Status], status) {
			return invalidf("appointment status has changed")
		}
		if status == StatusCancelled && current.VideoRoomUuid.Valid && s.video != nil {
			if _, err = s.video.WithQueries(q).EndRoom(ctx, org, uuid.UUID(current.VideoRoomUuid.Bytes)); err != nil {
				return err
			}
		}
		p := db.AppointmentSetStatusParams{OrganizationID: org, ID: a.ID, Status: status, CancelReason: reason}
		if status == StatusCancelled {
			p.CancelledBy = pgtype.Text{String: by, Valid: true}
		}
		row, err = q.AppointmentSetStatus(ctx, p)
		return err
	})
	if err != nil {
		return Appointment{}, notFound(err)
	}

	s.afterChange(ctx, org, row.ID, change{
		event: status, actorID: actorID, by: by, notifyInvitee: status == StatusCancelled || status == StatusConfirmed,
	})
	return s.Get(ctx, org, row.Uuid)
}

// Cancel cancels a booking (agent).
func (s *Service) Cancel(ctx context.Context, org, actorID int64, id uuid.UUID, reason string) (Appointment, error) {
	return s.SetStatus(ctx, org, actorID, id, StatusInput{Status: StatusCancelled, Reason: reason})
}

// DetailsInput edits the free-text fields of a booking.
type DetailsInput struct {
	Title         string `json:"title"`
	Notes         string `json:"notes"`
	LocationValue string `json:"location_value"`
}

// UpdateDetails edits title, notes and location details.
func (s *Service) UpdateDetails(ctx context.Context, org int64, id uuid.UUID, in DetailsInput) (Appointment, error) {
	a, _, _, err := s.loadForChange(ctx, org, id)
	if err != nil {
		return Appointment{}, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = a.Title
	}
	if len([]rune(title)) > 200 || len([]rune(in.Notes)) > 5000 || len([]rune(in.LocationValue)) > 500 {
		return Appointment{}, invalidf("title, notes or location too long")
	}
	row, err := s.q.AppointmentUpdateDetails(ctx, db.AppointmentUpdateDetailsParams{
		OrganizationID: org, ID: a.ID, Title: title, Notes: strings.TrimSpace(in.Notes), LocationValue: strings.TrimSpace(in.LocationValue),
	})
	if err != nil {
		return Appointment{}, notFound(err)
	}
	s.live(ctx, org, "appointment", row.Uuid)
	return s.Get(ctx, org, row.Uuid)
}

// findOrCreateContact links a public booking to the contact with its e-mail
// (creating one), so it shows on the contact and company timelines.
func (s *Service) findOrCreateContact(ctx context.Context, org int64, inv Invitee) (db.Contact, error) {
	c, err := s.q.FindContactByEmail(ctx, db.FindContactByEmailParams{OrganizationID: org, Email: inv.Email})
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return c, err
	}
	p := db.InsertContactParams{
		OrganizationID: org, Name: inv.Name, Email: pgtype.Text{String: inv.Email, Valid: true},
		CustomFields: []byte("{}"),
	}
	if inv.Phone != "" {
		p.Phone = pgtype.Text{String: inv.Phone, Valid: true}
	}
	return s.q.InsertContact(ctx, p)
}
