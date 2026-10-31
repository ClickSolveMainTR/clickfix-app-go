package demoseed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	billingmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	billingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
)

const (
	// FullPlanCode is the hidden plan that grants every feature; organizations
	// created with demo data start a trial on it. EnsureFullPlan creates it
	// when the catalog does not have it yet.
	FullPlanCode = "demo-full"
	// fullPlanTrialDays is the longest trial a plan version accepts.
	fullPlanTrialDays = 365
	// seedTimeout bounds a background demo fill.
	seedTimeout = 10 * time.Minute
)

// Reindexer schedules a search reindex after the demo rows exist.
type Reindexer interface {
	EnqueueReindex(ctx context.Context, spec string)
}

// Platform fills organizations created from the platform wizard with demo
// data (POST /v1/platform/organizations with demo_data).
type Platform struct {
	// base is cancelled when the API shuts down, so a fill in flight stops.
	base    context.Context
	pool    *pgxpool.Pool
	billing *billingusecase.Service
	search  Reindexer
	log     *slog.Logger
}

// NewPlatform wires the platform demo seeder; search may be nil. base bounds
// background fills (cancel it on shutdown).
func NewPlatform(base context.Context, pool *pgxpool.Pool, billing *billingusecase.Service, search Reindexer, log *slog.Logger) *Platform {
	if log == nil {
		log = slog.Default()
	}
	return &Platform{base: base, pool: pool, billing: billing, search: search, log: log}
}

// TrialPrice returns the trial price of the published FullPlanCode plan,
// creating the plan or publishing a new version first when it is missing or
// no longer grants every feature in the catalog.
func (p *Platform) TrialPrice(ctx context.Context, actor int64) (uuid.UUID, error) {
	return EnsureFullPlan(ctx, p.billing, actor)
}

// NewPassword returns the shared password of a new demo organization's
// staff. It is generated before the organization is created so nothing can
// fail after the creation.
func (p *Platform) NewPassword() (string, error) { return GeneratePassword() }

// Accounts lists the demo logins an organization with this slug gets.
func Accounts(slug string) []Account {
	return (&seeder{tenant: true, domain: tenantDomain(slug)}).accounts()
}

// Start fills a freshly created organization in the background (a full fill
// can outlast an HTTP write timeout). The outcome is logged and written to
// the organization's activity log (organizations.demo_data_finished /
// organizations.demo_data_failed); a failed fill leaves a partly filled
// organization, which is deleted and created again.
func (p *Platform) Start(org uuid.UUID, slug, pw string, actor int64) {
	p.log.Info("demo_seed_started", "organization", org, "slug", slug)
	go func() {
		ctx, cancel := context.WithTimeout(p.base, seedTimeout)
		defer cancel()
		started := time.Now()
		var (
			out Result
			err error
		)
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			out, err = seedOrganization(ctx, p.pool, org, Options{Password: pw, Log: p.log})
		}()
		var payload map[string]any
		action := "organizations.demo_data_finished"
		if err != nil {
			p.log.Error("demo_seed_failed", "organization", org, "error", err)
			action, payload = "organizations.demo_data_failed", map[string]any{"error": err.Error()}
		} else {
			p.log.Info("demo_seed_finished", "organization", org, "conversations", out.Conversations,
				"articles", out.Articles, "duration", time.Since(started).Round(time.Millisecond))
			payload = map[string]any{"conversations": out.Conversations, "articles": out.Articles}
			// The reindex is queued (or run inline without a queue) before cancel.
			if p.search != nil {
				p.search.EnqueueReindex(ctx, "")
			}
		}
		// Record with a fresh context: the fill's may be cancelled or timed out.
		rctx, rcancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer rcancel()
		q := db.New(p.pool)
		if row, lerr := q.GetOrganizationByUUID(rctx, org); lerr == nil {
			activity.NewRecorder(q, p.log).Record(orgctx.WithOrganizationID(rctx, row.ID), &actor, action,
				"platform.organizations", &org, payload, nil)
		}
	}()
}

// tenantDomain is the e-mail domain of a customer organization's demo staff.
func tenantDomain(slug string) string { return slug + ".demo.example.com" }

// seedOrganization fills an existing, freshly created organization with the
// demo content. The organization keeps its owner; the demo staff join with
// their roles (the demo owner as admin) and the setup wizard is marked done.
// It refuses an organization that already has conversations, so it never
// mixes demo rows into a customer's real data.
func seedOrganization(ctx context.Context, pool *pgxpool.Pool, orgUUID uuid.UUID, opts Options) (Result, error) {
	org, err := db.New(pool).GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return Result{}, fmt.Errorf("organization: %w", err)
	}
	var used bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM conversations WHERE organization_id = $1)`, org.ID).Scan(&used); err != nil {
		return Result{}, err
	}
	if used {
		return Result{}, fmt.Errorf("organization %s already has conversations; demo data is only written into new organizations", org.Slug)
	}
	s, err := newSeeder(pool, opts, tenantDomain(org.Slug))
	if err != nil {
		return Result{}, err
	}
	s.org, s.tenant = org, true
	res := Result{OrganizationUUID: org.Uuid, OrganizationSlug: org.Slug, Password: s.opts.Password}
	if err := s.upsertUsers(ctx); err != nil {
		return res, fmt.Errorf("users: %w", err)
	}
	if err := s.q.CreateOrganizationOnboarding(ctx, org.ID); err != nil {
		return res, fmt.Errorf("onboarding: %w", err)
	}
	if _, err := s.q.FinishOrganizationOnboarding(ctx, org.ID); err != nil {
		return res, fmt.Errorf("onboarding: %w", err)
	}
	articles, err := s.fill(ctx, false)
	if err != nil {
		return res, err
	}
	res.Articles = articles
	res.Conversations = s.opts.Conversations
	res.WidgetPublicID = s.channels["widget"].Uuid
	res.Accounts = s.accounts()
	return res, nil
}

// EnsureFullPlan returns the trial price of the published FullPlanCode plan
// (monthly TRY), creating the plan and publishing a version with every
// catalog feature enabled or unlimited when needed.
func EnsureFullPlan(ctx context.Context, billing *billingusecase.Service, actor int64) (uuid.UUID, error) {
	features, err := billing.Features(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	plan, err := fullPlan(ctx, billing)
	if errors.Is(err, billingmodel.ErrNotFound) {
		_, err = billing.Create(ctx, billingmodel.PlanInput{
			Code: FullPlanCode, Name: "Demo · Tüm özellikler",
			Description: "Demo verili organizasyonlar için tüm özelliklerin açık ve sınırsız olduğu gizli plan.",
			Visibility:  "hidden", SortOrder: 1000,
		})
		// A parallel request may have created it first.
		if err != nil && !errors.Is(err, billingmodel.ErrConflict) {
			return uuid.Nil, fmt.Errorf("plan: %w", err)
		}
		plan, err = fullPlan(ctx, billing)
	}
	if err != nil {
		return uuid.Nil, err
	}
	if plan.ArchivedAt != nil {
		return uuid.Nil, fmt.Errorf("plan %s is archived; restore it or remove the code from the catalog", FullPlanCode)
	}
	for _, v := range plan.Versions {
		if v.Status != "published" {
			continue
		}
		// Versions are listed newest first; the first published one is current.
		if v.TrialDays > 0 && grantsEverything(v, features) {
			if price := trialPriceOf(v); price != uuid.Nil {
				return price, nil
			}
		}
		break
	}
	on := true
	grants := make([]billingmodel.Grant, 0, len(features))
	for _, f := range features {
		g := billingmodel.Grant{Feature: f.Key}
		if f.Kind == "boolean" {
			g.Enabled = &on
		} else {
			g.Unlimited = true
		}
		grants = append(grants, g)
	}
	v, err := billing.SaveVersion(ctx, plan.UUID, uuid.Nil, billingmodel.VersionInput{
		TrialDays: fullPlanTrialDays, Entitlements: grants,
		Prices: []billingmodel.Price{
			{Interval: "monthly", Currency: "TRY", Amount: 0, VAT: 2000},
			{Interval: "yearly", Currency: "TRY", Amount: 0, VAT: 2000},
		},
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("plan version: %w", err)
	}
	v, err = billing.Publish(ctx, plan.UUID, v.UUID, actor)
	if err != nil {
		return uuid.Nil, fmt.Errorf("publish plan: %w", err)
	}
	if price := trialPriceOf(v); price != uuid.Nil {
		return price, nil
	}
	return uuid.Nil, fmt.Errorf("plan %s has no price", FullPlanCode)
}

func fullPlan(ctx context.Context, billing *billingusecase.Service) (billingmodel.Plan, error) {
	plans, _, err := billing.List(ctx, false, FullPlanCode, 50, 0)
	if err != nil {
		return billingmodel.Plan{}, err
	}
	for _, p := range plans {
		if p.Code == FullPlanCode {
			return p, nil
		}
	}
	return billingmodel.Plan{}, billingmodel.ErrNotFound
}

// grantsEverything reports whether v enables (or leaves unlimited) every
// feature of the current catalog.
func grantsEverything(v billingmodel.Version, features []billingmodel.Feature) bool {
	grants := make(map[string]billingmodel.Grant, len(v.Entitlements))
	for _, g := range v.Entitlements {
		grants[g.Feature] = g
	}
	for _, f := range features {
		g, ok := grants[f.Key]
		switch {
		case !ok:
			return false
		case f.Kind == "boolean" && (g.Enabled == nil || !*g.Enabled):
			return false
		case f.Kind != "boolean" && !g.Unlimited:
			return false
		}
	}
	return true
}

func trialPriceOf(v billingmodel.Version) uuid.UUID {
	for _, p := range v.Prices {
		if p.Interval == "monthly" && p.Currency == "TRY" {
			return p.UUID
		}
	}
	if len(v.Prices) > 0 {
		return v.Prices[0].UUID
	}
	return uuid.Nil
}
