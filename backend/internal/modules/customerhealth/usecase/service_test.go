package usecase_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customerhealth/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	pool *pgxpool.Pool
	svc  *usecase.Service
	org  int64
}

func setup(t *testing.T) fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var org int64
	if err := pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, 'Health') RETURNING id", "health-"+uuid.NewString()[:12]).Scan(&org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org) })
	return fixture{pool: pool, svc: usecase.New(pool, db.New(pool), nil, nil), org: org}
}

func (f fixture) id(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (f fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (f fixture) alerts(t *testing.T, company uuid.UUID) int64 {
	return f.id(t, "SELECT COUNT(*) FROM outbox_events WHERE event_name = 'company.health_dropped' AND (payload->>'tenant_id')::bigint = $1 AND payload->'data'->>'company_uuid' = $2", f.org, company.String())
}

func TestHealthScoreAlertsOncePerCrossing(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	owner := f.id(t, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Ece', 'CSM', 'active') RETURNING id", uuid.NewString()+"@example.test")
	t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", owner) })
	company := f.id(t, "INSERT INTO companies (organization_id, name, account_owner_id) VALUES ($1, 'Acme', $2) RETURNING id", f.org, owner)
	var companyUUID uuid.UUID
	_ = f.pool.QueryRow(ctx, "SELECT uuid FROM companies WHERE id = $1", company).Scan(&companyUUID)
	healthy := f.id(t, "INSERT INTO companies (organization_id, name) VALUES ($1, 'Calm') RETURNING id", f.org)
	contact := f.id(t, "INSERT INTO contacts (organization_id, company_id) VALUES ($1, $2) RETURNING id", f.org, company)

	// Nothing yet: perfect score, no alert.
	h, err := f.svc.Recompute(ctx, f.org, companyUUID)
	if err != nil || h.Score == nil || *h.Score != 100 || h.BelowThreshold {
		t.Fatalf("fresh: %+v %v", h, err)
	}

	// Many open conversations with SLA breaches, bad ratings, canceled billing.
	now := time.Now()
	for i := 1; i <= 8; i++ {
		conv := f.id(t, `INSERT INTO conversations (organization_id, number, channel_type, contact_id, company_id, status, sla_breached_at)
			VALUES ($1, $2, 'test', $3, $4, 'open', $5) RETURNING id`, f.org, i, contact, company, now.Add(-time.Hour))
		f.exec(t, "INSERT INTO conversation_ratings (organization_id, conversation_id, contact_id, score) VALUES ($1, $2, $3, 1)", f.org, conv, contact)
	}
	f.exec(t, "INSERT INTO company_revenue (organization_id, company_id, billing_status) VALUES ($1, $2, 'canceled')", f.org, company)

	n, alerts, err := f.svc.ComputeOrganization(ctx, f.org)
	if err != nil || n != 2 || alerts != 1 {
		t.Fatalf("sweep: n=%d alerts=%d err=%v", n, alerts, err)
	}
	h, _ = f.svc.GetHealth(ctx, f.org, companyUUID)
	if h.Score == nil || *h.Score >= 40 || !h.BelowThreshold || h.AlertedAt == nil || len(h.History) != 2 {
		t.Fatalf("at risk: %+v", h)
	}
	if !h.Signals["csat"].Used || !h.Signals["billing"].Used || h.Signals["usage"].Used {
		t.Fatalf("signals: %+v", h.Signals)
	}
	if got := f.alerts(t, companyUUID); got != 1 {
		t.Fatalf("alerts in outbox = %d", got)
	}
	// Still below: no second alert.
	if _, alerts, _ := f.svc.ComputeOrganization(ctx, f.org); alerts != 0 {
		t.Fatalf("repeat alert: %d", alerts)
	}

	// Recovery (fresh high usage, weights favor it), then a new drop alerts again.
	if _, err := f.svc.ReportUsage(ctx, f.org, usecase.UsageKey{UUID: &companyUUID}, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UpdateSettings(ctx, f.org, usecase.SettingsInput{Weights: map[string]int{"csat": 0, "open_conversations": 0, "sla_breaches": 0, "frequency_trend": 0, "billing": 0, "usage": 100}}); err != nil {
		t.Fatal(err)
	}
	if h, _ := f.svc.Recompute(ctx, f.org, companyUUID); h.Score == nil || *h.Score != 100 || h.BelowThreshold {
		t.Fatalf("recovered: %+v", h)
	}
	if _, err := f.svc.ReportUsage(ctx, f.org, usecase.UsageKey{UUID: &companyUUID}, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Recompute(ctx, f.org, companyUUID); err != nil {
		t.Fatal(err)
	}
	if got := f.alerts(t, companyUUID); got != 2 {
		t.Fatalf("second crossing alerts = %d", got)
	}

	// Disabled: scores still computed on demand, but no alerts and no sweep.
	off := false
	if _, err := f.svc.UpdateSettings(ctx, f.org, usecase.SettingsInput{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	_ = healthy
	if err := f.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}

	// Validation and isolation.
	bad := 101
	if _, err := f.svc.UpdateSettings(ctx, f.org, usecase.SettingsInput{Threshold: &bad}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("threshold: %v", err)
	}
	if _, err := f.svc.UpdateSettings(ctx, f.org, usecase.SettingsInput{Weights: map[string]int{"vibes": 5}}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("unknown signal: %v", err)
	}
	if _, err := f.svc.ReportUsage(ctx, f.org, usecase.UsageKey{UUID: &companyUUID}, 150); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("usage range: %v", err)
	}
	other := setup(t)
	if _, err := other.svc.GetHealth(ctx, other.org, companyUUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross org: %v", err)
	}
	set, _ := f.svc.GetSettings(ctx, f.org)
	if set.Enabled || set.Threshold != 40 || set.Weights["usage"] != 100 {
		t.Fatalf("settings: %+v", set)
	}
}
