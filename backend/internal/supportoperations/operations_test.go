package supportoperations_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	aiusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	billingmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	billingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/enterprise"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/feedback"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/quality"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/schedules"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teamchat"
	widgetusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	widgetplatform "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fixture struct {
	ctx                                        context.Context
	pool                                       *pgxpool.Pool
	repo                                       *repository.Repository
	org, other, actor, peer, outsider, contact int64
	user, peerUUID, outsiderUUID, conv         uuid.UUID
	orgUUID                                    uuid.UUID
}

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := &fixture{pool: pool, repo: repository.New(pool, db.New(pool))}
	ctx := context.Background()
	f.org = f.id(t, "INSERT INTO organizations(slug,name) VALUES($1,'C2C fixture') RETURNING id", uuid.NewString())
	f.other = f.id(t, "INSERT INTO organizations(slug,name) VALUES($1,'Other C2C fixture') RETURNING id", uuid.NewString())
	for i, dst := range []*int64{&f.actor, &f.peer, &f.outsider} {
		*dst = f.id(t, "INSERT INTO users(email,password_hash,name,surname,status) VALUES($1,'x','C2C','Agent','active') RETURNING id", fmt.Sprintf("%d-%s@example.test", i, uuid.NewString()))
		f.exec(t, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE scope='tenant' AND organization_id IS NULL AND slug='agent'", f.org, *dst)
	}
	f.scanUUID(t, "users", f.actor, &f.user)
	f.scanUUID(t, "users", f.peer, &f.peerUUID)
	f.scanUUID(t, "users", f.outsider, &f.outsiderUUID)
	f.scanUUID(t, "organizations", f.org, &f.orgUUID)
	f.contact = f.id(t, "INSERT INTO contacts(organization_id,name,email) VALUES($1,'Contact','contact@example.test') RETURNING id", f.org)
	conv := f.id(t, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,status,assignee_user_id) VALUES($1,1,'test',$2,'resolved',$3) RETURNING id", f.org, f.contact, f.actor)
	f.scanUUID(t, "conversations", conv, &f.conv)
	f.exec(t, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,content,status) VALUES($1,$2,'in','contact','Please add a dark theme','received')", f.org, conv)
	f.ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: f.org, UUID: f.orgUUID, Permissions: []string{"tenant.conversations.read", "tenant.conversations.read_all"}})
	f.ctx = authctx.WithPrincipal(f.ctx, authctx.Principal{UserInternal: f.actor, UserID: f.user})
	t.Cleanup(func() { // New tables have no production data in these random organizations.
		for _, table := range []string{"roadmap_entries", "enterprise_quotes", "qa_reviews", "qa_scorecards", "internal_messages", "internal_thread_members", "internal_threads", "agent_time_off", "agent_shifts", "shift_templates", "schedule_settings", "changelog_entries", "feedback_comments", "feature_sources", "feature_votes"} {
			_, _ = pool.Exec(ctx, "DELETE FROM "+table+" WHERE organization_id=$1", f.org)
		}
		_, _ = pool.Exec(ctx, "UPDATE feature_requests SET status='reviewing',merged_into_id=NULL WHERE organization_id=$1", f.org)
		_, _ = pool.Exec(ctx, "DELETE FROM feature_requests WHERE organization_id=$1", f.org)
		// Billing append-only history intentionally prevents deleting converted subscriptions.
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id IN ($1,$2)", f.org, f.other)
	})
	return f
}
func (f *fixture) id(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
func (f *fixture) scanUUID(t *testing.T, table string, id int64, dst *uuid.UUID) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(), "SELECT uuid FROM "+table+" WHERE id=$1", id).Scan(dst); err != nil {
		t.Fatal(err)
	}
}
func TestFeedbackMergeIsolationAndPublication(t *testing.T) {
	f := setup(t)
	s := &feedback.Service{Repo: f.repo}
	a, err := s.Create(f.ctx, f.org, f.actor, feedback.RequestInput{Title: "Theme", Conversation: &f.conv})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(f.ctx, f.org, f.actor, feedback.RequestInput{Title: "Dark mode", Public: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{a.ID, b.ID} {
		if err = f.repo.Q.FeedbackVote(f.ctx, db.FeedbackVoteParams{OrganizationID: f.org, RequestID: id, ContactID: f.contact}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Merge(f.ctx, f.other, f.actor, a.Uuid, b.Uuid); !errors.Is(err, billingmodel.ErrNotFound) {
		t.Fatalf("cross org: %v", err)
	}
	if err = s.Merge(f.ctx, f.org, f.actor, a.Uuid, b.Uuid); err != nil {
		t.Fatal(err)
	}
	voters, err := f.repo.Q.FeedbackVoters(f.ctx, db.FeedbackVotersParams{OrganizationID: f.org, RequestID: b.ID})
	if err != nil || len(voters) != 1 {
		t.Fatalf("dedup votes: %+v %v", voters, err)
	}
	if err = s.Merge(f.ctx, f.org, f.actor, b.Uuid, a.Uuid); !errors.Is(err, supportops.ErrConflict) {
		t.Fatalf("merge cycle: %v", err)
	}
	if _, err = s.Status(f.ctx, f.org, f.actor, b.Uuid, "released", true); err != nil {
		t.Fatal(err)
	}
	entry, err := s.Changelog(f.ctx, f.org, feedback.ChangelogInput{Title: "Theme shipped", Body: "**Dark theme** is available", Request: &b.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(f.ctx, f.org, f.actor, entry.Uuid); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	feedback.RegisterRoutes(supportops.Router{Mux: mux}, s, nil)
	req := httptest.NewRequest("GET", "/v1/public/changelog/"+f.orgUUID.String(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("private board: %d %s", rec.Code, rec.Body.String())
	}
	if err = f.repo.Q.FeedbackSaveSettings(f.ctx, db.FeedbackSaveSettingsParams{OrganizationID: f.org, PublicEnabled: true}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Theme shipped") {
		t.Fatalf("public changelog: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/public/changelog/"+f.orgUUID.String()+"/rss", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<rss") {
		t.Fatalf("rss %s", rec.Body.String())
	}
}
func TestScheduleBoundariesOverlapAndTimeOff(t *testing.T) {
	f := setup(t)
	s := &schedules.Service{Repo: f.repo}
	a := time.Now().UTC().Truncate(time.Second)
	b := a.Add(8 * time.Hour)
	in := schedules.ShiftInput{User: f.user, Starts: a, Ends: b, Timezone: "Europe/Istanbul"}
	if _, err := s.Shift(f.ctx, f.org, f.actor, in); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Shift(f.ctx, f.org, f.actor, in); !errors.Is(err, supportops.ErrConflict) {
		t.Fatalf("overlap %v", err)
	}
	if err := f.repo.Q.ScheduleSaveSettings(f.ctx, db.ScheduleSaveSettingsParams{OrganizationID: f.org, EnforceShifts: true}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at   time.Time
		want bool
	}{{a, true}, {b, false}, {a.Add(-time.Second), false}} {
		got, err := f.repo.Q.ScheduleAvailable(f.ctx, db.ScheduleAvailableParams{OrganizationID: f.org, UserID: f.actor, StartsAt: repository.TS(tc.at)})
		if err != nil || got != tc.want {
			t.Fatalf("availability %s %v %v", tc.at, got, err)
		}
	}
	leave, err := s.Leave(f.ctx, f.org, f.actor, schedules.LeaveInput{Starts: a, Ends: b, Reason: "Vacation"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Review(f.ctx, f.org, f.actor, leave.Uuid, "approved"); !errors.Is(err, billingmodel.ErrNotFound) {
		t.Fatalf("self approval %v", err)
	}
	if _, err = s.Review(f.ctx, f.org, f.peer, leave.Uuid, "approved"); err != nil {
		t.Fatal(err)
	}
	got, err := f.repo.Q.ScheduleAvailable(f.ctx, db.ScheduleAvailableParams{OrganizationID: f.org, UserID: f.actor, StartsAt: repository.TS(a)})
	if err != nil || got {
		t.Fatalf("leave availability %v %v", got, err)
	}
}
func TestInternalChatParticipantsMentionsAndRetries(t *testing.T) {
	f := setup(t)
	s := &teamchat.Service{Repo: f.repo}
	in := teamchat.ThreadInput{Kind: "direct", Members: []uuid.UUID{f.peerUUID}}
	thread, err := s.Create(f.ctx, f.org, f.actor, in)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(f.ctx, f.org, f.peer, teamchat.ThreadInput{Kind: "direct", Members: []uuid.UUID{f.user}})
	if err != nil || other.Uuid != thread.Uuid {
		t.Fatalf("direct dedup %+v %v", other, err)
	}
	if _, err = s.Thread(f.ctx, f.org, f.outsider, thread.Uuid); err == nil {
		t.Fatal("nonparticipant read thread")
	}
	post := teamchat.PostInput{Body: "@peer can you help?", Client: uuid.New(), Mentions: []uuid.UUID{f.peerUUID}}
	first, err := s.Post(f.ctx, f.org, f.actor, thread.Uuid, post, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Post(f.ctx, f.org, f.actor, thread.Uuid, post, false)
	if err != nil || second.Uuid != first.Uuid {
		t.Fatalf("idempotent post: %v", err)
	}
	post.Client = uuid.New()
	post.Mentions = []uuid.UUID{f.outsiderUUID}
	if _, err = s.Post(f.ctx, f.org, f.actor, thread.Uuid, post, false); !errors.Is(err, supportops.ErrInvalid) {
		t.Fatalf("external mention %v", err)
	}
}

type fakeAI struct {
	calls atomic.Int32
	text  string
}

func (f *fakeAI) RunAgent(_ context.Context, _, _ int64, _ aimodel.RunInput, _ aiusecase.AgentOptions) (aiusecase.RunResult, error) {
	f.calls.Add(1)
	return aiusecase.RunResult{UUID: uuid.New(), Result: ai.Result{Text: f.text}}, nil
}
func TestQualityManualReviewAndUntrustedAI(t *testing.T) {
	f := setup(t)
	provider := &fakeAI{text: `{"scores":[{"key":"accuracy","value":101,"rationale":"invalid"}]}`}
	s := &quality.Service{Repo: f.repo, AI: provider}
	card, err := s.Card(f.ctx, f.org, quality.CardInput{Name: "Support", Criteria: []quality.Criterion{{Key: "accuracy", Label: "Accuracy", Weight: 100, Critical: true}}, Threshold: 70})
	if err != nil {
		t.Fatal(err)
	}
	in := quality.ReviewInput{Scorecard: card.Uuid, Conversation: f.conv, Scores: []quality.Score{{Key: "accuracy", Value: 0, Rationale: "Incorrect answer"}}}
	review, err := s.Review(f.ctx, f.org, f.actor, in, false)
	if err != nil || !review.CriticalFailure || !review.NeedsReview {
		t.Fatalf("critical review %+v %v", review, err)
	}
	if _, err = s.Review(f.ctx, f.org, f.actor, in, true); !errors.Is(err, supportops.ErrInvalid) {
		t.Fatalf("AI output validation %v", err)
	}
	f.exec(t, "UPDATE conversations SET status='open' WHERE organization_id=$1", f.org)
	if _, err = s.Review(f.ctx, f.org, f.actor, in, true); !errors.Is(err, supportops.ErrConflict) {
		t.Fatalf("unresolved AI %v", err)
	}
	if provider.calls.Load() != 1 {
		t.Fatal("unresolved conversation called provider")
	}
}
func TestEnterpriseQuoteDecisionConversionAndRevenueSnapshots(t *testing.T) {
	f := setup(t)
	ctx := f.ctx
	plan, err := f.repo.Q.BillingCreatePlan(ctx, db.BillingCreatePlanParams{Code: "c2c-" + uuid.NewString(), Name: "Enterprise base", Visibility: "hidden"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := f.repo.Q.BillingCreateVersion(ctx, db.BillingCreateVersionParams{PlanID: plan.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.Q.BillingPublishVersion(ctx, version.ID); err != nil {
		t.Fatal(err)
	}
	f.exec(t, "INSERT INTO billing_profiles(organization_id,legal_name,email) VALUES($1,'Enterprise legal','billing@example.test')", f.org)
	billing := billingusecase.New(f.repo)
	profile := billingusecase.ProfileInput{Kind: "company", LegalName: "Fixture Teknoloji AŞ", TaxID: "1234567890", TaxOffice: "Konak", CountryCode: "TR", Address: billingusecase.BillingAddress{Street: "Test Caddesi 1", District: "Konak", City: "İzmir", PostalCode: "35000"}, Emails: []string{"billing@example.test"}}
	if _, err = billing.SaveProfile(ctx, f.org, profile); err != nil {
		t.Fatal(err)
	}
	if err = billing.ConfigureInvoicing(&billingusecase.FakeEInvoiceProvider{}, profile, "CFX", "test"); err != nil {
		t.Fatal(err)
	}
	svc := &enterprise.Service{Repo: f.repo, Billing: billing}
	quote, err := svc.Create(ctx, f.actor, enterprise.Input{Organization: f.orgUUID, BaseVersion: version.Uuid, Title: "Annual contract", Currency: "TRY", Interval: "yearly", Lines: []enterprise.Line{{Description: "Platform", Quantity: 1, UnitAmount: 120000}}, VAT: 2000, ContractMonths: 12, PaymentTerms: 30, PO: "PO-001", ValidUntil: time.Now().Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Decide(ctx, quote.ShareToken, true, "127.0.0.1"); err == nil {
		t.Fatal("draft accepted")
	}
	if _, err = f.repo.Q.QuoteSend(ctx, db.QuoteSendParams{OrganizationID: f.org, Uuid: quote.Uuid}); err != nil {
		t.Fatal(err)
	}
	accepted, err := svc.Decide(ctx, quote.ShareToken, true, "127.0.0.1")
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("accept %+v %v", accepted, err)
	}
	if _, err = svc.Decide(ctx, quote.ShareToken, false, "127.0.0.1"); err == nil {
		t.Fatal("second decision accepted")
	}
	converted, err := svc.Convert(ctx, f.org, f.actor, quote.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.Convert(ctx, f.org, f.actor, quote.Uuid)
	if err != nil || again.SubscriptionID != converted.SubscriptionID {
		t.Fatalf("conversion retry %v", err)
	}
	sub, err := f.repo.Q.BillingGetSubscription(ctx, f.org)
	if err != nil || sub.Status != "active" {
		t.Fatalf("net30 active %+v %v", sub, err)
	}
	invoice, err := f.repo.Q.BillingInvoiceKey(ctx, db.BillingInvoiceKeyParams{OrganizationID: f.org, IdempotencyKey: "enterprise:" + quote.Uuid.String()})
	if err != nil {
		t.Fatal(err)
	}
	if !invoice.DueAt.Time.Equal(sub.PeriodStart.Time.AddDate(0, 0, 30)) {
		t.Fatal("net30 due date not applied")
	}
	var mrr string
	if err = f.pool.QueryRow(ctx, "SELECT mrr_minor::text FROM billing_revenue_snapshots WHERE organization_id=$1 ORDER BY id DESC LIMIT 1", f.org).Scan(&mrr); err != nil || mrr != "10000.0000" {
		t.Fatalf("annual MRR %s %v", mrr, err)
	}
	b, err := enterprise.PDF(quote)
	if err != nil || !strings.HasPrefix(string(b), "%PDF") {
		t.Fatalf("PDF %v", err)
	}

	// A quote scheduled onto an existing subscription must not issue an extra bill
	// when conversion is retried after renewal has applied its pending version.
	q2, err := svc.Create(ctx, f.actor, enterprise.Input{Organization: f.orgUUID, BaseVersion: version.Uuid, Title: "Next period", Currency: "TRY", Interval: "yearly", Lines: []enterprise.Line{{Description: "Platform", Quantity: 1, UnitAmount: 240000}}, VAT: 2000, ContractMonths: 12, PaymentTerms: 30, ValidUntil: time.Now().Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.Q.QuoteSend(ctx, db.QuoteSendParams{OrganizationID: f.org, Uuid: q2.Uuid}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Decide(ctx, q2.ShareToken, true, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	scheduled, err := svc.Convert(ctx, f.org, f.actor, q2.Uuid)
	if err != nil || scheduled.RequiresInitialInvoice {
		t.Fatalf("scheduled invoice flag: %v", err)
	}
	f.exec(t, "UPDATE billing_subscriptions SET plan_version_id=pending_plan_version_id,price_id=pending_price_id,pending_plan_version_id=NULL,pending_price_id=NULL,period_start=period_end,period_end=period_end+interval '1 year' WHERE organization_id=$1 AND id=$2", f.org, sub.ID)
	if _, err = svc.Convert(ctx, f.org, f.actor, q2.Uuid); err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.Q.BillingInvoiceKey(ctx, db.BillingInvoiceKeyParams{OrganizationID: f.org, IdempotencyKey: "enterprise:" + q2.Uuid.String()}); err == nil {
		t.Fatal("scheduled conversion retry created an extra invoice")
	}
	public, _ := json.Marshal(enterprise.PublicView(quote))
	if strings.Contains(string(public), "share_token") || strings.Contains(string(public), "decision_ip") {
		t.Fatal("public quote exposes private fields")
	}
}
func TestQualityRejectsMissingDuplicateCriteriaAndOverflow(t *testing.T) {
	criteria := []quality.Criterion{{Key: "a", Weight: 50}, {Key: "b", Weight: 50, Critical: true}}
	for _, scores := range [][]quality.Score{{{Key: "a", Value: 100, Rationale: "ok"}}, {{Key: "a", Value: 100, Rationale: "ok"}, {Key: "a", Value: 90, Rationale: "ok"}}, {{Key: "a", Value: -1, Rationale: "bad"}, {Key: "b", Value: 100, Rationale: "ok"}}} {
		if _, _, err := quality.Calculate(criteria, scores); err == nil {
			t.Fatal("invalid scores accepted")
		}
	}
	score, critical, err := quality.Calculate(criteria, []quality.Score{{Key: "a", Value: 100, Rationale: "ok"}, {Key: "b", Value: 0, Rationale: "critical"}})
	if err != nil || score != 0 || !critical {
		t.Fatalf("critical rule %v %v %v", score, critical, err)
	}
	if _, err = enterprise.Total([]enterprise.Line{{Description: "Overflow", Quantity: 100001, UnitAmount: 1000000000000}}); err == nil {
		t.Fatal("quote overflow accepted")
	}
}

type widgetSource struct{ instance channels.Instance }

func (w widgetSource) ForWebhook(_ context.Context, _ string, id uuid.UUID) (channels.Instance, channels.Adapter, error) {
	if id != w.instance.UUID {
		return channels.Instance{}, nil, errors.New("unknown widget")
	}
	return w.instance, nil, nil
}
func TestPublicVotesRequireVerifiedOrganizationBoundIdentity(t *testing.T) {
	f := setup(t)
	s := &feedback.Service{Repo: f.repo}
	request, err := s.Create(f.ctx, f.org, f.actor, feedback.RequestInput{Title: "Public request", Public: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.repo.Q.FeedbackSaveSettings(f.ctx, db.FeedbackSaveSettingsParams{OrganizationID: f.org, PublicEnabled: true}); err != nil {
		t.Fatal(err)
	}
	instanceID := f.id(t, "INSERT INTO channel_instances(organization_id,channel_type,name) VALUES($1,'widget','Feedback widget') RETURNING id", f.org)
	var widgetID uuid.UUID
	f.scanUUID(t, "channel_instances", instanceID, &widgetID)
	f.exec(t, "INSERT INTO contact_identities(organization_id,contact_id,channel_type,channel_instance_id,external_id) VALUES($1,$2,'widget',$3,'user:customer-123')", f.org, f.contact, instanceID)
	signer := widgetplatform.NewSigner("test-only-c2c-widget-signing-key")
	widget := widgetusecase.New(widgetusecase.Deps{Signer: signer, Instances: widgetSource{instance: channels.Instance{ID: instanceID, UUID: widgetID, OrganizationID: f.org, Type: "widget", Status: "active"}}, AppOrigin: "https://customer.example.test"})
	mux := http.NewServeMux()
	feedback.RegisterRoutes(supportops.Router{Mux: mux}, s, widget)
	post := func(token string) int {
		body := fmt.Sprintf(`{"widget_uuid":%q}`, widgetID.String())
		r := httptest.NewRequest("POST", "/v1/public/feedback/"+f.orgUUID.String()+"/"+request.Uuid.String()+"/votes", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", "https://customer.example.test")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code
	}
	anonymous, _ := signer.Issue(widgetplatform.Claims{Instance: widgetID, Visitor: "visitor"})
	if code := post(anonymous); code != 403 {
		t.Fatalf("anonymous vote: %d", code)
	}
	valid, _ := signer.Issue(widgetplatform.Claims{Instance: widgetID, Visitor: "visitor", User: "customer-123"})
	for i := 0; i < 2; i++ {
		if code := post(valid); code != 200 {
			t.Fatalf("verified vote: %d", code)
		}
	}
	wrong, _ := signer.Issue(widgetplatform.Claims{Instance: uuid.New(), Visitor: "visitor", User: "customer-123"})
	if code := post(wrong); code != 403 {
		t.Fatalf("cross widget vote: %d", code)
	}
	voters, err := f.repo.Q.FeedbackVoters(f.ctx, db.FeedbackVotersParams{OrganizationID: f.org, RequestID: request.ID})
	if err != nil || len(voters) != 1 {
		t.Fatalf("vote dedup: %d %v", len(voters), err)
	}
	entry, err := s.Roadmap(f.ctx, f.org, uuid.Nil, feedback.RoadmapInput{Title: "Engineering item", Body: "Public release details", Status: "planned", SourceURL: "https://issues.example.test/CF-42", Public: true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/v1/public/roadmap/"+f.orgUUID.String(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), entry.Title) {
		t.Fatalf("issue roadmap: %s", w.Body.String())
	}
}

func TestJSONBRubricsAndQuoteLinesRemainStructuredAtHTTPBoundary(t *testing.T) {
	cases := []struct {
		v     any
		field string
	}{{quality.CardView(db.QaScorecard{Criteria: []byte(`[{"key":"accuracy","label":"Doğruluk","weight":100}]`)}), "criteria"}, {quality.ReviewView(db.QaReview{Rubric: []byte(`[]`), Scores: []byte(`[{"key":"accuracy","value":90,"rationale":"Verified"}]`)}), "scores"}, {enterprise.PrivateView(db.EnterpriseQuote{Lines: []byte(`[{"description":"Seats","quantity":2,"unit_amount_minor":1000}]`), Grants: []byte(`[]`)}), "lines"}}
	for _, tc := range cases {
		raw, err := json.Marshal(tc.v)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err = json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		if _, ok := out[tc.field].([]any); !ok {
			t.Fatalf("%s became %T instead of JSON array", tc.field, out[tc.field])
		}
	}
}
