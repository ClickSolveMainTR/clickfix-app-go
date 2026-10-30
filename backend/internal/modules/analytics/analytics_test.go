package analytics_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	am "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/model"
	ar "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/repository"
	au "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/usecase"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	nm "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	tm "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/model"
	tr "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/repository"
	tu "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/usecase"
	sm "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/model"
	sr "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/repository"
	su "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	pool                      *pgxpool.Pool
	q                         *db.Queries
	org, other, contact, conv int64
	convUUID                  uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["timezone"] = "America/Los_Angeles"
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	f := fixture{pool: p, q: db.New(p)}
	for _, dst := range []*int64{&f.org, &f.other} {
		if err = p.QueryRow(ctx, "INSERT INTO organizations(slug,name,timezone) VALUES($1,'Test','America/New_York') RETURNING id", "test-"+uuid.NewString()).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		// Avoid leaving pointer events for the later platform-wide publisher test after deleting fixture organizations.
		_, _ = p.Exec(context.Background(), "DELETE FROM outbox_events WHERE organization_id=ANY($1::bigint[])", []int64{f.org, f.other})
		_, _ = p.Exec(context.Background(), "DELETE FROM organizations WHERE id=ANY($1::bigint[])", []int64{f.org, f.other})
	})
	if err = p.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Test') RETURNING id", f.org).Scan(&f.contact); err != nil {
		t.Fatal(err)
	}
	if err = p.QueryRow(ctx, `INSERT INTO conversations(organization_id,number,channel_type,contact_id,status,created_at,first_response_at,resolved_at) VALUES($1,1,'email',$2,'resolved','2026-10-20T02:00:00Z','2026-10-20T02:02:00Z','2026-10-20T03:00:00Z') RETURNING id,uuid`, f.org, f.contact).Scan(&f.conv, &f.convUUID); err != nil {
		t.Fatal(err)
	}
	return f
}
func TestAnalyticsProjectionReplayAndTimezone(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	svc := au.New(ar.New(f.pool, f.q))
	for i := 0; i < 2; i++ {
		if err := svc.Backfill(ctx, f.org); err != nil {
			t.Fatal(err)
		}
	}
	filter := am.Filter{From: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC), Bucket: "day"}
	out, err := svc.Dashboard(ctx, f.org, filter)
	if err != nil {
		t.Fatal(err)
	}
	cards := out["cards"].(map[string]any)
	if cards["volume"] != float64(1) || cards["first_response_seconds"] != float64(120) || cards["resolution_seconds"] != float64(3600) {
		t.Fatalf("unexpected cards: %#v", cards)
	}
	series := out["series"].([]any)
	if len(series) != 1 || !strings.HasPrefix(series[0].(map[string]any)["bucket"].(string), "2026-10-19") {
		t.Fatalf("not organization timezone: %#v", series)
	}
	foreign, err := svc.Dashboard(ctx, f.other, filter)
	if err != nil {
		t.Fatal(err)
	}
	if foreign["cards"].(map[string]any)["volume"] != float64(0) {
		t.Fatal("cross-org metric leak")
	}
	filter.Bucket = "DROP TABLE"
	if _, err = svc.Dashboard(ctx, f.org, filter); !errors.Is(err, au.ErrInvalid) {
		t.Fatalf("invalid bucket: %v", err)
	}
	filter.Bucket = "day"
	filter.Timezone = "Invalid/Timezone"
	if _, err = svc.Dashboard(ctx, f.org, filter); !errors.Is(err, au.ErrInvalid) {
		t.Fatalf("invalid timezone: %v", err)
	}
}

type sender struct{ posts []conv.PostInput }

func (s *sender) Post(_ context.Context, in conv.PostInput) (conv.Posted, error) {
	s.posts = append(s.posts, in)
	return conv.Posted{}, nil
}

type notifier struct{ messages []nm.EnqueueInput }

func (n *notifier) Enqueue(_ context.Context, in nm.EnqueueInput) ([]nm.Notification, error) {
	n.messages = append(n.messages, in)
	return nil, nil
}

const testSecret = "test-only-signing-secret-0123456789"

func TestSurveyCanonicalCSATAndNPS(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	send := &sender{}
	svc := su.New(sr.New(f.pool, f.q), send, testSecret, "https://app.example.test")
	v, err := svc.SaveTemplate(ctx, f.org, uuid.Nil, sm.TemplateInput{Name: "CSAT", Kind: "csat", Question: "How was support?", Enabled: true, OnResolve: true})
	if err != nil {
		t.Fatal(err)
	}
	template := v.(db.SurveyTemplate)
	if err = svc.Resolve(ctx, f.org, f.conv); err != nil {
		t.Fatal(err)
	}
	id, err := svc.Invite(ctx, f.org, sm.InviteInput{Template: template.Uuid, Conversation: f.convUUID})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Deliver(ctx, f.org, id); err != nil {
		t.Fatal(err)
	}
	if err = svc.Deliver(ctx, f.org, id); err != nil {
		t.Fatal(err)
	}
	if len(send.posts) != 2 || send.posts[0].ClientID != send.posts[1].ClientID {
		t.Fatal("delivery retries must share message identity")
	}
	token := strings.Split(send.posts[0].Content, "/survey/")[1]
	if _, err = svc.Public(ctx, token+"x"); !errors.Is(err, sm.ErrInvalid) {
		t.Fatalf("tampered token accepted: %v", err)
	}
	if _, err = svc.Answer(ctx, token, sm.Answer{Score: 0}); !errors.Is(err, sm.ErrInvalid) {
		t.Fatalf("invalid CSAT: %v", err)
	}
	a, err := svc.Answer(ctx, token, sm.Answer{Score: 5, Comment: "Great"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Score != 5 {
		t.Fatal(a)
	}
	a, err = svc.Answer(ctx, token, sm.Answer{Score: 1})
	if err != nil || a.Score != 5 {
		t.Fatalf("CSAT overwritten: %#v %v", a, err)
	}
	rating, err := f.q.GetConversationRating(ctx, db.GetConversationRatingParams{OrganizationID: f.org, ConversationID: f.conv})
	if err != nil || rating.Score != 5 {
		t.Fatalf("widget canonical row absent: %#v %v", rating, err)
	}
	// Simulate a widget answer: invitation itself is unmarked, canonical rating exists.
	if _, err = f.pool.Exec(ctx, "UPDATE survey_invitations SET answered_at=NULL WHERE organization_id=$1 AND uuid=$2", f.org, id); err != nil {
		t.Fatal(err)
	}
	public, err := svc.Public(ctx, token)
	if err != nil || !public.(map[string]any)["answered"].(bool) {
		t.Fatalf("widget answer was not visible: %#v %v", public, err)
	}
	before := len(send.posts)
	if err = svc.Deliver(ctx, f.org, id); err != nil || len(send.posts) != before {
		t.Fatalf("duplicate survey prompt after widget rating: %v", err)
	}
	if _, err = svc.Invite(ctx, f.other, sm.InviteInput{Template: template.Uuid, Conversation: f.convUUID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-org invite: %v", err)
	}
	v, err = svc.SaveTemplate(ctx, f.org, uuid.Nil, sm.TemplateInput{Name: "NPS", Kind: "nps", Question: "Recommend us?", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	nps := v.(db.SurveyTemplate)
	npsID, err := svc.Invite(ctx, f.org, sm.InviteInput{Template: nps.Uuid, Conversation: f.convUUID, PeriodKey: "2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Deliver(ctx, f.org, npsID); err != nil {
		t.Fatal(err)
	}
	npsToken := strings.Split(send.posts[len(send.posts)-1].Content, "/survey/")[1]
	if _, err = svc.Answer(ctx, npsToken, sm.Answer{Score: 10}); err != nil {
		t.Fatal(err)
	}
	raw, err := svc.Results(ctx, f.org, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var results []map[string]any
	if err = json.Unmarshal(raw, &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("CSAT/NPS results: %s", raw)
	}
	for _, row := range results {
		if row["kind"] == "nps" && row["nps"] != float64(100) {
			t.Fatalf("NPS calculation: %s", raw)
		}
	}
	if _, err = f.pool.Exec(ctx, "UPDATE survey_invitations SET expires_at=now()-interval '1 day' WHERE organization_id=$1 AND uuid=$2", f.org, npsID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Answer(ctx, npsToken, sm.Answer{Score: 9}); !errors.Is(err, sm.ErrExpired) {
		t.Fatalf("expired row accepted: %v", err)
	}
}

func TestStatusVisibilityUptimeAndBulkNotification(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()
	send := &sender{}
	notifications := &notifier{}
	svc := tu.New(tr.New(f.pool, f.q), send, notifications, testSecret, "https://app.example.test")
	slug := "status-" + uuid.NewString()[:12]
	_, err := svc.SavePage(ctx, f.org, tm.PageInput{Slug: slug, Title: "Status", Published: true})
	if err != nil {
		t.Fatal(err)
	}
	component, err := svc.SaveComponent(ctx, f.org, uuid.Nil, tm.ComponentInput{Name: "API", Group: "Core"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(-time.Hour)
	incident, err := svc.CreateIncident(ctx, f.org, tm.IncidentInput{Title: "Outage", Kind: "incident", Severity: "major_outage", Public: true, StartedAt: start, Components: []uuid.UUID{component.Uuid}, Body: "Investigating", NotificationBody: "Your issue is fixed"})
	if err != nil {
		t.Fatal(err)
	}
	private, err := svc.CreateIncident(ctx, f.org, tm.IncidentInput{Title: "Private", Severity: "major_outage", StartedAt: start, Components: []uuid.UUID{component.Uuid}, Body: "Internal"})
	if err != nil {
		t.Fatal(err)
	}
	_ = private
	page, err := svc.Public(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	var components []map[string]any
	if err = json.Unmarshal(page["components"].(json.RawMessage), &components); err != nil {
		t.Fatal(err)
	}
	if components[0]["status"] != "major_outage" {
		t.Fatal(components)
	}
	var incidents []map[string]any
	if err = json.Unmarshal(page["incidents"].(json.RawMessage), &incidents); err != nil {
		t.Fatal(err)
	}
	if len(incidents) != 1 {
		t.Fatalf("private incident leaked: %#v", incidents)
	}
	if _, err = svc.Update(ctx, f.other, incident.Uuid, tm.UpdateInput{Status: "resolved", Body: "Fixed"}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-org mutation: %v", err)
	}
	// A second public incident with the same interval must not double the downtime.
	second, err := svc.CreateIncident(ctx, f.org, tm.IncidentInput{Title: "Overlap", Severity: "partial_outage", Public: true, StartedAt: start, Components: []uuid.UUID{component.Uuid}, Body: "Investigating"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.History(ctx, slug, 1)
	if err != nil {
		t.Fatal(err)
	}
	var uptime []map[string]any
	if err = json.Unmarshal(history, &uptime); err != nil {
		t.Fatal(err)
	}
	if len(uptime) != 1 || !strings.HasSuffix(uptime[0]["day"].(string), "Z") {
		t.Fatalf("missing current day: %s", history)
	}
	now := time.Now().UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	outageStart := start
	if outageStart.Before(day) {
		outageStart = day
	}
	expected := 100 * (1 - now.Sub(outageStart).Seconds()/now.Sub(day).Seconds())
	actual := uptime[0]["uptime_percent"].(float64)
	if actual < expected-0.01 || actual > expected+0.01 {
		t.Fatalf("overlapping downtime counted twice: %s expected %v", history, expected)
	}
	var instance int64
	if err = f.pool.QueryRow(ctx, "INSERT INTO channel_instances(organization_id,channel_type,name) VALUES($1,'email','Mail') RETURNING id", f.org).Scan(&instance); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, "UPDATE conversations SET channel_instance_id=$1 WHERE organization_id=$2 AND id=$3", instance, f.org, f.conv); err != nil {
		t.Fatal(err)
	}
	if err = svc.Link(ctx, f.org, incident.Uuid, f.convUUID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,channel_instance_id) VALUES($1,2,'email',$2,$3)", f.org, f.contact, instance); err != nil {
		t.Fatal(err)
	}
	var duplicate uuid.UUID
	if err = f.pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE organization_id=$1 AND number=2", f.org).Scan(&duplicate); err != nil {
		t.Fatal(err)
	}
	if err = svc.Link(ctx, f.org, incident.Uuid, duplicate); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.Preview(ctx, f.org, incident.Uuid)
	if err != nil || preview["contacts"] != float64(1) {
		t.Fatalf("contacts not deduplicated: %#v %v", preview, err)
	}
	if _, err = svc.Update(ctx, f.org, incident.Uuid, tm.UpdateInput{Status: "resolved", Body: "Fixed"}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Update(ctx, f.org, second.Uuid, tm.UpdateInput{Status: "resolved", Body: "Fixed"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		// Move the other conversation above the frozen destination between retries.
		if i == 1 {
			if _, err = f.pool.Exec(ctx, "UPDATE conversations SET last_message_at=now()+interval '1 hour' WHERE organization_id=$1 AND uuid=$2", f.org, f.convUUID); err != nil {
				t.Fatal(err)
			}
		}
		if err = svc.NotifyContacts(ctx, f.org, incident.Uuid); err != nil {
			t.Fatal(err)
		}
	}
	if len(send.posts) != 2 || send.posts[0].ClientID != send.posts[1].ClientID || send.posts[0].ConversationUUID != send.posts[1].ConversationUUID {
		t.Fatal("bulk delivery replay changed recipient identity or destination")
	}
	if err = svc.Subscribe(ctx, slug, "customer@example.test"); err != nil {
		t.Fatal(err)
	}
	var sub uuid.UUID
	if err = f.pool.QueryRow(ctx, "SELECT uuid FROM status_subscriptions WHERE organization_id=$1", f.org).Scan(&sub); err != nil {
		t.Fatal(err)
	}
	if err = svc.SendConfirmation(ctx, f.org, sub); err != nil {
		t.Fatal(err)
	}
	if len(notifications.messages) != 1 {
		t.Fatal("missing confirmation")
	}
	link := strings.Split(notifications.messages[0].Body, "\n")
	token := link[len(link)-1]
	token = token[strings.LastIndex(token, "/")+1:]
	if !strings.Contains(link[len(link)-1], "/status/"+slug+"/subscription/") {
		t.Fatalf("e-mail must link to the confirmation page, not the API: %q", link[len(link)-1])
	}
	if err = svc.SubscriptionAction(ctx, slug, token+"x"); !errors.Is(err, tm.ErrInvalid) {
		t.Fatalf("tampered confirmation accepted: %v", err)
	}
	// Opening the link (preview, GET) changes nothing: link scanners and
	// previews cannot confirm a subscription (CF-157).
	sp, err := svc.PreviewSubscription(ctx, slug, token)
	if err != nil || sp.Action != "confirm" || sp.PageTitle == "" {
		t.Fatalf("preview %+v %v", sp, err)
	}
	if err = svc.NotifySubscribers(ctx, f.org, incident.Uuid, "Investigating"); err != nil {
		t.Fatal(err)
	}
	if len(notifications.messages) != 1 {
		t.Fatal("an unconfirmed subscriber was notified after a preview")
	}
	if err = svc.SubscriptionAction(ctx, slug, token); err != nil {
		t.Fatal(err)
	}
	if err = svc.NotifySubscribers(ctx, f.org, incident.Uuid, "Resolved"); err != nil {
		t.Fatal(err)
	}
	if len(notifications.messages) != 2 {
		t.Fatal("confirmed subscriber not notified")
	}
	unsub := strings.Split(notifications.messages[1].Body, "\n")
	unsubToken := unsub[len(unsub)-1][strings.LastIndex(unsub[len(unsub)-1], "/")+1:]
	if p, err := svc.PreviewSubscription(ctx, slug, unsubToken); err != nil || p.Action != "unsubscribe" {
		t.Fatalf("unsubscribe preview %+v %v", p, err)
	}
	_, err = svc.SavePage(ctx, f.org, tm.PageInput{Slug: slug, Title: "Status", Published: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Public(ctx, slug); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unpublished page visible: %v", err)
	}
}
