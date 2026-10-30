package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	formsmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/google/uuid"
)

// Manage tokens are signed {org, appointment, version}. A reschedule or a
// cancellation bumps token_version, so links from older e-mails stop
// working; no token is stored.

type tokenClaims struct {
	Org     int64     `json:"o"`
	ID      uuid.UUID `json:"i"`
	Version int32     `json:"v"`
}

func (s *Service) mac(enc string) string {
	h := hmac.New(sha256.New, []byte(s.secret))
	_, _ = h.Write([]byte("clickfix/appointment-manage/v1:" + enc))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// Token signs a manage token.
func (s *Service) Token(org int64, id uuid.UUID, version int32) string {
	raw, _ := json.Marshal(tokenClaims{Org: org, ID: id, Version: version})
	enc := base64.RawURLEncoding.EncodeToString(raw)
	return enc + "." + s.mac(enc)
}

func (s *Service) parseToken(token string) (tokenClaims, error) {
	var c tokenClaims
	enc, sig, ok := strings.Cut(token, ".")
	if !ok || len(token) > 512 || s.secret == "" || !hmac.Equal([]byte(sig), []byte(s.mac(enc))) {
		return c, ErrNotFound
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil || json.Unmarshal(raw, &c) != nil || c.ID == uuid.Nil || c.Org == 0 {
		return c, ErrNotFound
	}
	return c, nil
}

func (s *Service) links(org int64, id uuid.UUID, version int32, a Appointment) Links {
	token := s.Token(org, id, version)
	l := Links{
		Manage: s.baseURL + "/book/manage/" + token,
		ICS:    s.baseURL + "/api/v1/public/appointments/" + token + "/ics",
	}
	if a.Status == StatusCancelled {
		return l
	}
	const f = "20060102T150405Z"
	details := strings.TrimSpace(a.Notes + "\n" + videoText(a))
	if a.LocationKind == LocationCustomURL || a.LocationKind == LocationVideo {
		details = strings.TrimSpace(a.LocationValue + "\n" + details)
	}
	g := url.Values{}
	g.Set("action", "TEMPLATE")
	g.Set("text", a.Title)
	g.Set("dates", a.StartsAt.UTC().Format(f)+"/"+a.EndsAt.UTC().Format(f))
	g.Set("details", details)
	g.Set("location", a.LocationValue)
	l.Google = "https://calendar.google.com/calendar/render?" + g.Encode()
	o := url.Values{}
	o.Set("path", "/calendar/action/compose")
	o.Set("rru", "addevent")
	o.Set("subject", a.Title)
	o.Set("startdt", a.StartsAt.UTC().Format(time.RFC3339))
	o.Set("enddt", a.EndsAt.UTC().Format(time.RFC3339))
	o.Set("body", details)
	o.Set("location", a.LocationValue)
	l.Outlook = "https://outlook.live.com/calendar/0/deeplink/compose?" + o.Encode()
	return l
}

// ICS renders an iCalendar (RFC 5545) document for one booking. SEQUENCE
// grows with every change so calendar apps replace the earlier copy.
func ICS(a Appointment, sequence int32, organizer string) []byte {
	const f = "20060102T150405Z"
	var b strings.Builder
	line := func(s string) {
		// Fold lines longer than 75 octets.
		for len(s) > 75 {
			cut := 75
			for cut > 0 && (s[cut]&0xC0) == 0x80 {
				cut--
			}
			b.WriteString(s[:cut] + "\r\n")
			s = " " + s[cut:]
		}
		b.WriteString(s + "\r\n")
	}
	status := "CONFIRMED"
	method := "PUBLISH"
	switch a.Status {
	case StatusCancelled:
		status, method = "CANCELLED", "CANCEL"
	case StatusScheduled:
		status = "TENTATIVE"
	}
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//ClickFix//Appointments//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:" + method)
	line("BEGIN:VEVENT")
	line("UID:" + a.UUID.String() + "@clickfix")
	line(fmt.Sprintf("SEQUENCE:%d", sequence))
	line("DTSTAMP:" + a.UpdatedAt.UTC().Format(f))
	line("DTSTART:" + a.StartsAt.UTC().Format(f))
	line("DTEND:" + a.EndsAt.UTC().Format(f))
	line("SUMMARY:" + icsText(a.Title))
	desc := strings.TrimSpace(a.Notes + "\n" + videoText(a))
	if a.Links.Manage != "" && a.Status != StatusCancelled {
		desc = strings.TrimSpace(desc + "\n\n" + a.Links.Manage)
	}
	if desc != "" {
		line("DESCRIPTION:" + icsText(desc))
	}
	if a.LocationValue != "" {
		line("LOCATION:" + icsText(a.LocationValue))
		if strings.HasPrefix(a.LocationValue, "https://") {
			line("URL:" + a.LocationValue)
		}
	}
	if organizer != "" {
		line("ORGANIZER;CN=" + icsParam(organizer) + ":mailto:noreply@clickfix")
	}
	line("STATUS:" + status)
	line("END:VEVENT")
	line("END:VCALENDAR")
	return []byte(b.String())
}

func icsText(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", "")
	return r.Replace(s)
}

func icsParam(s string) string {
	s = strings.NewReplacer(`"`, "", "\r", "", "\n", " ", ";", " ", ":", " ", ",", " ").Replace(s)
	return `"` + s + `"`
}

// ICSFor renders the ICS of a booking of the organization.
func (s *Service) ICSFor(ctx context.Context, org int64, id uuid.UUID) ([]byte, error) {
	a, err := s.Get(ctx, org, id)
	if err != nil {
		return nil, err
	}
	row, err := s.q.AppointmentDetail(ctx, db.AppointmentDetailParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, notFound(err)
	}
	st, err := s.settings(ctx, org)
	if err != nil {
		return nil, err
	}
	return ICS(a, row.Sequence, st.OrganizationName), nil
}

// ---------------------------------------------------------------------------
// Public booking page (/book/<org>/<type>): no auth, rate limited by the
// handler, abuse limited per e-mail here.

// PublicOrganization is an organization's public booking index.
type PublicOrganization struct {
	Name     string       `json:"name"`
	Slug     string       `json:"slug"`
	Timezone string       `json:"timezone"`
	Locale   string       `json:"locale"`
	Types    []PublicType `json:"types"`
}

// PublicType is what an invitee sees about a type.
type PublicType struct {
	Slug              string                   `json:"slug"`
	Name              string                   `json:"name"`
	Description       string                   `json:"description"`
	Color             string                   `json:"color"`
	DurationOptions   []int32                  `json:"duration_options"`
	DefaultDuration   int32                    `json:"default_duration"`
	LocationKind      string                   `json:"location_kind"`
	BookingWindowDays int32                    `json:"booking_window_days"`
	Fields            []formsmodel.PublicField `json:"fields,omitempty"`
}

type publicScope struct {
	org db.AppointmentPublicOrganizationRow
	st  orgSettings
}

func (s *Service) publicOrg(ctx context.Context, slug string) (publicScope, error) {
	o, err := s.q.AppointmentPublicOrganization(ctx, strings.ToLower(strings.TrimSpace(slug)))
	if err != nil {
		return publicScope{}, notFound(err)
	}
	st, err := s.settings(ctx, o.ID)
	if err != nil {
		return publicScope{}, err
	}
	if !st.Enabled || !st.PublicBooking || entitlement.Require(ctx, s.ent, o.ID, Feature) != nil {
		return publicScope{}, ErrNotFound
	}
	return publicScope{org: o, st: st}, nil
}

func publicType(t db.AppointmentType) PublicType {
	return PublicType{
		Slug: t.Slug, Name: t.Name, Description: t.Description, Color: t.Color, DurationOptions: nonNil(t.DurationOptions),
		DefaultDuration: t.DefaultDuration, LocationKind: t.LocationKind, BookingWindowDays: t.BookingWindowDays,
	}
}

// PublicIndex lists an organization's bookable types (booking page index
// and the widget entry point).
func (s *Service) PublicIndex(ctx context.Context, slug string) (PublicOrganization, error) {
	sc, err := s.publicOrg(ctx, slug)
	if err != nil {
		return PublicOrganization{}, err
	}
	rows, err := s.q.AppointmentTypes(ctx, db.AppointmentTypesParams{OrganizationID: sc.org.ID, OnlyPublic: true})
	if err != nil {
		return PublicOrganization{}, err
	}
	out := PublicOrganization{Name: sc.org.Name, Slug: sc.org.Slug, Timezone: sc.st.Timezone, Locale: sc.org.Locale, Types: []PublicType{}}
	for _, r := range rows {
		out.Types = append(out.Types, publicType(r))
	}
	return out, nil
}

func (s *Service) publicType(ctx context.Context, orgSlug, typeSlug string) (publicScope, db.AppointmentType, error) {
	sc, err := s.publicOrg(ctx, orgSlug)
	if err != nil {
		return sc, db.AppointmentType{}, err
	}
	t, err := s.q.AppointmentTypeBySlug(ctx, db.AppointmentTypeBySlugParams{OrganizationID: sc.org.ID, Slug: strings.ToLower(typeSlug)})
	if err != nil {
		return sc, t, notFound(err)
	}
	return sc, t, nil
}

// PublicTypeDetail is a type's booking page.
type PublicTypeDetail struct {
	Organization PublicOrganization `json:"organization"`
	Type         PublicType         `json:"type"`
}

// PublicGetType returns a type's booking page with its intake form.
func (s *Service) PublicGetType(ctx context.Context, orgSlug, typeSlug string) (PublicTypeDetail, error) {
	sc, t, err := s.publicType(ctx, orgSlug, typeSlug)
	if err != nil {
		return PublicTypeDetail{}, err
	}
	pt := publicType(t)
	_, fields, err := s.publicIntake(ctx, sc.org.ID, t)
	if err != nil {
		return PublicTypeDetail{}, err
	}
	pt.Fields = fields
	return PublicTypeDetail{
		Organization: PublicOrganization{Name: sc.org.Name, Slug: sc.org.Slug, Timezone: sc.st.Timezone, Locale: sc.org.Locale, Types: []PublicType{}},
		Type:         pt,
	}, nil
}

// PublicSlot is a free start (hosts are not disclosed).
type PublicSlot struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

// PublicSlots returns the free slots of a public type.
func (s *Service) PublicSlots(ctx context.Context, orgSlug, typeSlug, fromStr, toStr string, duration int32) ([]PublicSlot, error) {
	sc, t, err := s.publicType(ctx, orgSlug, typeSlug)
	if err != nil {
		return nil, err
	}
	from, to, err := SlotRange(sc.st.loc, fromStr, toStr)
	if err != nil {
		return nil, err
	}
	slots, _, err := s.slots(ctx, s.q, sc.org.ID, t, sc.st, SlotQuery{From: from, To: to, Duration: duration})
	if err != nil {
		return nil, err
	}
	out := make([]PublicSlot, 0, len(slots))
	for _, sl := range slots {
		out = append(out, PublicSlot{StartsAt: sl.StartsAt, EndsAt: sl.EndsAt})
	}
	return out, nil
}

// PublicBookInput is an invitee's booking.
type PublicBookInput struct {
	StartsAt time.Time      `json:"starts_at"`
	Duration int32          `json:"duration"`
	Timezone string         `json:"timezone"`
	Name     string         `json:"name"`
	Email    string         `json:"email"`
	Phone    string         `json:"phone"`
	Notes    string         `json:"notes"`
	Values   map[string]any `json:"values"`
	// Source is "public" (booking page) or "widget".
	Source string `json:"source"`
}

// maxBookingsPerEmail caps public bookings one e-mail makes in a day.
const maxBookingsPerEmail = 5

// PublicManage is an invitee's view of their booking.
type PublicManage struct {
	Video          *GuestVideo `json:"video,omitempty"`
	VideoWarning   string      `json:"video_warning,omitempty"`
	Organization   string      `json:"organization"`
	UUID           uuid.UUID   `json:"uuid"`
	Status         string      `json:"status"`
	Title          string      `json:"title"`
	TypeName       string      `json:"type_name"`
	TypeSlug       string      `json:"type_slug"`
	OrgSlug        string      `json:"organization_slug"`
	StartsAt       time.Time   `json:"starts_at"`
	EndsAt         time.Time   `json:"ends_at"`
	Timezone       string      `json:"timezone"`
	LocationKind   string      `json:"location_kind"`
	LocationValue  string      `json:"location_value"`
	HostName       string      `json:"host_name"`
	InviteeName    string      `json:"invitee_name"`
	Duration       int32       `json:"duration"`
	CanCancel      bool        `json:"can_cancel"`
	CanReschedule  bool        `json:"can_reschedule"`
	Token          string      `json:"token"`
	Links          Links       `json:"links"`
	RequiresReview bool        `json:"requires_confirmation"`
}

// PublicBook books a public type.
func (s *Service) PublicBook(ctx context.Context, orgSlug, typeSlug string, in PublicBookInput) (PublicManage, error) {
	sc, t, err := s.publicType(ctx, orgSlug, typeSlug)
	if err != nil {
		return PublicManage{}, err
	}
	org := sc.org.ID
	inv, err := normalizeInvitee(in.Name, in.Email, in.Phone, true)
	if err != nil {
		return PublicManage{}, err
	}
	dur, err := s.durationOf(t, in.Duration)
	if err != nil {
		return PublicManage{}, err
	}
	tz, err := zoneOr(in.Timezone, sc.st.Timezone)
	if err != nil {
		return PublicManage{}, err
	}
	notes := strings.TrimSpace(in.Notes)
	if len([]rune(notes)) > 2000 {
		return PublicManage{}, invalidf("notes are limited to 2000 characters")
	}
	values, err := s.checkIntake(ctx, org, t, in.Values, false)
	if err != nil {
		return PublicManage{}, err
	}
	n, err := s.q.AppointmentCountRecentByEmail(ctx, db.AppointmentCountRecentByEmailParams{
		OrganizationID: org, Email: inv.Email, Since: ts(s.now().Add(-24 * time.Hour)),
	})
	if err != nil {
		return PublicManage{}, err
	}
	if n >= maxBookingsPerEmail {
		return PublicManage{}, ErrRateLimited
	}
	contact, err := s.findOrCreateContact(ctx, org, inv)
	if err != nil {
		return PublicManage{}, err
	}
	source := SourcePublic
	if in.Source == SourceWidget {
		source = SourceWidget
	}
	row, err := s.book(ctx, org, booking{
		t: t, st: sc.st, start: in.StartsAt.UTC(), duration: time.Duration(dur) * time.Minute,
		insert: db.AppointmentCreateParams{
			OrganizationID: org, TypeID: t.ID, Source: source, Notes: notes, Timezone: tz,
			InviteeName: inv.Name, InviteeEmail: inv.Email, InviteePhone: inv.Phone, IntakeValues: values,
			ContactID: i8(contact.ID), CompanyID: contact.CompanyID,
		},
	})
	if err != nil {
		return PublicManage{}, err
	}
	s.afterChange(ctx, org, row.ID, change{event: "booked", by: "invitee", notifyInvitee: true})
	return s.manageView(ctx, org, row.Uuid)
}

func (s *Service) manageView(ctx context.Context, org int64, id uuid.UUID) (PublicManage, error) {
	row, err := s.q.AppointmentDetail(ctx, db.AppointmentDetailParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return PublicManage{}, notFound(err)
	}
	t, err := s.q.AppointmentTypeByID(ctx, db.AppointmentTypeByIDParams{OrganizationID: org, ID: row.TypeID})
	if err != nil {
		return PublicManage{}, err
	}
	st, err := s.settings(ctx, org)
	if err != nil {
		return PublicManage{}, err
	}
	a := s.mapRow(ctx, org, row)
	cutoff := a.StartsAt.Add(-time.Duration(t.CancelNoticeMinutes) * time.Minute)
	open := active(a.Status) && s.now().Before(cutoff)
	return PublicManage{
		Video: guestVideo(a.Video), VideoWarning: a.VideoWarning,
		Organization: st.OrganizationName, OrgSlug: st.OrganizationSlug, UUID: a.UUID, Status: a.Status, Title: a.Title,
		TypeName: t.Name, TypeSlug: t.Slug, StartsAt: a.StartsAt, EndsAt: a.EndsAt, Timezone: a.Timezone,
		LocationKind: a.LocationKind, LocationValue: a.LocationValue, InviteeName: a.Invitee.Name,
		HostName: hostName(a), Duration: minutes(a.EndsAt.Sub(a.StartsAt)),
		CanCancel: open && t.AllowCancel, CanReschedule: open && t.AllowReschedule && t.IsActive,
		Token: s.Token(org, a.UUID, row.TokenVersion), Links: a.Links, RequiresReview: a.Status == StatusScheduled,
	}, nil
}

func hostName(a Appointment) string {
	if a.Host == nil {
		return ""
	}
	return a.Host.Name
}

// fromToken resolves a manage token to its booking (current version only).
func (s *Service) fromToken(ctx context.Context, token string) (int64, db.AppointmentDetailRow, error) {
	c, err := s.parseToken(token)
	if err != nil {
		return 0, db.AppointmentDetailRow{}, err
	}
	row, err := s.q.AppointmentDetail(ctx, db.AppointmentDetailParams{OrganizationID: c.Org, Uuid: c.ID})
	if err != nil {
		return 0, row, notFound(err)
	}
	if row.TokenVersion != c.Version {
		return 0, row, ErrNotFound
	}
	if err := entitlement.Require(ctx, s.ent, c.Org, Feature); err != nil {
		return 0, row, ErrNotFound
	}
	return c.Org, row, nil
}

// PublicManageGet returns the booking of a manage token.
func (s *Service) PublicManageGet(ctx context.Context, token string) (PublicManage, error) {
	org, row, err := s.fromToken(ctx, token)
	if err != nil {
		return PublicManage{}, err
	}
	return s.manageView(ctx, org, row.Uuid)
}

// PublicICS returns the ICS of a manage token's booking.
func (s *Service) PublicICS(ctx context.Context, token string) ([]byte, error) {
	org, row, err := s.fromToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return s.ICSFor(ctx, org, row.Uuid)
}

// PublicCancel cancels by manage token when the type's policy allows it.
func (s *Service) PublicCancel(ctx context.Context, token, reason string) (PublicManage, error) {
	org, row, err := s.fromToken(ctx, token)
	if err != nil {
		return PublicManage{}, err
	}
	view, err := s.manageView(ctx, org, row.Uuid)
	if err != nil {
		return PublicManage{}, err
	}
	if !view.CanCancel {
		return PublicManage{}, ErrPolicy
	}
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > 1000 {
		return PublicManage{}, invalidf("reason is limited to 1000 characters")
	}
	a, err := s.q.AppointmentByID(ctx, db.AppointmentByIDParams{OrganizationID: org, ID: row.ID})
	if err != nil {
		return PublicManage{}, notFound(err)
	}
	if _, err := s.setStatus(ctx, org, 0, a, StatusCancelled, "invitee", reason); err != nil {
		return PublicManage{}, err
	}
	return s.manageView(ctx, org, row.Uuid)
}

// PublicRescheduleInput is an invitee's new time.
type PublicRescheduleInput struct {
	StartsAt time.Time `json:"starts_at"`
}

// PublicReschedule moves a booking by manage token (same duration; the
// host is kept when free, else any host of the type takes it).
func (s *Service) PublicReschedule(ctx context.Context, token string, in PublicRescheduleInput) (PublicManage, error) {
	org, row, err := s.fromToken(ctx, token)
	if err != nil {
		return PublicManage{}, err
	}
	view, err := s.manageView(ctx, org, row.Uuid)
	if err != nil {
		return PublicManage{}, err
	}
	if !view.CanReschedule {
		return PublicManage{}, ErrPolicy
	}
	a, t, st, err := s.loadForChange(ctx, org, row.Uuid)
	if err != nil {
		return PublicManage{}, err
	}
	dur := a.EndsAt.Time.Sub(a.StartsAt.Time)
	if _, err := s.durationOf(t, minutes(dur)); err != nil {
		dur = time.Duration(t.DefaultDuration) * time.Minute
	}
	if _, err := s.move(ctx, org, 0, a, booking{
		t: t, st: st, start: in.StartsAt.UTC(), duration: dur, hostUUID: s.hostUUIDOf(ctx, org, a), exceptID: a.ID,
	}, "invitee"); err != nil {
		return PublicManage{}, err
	}
	return s.manageView(ctx, org, row.Uuid)
}

// PublicRescheduleSlots lists the free slots for a manage token's booking.
func (s *Service) PublicRescheduleSlots(ctx context.Context, token, fromStr, toStr string) ([]PublicSlot, error) {
	org, row, err := s.fromToken(ctx, token)
	if err != nil {
		return nil, err
	}
	a, t, st, err := s.loadForChange(ctx, org, row.Uuid)
	if err != nil {
		return nil, err
	}
	from, to, err := SlotRange(st.loc, fromStr, toStr)
	if err != nil {
		return nil, err
	}
	dur := minutes(a.EndsAt.Time.Sub(a.StartsAt.Time))
	if _, err := s.durationOf(t, dur); err != nil {
		dur = t.DefaultDuration
	}
	slots, _, err := s.slots(ctx, s.q, org, t, st, SlotQuery{From: from, To: to, Duration: dur, ExceptID: a.ID})
	if err != nil {
		return nil, err
	}
	out := make([]PublicSlot, 0, len(slots))
	for _, sl := range slots {
		out = append(out, PublicSlot{StartsAt: sl.StartsAt, EndsAt: sl.EndsAt})
	}
	return out, nil
}

// BookingURL is the widget entry point: the organization's public booking
// page (`?source=widget` marks bookings made from it) when it has at least
// one bookable type.
func (s *Service) BookingURL(ctx context.Context, org int64) (string, bool) {
	st, err := s.settings(ctx, org)
	if err != nil || !st.Enabled || !st.PublicBooking || entitlement.Require(ctx, s.ent, org, Feature) != nil {
		return "", false
	}
	rows, err := s.q.AppointmentTypes(ctx, db.AppointmentTypesParams{OrganizationID: org, OnlyPublic: true})
	if err != nil || len(rows) == 0 {
		return "", false
	}
	return s.bookingBase(st.OrganizationSlug) + "?source=widget", true
}
