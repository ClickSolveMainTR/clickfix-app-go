package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var countryRe = regexp.MustCompile(`^[A-Z]{2}$`)

// Channel types a policy may name.
var channelTypes = []string{
	channels.TypeWidget, channels.TypeEmail, channels.TypeWhatsApp, channels.TypeInstagram,
	channels.TypeMessenger, channels.TypeTelegram, channels.TypeTest,
}

// ---------------------------------------------------------------------------
// Business hours

// ListBusinessHours returns the organization's schedules.
func (s *Service) ListBusinessHours(ctx context.Context, orgID int64) ([]model.BusinessHours, error) {
	rows, err := s.q.ListBusinessHours(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]model.BusinessHours, 0, len(rows))
	for _, r := range rows {
		out = append(out, toBusinessHours(r))
	}
	return out, nil
}

// GetBusinessHours returns one schedule.
func (s *Service) GetBusinessHours(ctx context.Context, orgID int64, id uuid.UUID) (model.BusinessHours, error) {
	r, err := s.q.GetBusinessHoursByUUID(ctx, db.GetBusinessHoursByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.BusinessHours{}, notFound(err)
	}
	return toBusinessHours(r), nil
}

func normalizeHours(in *model.BusinessHoursInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 120 {
		return invalidf("name is required (max 120 characters)")
	}
	if in.Timezone == "" {
		in.Timezone = "Europe/Istanbul"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return invalidf("unknown time zone %q", in.Timezone)
	}
	in.HolidayCountry = strings.ToUpper(strings.TrimSpace(in.HolidayCountry))
	if in.HolidayCountry == "" {
		in.HolidayCountry = "TR"
	}
	if !countryRe.MatchString(in.HolidayCountry) {
		return invalidf("holiday_country must be an ISO country code")
	}
	if err := businesshours.ValidateWeek(in.WorkingHours); err != nil {
		return invalidf("%s", strings.TrimPrefix(err.Error(), businesshours.ErrInvalid.Error()+": "))
	}
	for k, d := range in.WorkingHours {
		if d.Closed {
			in.WorkingHours[k] = businesshours.Day{Closed: true}
		}
	}
	return nil
}

// CreateBusinessHours adds a schedule. The first schedule becomes the default.
func (s *Service) CreateBusinessHours(ctx context.Context, orgID int64, in model.BusinessHoursInput) (model.BusinessHours, error) {
	if err := normalizeHours(&in); err != nil {
		return model.BusinessHours{}, err
	}
	var out db.BusinessHour
	err := s.inTx(ctx, func(ctx context.Context, _ pgx.Tx, q *db.Queries) error {
		if _, err := q.GetDefaultBusinessHours(ctx, orgID); err != nil {
			if !isNoRows(err) {
				return err
			}
			in.IsDefault = true
		}
		if in.IsDefault {
			if err := q.ClearDefaultBusinessHours(ctx, orgID); err != nil {
				return err
			}
		}
		week, _ := json.Marshal(in.WorkingHours)
		var err error
		out, err = q.InsertBusinessHours(ctx, db.InsertBusinessHoursParams{
			OrganizationID: orgID, Name: in.Name, Timezone: in.Timezone, WorkingHours: week,
			UsePublicHolidays: in.UsePublicHolidays == nil || *in.UsePublicHolidays, HolidayCountry: in.HolidayCountry,
			TeamID: int8p(in.TeamID), IsDefault: in.IsDefault,
		})
		return uniqueConflict(err, "this team already has a schedule")
	})
	if err != nil {
		return model.BusinessHours{}, err
	}
	s.resyncLater(orgID)
	return toBusinessHours(out), nil
}

// UpdateBusinessHours replaces a schedule.
func (s *Service) UpdateBusinessHours(ctx context.Context, orgID int64, id uuid.UUID, in model.BusinessHoursInput) (model.BusinessHours, error) {
	if err := normalizeHours(&in); err != nil {
		return model.BusinessHours{}, err
	}
	var out db.BusinessHour
	err := s.inTx(ctx, func(ctx context.Context, _ pgx.Tx, q *db.Queries) error {
		cur, err := q.GetBusinessHoursByUUID(ctx, db.GetBusinessHoursByUUIDParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return notFound(err)
		}
		if cur.IsDefault && !in.IsDefault {
			return invalidf("make another schedule the default instead")
		}
		if in.IsDefault && !cur.IsDefault {
			if err := q.ClearDefaultBusinessHours(ctx, orgID); err != nil {
				return err
			}
		}
		week, _ := json.Marshal(in.WorkingHours)
		out, err = q.UpdateBusinessHours(ctx, db.UpdateBusinessHoursParams{
			OrganizationID: orgID, ID: cur.ID, Name: in.Name, Timezone: in.Timezone, WorkingHours: week,
			UsePublicHolidays: in.UsePublicHolidays == nil || *in.UsePublicHolidays, HolidayCountry: in.HolidayCountry,
			TeamID: int8p(in.TeamID), IsDefault: in.IsDefault,
		})
		return uniqueConflict(err, "this team already has a schedule")
	})
	if err != nil {
		return model.BusinessHours{}, err
	}
	s.resyncLater(orgID)
	return toBusinessHours(out), nil
}

// DeleteBusinessHours removes a schedule; the default cannot be removed while
// others exist.
func (s *Service) DeleteBusinessHours(ctx context.Context, orgID int64, id uuid.UUID) error {
	err := s.inTx(ctx, func(ctx context.Context, _ pgx.Tx, q *db.Queries) error {
		cur, err := q.GetBusinessHoursByUUID(ctx, db.GetBusinessHoursByUUIDParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return notFound(err)
		}
		if cur.IsDefault {
			all, err := q.ListBusinessHours(ctx, orgID)
			if err != nil {
				return err
			}
			if len(all) > 1 {
				return invalidf("make another schedule the default before deleting this one")
			}
		}
		_, err = q.DeleteBusinessHours(ctx, db.DeleteBusinessHoursParams{OrganizationID: orgID, ID: cur.ID})
		return err
	})
	if err == nil {
		s.resyncLater(orgID)
	}
	return err
}

func toBusinessHours(r db.BusinessHour) model.BusinessHours {
	week := businesshours.Week{}
	_ = json.Unmarshal(r.WorkingHours, &week)
	return model.BusinessHours{
		UUID: r.Uuid, Name: r.Name, Timezone: r.Timezone, WorkingHours: week,
		UsePublicHolidays: r.UsePublicHolidays, HolidayCountry: r.HolidayCountry, TeamID: int8ptr(r.TeamID),
		IsDefault: r.IsDefault, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
}

// ---------------------------------------------------------------------------
// Holidays

// ListHolidays returns organization holidays from the given day on.
func (s *Service) ListHolidays(ctx context.Context, orgID int64, from time.Time) ([]model.Holiday, error) {
	rows, err := s.q.ListAllBusinessHolidays(ctx, db.ListAllBusinessHolidaysParams{OrganizationID: orgID, FromDate: pgDate(from)})
	if err != nil {
		return nil, err
	}
	out := make([]model.Holiday, 0, len(rows))
	for _, r := range rows {
		h := model.Holiday{
			UUID: r.Uuid, Date: r.Date.Time.Format(time.DateOnly), Name: r.Name, Closed: r.Closed,
			Open: clockPtr(r.OpenTime), Close: clockPtr(r.CloseTime),
		}
		if r.BusinessHoursUuid.Valid {
			u := uuid.UUID(r.BusinessHoursUuid.Bytes)
			h.BusinessHoursUUID = &u
		}
		out = append(out, h)
	}
	return out, nil
}

// CreateHoliday adds an organization holiday (all schedules or one).
func (s *Service) CreateHoliday(ctx context.Context, orgID int64, in model.HolidayInput) (model.Holiday, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 120 {
		return model.Holiday{}, invalidf("name is required (max 120 characters)")
	}
	day, err := time.Parse(time.DateOnly, in.Date)
	if err != nil {
		return model.Holiday{}, invalidf("date must be YYYY-MM-DD")
	}
	p := db.InsertBusinessHolidayParams{OrganizationID: orgID, Date: pgDate(day), Name: in.Name, Closed: in.Closed}
	if !in.Closed {
		from, err1 := businesshours.ParseClock(in.Open)
		to, err2 := businesshours.ParseClock(in.Close)
		if err1 != nil || err2 != nil || to <= from || to >= 24*60 {
			return model.Holiday{}, invalidf("open and close must be HH:MM with close after open")
		}
		p.OpenTime = pgtype.Time{Microseconds: int64(from) * 60e6, Valid: true}
		p.CloseTime = pgtype.Time{Microseconds: int64(to) * 60e6, Valid: true}
	}
	if in.BusinessHoursUUID != nil {
		bh, err := s.q.GetBusinessHoursByUUID(ctx, db.GetBusinessHoursByUUIDParams{OrganizationID: orgID, Uuid: *in.BusinessHoursUUID})
		if err != nil {
			return model.Holiday{}, invalidf("unknown business hours")
		}
		p.BusinessHoursID = pgtype.Int8{Int64: bh.ID, Valid: true}
	}
	r, err := s.q.InsertBusinessHoliday(ctx, p)
	if err != nil {
		return model.Holiday{}, err
	}
	s.resyncLater(orgID)
	return model.Holiday{
		UUID: r.Uuid, BusinessHoursUUID: in.BusinessHoursUUID, Date: r.Date.Time.Format(time.DateOnly), Name: r.Name,
		Closed: r.Closed, Open: clockPtr(r.OpenTime), Close: clockPtr(r.CloseTime),
	}, nil
}

// DeleteHoliday removes an organization holiday.
func (s *Service) DeleteHoliday(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := s.q.DeleteBusinessHoliday(ctx, db.DeleteBusinessHolidayParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	s.resyncLater(orgID)
	return nil
}

// PublicHolidays lists the national holidays of a country in a year.
func (s *Service) PublicHolidays(ctx context.Context, country string, year int) ([]model.PublicHoliday, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		country = "TR"
	}
	if !countryRe.MatchString(country) {
		return nil, invalidf("country must be an ISO country code")
	}
	if year < 2000 || year > 2100 {
		return nil, invalidf("year out of range")
	}
	rows, err := s.q.ListPublicHolidays(ctx, db.ListPublicHolidaysParams{
		CountryCode: country,
		FromDate:    pgDate(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)),
		ToDate:      pgDate(time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.PublicHoliday, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.PublicHoliday{Date: r.Date.Time.Format(time.DateOnly), Name: r.Name, NameEN: r.NameEn, HalfDay: r.HalfDay})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Policies

// ListPolicies returns the organization's policies in evaluation order.
func (s *Service) ListPolicies(ctx context.Context, orgID int64) ([]model.Policy, error) {
	rows, err := s.q.ListSLAPolicies(ctx, orgID)
	if err != nil {
		return nil, err
	}
	targets, err := s.q.ListSLAPolicyTargets(ctx, orgID)
	if err != nil {
		return nil, err
	}
	hours, err := s.hoursUUIDs(ctx, orgID)
	if err != nil {
		return nil, err
	}
	contracted, err := s.contracted(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Policy, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPolicy(r, targets, hours, contracted))
	}
	return out, nil
}

// GetPolicy returns one policy.
func (s *Service) GetPolicy(ctx context.Context, orgID int64, id uuid.UUID) (model.Policy, error) {
	r, err := s.q.GetSLAPolicyByUUID(ctx, db.GetSLAPolicyByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Policy{}, notFound(err)
	}
	targets, err := s.q.ListSLAPolicyTargetsFor(ctx, db.ListSLAPolicyTargetsForParams{OrganizationID: orgID, PolicyID: r.ID})
	if err != nil {
		return model.Policy{}, err
	}
	hours, err := s.hoursUUIDs(ctx, orgID)
	if err != nil {
		return model.Policy{}, err
	}
	contracted, err := s.contracted(ctx, orgID)
	if err != nil {
		return model.Policy{}, err
	}
	return toPolicy(r, targets, hours, contracted), nil
}

func (s *Service) contracted(ctx context.Context, orgID int64) (map[int64]int32, error) {
	rows, err := s.q.CountCompaniesBySLAPolicy(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := map[int64]int32{}
	for _, r := range rows {
		out[r.PolicyID] = r.Companies
	}
	return out, nil
}

func (s *Service) hoursUUIDs(ctx context.Context, orgID int64) (map[int64]uuid.UUID, error) {
	rows, err := s.q.ListBusinessHours(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := map[int64]uuid.UUID{}
	for _, r := range rows {
		out[r.ID] = r.Uuid
	}
	return out, nil
}

func normalizePolicy(in *model.PolicyInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 120 {
		return invalidf("name is required (max 120 characters)")
	}
	in.Description = strings.TrimSpace(in.Description)
	if len([]rune(in.Description)) > 1000 {
		return invalidf("description is too long")
	}
	if in.WarningPercent != nil && (*in.WarningPercent < 1 || *in.WarningPercent > 99) {
		return invalidf("warning_percent must be between 1 and 99")
	}
	c := &in.Conditions
	for _, p := range c.Priorities {
		if !slices.Contains(model.Priorities, p) {
			return invalidf("unknown priority %q", p)
		}
	}
	for _, ch := range c.ChannelTypes {
		if !slices.Contains(channelTypes, ch) {
			return invalidf("unknown channel type %q", ch)
		}
	}
	c.Priorities = uniq(c.Priorities)
	c.ChannelTypes = uniq(c.ChannelTypes)
	c.TagUUIDs = uniq(c.TagUUIDs)
	c.CompanyUUIDs = uniq(c.CompanyUUIDs)
	tiers := make([]string, 0, len(c.CompanyTiers))
	for _, t := range c.CompanyTiers {
		if t = strings.TrimSpace(t); t != "" && len([]rune(t)) <= 64 && !slices.Contains(tiers, t) {
			tiers = append(tiers, t)
		}
	}
	c.CompanyTiers = tiers
	plans := make([]string, 0, len(c.CompanyPlans))
	for _, p := range c.CompanyPlans {
		if p = strings.TrimSpace(p); p != "" && len([]rune(p)) <= 120 && !slices.ContainsFunc(plans, func(x string) bool { return strings.EqualFold(x, p) }) {
			plans = append(plans, p)
		}
	}
	c.CompanyPlans = plans
	if c.MinCompanyMRRMinor != nil && *c.MinCompanyMRRMinor < 0 {
		return invalidf("min_company_mrr_minor must not be negative")
	}
	if len(in.Targets) == 0 {
		return invalidf("at least one target is required")
	}
	seen := map[string]bool{}
	for _, t := range in.Targets {
		if !slices.Contains(model.Priorities, t.Priority) || seen[t.Priority] {
			return invalidf("targets need one entry per priority (low, normal, high, urgent)")
		}
		seen[t.Priority] = true
		for _, m := range []*int32{t.FirstResponseMinutes, t.NextResponseMinutes, t.ResolutionMinutes} {
			if m != nil && (*m < 1 || *m > 525600) {
				return invalidf("target minutes must be between 1 and 525600")
			}
		}
	}
	return nil
}

func uniq[T comparable](in []T) []T {
	out := make([]T, 0, len(in))
	for _, v := range in {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func (s *Service) policyParams(ctx context.Context, q *db.Queries, orgID int64, in model.PolicyInput) (pgtype.Int8, []byte, error) {
	var hours pgtype.Int8
	if in.BusinessHoursUUID != nil {
		bh, err := q.GetBusinessHoursByUUID(ctx, db.GetBusinessHoursByUUIDParams{OrganizationID: orgID, Uuid: *in.BusinessHoursUUID})
		if err != nil {
			return hours, nil, invalidf("unknown business hours")
		}
		hours = pgtype.Int8{Int64: bh.ID, Valid: true}
	}
	cond, err := json.Marshal(in.Conditions)
	return hours, cond, err
}

func writeTargets(ctx context.Context, q *db.Queries, orgID, policyID int64, targets []model.Target) error {
	if err := q.DeleteSLAPolicyTargets(ctx, db.DeleteSLAPolicyTargetsParams{OrganizationID: orgID, PolicyID: policyID}); err != nil {
		return err
	}
	for _, t := range targets {
		if err := q.InsertSLAPolicyTarget(ctx, db.InsertSLAPolicyTargetParams{
			OrganizationID: orgID, PolicyID: policyID, Priority: t.Priority,
			FirstResponseMinutes: int4p(t.FirstResponseMinutes), NextResponseMinutes: int4p(t.NextResponseMinutes),
			ResolutionMinutes: int4p(t.ResolutionMinutes),
		}); err != nil {
			return err
		}
	}
	return nil
}

// CreatePolicy adds a policy (last in evaluation order unless a position is given).
func (s *Service) CreatePolicy(ctx context.Context, orgID int64, in model.PolicyInput) (model.Policy, error) {
	if err := normalizePolicy(&in); err != nil {
		return model.Policy{}, err
	}
	if err := s.normalizeFieldMatches(ctx, orgID, &in.Conditions); err != nil {
		return model.Policy{}, err
	}
	var id uuid.UUID
	err := s.inTx(ctx, func(ctx context.Context, _ pgx.Tx, q *db.Queries) error {
		hours, cond, err := s.policyParams(ctx, q, orgID, in)
		if err != nil {
			return err
		}
		pos := int32(0)
		if in.Position != nil {
			pos = *in.Position
		} else if pos, err = q.NextSLAPolicyPosition(ctx, orgID); err != nil {
			return err
		}
		r, err := q.InsertSLAPolicy(ctx, db.InsertSLAPolicyParams{
			OrganizationID: orgID, Name: in.Name, Description: in.Description, IsActive: in.IsActive == nil || *in.IsActive,
			Position: pos, BusinessHoursID: hours, Conditions: cond, WarningPercent: warnPct(in.WarningPercent, 80),
		})
		if err != nil {
			return err
		}
		id = r.Uuid
		return writeTargets(ctx, q, orgID, r.ID, in.Targets)
	})
	if err != nil {
		return model.Policy{}, err
	}
	s.resyncLater(orgID)
	return s.GetPolicy(ctx, orgID, id)
}

// UpdatePolicy replaces a policy; open conversations are recomputed.
func (s *Service) UpdatePolicy(ctx context.Context, orgID int64, id uuid.UUID, in model.PolicyInput) (model.Policy, error) {
	if err := normalizePolicy(&in); err != nil {
		return model.Policy{}, err
	}
	if err := s.normalizeFieldMatches(ctx, orgID, &in.Conditions); err != nil {
		return model.Policy{}, err
	}
	err := s.inTx(ctx, func(ctx context.Context, _ pgx.Tx, q *db.Queries) error {
		cur, err := q.GetSLAPolicyByUUID(ctx, db.GetSLAPolicyByUUIDParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return notFound(err)
		}
		hours, cond, err := s.policyParams(ctx, q, orgID, in)
		if err != nil {
			return err
		}
		pos := cur.Position
		if in.Position != nil {
			pos = *in.Position
		}
		active := cur.IsActive
		if in.IsActive != nil {
			active = *in.IsActive
		}
		if _, err := q.UpdateSLAPolicy(ctx, db.UpdateSLAPolicyParams{
			OrganizationID: orgID, ID: cur.ID, Name: in.Name, Description: in.Description, IsActive: active,
			Position: pos, BusinessHoursID: hours, Conditions: cond, WarningPercent: warnPct(in.WarningPercent, cur.WarningPercent),
		}); err != nil {
			return err
		}
		return writeTargets(ctx, q, orgID, cur.ID, in.Targets)
	})
	if err != nil {
		return model.Policy{}, err
	}
	s.resyncLater(orgID)
	return s.GetPolicy(ctx, orgID, id)
}

// DeletePolicy removes a policy; conversations under it are re-matched.
func (s *Service) DeletePolicy(ctx context.Context, orgID int64, id uuid.UUID) error {
	cur, err := s.q.GetSLAPolicyByUUID(ctx, db.GetSLAPolicyByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return notFound(err)
	}
	if _, err := s.q.DeleteSLAPolicy(ctx, db.DeleteSLAPolicyParams{OrganizationID: orgID, ID: cur.ID}); err != nil {
		return err
	}
	s.resyncLater(orgID)
	return nil
}

func toPolicy(r db.SlaPolicy, targets []db.SlaPolicyTarget, hours map[int64]uuid.UUID, contracted map[int64]int32) model.Policy {
	p := model.Policy{ContractedCompanies: int(contracted[r.ID]),
		UUID: r.Uuid, Name: r.Name, Description: r.Description, IsActive: r.IsActive, Position: r.Position,
		WarningPercent: r.WarningPercent, Targets: []model.Target{}, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	_ = json.Unmarshal(r.Conditions, &p.Conditions)
	c := &p.Conditions
	c.Priorities, c.ChannelTypes = nonNil(c.Priorities), nonNil(c.ChannelTypes)
	c.TagUUIDs, c.CompanyUUIDs = nonNil(c.TagUUIDs), nonNil(c.CompanyUUIDs)
	c.CompanyTiers, c.CompanyPlans = nonNil(c.CompanyTiers), nonNil(c.CompanyPlans)
	c.CustomFields = nonNil(c.CustomFields)
	if r.BusinessHoursID.Valid {
		if u, ok := hours[r.BusinessHoursID.Int64]; ok {
			p.BusinessHoursUUID = &u
		}
	}
	for _, t := range targets {
		if t.PolicyID != r.ID {
			continue
		}
		p.Targets = append(p.Targets, model.Target{
			Priority: t.Priority, FirstResponseMinutes: int4ptr(t.FirstResponseMinutes),
			NextResponseMinutes: int4ptr(t.NextResponseMinutes), ResolutionMinutes: int4ptr(t.ResolutionMinutes),
		})
	}
	return p
}

// ---------------------------------------------------------------------------
// helpers

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func warnPct(v *int32, def int32) int32 {
	if v == nil {
		return def
	}
	return *v
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func uniqueConflict(err error, msg string) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return fmt.Errorf("%w: %s", ErrConflict, msg)
	}
	return err
}

func int8p(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func int8ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

func int4p(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func int4ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	x := v.Int32
	return &x
}

func pgDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func clockString(t pgtype.Time) string {
	m := t.Microseconds / 60e6
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

func clockPtr(t pgtype.Time) *string {
	if !t.Valid {
		return nil
	}
	v := clockString(t)
	return &v
}
