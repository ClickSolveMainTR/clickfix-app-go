// Package usecase computes customer health scores and raises churn-risk
// alerts (CF-87). A daily sweep scores every company of every organization;
// agents can recompute one company on demand. When a score falls below the
// organization's threshold, a company.health_dropped event goes through the
// outbox once per crossing: the notifications consumer tells the account
// owner (CSM) through the preference matrix.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customerhealth/engine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors.
var (
	ErrNotFound = errors.New("customerhealth: not found")
	ErrInvalid  = errors.New("customerhealth: invalid request")
)

const (
	pageSize     = 200
	csatWindow   = 90 * 24 * time.Hour
	signalWindow = 30 * 24 * time.Hour
)

// Settings are an organization's health score settings.
type Settings struct {
	Enabled   bool           `json:"enabled"`
	Threshold int            `json:"threshold"`
	Weights   map[string]int `json:"weights"`
}

// SettingsInput edits settings; nil fields keep the stored value.
type SettingsInput struct {
	Enabled   *bool          `json:"enabled"`
	Threshold *int           `json:"threshold"`
	Weights   map[string]int `json:"weights"`
}

// Point is one history entry.
type Point struct {
	Score     int       `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

// Health is a company's current score with its signals.
type Health struct {
	CompanyUUID     uuid.UUID                  `json:"company_uuid"`
	Score           *int                       `json:"score"`
	Threshold       int                        `json:"threshold"`
	BelowThreshold  bool                       `json:"below_threshold"`
	Signals         map[string]engine.SubScore `json:"signals"`
	UsageScore      *int                       `json:"usage_score"`
	UsageReportedAt *time.Time                 `json:"usage_reported_at"`
	ComputedAt      *time.Time                 `json:"computed_at"`
	AlertedAt       *time.Time                 `json:"alerted_at"`
	History         []Point                    `json:"history"`
}

// Service computes health scores.
type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	activity *activity.Recorder
	log      *slog.Logger
	now      func() time.Time
}

// New builds the service.
func New(pool *pgxpool.Pool, q *db.Queries, rec *activity.Recorder, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{pool: pool, q: q, activity: rec, log: log, now: time.Now}
}

// WithClock overrides the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// GetSettings returns the settings with defaults filled in.
func (s *Service) GetSettings(ctx context.Context, orgID int64) (Settings, error) {
	out := Settings{Enabled: true, Threshold: engine.DefaultThreshold, Weights: engine.DefaultWeights()}
	row, err := s.q.GetCompanyHealthSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Enabled, out.Threshold = row.Enabled, int(row.Threshold)
	var stored map[string]int
	_ = json.Unmarshal(row.Weights, &stored)
	for k, v := range stored {
		if _, ok := out.Weights[k]; ok {
			out.Weights[k] = v
		}
	}
	return out, nil
}

// UpdateSettings validates and stores settings.
func (s *Service) UpdateSettings(ctx context.Context, orgID int64, in SettingsInput) (Settings, error) {
	cur, err := s.GetSettings(ctx, orgID)
	if err != nil {
		return cur, err
	}
	if in.Enabled != nil {
		cur.Enabled = *in.Enabled
	}
	if in.Threshold != nil {
		if *in.Threshold < 0 || *in.Threshold > 100 {
			return cur, fmt.Errorf("%w: threshold must be between 0 and 100", ErrInvalid)
		}
		cur.Threshold = *in.Threshold
	}
	for k, v := range in.Weights {
		if _, ok := cur.Weights[k]; !ok {
			return cur, fmt.Errorf("%w: unknown signal %q", ErrInvalid, k)
		}
		if v < 0 || v > 100 {
			return cur, fmt.Errorf("%w: weight of %s must be between 0 and 100", ErrInvalid, k)
		}
		cur.Weights[k] = v
	}
	wb, _ := json.Marshal(cur.Weights)
	if _, err := s.q.UpsertCompanyHealthSettings(ctx, db.UpsertCompanyHealthSettingsParams{
		OrganizationID: orgID, Enabled: cur.Enabled, Threshold: safeconv.ToInt16(cur.Threshold), Weights: wb,
	}); err != nil {
		return cur, err
	}
	return cur, nil
}

// GetHealth returns a company's stored health and its latest history.
func (s *Service) GetHealth(ctx context.Context, orgID int64, companyUUID uuid.UUID) (Health, error) {
	co, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: companyUUID})
	if err != nil {
		return Health{}, notFound(err)
	}
	set, err := s.GetSettings(ctx, orgID)
	if err != nil {
		return Health{}, err
	}
	out := Health{CompanyUUID: co.Uuid, Threshold: set.Threshold, Signals: map[string]engine.SubScore{}, History: []Point{}}
	rows, err := s.q.ListCompanyHealthByCompanyIDs(ctx, db.ListCompanyHealthByCompanyIDsParams{OrganizationID: orgID, CompanyIds: []int64{co.ID}})
	if err != nil {
		return out, err
	}
	if len(rows) == 1 {
		h := rows[0]
		out.Score = int2(h.Score)
		out.BelowThreshold = h.BelowThreshold
		out.UsageScore = int2(h.UsageScore)
		out.UsageReportedAt, out.ComputedAt, out.AlertedAt = ts(h.UsageReportedAt), ts(h.ComputedAt), ts(h.AlertedAt)
		_ = json.Unmarshal(h.Signals, &out.Signals)
	}
	hist, err := s.q.ListCompanyHealthHistory(ctx, db.ListCompanyHealthHistoryParams{OrganizationID: orgID, CompanyID: co.ID, RowLimit: 30})
	if err != nil {
		return out, err
	}
	for _, p := range hist {
		out.History = append(out.History, Point{Score: int(p.Score), CreatedAt: p.CreatedAt.Time.UTC()})
	}
	return out, nil
}

// Recompute scores one company now and returns its health.
func (s *Service) Recompute(ctx context.Context, orgID int64, companyUUID uuid.UUID) (Health, error) {
	co, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: companyUUID})
	if err != nil {
		return Health{}, notFound(err)
	}
	set, err := s.GetSettings(ctx, orgID)
	if err != nil {
		return Health{}, err
	}
	if _, err := s.computePage(ctx, orgID, set, 0, &co.ID); err != nil {
		return Health{}, err
	}
	return s.GetHealth(ctx, orgID, companyUUID)
}

// UsageKey names a company in a usage report.
type UsageKey struct {
	UUID       *uuid.UUID
	ExternalID string
	Domain     string
}

// ReportUsage stores the product usage signal (0-100) of a company, sent
// by the organization's own product through the API.
func (s *Service) ReportUsage(ctx context.Context, orgID int64, key UsageKey, score int) (uuid.UUID, error) {
	if score < 0 || score > 100 {
		return uuid.Nil, fmt.Errorf("%w: score must be between 0 and 100", ErrInvalid)
	}
	var (
		co  db.Company
		err error
	)
	switch {
	case key.UUID != nil:
		co, err = s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: *key.UUID})
	case strings.TrimSpace(key.ExternalID) != "":
		co, err = s.q.GetCompanyByExternalID(ctx, db.GetCompanyByExternalIDParams{OrganizationID: orgID, ExternalID: strings.TrimSpace(key.ExternalID)})
	case strings.TrimSpace(key.Domain) != "":
		d := strings.ToLower(strings.TrimSpace(key.Domain))
		d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
		if i := strings.IndexAny(d, "/?#"); i >= 0 {
			d = d[:i]
		}
		co, err = s.q.GetCompanyByDomain(ctx, db.GetCompanyByDomainParams{OrganizationID: orgID, Domain: strings.TrimPrefix(d, "www.")})
	default:
		return uuid.Nil, fmt.Errorf("%w: company_uuid, external_id or domain is required", ErrInvalid)
	}
	if err != nil {
		return uuid.Nil, notFound(err)
	}
	if _, err := s.q.SetCompanyUsageScore(ctx, db.SetCompanyUsageScoreParams{
		OrganizationID: orgID, CompanyID: co.ID, UsageScore: pgtype.Int2{Int16: safeconv.ToInt16(score), Valid: true},
	}); err != nil {
		return uuid.Nil, err
	}
	return co.Uuid, nil
}

// Sweep scores every company of every organization that has the score on.
func (s *Service) Sweep(ctx context.Context) error {
	orgs, err := s.q.ListHealthOrganizations(ctx)
	if err != nil {
		return err
	}
	scored, alerts := 0, 0
	for _, org := range orgs {
		n, a, err := s.ComputeOrganization(ctx, org)
		if err != nil {
			// One organization's failure must not stop the others.
			s.log.Error("customer_health_org_failed", "organization_id", org, "error", err)
			continue
		}
		scored += n
		alerts += a
	}
	s.log.Info("customer_health_swept", "organizations", len(orgs), "companies", scored, "alerts", alerts)
	return nil
}

// ComputeOrganization scores every company of one organization and returns
// how many were scored and how many alerts were raised.
func (s *Service) ComputeOrganization(ctx context.Context, orgID int64) (int, int, error) {
	set, err := s.GetSettings(ctx, orgID)
	if err != nil {
		return 0, 0, err
	}
	var after int64
	total, alerts := 0, 0
	for {
		res, err := s.computePage(ctx, orgID, set, after, nil)
		if err != nil {
			return total, alerts, err
		}
		total += res.count
		alerts += res.alerts
		if res.count < pageSize {
			return total, alerts, nil
		}
		after = res.lastID
	}
}

type pageResult struct {
	count, alerts int
	lastID        int64
}

func (s *Service) computePage(ctx context.Context, orgID int64, set Settings, after int64, companyID *int64) (pageResult, error) {
	now := s.now().UTC()
	p := db.CompanyHealthStatsParams{
		OrganizationID: orgID, AfterID: after, RowLimit: pageSize,
		CsatSince:     pgtype.Timestamptz{Time: now.Add(-csatWindow), Valid: true},
		WindowStart:   pgtype.Timestamptz{Time: now.Add(-signalWindow), Valid: true},
		PreviousStart: pgtype.Timestamptz{Time: now.Add(-2 * signalWindow), Valid: true},
	}
	if companyID != nil {
		p.CompanyID = pgtype.Int8{Int64: *companyID, Valid: true}
	}
	rows, err := s.q.CompanyHealthStats(ctx, p)
	if err != nil {
		return pageResult{}, err
	}
	out := pageResult{count: len(rows)}
	for _, r := range rows {
		out.lastID = r.ID
		alerted, err := s.scoreOne(ctx, orgID, set, r, now)
		if err != nil {
			return out, err
		}
		if alerted {
			out.alerts++
		}
	}
	return out, nil
}

func (s *Service) scoreOne(ctx context.Context, orgID int64, set Settings, r db.CompanyHealthStatsRow, now time.Time) (bool, error) {
	in := engine.Inputs{
		CSATAvg: r.CsatAvg, CSATCount: r.CsatCount, OpenConversations: r.OpenConversations,
		SLABreaches: r.SlaBreaches, RecentConversations: r.RecentConversations,
		PreviousConversations: r.PreviousConversations, BillingStatus: r.BillingStatus,
		UsageScore: int2(r.UsageScore), UsageReportedAt: ts(r.UsageReportedAt),
	}
	res := engine.Compute(in, set.Weights, now)
	below, alert := engine.Crossing(res.Score, set.Threshold, r.BelowThreshold)
	if !set.Enabled {
		alert = false
	}
	signals, err := json.Marshal(res.Signals)
	if err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	params := db.UpsertCompanyHealthParams{
		OrganizationID: orgID, CompanyID: r.ID, Signals: signals, BelowThreshold: below,
		ComputedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	if res.Score != nil {
		params.Score = pgtype.Int2{Int16: safeconv.ToInt16(*res.Score), Valid: true}
	}
	if alert {
		params.AlertedAt = pgtype.Timestamptz{Time: now, Valid: true}
	}
	if _, err := q.UpsertCompanyHealth(ctx, params); err != nil {
		return false, err
	}
	if res.Score != nil {
		if err := q.InsertCompanyHealthHistory(ctx, db.InsertCompanyHealthHistoryParams{
			OrganizationID: orgID, CompanyID: r.ID, Score: safeconv.ToInt16(*res.Score), Signals: signals,
		}); err != nil {
			return false, err
		}
	}
	if alert {
		payload := events.CompanyHealthPayload{
			CompanyID: r.ID, CompanyUUID: r.Uuid, CompanyName: r.Name, Score: *res.Score,
			PreviousScore: int2(r.PreviousScore), Threshold: set.Threshold,
		}
		if r.AccountOwnerID.Valid {
			id := r.AccountOwnerID.Int64
			payload.AccountOwnerUserID = &id
		}
		ev, err := events.NewDomain(events.CompanyHealthDropped, orgID, payload)
		if err != nil {
			return false, err
		}
		if err := outbox.EnqueueQueries(ctx, q, ev); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	if alert && s.activity != nil {
		id := r.Uuid
		s.activity.Record(orgctx.WithOrganizationID(ctx, orgID), nil, "companies.health_dropped", "companies", &id, map[string]any{
			"score": *res.Score, "threshold": set.Threshold, "name": r.Name,
		}, nil)
	}
	return alert, nil
}

func int2(v pgtype.Int2) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int16)
	return &n
}

func ts(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
