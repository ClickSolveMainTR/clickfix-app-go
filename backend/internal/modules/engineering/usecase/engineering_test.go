package usecase

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/inbox"
	channelsuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/repository"
	slauc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	testchannel "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/testchannel"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testEnv struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	svc       *Service
	ib        *inbox.Inbox
	org, user int64
	instance  channels.Instance
	queued    []int64
}
type sendQueue struct{ env *testEnv }

func (q sendQueue) EnqueueSend(_ context.Context, _ int64, message int64, _ time.Duration) error {
	q.env.queued = append(q.env.queued, message)
	return nil
}
func setup(t *testing.T) *testEnv {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	e := &testEnv{pool: pool, q: db.New(pool)}
	if err = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Engineering test') RETURNING id", "eng-"+uuid.NewString()).Scan(&e.org); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname,status) VALUES($1,'x','Test','Agent','active') RETURNING id", uuid.NewString()+"@example.test").Scan(&e.user); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id=$1", e.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id=$1", e.user)
	})
	if _, err = pool.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE scope='tenant' AND organization_id IS NULL AND slug='member'", e.org, e.user); err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	cfg.App.Env = "test"
	cfg.Encryption.Key = "0123456789abcdef0123456789abcdef"
	e.ib, err = inbox.New(inbox.Deps{Config: cfg, DB: pool, Queries: e.q})
	if err != nil {
		t.Fatal(err)
	}
	e.ib.Conversations.WithQueue(sendQueue{e})
	instance, err := e.ib.Channels.Create(ctx, channelsuc.CreateInput{OrganizationID: e.org, ActorID: e.user, Type: channels.TypeTest, Name: "Test", Credentials: channels.Credentials{testchannel.SecretField: "test-engineering-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	row, lookupErr := e.q.GetChannelInstanceByPublicUUID(ctx, instance.UUID)
	if lookupErr != nil {
		t.Fatal(lookupErr)
	}
	e.instance, _, err = e.ib.Channels.Instance(ctx, e.org, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	box, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		t.Fatal(err)
	}
	e.svc = New(repository.New(pool, e.q), e.ib.Conversations, box)
	return e
}
func (e *testEnv) conversation(t *testing.T, name string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	_, err := e.ib.Conversations.Receive(ctx, e.instance, channels.WebhookResult{Messages: []channels.InboundMessage{{ExternalID: uuid.NewString(), From: channels.Sender{ExternalID: name, Name: name}, Text: "Broken export " + name, At: time.Now()}}})
	if err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err = e.pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE organization_id=$1 ORDER BY id DESC LIMIT 1", e.org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestNativeTrackerIsolationResolutionAndOptOut(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	c1 := e.conversation(t, "one")
	c2 := e.conversation(t, "two")
	c3 := e.conversation(t, "three")
	issue, err := e.svc.Create(ctx, e.org, e.user, model.CreateInput{Conversation: &c1})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Title == "" || issue.UUID == uuid.Nil || issue.Revision != 1 {
		t.Fatalf("bad issue %+v", issue)
	}
	for _, c := range []uuid.UUID{c2, c3} {
		if err = e.svc.Link(ctx, e.org, e.user, issue.UUID, model.LinkInput{Conversation: c, NotifyCustomer: true}, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = e.svc.Get(ctx, e.org+999999, issue.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org get: %v", err)
	}
	if _, err = e.svc.Create(ctx, e.org+999999, e.user, model.CreateInput{Conversation: &c1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org conversation: %v", err)
	}
	status := "done"
	if _, err = e.svc.Patch(ctx, e.org, e.user, issue.UUID, model.PatchInput{Revision: 9, Status: &status}); !errors.Is(err, ErrConflict) {
		t.Fatalf("revision: %v", err)
	}
	// Opt-out before resolution excludes this customer from the batch entirely.
	if _, err = e.pool.Exec(ctx, "UPDATE contacts SET custom_fields='{"+`"engineering_notifications":false`+"}' WHERE organization_id=$1 AND id=(SELECT contact_id FROM conversations WHERE uuid=$2)", e.org, c2); err != nil {
		t.Fatal(err)
	}
	next, err := e.svc.Patch(ctx, e.org, e.user, issue.UUID, model.PatchInput{Revision: 1, Status: &status})
	if err != nil {
		t.Fatal(err)
	}
	if next.ResolvedAt == nil {
		t.Fatal("missing resolved_at")
	}
	if err = e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if len(e.queued) != 2 {
		t.Fatalf("queued %v", e.queued)
	}
	// A customer opts out after the sweep but before the channel send.
	if _, err = e.pool.Exec(ctx, "UPDATE contacts SET custom_fields='{"+`"notifications_opt_out":true`+"}' WHERE organization_id=$1 AND id=(SELECT contact_id FROM conversations WHERE uuid=$2)", e.org, c3); err != nil {
		t.Fatal(err)
	}
	for _, message := range e.queued {
		if err = e.ib.Conversations.Deliver(ctx, e.org, message, conv.DeliverAttempt{MaxRetry: 3}); err != nil {
			t.Fatal(err)
		}
	}
	if sent := e.ib.Test.Sent(0); len(sent) != 1 {
		t.Fatalf("opt-out send count: %d", len(sent))
	}
	if err = e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if len(e.queued) != 2 {
		t.Fatal("duplicate follow-up")
	}
	restricted := access.WithViewer(ctx, access.Viewer{UserID: e.user + 1000, All: false})
	if _, err = e.pool.Exec(ctx, "UPDATE conversations SET assignee_user_id=$1 WHERE organization_id=$2 AND uuid=$3", e.user, e.org, c1); err != nil {
		t.Fatal(err)
	}
	links, err := e.svc.Links(restricted, e.org, e.user+1000, issue.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("hidden conversation leaked: %+v", links)
	}
	if _, err = e.svc.List(ctx, e.org, apiquery.Query{Limit: 20, Sort: []apiquery.SortField{{Field: "bad"}}}, "", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid sort %v", err)
	}
}
func TestReleaseAndEscalation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	c := e.conversation(t, "release")
	issue, err := e.svc.Create(ctx, e.org, e.user, model.CreateInput{Title: "Export fix", Conversation: &c})
	if err != nil {
		t.Fatal(err)
	}
	release, err := e.svc.CreateRelease(ctx, e.org, "1.2.3", "https://example.test/changelog")
	if err != nil {
		t.Fatal(err)
	}
	ref := release.Uuid.String()
	if _, err = e.svc.Patch(ctx, e.org, e.user, issue.UUID, model.PatchInput{Revision: 1, Release: &ref}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err = e.svc.PublishRelease(ctx, e.org, e.user, release.Uuid); err != nil {
			t.Fatal(err)
		}
	}
	if err = e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if len(e.queued) != 1 {
		t.Fatal("release publication must be idempotent")
	}
	var content string
	if err = e.pool.QueryRow(ctx, "SELECT content FROM messages WHERE id=$1", e.queued[0]).Scan(&content); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "1.2.3") {
		t.Fatal(content)
	}
	var team uuid.UUID
	if err = e.pool.QueryRow(ctx, "INSERT INTO teams(organization_id,name) VALUES($1,'Engineering') RETURNING uuid", e.org).Scan(&team); err != nil {
		t.Fatal(err)
	}
	in := model.EscalationInput{Tier: "engineering", Team: team, Reason: "Needs fix", SLASeconds: 3600}
	if _, err = e.svc.Escalate(ctx, e.org, conv.Actor{ID: e.user}, c, in); !errors.Is(err, ErrInvalid) {
		t.Fatalf("skipped tier: %v", err)
	}
	in.Tier = "l2"
	if _, err = e.svc.Escalate(ctx, e.org, conv.Actor{ID: e.user}, c, in); err != nil {
		t.Fatal(err)
	}
	in.Tier = "engineering"
	if _, err = e.svc.Escalate(ctx, e.org, conv.Actor{ID: e.user}, c, in); err != nil {
		t.Fatal(err)
	}
	conversation, err := e.q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: e.org, Uuid: c})
	if err != nil {
		t.Fatal(err)
	}
	sla := slauc.New(e.pool, e.q, nil, nil)
	if err = sla.Sync(ctx, e.org, conversation.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	var clocks int
	if err = e.pool.QueryRow(ctx, "SELECT count(*) FROM conversation_sla_metrics WHERE organization_id=$1 AND metric='resolution' AND due_at>now()", e.org).Scan(&clocks); err != nil {
		t.Fatal(err)
	}
	if clocks != 1 {
		t.Fatal("handoff SLA missing")
	}
}
func sign(secret string, raw []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}
func TestWebhookSignatures(t *testing.T) {
	now := time.Now()
	secret := "test-webhook-secret-123"
	raw := []byte(fmt.Sprintf(`{"webhookTimestamp":%d}`, now.UnixMilli()))
	for _, provider := range []string{"linear", "github", "jira"} {
		h := http.Header{}
		signature := sign(secret, raw)
		switch provider {
		case "linear":
			h.Set("Linear-Signature", signature)
		case "github":
			h.Set("X-Hub-Signature-256", "sha256="+signature)
		case "jira":
			h.Set("X-Hub-Signature", "sha256="+signature)
		}
		if err := VerifySignature(provider, secret, h, raw, now); err != nil {
			t.Fatal(err)
		}
		if err := VerifySignature(provider, secret, h, append(raw, ' '), now); !errors.Is(err, ErrSignature) {
			t.Fatal("accepted altered body")
		}
		if provider == "linear" && VerifySignature(provider, secret, h, raw, now.Add(2*time.Minute)) == nil {
			t.Fatal("accepted stale timestamp")
		}
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}
func TestProviderAdaptersUseRealContractsWithoutNetwork(t *testing.T) {
	for _, provider := range []string{"linear", "github", "jira"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int32
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Fatal("missing bearer token")
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				switch provider {
				case "linear":
					if r.URL.Host != "api.linear.app" || !bytes.Contains(raw, []byte("IssueCreateInput")) {
						t.Fatalf("bad Linear request: %s %s", r.URL, raw)
					}
					return fakeResponse(`{"data":{"issueCreate":{"success":true,"issue":{"id":"remote-1","identifier":"ENG-1","url":"https://linear.app/eng/issue/1","title":"Export"}}}}`), nil
				case "github":
					if r.URL.Path != "/repos/owner/repo/issues" {
						t.Fatal(r.URL)
					}
					return fakeResponse(`{"number":12,"html_url":"https://github.com/owner/repo/issues/12","title":"Export"}`), nil
				default:
					if r.URL.Path != "/ex/jira/cloud/rest/api/3/issue" || !bytes.Contains(raw, []byte(`"version":1`)) {
						t.Fatalf("Jira ADF missing: %s", raw)
					}
					return fakeResponse(`{"id":"123","key":"ENG-123"}`), nil
				}
			})}
			a := &providerAdapter{Provider: provider, Client: client}
			result, err := a.Create(context.Background(), Credentials{AccessToken: "test-token"}, IntegrationConfig{TeamID: "team", Repository: "owner/repo", CloudID: "cloud", ProjectID: "ENG", SiteURL: "https://example.atlassian.net"}, model.Issue{UUID: uuid.New(), Title: "Export", Description: "Broken", Labels: []string{}})
			if err != nil || result.ID == "" || calls.Load() != 1 {
				t.Fatalf("create %+v %v", result, err)
			}
		})
	}
}
func TestOAuthStateIsBoundSingleUseAndSecretsMasked(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	saved, err := e.svc.SaveIntegration(ctx, e.org, "linear", IntegrationInput{Enabled: true, Config: IntegrationConfig{TeamID: "t", RedirectURI: "https://example.test/callback"}, Credentials: &Credentials{ClientID: "client", ClientSecret: "test-client-secret", WebhookSecret: "test-webhook-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), "test-client-secret") {
		t.Fatal("secret leak")
	}
	var sealed string
	if err = e.pool.QueryRow(ctx, "SELECT credentials_enc FROM engineering_integrations WHERE uuid=$1", saved.UUID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "test-client-secret") {
		t.Fatal("plaintext at rest")
	}
	authorize, err := e.svc.StartOAuth(ctx, e.org, e.user, "linear")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authorize)
	if err != nil {
		t.Fatal(err)
	}
	state := u.Query().Get("state")
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.linear.app/oauth/token" {
			t.Fatal(r.URL)
		}
		return fakeResponse(`{"access_token":"test-access","refresh_token":"test-refresh","expires_in":3600}`), nil
	})}
	e.svc.Adapters["linear"] = &providerAdapter{Provider: "linear", Client: client}
	if _, err = e.svc.CompleteOAuth(ctx, e.org, e.user+1, "linear", state, "code"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong user %v", err)
	}
	if _, err = e.svc.CompleteOAuth(ctx, e.org, e.user, "linear", state, "code"); err != nil {
		t.Fatal(err)
	}
	if _, err = e.svc.CompleteOAuth(ctx, e.org, e.user, "linear", state, "code"); !errors.Is(err, ErrInvalid) {
		t.Fatal("reused state")
	}
}

type fakeAdapter struct {
	Remote                              RemoteIssue
	Creates, Updates, Fetches, Comments int
}

func (a *fakeAdapter) Create(_ context.Context, _ Credentials, _ IntegrationConfig, in model.Issue) (RemoteIssue, error) {
	a.Creates++
	return RemoteIssue{ID: "remote-1", Key: "ENG-1", URL: "https://linear.app/issue/ENG-1", Title: in.Title, Description: in.Description, Status: in.Status}, nil
}
func (a *fakeAdapter) Update(_ context.Context, _ Credentials, _ IntegrationConfig, _ string, in model.Issue) error {
	a.Updates++
	a.Remote = RemoteIssue{ID: "remote-1", Title: in.Title, Description: in.Description, Status: in.Status}
	return nil
}
func (a *fakeAdapter) Fetch(context.Context, Credentials, IntegrationConfig, string) (RemoteIssue, error) {
	a.Fetches++
	return a.Remote, nil
}
func (a *fakeAdapter) Comment(context.Context, Credentials, IntegrationConfig, string, string) error {
	a.Comments++
	return nil
}
func TestTwoWaySyncWebhookReplayAndNativeCommentEcho(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	c := e.conversation(t, "sync")
	issue, err := e.svc.Create(ctx, e.org, e.user, model.CreateInput{Title: "Original", Conversation: &c})
	if err != nil {
		t.Fatal(err)
	}
	secret := "webhook-secret-123456"
	integration, err := e.svc.SaveIntegration(ctx, e.org, "linear", IntegrationInput{Enabled: true, Config: IntegrationConfig{TeamID: "team"}, Credentials: &Credentials{AccessToken: "test-token", WebhookSecret: secret}})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &fakeAdapter{}
	e.svc.Adapters["linear"] = adapter
	if _, err = e.svc.ConnectIssue(ctx, e.org, e.user, issue.UUID, "linear", ""); err != nil {
		t.Fatal(err)
	}
	if err = e.svc.SyncSweep(ctx); err != nil {
		t.Fatal(err)
	}
	if adapter.Creates != 1 {
		t.Fatal("provider issue not created")
	}
	if _, err = e.svc.Comment(ctx, e.org, e.user, issue.UUID, "Investigating"); err != nil {
		t.Fatal(err)
	}
	if err = e.svc.SyncSweep(ctx); err != nil {
		t.Fatal(err)
	}
	if adapter.Comments != 1 || adapter.Updates != 0 {
		t.Fatalf("sync counts %+v", adapter)
	}
	if err = e.svc.SyncSweep(ctx); err != nil {
		t.Fatal(err)
	}
	if adapter.Comments != 1 {
		t.Fatal("comment redelivered")
	}
	adapter.Remote = RemoteIssue{ID: "remote-1", Title: "Fixed export", Description: "Remote description", Status: "done"}
	raw := []byte(fmt.Sprintf(`{"type":"Issue","action":"update","webhookTimestamp":%d,"data":{"id":"remote-1"}}`, time.Now().UnixMilli()))
	headers := http.Header{}
	headers.Set("Linear-Signature", sign(secret, raw))
	for range 2 {
		if err = e.svc.Webhook(ctx, integration.UUID, headers, raw); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := e.svc.Get(ctx, e.org, issue.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "done" || updated.Title != "Fixed export" || adapter.Fetches != 1 {
		t.Fatalf("webhook/replay %+v %+v", updated, adapter)
	}
	deliveries, err := e.svc.Deliveries(ctx, e.org, issue.UUID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("resolution follow-up %+v %v", deliveries, err)
	}
	raw = []byte(fmt.Sprintf(`{"type":"Comment","action":"create","webhookTimestamp":%d,"data":{"id":"comment-1","issueId":"remote-1","body":"Remote comment"}}`, time.Now().UnixMilli()))
	headers.Set("Linear-Signature", sign(secret, raw))
	for range 2 {
		if err = e.svc.Webhook(ctx, integration.UUID, headers, raw); err != nil {
			t.Fatal(err)
		}
	}
	comments, err := e.svc.Comments(ctx, e.org, issue.UUID)
	if err != nil || len(comments) != 2 {
		t.Fatalf("comments %+v %v", comments, err)
	}
	if err = e.svc.SyncSweep(ctx); err != nil {
		t.Fatal(err)
	}
	if adapter.Comments != 1 {
		t.Fatal("remote comment echoed")
	}
	// Optional summary calls the injected guarded AI boundary only on opt-in.
	var called bool
	e.svc.Summarize = func(_ context.Context, org, user int64, input string) (string, error) {
		called = org == e.org && user == e.user && strings.Contains(input, "Broken export")
		return "Guarded summary", nil
	}
	summarized, err := e.svc.Create(ctx, e.org, e.user, model.CreateInput{Conversation: &c, AISummary: true})
	if err != nil || !called || summarized.Description != "Guarded summary" {
		t.Fatalf("AI boundary %+v %v", summarized, err)
	}
}

func TestSentryErrorAttachmentAndProactiveResolution(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	conversation := e.conversation(t, "sentry")
	issue, err := e.svc.Create(ctx, e.org, e.user, model.CreateInput{Title: "Crash", Description: "Support reproduction", Conversation: &conversation})
	if err != nil {
		t.Fatal(err)
	}
	secret := "sentry-app-client-secret"
	integration, err := e.svc.SaveIntegration(ctx, e.org, "sentry", IntegrationInput{Enabled: true, Credentials: &Credentials{AccessToken: "test-sentry-token", WebhookSecret: secret}})
	if err != nil {
		t.Fatal(err)
	}
	event := ErrorInput{IssueUUID: issue.UUID, SentryIssueID: "123", EventID: "0123456789abcdef0123456789abcdef", EventURL: "https://example.sentry.io/issues/123/events/0123456789abcdef0123456789abcdef/", Summary: "Widget export crash"}
	for range 2 {
		if _, err = e.svc.AttachError(ctx, e.org, e.user, conversation, event); err != nil {
			t.Fatal(err)
		}
	}
	events, err := e.svc.Errors(ctx, e.org, e.user, conversation)
	if err != nil || len(events) != 1 {
		t.Fatalf("events %+v %v", events, err)
	}
	var fetched int
	e.svc.SentryClient = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		fetched++
		if r.URL.String() != "https://sentry.io/api/0/issues/123/" || r.Header.Get("Authorization") != "Bearer test-sentry-token" {
			t.Fatal("bad Sentry request")
		}
		return fakeResponse(`{"id":"123","title":"Crash fixed","status":"resolved","permalink":"https://example.sentry.io/issues/123/"}`), nil
	})}
	raw := []byte(`{"action":"resolved","data":{"issue":{"id":"123"}}}`)
	headers := http.Header{}
	headers.Set("Sentry-Hook-Resource", "issue")
	headers.Set("Sentry-Hook-Signature", sign(secret, raw))
	for range 2 {
		if err = e.svc.Webhook(ctx, integration.UUID, headers, raw); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := e.svc.Get(ctx, e.org, issue.UUID)
	if err != nil || updated.Status != "done" || updated.Description != "Support reproduction" || fetched != 1 {
		t.Fatalf("Sentry resolution %+v calls=%d %v", updated, fetched, err)
	}
	if err = e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if len(e.queued) != 1 {
		t.Fatal("Sentry follow-up not queued")
	}
}

func TestApprovedFollowupTemplateSelection(t *testing.T) {
	settings := []byte(`{"engineering_followup":{"resolution_template":"issue_resolved","release_template":"issue_released","language":"tr"}}`)
	for _, test := range []struct{ name, generation, want string }{{"resolution", "done:1", "issue_resolved"}, {"release", "release:1", "issue_released"}} {
		t.Run(test.name, func(t *testing.T) {
			template, err := followupTemplate(settings, test.generation, "Approved copy")
			if err != nil || template == nil || template.Name != test.want || len(template.Parameters) != 1 || template.Parameters[0] != "Approved copy" {
				t.Fatalf("template %+v %v", template, err)
			}
		})
	}
	if _, err := followupTemplate([]byte(`{"engineering_followup":{"resolution_template":"INVALID NAME"}}`), "done:1", "copy"); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid provider template accepted")
	}
}
