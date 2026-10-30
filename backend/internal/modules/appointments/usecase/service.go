// Package usecase is the appointments module (CF-138): organization-defined
// appointment types, availability (agent shifts, business hours, time off),
// bookings with a status lifecycle, the public booking page, reminders and
// the organization calendar (ported from the otopoly-go calendar module).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/engine"
	notif "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/videorooms"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Feature is the plan feature that turns the module on (billing_features).
const Feature = "module.appointments"

var (
	ErrNotFound  = errors.New("appointments: not found")
	ErrInvalid   = errors.New("appointments: invalid request")
	ErrForbidden = errors.New("appointments: forbidden")
	// ErrDisabled: the organization switched the module off in its settings.
	ErrDisabled = errors.New("appointments: module disabled")
	// ErrSlotTaken: the slot is no longer free (another booking, time off or
	// the daily limit).
	ErrSlotTaken = errors.New("appointments: slot is not available")
	// ErrPolicy: the type's cancellation / reschedule policy refuses it.
	ErrPolicy = errors.New("appointments: not allowed by the booking policy")
	// ErrRateLimited: public booking abuse limit.
	ErrRateLimited = errors.New("appointments: too many bookings")
)

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Colors a type may use; mirrors appointment_types_color_ck (same palette as
// the ported calendar).
var Colors = []string{"blue", "emerald", "amber", "rose", "violet", "sky", "orange", "teal", "pink", "slate"}

// Statuses of the booking lifecycle.
const (
	StatusScheduled = "scheduled"
	StatusConfirmed = "confirmed"
	StatusCompleted = "completed"
	StatusNoShow    = "no_show"
	StatusCancelled = "cancelled"
)

// Statuses in lifecycle order.
var Statuses = []string{StatusScheduled, StatusConfirmed, StatusCompleted, StatusNoShow, StatusCancelled}

// Location kinds.
const (
	LocationVideo     = "video"
	LocationPhone     = "phone"
	LocationInPerson  = "in_person"
	LocationCustomURL = "custom_url"
)

var LocationKinds = []string{LocationVideo, LocationPhone, LocationInPerson, LocationCustomURL}

// Host modes.
const (
	HostAgents     = "agents"
	HostTeam       = "team"
	HostRoundRobin = "round_robin"
)

var HostModes = []string{HostAgents, HostTeam, HostRoundRobin}

// Booking sources.
const (
	SourceInternal = "internal"
	SourcePublic   = "public"
	SourceWidget   = "widget"
)

// Notifier enqueues notifications (in-app, e-mail, push, SMS) on the queue.
type Notifier interface {
	Enqueue(ctx context.Context, in notif.EnqueueInput) ([]notif.Notification, error)
}

// Timeline writes appointment events into a conversation timeline.
type Timeline interface {
	AppointmentEvent(ctx context.Context, orgID, convID int64, actor convusecase.Actor, attrs map[string]any) error
}

// Intake resolves intake form fields with the forms engine (CF-52).
type Intake interface {
	ResolveFields(ctx context.Context, orgID int64, fields []engine.Field, strict bool) ([]engine.Resolved, error)
}

// Logger is the subset of slog the service uses.
type Logger interface {
	Warn(msg string, args ...any)
}

// Deps wires the service.
type Deps struct {
	Video        *videorooms.Service
	Activity     *activity.Recorder
	Pool         *pgxpool.Pool
	Queries      *db.Queries
	Entitlements entitlement.Engine
	Notifier     Notifier
	Timeline     Timeline
	Intake       Intake
	Realtime     realtime.Publisher
	Secret       string
	// BaseURL is the public app origin (manage links, ICS links).
	BaseURL string
	Log     Logger
}

// Service is the appointments use case.
type Service struct {
	video    *videorooms.Service
	activity *activity.Recorder
	pool     *pgxpool.Pool
	q        *db.Queries
	ent      entitlement.Engine
	notifier Notifier
	timeline Timeline
	intake   Intake
	pub      realtime.Publisher
	secret   string
	baseURL  string
	log      Logger
	now      func() time.Time
}

// New builds the service.
func New(d Deps) *Service {
	return &Service{
		video: d.Video, activity: d.Activity, pool: d.Pool, q: d.Queries, ent: d.Entitlements, notifier: d.Notifier, timeline: d.Timeline,
		intake: d.Intake, pub: d.Realtime, secret: d.Secret, baseURL: strings.TrimRight(d.BaseURL, "/"),
		log: d.Log, now: time.Now,
	}
}

// SetClock overrides time.Now (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	if s.pool == nil {
		return fn(s.q)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) warn(msg string, args ...any) {
	if s.log != nil {
		s.log.Warn(msg, args...)
	}
}

// Settings are the organization's appointment settings.
type Settings struct {
	Enabled         bool    `json:"enabled"`
	Timezone        string  `json:"timezone"`
	PublicBooking   bool    `json:"public_booking"`
	ReminderMinutes []int32 `json:"reminder_minutes"`
	// BookingBaseURL is where the organization's public booking pages live.
	BookingBaseURL string `json:"booking_base_url"`
	// FeatureEnabled: the plan includes the module.
	FeatureEnabled bool `json:"feature_enabled"`
}

type orgSettings struct {
	db.AppointmentSettingsRow
	loc *time.Location
}

func (s *Service) settings(ctx context.Context, org int64) (orgSettings, error) {
	row, err := s.q.AppointmentSettings(ctx, org)
	if err != nil {
		return orgSettings{}, notFound(err)
	}
	loc, err := time.LoadLocation(row.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return orgSettings{AppointmentSettingsRow: row, loc: loc}, nil
}

func (s *Service) bookingBase(slug string) string {
	return s.baseURL + "/book/" + slug
}

// GetSettings returns the settings (with plan state) of an organization.
func (s *Service) GetSettings(ctx context.Context, org int64) (Settings, error) {
	st, err := s.settings(ctx, org)
	if err != nil {
		return Settings{}, err
	}
	feature := entitlement.Require(ctx, s.ent, org, Feature) == nil
	return Settings{
		Enabled: st.Enabled, Timezone: st.Timezone, PublicBooking: st.PublicBooking,
		ReminderMinutes: nonNil(st.ReminderMinutes), BookingBaseURL: s.bookingBase(st.OrganizationSlug),
		FeatureEnabled: feature,
	}, nil
}

// SettingsInput replaces the settings.
type SettingsInput struct {
	Enabled         bool    `json:"enabled"`
	Timezone        string  `json:"timezone"`
	PublicBooking   bool    `json:"public_booking"`
	ReminderMinutes []int32 `json:"reminder_minutes"`
}

// SaveSettings validates and stores the settings.
func (s *Service) SaveSettings(ctx context.Context, org int64, in SettingsInput) (Settings, error) {
	if err := entitlement.Require(ctx, s.ent, org, Feature); err != nil {
		return Settings{}, err
	}
	in.Timezone = strings.TrimSpace(in.Timezone)
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "" || in.Timezone == "Local" {
		return Settings{}, invalidf("timezone must be an IANA time zone")
	}
	if len(in.ReminderMinutes) > 5 {
		return Settings{}, invalidf("at most 5 reminders")
	}
	seen := map[int32]bool{}
	reminders := []int32{}
	for _, m := range in.ReminderMinutes {
		if m < 5 || m > 10080 {
			return Settings{}, invalidf("reminders must be between 5 minutes and 7 days")
		}
		if !seen[m] {
			seen[m] = true
			reminders = append(reminders, m)
		}
	}
	slices.SortFunc(reminders, func(a, b int32) int { return int(b - a) })
	if err := s.q.AppointmentSaveSettings(ctx, db.AppointmentSaveSettingsParams{
		OrganizationID: org, Enabled: in.Enabled, Timezone: in.Timezone, PublicBooking: in.PublicBooking, ReminderMinutes: reminders,
	}); err != nil {
		return Settings{}, err
	}
	return s.GetSettings(ctx, org)
}

// Gate checks the plan feature and the organization switch. Every route
// except the settings ones goes through it.
func (s *Service) Gate(ctx context.Context, org int64) error {
	if err := entitlement.Require(ctx, s.ent, org, Feature); err != nil {
		return err
	}
	st, err := s.settings(ctx, org)
	if err != nil {
		return err
	}
	if !st.Enabled {
		return ErrDisabled
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

// minutes is a duration in whole minutes (clamped to int32).
func minutes(d time.Duration) int32 { return safeconv.ToInt32(int(d / time.Minute)) }

func i8(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}
