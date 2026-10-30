package usecase_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	pool *pgxpool.Pool
	svc  *usecase.Service
	org  int64
	now  time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for SLA tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	f := &fixture{pool: pool, now: time.Now().UTC().Truncate(time.Second)}
	f.svc = usecase.New(pool, q, outbox.NewStore(pool, q), nil).WithClock(func() time.Time { return f.now })
	f.org = scalar(t, pool, "INSERT INTO organizations (slug, name) VALUES ($1, 'SLA test') RETURNING id", "sla-"+uuid.NewString()[:12])
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM outbox_events WHERE organization_id = $1", f.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
	})
	return f
}

func scalar(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (f *fixture) conversation(t *testing.T, channel, priority string, created time.Time) (int64, uuid.UUID) {
	t.Helper()
	contact := scalar(t, f.pool, "INSERT INTO contacts (organization_id, name) VALUES ($1, 'Müşteri') RETURNING id", f.org)
	var id int64
	var u uuid.UUID
	err := f.pool.QueryRow(context.Background(), `
		INSERT INTO conversations (organization_id, number, channel_type, contact_id, priority, created_at, waiting_since)
		VALUES ($1, (SELECT COALESCE(MAX(number), 0) + 1 FROM conversations WHERE organization_id = $1), $2, $3, $4, $5, $5)
		RETURNING id, uuid`, f.org, channel, contact, priority, created).Scan(&id, &u)
	if err != nil {
		t.Fatal(err)
	}
	return id, u
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (f *fixture) events(t *testing.T, name string) int64 {
	t.Helper()
	return scalar(t, f.pool, "SELECT COUNT(*) FROM outbox_events WHERE organization_id = $1 AND event_name = $2", f.org, name)
}

func mins(v int32) *int32 { return &v }

func standardPolicy(name string, cond model.Conditions) model.PolicyInput {
	return model.PolicyInput{Name: name, Conditions: cond, Targets: []model.Target{
		{Priority: "normal", FirstResponseMinutes: mins(60), NextResponseMinutes: mins(30), ResolutionMinutes: mins(480)},
		{Priority: "urgent", FirstResponseMinutes: mins(15), ResolutionMinutes: mins(120)},
	}}
}

func metric(t *testing.T, sla model.ConversationSLA, name string) model.Metric {
	t.Helper()
	for _, m := range sla.Metrics {
		if m.Metric == name {
			return m
		}
	}
	t.Fatalf("metric %s missing in %+v", name, sla.Metrics)
	return model.Metric{}
}

func TestClocksFollowConversationAndSweepBreaches(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// The e-mail-only policy comes first but does not match a widget chat.
	if _, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("E-posta", model.Conditions{ChannelTypes: []string{"email"}})); err != nil {
		t.Fatal(err)
	}
	pol, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Varsayılan", model.Conditions{}))
	if err != nil {
		t.Fatal(err)
	}
	created := f.now.Add(-90 * time.Minute)
	id, u := f.conversation(t, "widget", "normal", created)

	if err := f.svc.Sync(ctx, f.org, id, f.now); err != nil {
		t.Fatal(err)
	}
	sla, err := f.svc.ConversationSLA(ctx, f.org, u)
	if err != nil {
		t.Fatal(err)
	}
	if sla.Policy == nil || sla.Policy.UUID != pol.UUID {
		t.Fatalf("policy %+v", sla.Policy)
	}
	fr := metric(t, sla, "first_response")
	if !fr.DueAt.Equal(created.Add(time.Hour)) || fr.RemainingSeconds >= 0 {
		t.Fatalf("first response %+v", fr)
	}
	if n := f.events(t, "sla.applied"); n != 2 {
		t.Fatalf("sla.applied = %d", n)
	}

	// The sweep breaches the overdue first response and warns nothing else
	// (resolution is at 90/480 minutes, below the 80% warning).
	n, err := f.svc.Sweep(ctx)
	if err != nil || n != 1 {
		t.Fatalf("sweep n=%d err=%v", n, err)
	}
	if got := f.events(t, "sla.breached"); got != 1 {
		t.Fatalf("sla.breached = %d", got)
	}
	var breached *time.Time
	if err := f.pool.QueryRow(ctx, "SELECT sla_breached_at FROM conversations WHERE id = $1", id).Scan(&breached); err != nil || breached == nil {
		t.Fatalf("conversation not marked breached: %v", err)
	}
	if n, _ := f.svc.Sweep(ctx); n != 0 {
		t.Fatalf("second sweep moved %d", n)
	}

	// The agent answers and sets pending: first response completes (already
	// breached, no met), resolution pauses.
	f.exec(t, "UPDATE conversations SET first_response_at = $2, last_outbound_at = $2, waiting_since = NULL, status = 'pending' WHERE id = $1", id, f.now)
	if err := f.svc.Sync(ctx, f.org, id, f.now); err != nil {
		t.Fatal(err)
	}
	sla, _ = f.svc.ConversationSLA(ctx, f.org, u)
	if m := metric(t, sla, "first_response"); m.Status != "breached" || m.CompletedAt == nil {
		t.Fatalf("first response %+v", m)
	}
	res := metric(t, sla, "resolution")
	if res.Status != "paused" || res.RemainingSeconds != int64((480-90)*60) {
		t.Fatalf("resolution %+v", res)
	}
	var resDue *time.Time
	_ = f.pool.QueryRow(ctx, "SELECT resolution_due_at FROM conversations WHERE id = $1", id).Scan(&resDue)
	if resDue != nil {
		t.Fatal("paused clock must not show a deadline")
	}

	// The customer answers an hour later: open again, next response starts.
	f.now = f.now.Add(time.Hour)
	f.exec(t, "UPDATE conversations SET status = 'open', waiting_since = $2 WHERE id = $1", id, f.now)
	if err := f.svc.Sync(ctx, f.org, id, f.now); err != nil {
		t.Fatal(err)
	}
	sla, _ = f.svc.ConversationSLA(ctx, f.org, u)
	if m := metric(t, sla, "next_response"); m.Status != "running" || !m.DueAt.Equal(f.now.Add(30*time.Minute)) {
		t.Fatalf("next response %+v", m)
	}
	if m := metric(t, sla, "resolution"); !m.DueAt.Equal(f.now.Add(390 * time.Minute)) {
		t.Fatalf("resumed resolution %+v", m)
	}

	// 25 minutes later the sweep warns about the next response (80%).
	f.now = f.now.Add(25 * time.Minute)
	if n, err := f.svc.Sweep(ctx); err != nil || n != 1 || f.events(t, "sla.warning") != 1 {
		t.Fatalf("warning sweep n=%d err=%v", n, err)
	}

	// Priority goes urgent: resolution target drops to 120 minutes and the
	// urgent policy has no next response target.
	f.exec(t, "UPDATE conversations SET priority = 'urgent' WHERE id = $1", id)
	if err := f.svc.Sync(ctx, f.org, id, f.now); err != nil {
		t.Fatal(err)
	}
	sla, _ = f.svc.ConversationSLA(ctx, f.org, u)
	for _, m := range sla.Metrics {
		if m.Metric == "next_response" {
			t.Fatal("next response must be dropped for urgent")
		}
	}

	// Resolve 10 minutes later: 90 + 25 + 10 working minutes are over the
	// urgent 120 → breached on completion.
	f.now = f.now.Add(10 * time.Minute)
	f.exec(t, "UPDATE conversations SET status = 'resolved', resolved_at = $2 WHERE id = $1", id, f.now)
	if err := f.svc.Sync(ctx, f.org, id, f.now); err != nil {
		t.Fatal(err)
	}
	sla, _ = f.svc.ConversationSLA(ctx, f.org, u)
	if m := metric(t, sla, "resolution"); m.Status != "breached" {
		t.Fatalf("resolution %+v", m)
	}
}

func TestBusinessHoursAndHolidays(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	week := businesshours.Week{
		"mon": {Open: "09:00", Close: "18:00"}, "tue": {Open: "09:00", Close: "18:00"},
		"wed": {Open: "09:00", Close: "18:00"}, "thu": {Open: "09:00", Close: "18:00"},
		"fri": {Open: "09:00", Close: "18:00"}, "sat": {Closed: true},
	}
	first, err := f.svc.CreateBusinessHours(ctx, f.org, model.BusinessHoursInput{Name: "Merkez", WorkingHours: week})
	if err != nil || !first.IsDefault || first.Timezone != "Europe/Istanbul" || first.HolidayCountry != "TR" {
		t.Fatalf("first schedule %+v err=%v", first, err)
	}
	team := int64(42)
	second, err := f.svc.CreateBusinessHours(ctx, f.org, model.BusinessHoursInput{Name: "Destek ekibi", WorkingHours: week, TeamID: &team})
	if err != nil || second.IsDefault {
		t.Fatalf("second schedule %+v err=%v", second, err)
	}
	if _, err := f.svc.CreateBusinessHours(ctx, f.org, model.BusinessHoursInput{Name: "Kopya", WorkingHours: week, TeamID: &team}); !errors.Is(err, usecase.ErrConflict) {
		t.Fatalf("duplicate team schedule err=%v", err)
	}
	if _, err := f.svc.CreateBusinessHours(ctx, f.org, model.BusinessHoursInput{Name: "Boş", WorkingHours: businesshours.Week{}}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("empty week err=%v", err)
	}
	if err := f.svc.DeleteBusinessHours(ctx, f.org, first.UUID); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("deleting the default with others left: %v", err)
	}
	if _, err := f.svc.CreateHoliday(ctx, f.org, model.HolidayInput{Date: "2026-12-31", Name: "Yıl sonu", Open: "09:00", Close: "12:00"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateHoliday(ctx, f.org, model.HolidayInput{Date: "31.12.2026", Name: "Bozuk", Closed: true}); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("bad date err=%v", err)
	}
	list, err := f.svc.ListHolidays(ctx, f.org, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(list) != 1 || *list[0].Close != "12:00" {
		t.Fatalf("holidays %+v err=%v", list, err)
	}
	pub, err := f.svc.PublicHolidays(ctx, "tr", 2026)
	if err != nil || len(pub) < 15 {
		t.Fatalf("public holidays %d err=%v", len(pub), err)
	}

	// The default schedule drives the clock: a conversation on Tuesday
	// 27 Oct 2026 17:30 Istanbul with a 60 minute first response target is
	// due Wednesday 09:30 (30m Tuesday + 30m Wednesday).
	if _, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Mesai", model.Conditions{})); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 10, 27, 14, 30, 0, 0, time.UTC)
	f.now = created
	id, u := f.conversation(t, "widget", "normal", created)
	if err := f.svc.Sync(ctx, f.org, id, created); err != nil {
		t.Fatal(err)
	}
	sla, _ := f.svc.ConversationSLA(ctx, f.org, u)
	if m := metric(t, sla, "first_response"); !m.DueAt.Equal(time.Date(2026, 10, 28, 6, 30, 0, 0, time.UTC)) {
		t.Fatalf("first response due %s", m.DueAt)
	}
	// Resolution 480m: 30m Tue + 4h Wed (half day) + Fri 3.5h → Fri 12:30.
	if m := metric(t, sla, "resolution"); !m.DueAt.Equal(time.Date(2026, 10, 30, 9, 30, 0, 0, time.UTC)) {
		t.Fatalf("resolution due %s", m.DueAt)
	}
}

func TestContractedCompanyPolicyAndTier(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enterprise, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Kurumsal", model.Conditions{CompanyTiers: []string{"Enterprise"}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Varsayılan", model.Conditions{})); err != nil {
		t.Fatal(err)
	}
	contract, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("ACME sözleşmesi", model.Conditions{ChannelTypes: []string{"email"}}))
	if err != nil {
		t.Fatal(err)
	}
	company := scalar(t, f.pool, "INSERT INTO companies (organization_id, name, tier) VALUES ($1, 'ACME', 'enterprise') RETURNING id", f.org)
	var companyUUID uuid.UUID
	_ = f.pool.QueryRow(ctx, "SELECT uuid FROM companies WHERE id = $1", company).Scan(&companyUUID)
	id, u := f.conversation(t, "widget", "normal", f.now)
	f.exec(t, "UPDATE conversations SET company_id = $2 WHERE id = $1", id, company)

	policyOf := func() uuid.UUID {
		t.Helper()
		if err := f.svc.Sync(ctx, f.org, id, f.now); err != nil {
			t.Fatal(err)
		}
		sla, err := f.svc.ConversationSLA(ctx, f.org, u)
		if err != nil || sla.Policy == nil {
			t.Fatalf("sla %+v err=%v", sla, err)
		}
		return sla.Policy.UUID
	}
	// Tier match (case-insensitive) beats the later catch-all.
	if got := policyOf(); got != enterprise.UUID {
		t.Fatalf("tier policy not applied: %s", got)
	}
	// The contracted policy wins even though its conditions (e-mail) do not match.
	if _, err := f.svc.SetCompanyPolicy(ctx, f.org, companyUUID, &contract.UUID); err != nil {
		t.Fatal(err)
	}
	if got := policyOf(); got != contract.UUID {
		t.Fatalf("contracted policy not applied: %s", got)
	}
	list, _ := f.svc.ListPolicies(ctx, f.org)
	for _, p := range list {
		if p.UUID == contract.UUID && p.ContractedCompanies != 1 {
			t.Fatalf("contracted companies = %d", p.ContractedCompanies)
		}
	}
	if _, err := f.svc.SetCompanyPolicy(ctx, f.org, companyUUID, nil); err != nil {
		t.Fatal(err)
	}
	if got := policyOf(); got != enterprise.UUID {
		t.Fatalf("after clearing the contract: %s", got)
	}
	bogus := uuid.New()
	if _, err := f.svc.SetCompanyPolicy(ctx, f.org, companyUUID, &bogus); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("unknown policy err=%v", err)
	}
}

func TestTeamWorkingHoursDriveTheClock(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Ekip", model.Conditions{})); err != nil {
		t.Fatal(err)
	}
	// The night team works 22:00-24:00 every weekday (CF-5 teams.working_hours).
	team := scalar(t, f.pool, `INSERT INTO teams (organization_id, name, timezone, working_hours) VALUES ($1, 'Gece', 'Europe/Istanbul',
		'{"days":[{"day":"mon","open":true,"start":"22:00","end":"24:00"},{"day":"tue","open":true,"start":"22:00","end":"24:00"},
		{"day":"wed","open":true,"start":"22:00","end":"24:00"},{"day":"thu","open":true,"start":"22:00","end":"24:00"},
		{"day":"fri","open":true,"start":"22:00","end":"24:00"}]}') RETURNING id`, f.org)
	created := time.Date(2026, 10, 27, 7, 0, 0, 0, time.UTC) // Tuesday 10:00 Istanbul
	f.now = created
	id, u := f.conversation(t, "widget", "normal", created)
	f.exec(t, "UPDATE conversations SET assignee_team_id = $2 WHERE id = $1", id, team)
	if err := f.svc.Sync(ctx, f.org, id, created); err != nil {
		t.Fatal(err)
	}
	sla, _ := f.svc.ConversationSLA(ctx, f.org, u)
	// First response 60 working minutes: Tuesday 22:00-23:00 Istanbul = 20:00 UTC.
	if m := metric(t, sla, "first_response"); !m.DueAt.Equal(time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)) {
		t.Fatalf("due %s", m.DueAt)
	}
}

func TestRevenuePlanAndMRRConditions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	min := int64(100000)
	big, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Büyük hesap", model.Conditions{MinCompanyMRRMinor: &min}))
	if err != nil {
		t.Fatal(err)
	}
	pro, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Pro plan", model.Conditions{CompanyPlans: []string{" pro ", "PRO"}}))
	if err != nil || len(pro.Conditions.CompanyPlans) != 1 {
		t.Fatalf("plans normalized: %+v %v", pro.Conditions, err)
	}
	fallback, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Varsayılan", model.Conditions{}))
	if err != nil {
		t.Fatal(err)
	}
	neg := int64(-1)
	if _, err := f.svc.CreatePolicy(ctx, f.org, standardPolicy("Bozuk", model.Conditions{MinCompanyMRRMinor: &neg})); err == nil {
		t.Fatal("negative min mrr accepted")
	}
	company := scalar(t, f.pool, "INSERT INTO companies (organization_id, name) VALUES ($1, 'ACME') RETURNING id", f.org)
	id, u := f.conversation(t, "widget", "normal", f.now)
	f.exec(t, "UPDATE conversations SET company_id = $2 WHERE id = $1", id, company)
	policyOf := func() uuid.UUID {
		t.Helper()
		if err := f.svc.Sync(ctx, f.org, id, f.now); err != nil {
			t.Fatal(err)
		}
		sla, err := f.svc.ConversationSLA(ctx, f.org, u)
		if err != nil || sla.Policy == nil {
			t.Fatalf("sla %+v err=%v", sla, err)
		}
		return sla.Policy.UUID
	}
	if got := policyOf(); got != fallback.UUID {
		t.Fatalf("no revenue: %s", got)
	}
	f.exec(t, "INSERT INTO company_revenue (organization_id, company_id, mrr_minor, plan) VALUES ($1, $2, 5000, 'Pro')", f.org, company)
	if got := policyOf(); got != pro.UUID {
		t.Fatalf("plan policy not applied: %s", got)
	}
	f.exec(t, "UPDATE company_revenue SET mrr_minor = 150000 WHERE company_id = $1", company)
	if got := policyOf(); got != big.UUID {
		t.Fatalf("mrr policy not applied: %s", got)
	}
}
