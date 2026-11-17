package onboarding_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	appointmentsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments/usecase"
	knowledgerepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	knowledgeusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/onboarding"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
)

// TestSetupApplyAndUndoAgainstPostgres applies every template on the real
// schema (each item must be written without a constraint failing), checks
// that a second run skips what exists, and that undo removes the entities
// and restores the settings it changed.
func TestSetupApplyAndUndoAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for onboarding DB tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)

	templates, err := onboarding.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, tpl := range templates {
		t.Run(tpl.Key, func(t *testing.T) {
			var userID int64
			if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Elif', 'Yıldız', 'active') RETURNING id",
				uuid.NewString()+"@example.test").Scan(&userID); err != nil {
				t.Fatal(err)
			}
			orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
			org, err := onboarding.New(q, orgSvc, onboarding.Config{SelfServe: true}).
				CreateOrganization(ctx, onboarding.CreateInput{UserID: userID, Name: "Setup " + uuid.NewString()[:6]})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				c := context.Background()
				for _, stmt := range []string{
					"DELETE FROM onboarding_setup_runs WHERE organization_id = $1",
					"DELETE FROM knowledge_article_versions WHERE organization_id = $1",
					"DELETE FROM knowledge_chunks WHERE organization_id = $1",
					"DELETE FROM knowledge_articles WHERE organization_id = $1",
					"DELETE FROM knowledge_collections WHERE organization_id = $1",
					"DELETE FROM business_context_versions WHERE organization_id = $1",
					"DELETE FROM business_context_sections WHERE organization_id = $1",
					"DELETE FROM ai_agent_configs WHERE organization_id = $1",
					"DELETE FROM ai_policies WHERE organization_id = $1",
					"DELETE FROM domain_event_log WHERE organization_id = $1",
					"DELETE FROM outbox_events WHERE organization_id = $1",
					"DELETE FROM organizations WHERE id = $1",
				} {
					_, _ = pool.Exec(c, stmt, org.ID)
				}
				_, _ = pool.Exec(c, "DELETE FROM notification_preferences WHERE user_id = $1", userID)
				_, _ = pool.Exec(c, "DELETE FROM users WHERE id = $1", userID)
			})
			if _, err := pool.Exec(ctx, `INSERT INTO channel_instances (organization_id, channel_type, name, external_id, settings)
				VALUES ($1, 'widget', 'Web sitesi', $2, '{"color":"#111111","greeting_title":"Eski"}')`, org.ID, uuid.NewString()); err != nil {
				t.Fatal(err)
			}

			appts := appointmentsusecase.New(appointmentsusecase.Deps{Pool: pool, Queries: q, Entitlements: allowAll{}})
			svc := &onboarding.SetupService{
				Q: q, Templates: templates, Pub: realtime.NoopPublisher{},
				Applier: &onboarding.Applier{
					Pool: pool, Q: q, KB: knowledgeusecase.New(knowledgerepo.New(pool)),
					Profiles:     orgusecase.NewManagement(orgSvc, orgrepository.NewPostgres(pool, q), orgusecase.ManagementConfig{}),
					Appointments: onboarding.AppointmentsAdapter{Svc: appts, Q: q},
				},
			}
			draft, err := svc.FromTemplate(ctx, org.ID, userID, tpl.Key, "Acme")
			if err != nil {
				t.Fatal(err)
			}
			selection := []string{}
			for _, it := range draft.Items {
				if it.Status == onboarding.PreviewExists {
					t.Errorf("fresh organization: %s already exists", it.Key)
				}
				if it.Selected {
					selection = append(selection, it.Key)
				}
			}
			run := applyAndWait(t, svc, org.ID, draft.UUID, selection)
			if run.Status != "succeeded" {
				for _, it := range run.Items {
					if it.Status == onboarding.ItemFailed {
						t.Errorf("%s failed: %s", it.Key, it.Reason)
					}
				}
				t.Fatalf("status %s (%d failed)", run.Status, run.Failed)
			}
			var teams, articles int
			_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM teams WHERE organization_id = $1", org.ID).Scan(&teams)
			_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM knowledge_articles WHERE organization_id = $1 AND published_version IS NOT NULL", org.ID).Scan(&articles)
			if teams != len(run.Plan.Teams) || articles == 0 {
				t.Fatalf("teams %d (want %d), published articles %d", teams, len(run.Plan.Teams), articles)
			}
			var apptTypes int
			_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM appointment_types WHERE organization_id = $1 AND deleted_at IS NULL", org.ID).Scan(&apptTypes)
			if apptTypes != len(run.Plan.Appointments) {
				t.Fatalf("appointment types %d, want %d", apptTypes, len(run.Plan.Appointments))
			}
			var color string
			_ = pool.QueryRow(ctx, "SELECT settings->>'color' FROM channel_instances WHERE organization_id = $1", org.ID).Scan(&color)
			if color != run.Plan.Widget.Color {
				t.Fatalf("widget color %q, want %q", color, run.Plan.Widget.Color)
			}

			// CF-243: the agent written by the setup covers the connected
			// widget channel (no row when the persona item was not applied).
			var widgetInScope bool
			if err := pool.QueryRow(ctx, "SELECT COALESCE(config->'channels' ? 'widget', false) FROM ai_agent_configs WHERE organization_id = $1", org.ID).Scan(&widgetInScope); err == nil && !widgetInScope {
				t.Fatal("agent config channels miss the connected widget type")
			}

			// A second run of the same template finds everything in place.
			again, err := svc.FromTemplate(ctx, org.ID, userID, tpl.Key, "Acme")
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range again.Items {
				if it.Status == onboarding.PreviewNew && isEntity(it.Kind) {
					t.Errorf("second run: %s should exist", it.Key)
				}
			}

			undone, err := svc.Undo(ctx, org.ID, userID, run.UUID)
			if err != nil {
				t.Fatal(err)
			}
			if undone.Status != "undone" || undone.CanUndo {
				t.Fatalf("after undo: %s can_undo=%v", undone.Status, undone.CanUndo)
			}
			for _, it := range undone.Items {
				if it.Undone && it.Reason != "" {
					t.Errorf("undo kept %s: %s", it.Key, it.Reason)
				}
			}
			_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM teams WHERE organization_id = $1", org.ID).Scan(&teams)
			_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM knowledge_articles WHERE organization_id = $1 AND deleted_at IS NULL", org.ID).Scan(&articles)
			_ = pool.QueryRow(ctx, "SELECT settings->>'color' FROM channel_instances WHERE organization_id = $1", org.ID).Scan(&color)
			_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM appointment_types WHERE organization_id = $1 AND deleted_at IS NULL", org.ID).Scan(&apptTypes)
			if teams != 0 || articles != 0 || color != "#111111" || apptTypes != 0 {
				t.Fatalf("undo left teams=%d articles=%d widget color=%s", teams, articles, color)
			}
			if _, err := svc.Undo(ctx, org.ID, userID, run.UUID); err == nil {
				t.Fatal("second undo must fail")
			}
		})
	}
}

// allowAll grants every plan feature (the appointments module here).
type allowAll struct{}

func (allowAll) Require(context.Context, int64, string) error { return nil }
func (allowAll) CheckLimit(context.Context, int64, string, int64, int64) (entitlement.Decision, error) {
	return entitlement.Decision{Allowed: true, Unlimited: true}, nil
}
func (allowAll) RequireWritable(context.Context, int64) error { return nil }

func isEntity(kind string) bool {
	switch kind {
	case onboarding.KindBranding, onboarding.KindWidget, onboarding.KindAIPersona, onboarding.KindAIGuardrail,
		onboarding.KindNotifications, onboarding.KindAppointment:
		return false
	}
	return true
}

func applyAndWait(t *testing.T, svc *onboarding.SetupService, org int64, id uuid.UUID, selection []string) onboarding.RunView {
	t.Helper()
	ctx := context.Background()
	if _, err := svc.Apply(ctx, org, id, selection); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		run, err := svc.Run(ctx, org, id)
		if err != nil {
			t.Fatal(err)
		}
		if run.FinishedAt != nil {
			return run
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("apply did not finish")
	return onboarding.RunView{}
}

// TestDiscoverFallsBackWithoutAI: no AI provider → the closest template,
// personalised with the answers, and a notice saying why.
func TestDiscoverFallsBackWithoutAI(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for onboarding DB tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	var userID int64
	if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Ada', 'Kaya', 'active') RETURNING id",
		uuid.NewString()+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	org, err := onboarding.New(q, orgusecase.New(orgrepository.NewPostgres(pool, q), nil), onboarding.Config{SelfServe: true}).
		CreateOrganization(ctx, onboarding.CreateInput{UserID: userID, Name: "Klinik " + uuid.NewString()[:6]})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, "DELETE FROM onboarding_setup_runs WHERE organization_id = $1", org.ID)
		_, _ = pool.Exec(c, "DELETE FROM ai_policies WHERE organization_id = $1", org.ID)
		_, _ = pool.Exec(c, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id = $1", userID)
	})
	templates, err := onboarding.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	svc := &onboarding.SetupService{Q: q, Templates: templates, Applier: &onboarding.Applier{Pool: pool, Q: q}}
	run, err := svc.Discover(ctx, org.ID, userID, onboarding.DiscoverInput{Locale: "tr", Answers: onboarding.Answers{
		CompanyName: "Şifa Klinik", Industry: "healthcare", SupportHours: "weekdays_8_20", Channels: []string{"whatsapp"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if run.Discovery.Mode != "fallback" || run.Discovery.Notice != "ai_unavailable" || run.TemplateKey != "healthcare" {
		t.Fatalf("discovery %+v template %s", run.Discovery, run.TemplateKey)
	}
	if run.Plan.Summary.CompanyName != "Şifa Klinik" || run.Plan.BusinessHours.Days[0].Open != "08:00" {
		t.Fatalf("answers not applied: %+v", run.Plan.Summary)
	}
	// The appointments module is not on this build: those items are unavailable.
	for _, it := range run.Items {
		if it.Kind == onboarding.KindAppointment && (it.Status != onboarding.PreviewUnavailable || it.Selected) {
			t.Fatalf("appointment item %+v", it)
		}
	}
}
