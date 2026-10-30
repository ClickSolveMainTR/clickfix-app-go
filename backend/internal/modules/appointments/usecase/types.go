package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/engine"
	formsmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/model"
	formsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Member is an organization member shown as a host.
type Member struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

// TeamRef names a team.
type TeamRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Type is an appointment type an organization defines.
type Type struct {
	UUID                 uuid.UUID      `json:"uuid"`
	Name                 string         `json:"name"`
	Slug                 string         `json:"slug"`
	Description          string         `json:"description"`
	Color                string         `json:"color"`
	DurationOptions      []int32        `json:"duration_options"`
	DefaultDuration      int32          `json:"default_duration"`
	SlotInterval         int32          `json:"slot_interval"`
	BufferBefore         int32          `json:"buffer_before"`
	BufferAfter          int32          `json:"buffer_after"`
	LocationKind         string         `json:"location_kind"`
	LocationValue        string         `json:"location_value"`
	HostMode             string         `json:"host_mode"`
	Team                 *TeamRef       `json:"team"`
	Hosts                []Member       `json:"hosts"`
	BookingWindowDays    int32          `json:"booking_window_days"`
	MinNoticeMinutes     int32          `json:"min_notice_minutes"`
	MaxPerDay            int32          `json:"max_per_day"`
	IntakeFields         []engine.Field `json:"intake_fields"`
	ConfirmationTemplate string         `json:"confirmation_template"`
	ReminderTemplate     string         `json:"reminder_template"`
	RequiresConfirmation bool           `json:"requires_confirmation"`
	AllowCancel          bool           `json:"allow_cancel"`
	AllowReschedule      bool           `json:"allow_reschedule"`
	CancelNoticeMinutes  int32          `json:"cancel_notice_minutes"`
	IsPublic             bool           `json:"is_public"`
	IsActive             bool           `json:"is_active"`
	Position             int32          `json:"position"`
	BookingURL           string         `json:"booking_url"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// TypeInput creates or replaces a type.
type TypeInput struct {
	Name                 string         `json:"name"`
	Slug                 string         `json:"slug"`
	Description          string         `json:"description"`
	Color                string         `json:"color"`
	DurationOptions      []int32        `json:"duration_options"`
	DefaultDuration      int32          `json:"default_duration"`
	SlotInterval         int32          `json:"slot_interval"`
	BufferBefore         int32          `json:"buffer_before"`
	BufferAfter          int32          `json:"buffer_after"`
	LocationKind         string         `json:"location_kind"`
	LocationValue        string         `json:"location_value"`
	HostMode             string         `json:"host_mode"`
	TeamUUID             *uuid.UUID     `json:"team_uuid"`
	HostUUIDs            []uuid.UUID    `json:"host_uuids"`
	BookingWindowDays    int32          `json:"booking_window_days"`
	MinNoticeMinutes     int32          `json:"min_notice_minutes"`
	MaxPerDay            int32          `json:"max_per_day"`
	IntakeFields         []engine.Field `json:"intake_fields"`
	ConfirmationTemplate string         `json:"confirmation_template"`
	ReminderTemplate     string         `json:"reminder_template"`
	RequiresConfirmation bool           `json:"requires_confirmation"`
	AllowCancel          bool           `json:"allow_cancel"`
	AllowReschedule      bool           `json:"allow_reschedule"`
	CancelNoticeMinutes  int32          `json:"cancel_notice_minutes"`
	IsPublic             bool           `json:"is_public"`
	IsActive             bool           `json:"is_active"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

// Slugify turns a name into a URL slug (Turkish letters folded).
func Slugify(name string) string {
	r := strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "İ", "i", "ö", "o", "ş", "s", "ü", "u",
		"Ç", "c", "Ğ", "g", "Ö", "o", "Ş", "s", "Ü", "u")
	var b strings.Builder
	dash := false
	for _, c := range strings.ToLower(r.Replace(name)) {
		switch {
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			b.WriteRune(c)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-")
	}
	return out
}

type typeFields struct {
	in      TypeInput
	teamID  int64
	hostIDs []int64
	intake  []byte
}

func (s *Service) normalizeType(ctx context.Context, org int64, in TypeInput) (typeFields, error) {
	f := typeFields{}
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.LocationValue = strings.TrimSpace(in.LocationValue)
	in.Slug = strings.TrimSpace(strings.ToLower(in.Slug))
	if in.Slug == "" {
		in.Slug = Slugify(in.Name)
	}
	switch {
	case in.Name == "" || len([]rune(in.Name)) > 120:
		return f, invalidf("name is required (at most 120 characters)")
	case !slugRe.MatchString(in.Slug):
		return f, invalidf("slug must be lowercase letters, digits and dashes")
	case len([]rune(in.Description)) > 2000:
		return f, invalidf("description is limited to 2000 characters")
	case len([]rune(in.LocationValue)) > 500:
		return f, invalidf("location is limited to 500 characters")
	case len([]rune(in.ConfirmationTemplate)) > 4000 || len([]rune(in.ReminderTemplate)) > 4000:
		return f, invalidf("templates are limited to 4000 characters")
	}
	if in.Color == "" {
		in.Color = "blue"
	}
	if !slices.Contains(Colors, in.Color) {
		return f, invalidf("invalid color")
	}
	if len(in.DurationOptions) == 0 {
		in.DurationOptions = []int32{30}
	}
	if len(in.DurationOptions) > 6 {
		return f, invalidf("at most 6 durations")
	}
	durations := []int32{}
	for _, d := range in.DurationOptions {
		if d < 5 || d > 480 {
			return f, invalidf("durations must be between 5 and 480 minutes")
		}
		if !slices.Contains(durations, d) {
			durations = append(durations, d)
		}
	}
	slices.Sort(durations)
	in.DurationOptions = durations
	if in.DefaultDuration == 0 {
		in.DefaultDuration = durations[0]
	}
	if !slices.Contains(durations, in.DefaultDuration) {
		return f, invalidf("default_duration must be one of duration_options")
	}
	if in.SlotInterval == 0 {
		in.SlotInterval = min(in.DefaultDuration, 60)
	}
	switch {
	case in.SlotInterval < 5 || in.SlotInterval > 240:
		return f, invalidf("slot_interval must be between 5 and 240 minutes")
	case in.BufferBefore < 0 || in.BufferBefore > 240 || in.BufferAfter < 0 || in.BufferAfter > 240:
		return f, invalidf("buffers must be between 0 and 240 minutes")
	case !slices.Contains(LocationKinds, in.LocationKind):
		return f, invalidf("invalid location_kind")
	case !slices.Contains(HostModes, in.HostMode):
		return f, invalidf("invalid host_mode")
	case in.BookingWindowDays < 1 || in.BookingWindowDays > 365:
		return f, invalidf("booking_window_days must be between 1 and 365")
	case in.MinNoticeMinutes < 0 || in.MinNoticeMinutes > 43200:
		return f, invalidf("min_notice_minutes must be between 0 and 43200")
	case in.MaxPerDay < 0 || in.MaxPerDay > 100:
		return f, invalidf("max_per_day must be between 0 (unlimited) and 100")
	case in.CancelNoticeMinutes < 0 || in.CancelNoticeMinutes > 43200:
		return f, invalidf("cancel_notice_minutes must be between 0 and 43200")
	}
	if in.LocationKind == LocationCustomURL && !strings.HasPrefix(in.LocationValue, "https://") {
		return f, invalidf("a custom URL location must start with https://")
	}
	if in.HostMode == HostTeam {
		if in.TeamUUID == nil {
			return f, invalidf("team_uuid is required for team hosting")
		}
		t, err := s.q.AppointmentTeamByUUID(ctx, db.AppointmentTeamByUUIDParams{OrganizationID: org, Uuid: *in.TeamUUID})
		if err != nil {
			return f, invalidf("unknown team")
		}
		f.teamID = t.ID
		in.HostUUIDs = nil
	} else {
		in.TeamUUID = nil
		if len(in.HostUUIDs) == 0 {
			return f, invalidf("at least one host is required")
		}
		if len(in.HostUUIDs) > 50 {
			return f, invalidf("at most 50 hosts")
		}
		rows, err := s.q.AppointmentMembers(ctx, db.AppointmentMembersParams{OrganizationID: org, UserUuids: in.HostUUIDs})
		if err != nil {
			return f, err
		}
		found := map[uuid.UUID]int64{}
		for _, r := range rows {
			found[r.Uuid] = r.ID
		}
		for _, id := range in.HostUUIDs {
			uid, ok := found[id]
			if !ok {
				return f, invalidf("host %s is not a member", id)
			}
			if !slices.Contains(f.hostIDs, uid) {
				f.hostIDs = append(f.hostIDs, uid)
			}
		}
	}
	if in.IntakeFields == nil {
		in.IntakeFields = []engine.Field{}
	}
	for _, fl := range in.IntakeFields {
		// Intake answers belong to the booking and the contact, never to a
		// conversation.
		if fl.Source == engine.SourceConversation {
			return f, invalidf("intake fields can use system or contact fields only")
		}
	}
	if s.intake != nil {
		resolved, err := s.intake.ResolveFields(ctx, org, in.IntakeFields, true)
		if err != nil {
			return f, invalidf("%s", strings.TrimPrefix(err.Error(), engine.ErrInvalid.Error()+": "))
		}
		in.IntakeFields = make([]engine.Field, 0, len(resolved))
		for _, r := range resolved {
			in.IntakeFields = append(in.IntakeFields, r.Field)
		}
	}
	raw, err := json.Marshal(in.IntakeFields)
	if err != nil {
		return f, err
	}
	f.in, f.intake = in, raw
	return f, nil
}

func (s *Service) checkSlug(ctx context.Context, org int64, slug string, except int64) error {
	taken, err := s.q.AppointmentSlugTaken(ctx, db.AppointmentSlugTakenParams{OrganizationID: org, Slug: slug, ExceptID: except})
	if err != nil {
		return err
	}
	if taken {
		return invalidf("slug %q is already used", slug)
	}
	return nil
}

// CreateType stores a new type.
func (s *Service) CreateType(ctx context.Context, org int64, in TypeInput) (Type, error) {
	f, err := s.normalizeType(ctx, org, in)
	if err != nil {
		return Type{}, err
	}
	if err := s.checkSlug(ctx, org, f.in.Slug, 0); err != nil {
		return Type{}, err
	}
	var row db.AppointmentType
	err = s.inTx(ctx, func(q *db.Queries) error {
		pos, err := q.AppointmentNextTypePosition(ctx, org)
		if err != nil {
			return err
		}
		v := f.in
		row, err = q.AppointmentCreateType(ctx, db.AppointmentCreateTypeParams{
			OrganizationID: org, Name: v.Name, Slug: v.Slug, Description: v.Description, Color: v.Color,
			DurationOptions: v.DurationOptions, DefaultDuration: v.DefaultDuration, SlotInterval: v.SlotInterval,
			BufferBefore: v.BufferBefore, BufferAfter: v.BufferAfter, LocationKind: v.LocationKind,
			LocationValue: v.LocationValue, HostMode: v.HostMode, TeamID: i8(f.teamID),
			BookingWindowDays: v.BookingWindowDays, MinNoticeMinutes: v.MinNoticeMinutes, MaxPerDay: v.MaxPerDay,
			IntakeFields: f.intake, ConfirmationTemplate: v.ConfirmationTemplate, ReminderTemplate: v.ReminderTemplate,
			RequiresConfirmation: v.RequiresConfirmation, AllowCancel: v.AllowCancel, AllowReschedule: v.AllowReschedule,
			CancelNoticeMinutes: v.CancelNoticeMinutes, IsPublic: v.IsPublic, IsActive: v.IsActive, Position: pos,
		})
		if err != nil {
			return err
		}
		return setHosts(ctx, q, org, row.ID, f.hostIDs)
	})
	if err != nil {
		return Type{}, err
	}
	s.live(ctx, org, "type", row.Uuid)
	return s.GetType(ctx, org, row.Uuid)
}

// UpdateType replaces a type. Existing bookings keep their times, buffers
// and location.
func (s *Service) UpdateType(ctx context.Context, org int64, id uuid.UUID, in TypeInput) (Type, error) {
	cur, err := s.q.AppointmentTypeByUUID(ctx, db.AppointmentTypeByUUIDParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return Type{}, notFound(err)
	}
	f, err := s.normalizeType(ctx, org, in)
	if err != nil {
		return Type{}, err
	}
	if err := s.checkSlug(ctx, org, f.in.Slug, cur.ID); err != nil {
		return Type{}, err
	}
	err = s.inTx(ctx, func(q *db.Queries) error {
		v := f.in
		_, err := q.AppointmentUpdateType(ctx, db.AppointmentUpdateTypeParams{
			OrganizationID: org, ID: cur.ID, Name: v.Name, Slug: v.Slug, Description: v.Description, Color: v.Color,
			DurationOptions: v.DurationOptions, DefaultDuration: v.DefaultDuration, SlotInterval: v.SlotInterval,
			BufferBefore: v.BufferBefore, BufferAfter: v.BufferAfter, LocationKind: v.LocationKind,
			LocationValue: v.LocationValue, HostMode: v.HostMode, TeamID: i8(f.teamID),
			BookingWindowDays: v.BookingWindowDays, MinNoticeMinutes: v.MinNoticeMinutes, MaxPerDay: v.MaxPerDay,
			IntakeFields: f.intake, ConfirmationTemplate: v.ConfirmationTemplate, ReminderTemplate: v.ReminderTemplate,
			RequiresConfirmation: v.RequiresConfirmation, AllowCancel: v.AllowCancel, AllowReschedule: v.AllowReschedule,
			CancelNoticeMinutes: v.CancelNoticeMinutes, IsPublic: v.IsPublic, IsActive: v.IsActive,
		})
		if err != nil {
			return notFound(err)
		}
		if err := q.AppointmentClearHosts(ctx, db.AppointmentClearHostsParams{OrganizationID: org, TypeID: cur.ID}); err != nil {
			return err
		}
		return setHosts(ctx, q, org, cur.ID, f.hostIDs)
	})
	if err != nil {
		return Type{}, err
	}
	s.live(ctx, org, "type", id)
	return s.GetType(ctx, org, id)
}

func setHosts(ctx context.Context, q *db.Queries, org, typeID int64, ids []int64) error {
	for _, uid := range ids {
		if err := q.AppointmentAddHost(ctx, db.AppointmentAddHostParams{OrganizationID: org, TypeID: typeID, UserID: uid}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteType archives a type; its bookings stay.
func (s *Service) DeleteType(ctx context.Context, org int64, id uuid.UUID) error {
	cur, err := s.q.AppointmentTypeByUUID(ctx, db.AppointmentTypeByUUIDParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return notFound(err)
	}
	n, err := s.q.AppointmentDeleteType(ctx, db.AppointmentDeleteTypeParams{OrganizationID: org, ID: cur.ID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	s.live(ctx, org, "type", id)
	return nil
}

// ListTypes returns the organization's types (onlyPublic: bookable ones).
func (s *Service) ListTypes(ctx context.Context, org int64, onlyPublic bool) ([]Type, error) {
	rows, err := s.q.AppointmentTypes(ctx, db.AppointmentTypesParams{OrganizationID: org, OnlyPublic: onlyPublic})
	if err != nil {
		return nil, err
	}
	return s.mapTypes(ctx, org, rows)
}

// GetType returns one type.
func (s *Service) GetType(ctx context.Context, org int64, id uuid.UUID) (Type, error) {
	row, err := s.q.AppointmentTypeByUUID(ctx, db.AppointmentTypeByUUIDParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return Type{}, notFound(err)
	}
	out, err := s.mapTypes(ctx, org, []db.AppointmentType{row})
	if err != nil {
		return Type{}, err
	}
	return out[0], nil
}

func (s *Service) mapTypes(ctx context.Context, org int64, rows []db.AppointmentType) ([]Type, error) {
	out := make([]Type, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	st, err := s.settings(ctx, org)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	var teamIDs []int64
	for _, r := range rows {
		ids = append(ids, r.ID)
		if r.TeamID.Valid && !slices.Contains(teamIDs, r.TeamID.Int64) {
			teamIDs = append(teamIDs, r.TeamID.Int64)
		}
	}
	hosts, err := s.q.AppointmentTypeHosts(ctx, db.AppointmentTypeHostsParams{OrganizationID: org, TypeIds: ids})
	if err != nil {
		return nil, err
	}
	byType := map[int64][]Member{}
	for _, h := range hosts {
		byType[h.TypeID] = append(byType[h.TypeID], Member{UUID: h.UserUuid, Name: fullName(h.Name, h.Surname), Email: h.Email})
	}
	teams := map[int64]*TeamRef{}
	for _, id := range teamIDs {
		t, err := s.q.AppointmentTeamByID(ctx, db.AppointmentTeamByIDParams{OrganizationID: org, ID: id})
		if err == nil {
			teams[id] = &TeamRef{UUID: t.Uuid, Name: t.Name}
		}
	}
	for _, r := range rows {
		t := Type{
			UUID: r.Uuid, Name: r.Name, Slug: r.Slug, Description: r.Description, Color: r.Color,
			DurationOptions: nonNil(r.DurationOptions), DefaultDuration: r.DefaultDuration, SlotInterval: r.SlotInterval,
			BufferBefore: r.BufferBefore, BufferAfter: r.BufferAfter, LocationKind: r.LocationKind,
			LocationValue: r.LocationValue, HostMode: r.HostMode, Hosts: nonNil(byType[r.ID]),
			BookingWindowDays: r.BookingWindowDays, MinNoticeMinutes: r.MinNoticeMinutes, MaxPerDay: r.MaxPerDay,
			IntakeFields: decodeFields(r.IntakeFields), ConfirmationTemplate: r.ConfirmationTemplate,
			ReminderTemplate: r.ReminderTemplate, RequiresConfirmation: r.RequiresConfirmation,
			AllowCancel: r.AllowCancel, AllowReschedule: r.AllowReschedule, CancelNoticeMinutes: r.CancelNoticeMinutes,
			IsPublic: r.IsPublic, IsActive: r.IsActive, Position: r.Position,
			BookingURL: s.bookingBase(st.OrganizationSlug) + "/" + r.Slug,
			CreatedAt:  r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		}
		if r.TeamID.Valid {
			t.Team = teams[r.TeamID.Int64]
		}
		out = append(out, t)
	}
	return out, nil
}

func decodeFields(raw []byte) []engine.Field {
	var f []engine.Field
	if err := json.Unmarshal(raw, &f); err != nil || f == nil {
		return []engine.Field{}
	}
	return f
}

func fullName(name, surname string) string { return strings.TrimSpace(name + " " + surname) }

// publicIntake resolves a type's intake form for invitees (fields of
// removed custom fields are skipped).
func (s *Service) publicIntake(ctx context.Context, org int64, row db.AppointmentType) ([]engine.Resolved, []formsmodel.PublicField, error) {
	if s.intake == nil {
		return nil, []formsmodel.PublicField{}, nil
	}
	resolved, err := s.intake.ResolveFields(ctx, org, decodeFields(row.IntakeFields), false)
	if err != nil {
		if errors.Is(err, engine.ErrInvalid) {
			return nil, []formsmodel.PublicField{}, nil
		}
		return nil, nil, err
	}
	return resolved, formsusecase.PublicFields(resolved), nil
}

func pgUUID(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}
