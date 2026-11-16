package remotesupport_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/remotesupport"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// All data here is synthetic (example.test addresses, generated names).

const lkSecret = "test-livekit-secret-0123456789abcdef"

type lkCall struct {
	Method string
	Body   map[string]any
}

type fakeLiveKit struct {
	mu    sync.Mutex
	calls []lkCall
}

func (f *fakeLiveKit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	f.mu.Lock()
	f.calls = append(f.calls, lkCall{Method: method, Body: body})
	f.mu.Unlock()
	if method == "StartRoomCompositeEgress" {
		_, _ = w.Write([]byte(`{"egressId":"EG_` + uuid.NewString()[:8] + `"}`))
		return
	}
	_, _ = w.Write([]byte(`{}`))
}

func (f *fakeLiveKit) last(method string) (lkCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].Method == method {
			return f.calls[i], true
		}
	}
	return lkCall{}, false
}

type fakeEnt struct {
	mu       sync.Mutex
	consumed map[string]int64
	denied   bool
}

func (f *fakeEnt) Require(_ context.Context, _ int64, key string) error {
	if f.denied {
		return &entitlement.Error{Code: entitlement.CodeFeatureRequired, Feature: key}
	}
	return nil
}
func (f *fakeEnt) CheckLimit(context.Context, int64, string, int64, int64) (entitlement.Decision, error) {
	return entitlement.Decision{Allowed: true, Unlimited: true}, nil
}
func (f *fakeEnt) Consume(_ context.Context, _ int64, _ string, n int64, key string) (entitlement.Decision, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consumed[key] += n
	return entitlement.Decision{Allowed: true, Unlimited: true}, nil
}

type fakePoster struct {
	mu     sync.Mutex
	texts  []string
	events []map[string]any
}

func (f *fakePoster) SystemEvent(_ context.Context, _, _ int64, _ convusecase.Actor, attrs map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, attrs)
	return nil
}

func (f *fakePoster) PostAuto(_ context.Context, _, _ int64, kind, content string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if kind != "remote_support" {
		return uuid.Nil, errors.New("unexpected kind")
	}
	f.texts = append(f.texts, content)
	return uuid.New(), nil
}

type fakePub struct {
	mu     sync.Mutex
	events []string
}

func (f *fakePub) Publish(_ context.Context, channel string, data any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, channel)
	return nil
}

type tenant struct {
	org, agent, conv int64
	orgUUID, convID  uuid.UUID
}

type fixture struct {
	ctx  context.Context
	pool *pgxpool.Pool
	q    *db.Queries
	svc  *remotesupport.Service
	lk   *fakeLiveKit
	ent  *fakeEnt
	post *fakePoster
	pub  *fakePub
	now  time.Time
	a, b tenant
}

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := &fixture{ctx: ctx, pool: pool, q: db.New(pool), lk: &fakeLiveKit{}, ent: &fakeEnt{consumed: map[string]int64{}}, post: &fakePoster{}, pub: &fakePub{},
		now: time.Now().UTC()}
	srv := httptest.NewServer(f.lk)
	t.Cleanup(srv.Close)
	f.svc = &remotesupport.Service{
		Q: f.q, LiveKit: livekit.New(livekit.Config{URL: "wss://media.example.test", APIURL: srv.URL, APIKey: "key", APISecret: lkSecret}),
		Entitlements: f.ent, Poster: f.post, Publisher: f.pub, Activity: activity.NewRecorder(f.q, slog.New(slog.DiscardHandler)),
		Hasher: remotesupport.NewHasher("test-secret"), FrontendURL: "https://app.example.test", Log: slog.New(slog.DiscardHandler),
		Now: func() time.Time { return f.now },
	}
	f.a, f.b = f.tenant(t, "A"), f.tenant(t, "B")
	return f
}

func (f *fixture) tenant(t *testing.T, label string) tenant {
	t.Helper()
	var tn tenant
	ctx := f.ctx
	if err := f.pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,$2) RETURNING id,uuid", "rs-"+uuid.NewString(), "Örnek İşletme "+label).Scan(&tn.org, &tn.orgUUID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'test-hash','Temsilci',$2) RETURNING id", uuid.NewString()+"@example.test", label).Scan(&tn.agent); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE slug='owner' AND scope='tenant' AND organization_id IS NULL", tn.org, tn.agent); err != nil {
		t.Fatal(err)
	}
	var contact int64
	if err := f.pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Örnek Müşteri') RETURNING id", tn.org).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,contact_id,number,channel_type) VALUES($1,$2,1,'test') RETURNING id,uuid", tn.org, contact).Scan(&tn.conv, &tn.convID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SaveSettings(ctx, tn.org, remotesupport.SettingsInput{Enabled: true, AllowControl: true, Recording: "optional"}); err != nil {
		t.Fatal(err)
	}
	return tn
}

func (f *fixture) conv(t *testing.T, tn tenant) db.Conversation {
	t.Helper()
	c, err := f.q.GetConversationByID(f.ctx, db.GetConversationByIDParams{OrganizationID: tn.org, ID: tn.conv})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *fixture) create(t *testing.T, tn tenant, mode string) remotesupport.Created {
	t.Helper()
	out, err := f.svc.Create(f.ctx, tn.org, remotesupport.Agent{ID: tn.agent, CanControl: true}, f.conv(t, tn), remotesupport.CreateInput{Mode: mode})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (f *fixture) session(t *testing.T, tn tenant, id uuid.UUID) db.RemoteSupportSession {
	t.Helper()
	row, err := f.svc.Session(f.ctx, tn.org, id)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (f *fixture) audited(t *testing.T, org int64, id uuid.UUID) map[string]bool {
	t.Helper()
	rows, err := f.pool.Query(f.ctx, "SELECT action FROM activity_events WHERE organization_id = $1 AND resource_uuid = $2", org, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		out[a] = true
	}
	return out
}

// grant decodes the LiveKit token's video grant.
func grant(t *testing.T, token string) map[string]any {
	t.Helper()
	cl := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(token, cl, func(*jwt.Token) (any, error) { return []byte(lkSecret), nil }); err != nil {
		t.Fatal(err)
	}
	v, _ := cl["video"].(map[string]any)
	return v
}

func joined(f *fixture, t *testing.T, row db.RemoteSupportSession) {
	t.Helper()
	ev := livekit.Event{Event: "participant_joined", Room: &livekit.Room{Name: row.RoomName},
		Participant: &livekit.Participant{SID: "PA_1", Identity: "rs:customer:" + row.Uuid.String()}}
	if err := f.svc.HandleEvent(f.ctx, ev); err != nil {
		t.Fatal(err)
	}
}

func TestFlowCreateResolveConsentTokenEnd(t *testing.T) {
	f := setup(t)
	created := f.create(t, f.a, remotesupport.ModeControl)
	if len(created.Code) != 9 || !strings.HasPrefix(created.JoinURL, "clickfix-support://join?t=") || !created.MessagePosted {
		t.Fatalf("unexpected create %+v", created)
	}
	if len(f.post.texts) != 1 || !strings.Contains(f.post.texts[0], remotesupport.FormatCode(created.Code)) || !strings.Contains(f.post.texts[0], "/download/support") {
		t.Fatalf("customer message missing code or download link: %v", f.post.texts)
	}
	row := f.session(t, f.a, created.Session.UUID)
	if strings.Contains(row.CodeHash, created.Code) || row.CodeHash == created.Code || row.Status != remotesupport.StatusPending {
		t.Fatal("code must be stored only as a digest")
	}

	// Agent cannot get a token before the customer approves viewing.
	if _, err := f.svc.AgentToken(f.ctx, f.a.org, remotesupport.Agent{ID: f.a.agent, CanControl: true}, row.Uuid); !errors.Is(err, remotesupport.ErrConsent) {
		t.Fatalf("token before consent: %v", err)
	}

	res, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: remotesupport.FormatCode(created.Code), ClientOS: "windows", ClientVersion: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionUUID != row.Uuid || res.Organization.Name != "Örnek İşletme A" || res.Agent.Name != "Temsilci A" || res.ModeRequested != "control" ||
		!res.RecordingRequested || !strings.Contains(res.Warning, "para transferi") || res.ClientSecret == "" {
		t.Fatalf("unexpected resolve %+v", res)
	}
	// Single use.
	if _, err = f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: created.Code}); !errors.Is(err, remotesupport.ErrCodeInvalid) {
		t.Fatalf("second resolve: %v", err)
	}
	if _, err = f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Token: strings.TrimPrefix(created.JoinURL, "clickfix-support://join?t=")}); !errors.Is(err, remotesupport.ErrCodeInvalid) {
		t.Fatalf("join link after resolve: %v", err)
	}
	// Customer token needs the viewing consent.
	if _, err = f.svc.CustomerToken(f.ctx, row.Uuid, remotesupport.ClientInput{ClientSecret: res.ClientSecret}); !errors.Is(err, remotesupport.ErrConsent) {
		t.Fatalf("customer token before consent: %v", err)
	}
	yes, no := true, false
	if _, err = f.svc.Consent(f.ctx, row.Uuid, remotesupport.ConsentInput{ClientSecret: res.ClientSecret, View: &yes, Recording: &yes}); err != nil {
		t.Fatal(err)
	}
	ct, err := f.svc.CustomerToken(f.ctx, row.Uuid, remotesupport.ClientInput{ClientSecret: res.ClientSecret})
	if err != nil {
		t.Fatal(err)
	}
	g := grant(t, ct.Token)
	if g["canPublishData"] != false || g["canPublish"] != true || len(g["canPublishSources"].([]any)) != 1 || g["canPublishSources"].([]any)[0] != "screen_share" || g["room"] != row.RoomName {
		t.Fatalf("customer grant %+v", g)
	}
	if _, ok := f.lk.last("CreateRoom"); !ok {
		t.Fatal("room not created")
	}
	agent := remotesupport.Agent{ID: f.a.agent, CanControl: true}
	at, err := f.svc.AgentToken(f.ctx, f.a.org, agent, row.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if g = grant(t, at.Token); g["canPublish"] != false || g["canSubscribe"] != true || g["canPublishData"] != false || at.CanPublishData {
		t.Fatalf("agent grant without control consent %+v", g)
	}

	joined(f, t, row)
	row = f.session(t, f.a, row.Uuid)
	if row.Status != remotesupport.StatusActive || !row.StartedAt.Valid {
		t.Fatalf("not active: %s", row.Status)
	}
	if _, ok := f.lk.last("StartRoomCompositeEgress"); !ok || f.session(t, f.a, row.Uuid).RecordingStatus != "active" {
		t.Fatal("recording with consent did not start")
	}

	// Control consent: the connected agent gets data publishing at once.
	if _, err = f.svc.Consent(f.ctx, row.Uuid, remotesupport.ConsentInput{ClientSecret: res.ClientSecret, Control: &yes}); err != nil {
		t.Fatal(err)
	}
	up, ok := f.lk.last("UpdateParticipant")
	if !ok || up.Body["identity"] != "rs:agent:"+row.Uuid.String() || up.Body["permission"].(map[string]any)["canPublishData"] != true {
		t.Fatalf("permission not pushed: %+v", up)
	}
	if at, err = f.svc.AgentToken(f.ctx, f.a.org, agent, row.Uuid); err != nil || grant(t, at.Token)["canPublishData"] != true {
		t.Fatalf("agent token with control: %v", err)
	}
	// An agent without tenant.remote_support.control never gets data.
	if at, err = f.svc.AgentToken(f.ctx, f.a.org, remotesupport.Agent{ID: f.a.agent}, row.Uuid); err != nil || grant(t, at.Token)["canPublishData"] != false {
		t.Fatalf("agent without control permission: %v", err)
	}
	// Revoking control takes it away from the live participant and new tokens.
	if _, err = f.svc.Consent(f.ctx, row.Uuid, remotesupport.ConsentInput{ClientSecret: res.ClientSecret, Control: &no}); err != nil {
		t.Fatal(err)
	}
	if up, _ = f.lk.last("UpdateParticipant"); up.Body["permission"].(map[string]any)["canPublishData"] != false {
		t.Fatalf("revoke not pushed: %+v", up)
	}
	if at, err = f.svc.AgentToken(f.ctx, f.a.org, agent, row.Uuid); err != nil || grant(t, at.Token)["canPublishData"] != false {
		t.Fatalf("agent token after revoke: %v", err)
	}

	f.now = f.now.Add(3 * time.Minute)
	v, err := f.svc.EndByAgent(f.ctx, f.a.org, agent, row.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != remotesupport.StatusEnded || v.EndedBy != "agent" || v.BilledMinutes < 3 {
		t.Fatalf("end %+v", v)
	}
	if del, ok := f.lk.last("DeleteRoom"); !ok || del.Body["room"] != row.RoomName {
		t.Fatal("room not deleted")
	}
	if f.ent.consumed["remote_support:"+row.Uuid.String()] != v.BilledMinutes {
		t.Fatalf("metering %v", f.ent.consumed)
	}
	// The summary lands on the conversation timeline (CF-237).
	if n := len(f.post.events); n == 0 || f.post.events[n-1]["event"] != "remote_support_ended" ||
		f.post.events[n-1]["session_uuid"] != row.Uuid.String() || f.post.events[n-1]["control_consent"] != true ||
		f.post.events[n-1]["duration_seconds"].(int64) < 180 {
		t.Fatalf("timeline %+v", f.post.events)
	}
	if _, err = f.svc.CustomerToken(f.ctx, row.Uuid, remotesupport.ClientInput{ClientSecret: res.ClientSecret}); !errors.Is(err, remotesupport.ErrEnded) {
		t.Fatalf("token after end: %v", err)
	}
	got := f.audited(t, f.a.org, row.Uuid)
	for _, a := range []string{"remote_support.created", "remote_support.resolved", "remote_support.view_consented", "remote_support.recording_consented",
		"remote_support.started", "remote_support.control_consented", "remote_support.control_revoked", "remote_support.ended"} {
		if !got[a] {
			t.Errorf("audit %s missing (have %v)", a, got)
		}
	}
	if len(f.pub.events) == 0 || f.pub.events[0] != remotesupport.Channel(row.Uuid) {
		t.Fatalf("realtime %v", f.pub.events)
	}
}

func TestControlNeedsRequestAndPermission(t *testing.T) {
	f := setup(t)
	conv := f.conv(t, f.a)
	if _, err := f.svc.Create(f.ctx, f.a.org, remotesupport.Agent{ID: f.a.agent}, conv, remotesupport.CreateInput{Mode: remotesupport.ModeControl}); !errors.Is(err, remotesupport.ErrDenied) {
		t.Fatalf("control without permission: %v", err)
	}
	created := f.create(t, f.a, remotesupport.ModeView)
	res, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: created.Code})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err = f.svc.Consent(f.ctx, res.SessionUUID, remotesupport.ConsentInput{ClientSecret: res.ClientSecret, Control: &yes}); !errors.Is(err, remotesupport.ErrConsent) {
		t.Fatalf("control before viewing: %v", err)
	}
	if _, err = f.svc.Consent(f.ctx, res.SessionUUID, remotesupport.ConsentInput{ClientSecret: res.ClientSecret, View: &yes, Control: &yes}); !errors.Is(err, remotesupport.ErrInvalid) {
		t.Fatalf("control on a view-only session: %v", err)
	}
	if _, err = f.svc.Consent(f.ctx, res.SessionUUID, remotesupport.ConsentInput{ClientSecret: res.ClientSecret, View: &yes}); err != nil {
		t.Fatal(err)
	}
	at, err := f.svc.AgentToken(f.ctx, f.a.org, remotesupport.Agent{ID: f.a.agent, CanControl: true}, res.SessionUUID)
	if err != nil || grant(t, at.Token)["canPublishData"] != false {
		t.Fatalf("view-only agent token must not publish data: %v", err)
	}
	// Organization turns control off: an existing consent stops counting.
	f.create(t, f.b, remotesupport.ModeControl)
	if _, err = f.svc.SaveSettings(f.ctx, f.a.org, remotesupport.SettingsInput{Enabled: true, AllowControl: false}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.Create(f.ctx, f.a.org, remotesupport.Agent{ID: f.a.agent, CanControl: true}, conv, remotesupport.CreateInput{Mode: remotesupport.ModeControl}); !errors.Is(err, remotesupport.ErrDenied) {
		t.Fatalf("control when the organization forbids it: %v", err)
	}
}

func TestCodeExpiryAndSweep(t *testing.T) {
	f := setup(t)
	created := f.create(t, f.a, remotesupport.ModeView)
	f.now = f.now.Add(remotesupport.CodeTTL + time.Second)
	if _, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: created.Code}); !errors.Is(err, remotesupport.ErrCodeInvalid) {
		t.Fatalf("expired code: %v", err)
	}
	if row := f.session(t, f.a, created.Session.UUID); row.Status != remotesupport.StatusExpired || row.EndedReason != "code_expired" {
		t.Fatalf("status %s/%s", row.Status, row.EndedReason)
	}

	// The sweep expires codes nobody tried.
	f.now = time.Now().UTC()
	other := f.create(t, f.b, remotesupport.ModeView)
	f.now = f.now.Add(remotesupport.CodeTTL + time.Second)
	if err := f.svc.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	if row := f.session(t, f.b, other.Session.UUID); row.Status != remotesupport.StatusExpired {
		t.Fatalf("sweep left %s", row.Status)
	}
	if !f.audited(t, f.b.org, other.Session.UUID)["remote_support.expired"] {
		t.Fatal("expiry not audited")
	}

	// Time limit: an active session ends after max_duration_minutes.
	f.now = time.Now().UTC()
	live := f.create(t, f.a, remotesupport.ModeView)
	res, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: live.Code})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err = f.svc.Consent(f.ctx, res.SessionUUID, remotesupport.ConsentInput{ClientSecret: res.ClientSecret, View: &yes}); err != nil {
		t.Fatal(err)
	}
	joined(f, t, f.session(t, f.a, res.SessionUUID))
	f.now = time.Now().UTC().Add(time.Duration(remotesupport.DefaultMaxDuration+1) * time.Minute)
	if err = f.svc.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	if row := f.session(t, f.a, res.SessionUUID); row.Status != remotesupport.StatusEnded || row.EndedReason != "max_duration" || row.BilledMinutes < 1 {
		t.Fatalf("max duration: %s/%s/%d", row.Status, row.EndedReason, row.BilledMinutes)
	}
}

func TestWrongCodeAttemptsInvalidate(t *testing.T) {
	f := setup(t)
	created := f.create(t, f.a, remotesupport.ModeView)
	wrong := func(i int) string {
		// Same 5-digit prefix, different tail: charged to this session.
		tail := []byte(created.Code[5:])
		tail[3] = byte('0' + (int(tail[3]-'0')+1+i%9)%10)
		return created.Code[:5] + string(tail)
	}
	for i := 0; i < remotesupport.MaxCodeAttempts-1; i++ {
		if _, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: wrong(i)}); !errors.Is(err, remotesupport.ErrCodeInvalid) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if row := f.session(t, f.a, created.Session.UUID); row.Status != remotesupport.StatusPending || row.CodeAttempts != remotesupport.MaxCodeAttempts-1 {
		t.Fatalf("after 4 attempts: %s %d", row.Status, row.CodeAttempts)
	}
	if _, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: wrong(5)}); !errors.Is(err, remotesupport.ErrCodeInvalid) {
		t.Fatal(err)
	}
	row := f.session(t, f.a, created.Session.UUID)
	if row.Status != remotesupport.StatusExpired || row.EndedReason != "attempts" {
		t.Fatalf("after 5 attempts: %s/%s", row.Status, row.EndedReason)
	}
	if _, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: created.Code}); !errors.Is(err, remotesupport.ErrCodeInvalid) {
		t.Fatalf("correct code after invalidation: %v", err)
	}
	if !f.audited(t, f.a.org, row.Uuid)["remote_support.code_invalidated"] {
		t.Fatal("invalidation not audited")
	}
	// Malformed input is rejected without touching any session.
	if _, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: "12345"}); !errors.Is(err, remotesupport.ErrInvalid) {
		t.Fatalf("short code: %v", err)
	}
}

func TestTenantIsolation(t *testing.T) {
	f := setup(t)
	created := f.create(t, f.a, remotesupport.ModeControl)
	id := created.Session.UUID
	other := remotesupport.Agent{ID: f.b.agent, CanControl: true, Manager: true}
	if _, err := f.svc.Session(f.ctx, f.b.org, id); !errors.Is(err, remotesupport.ErrNotFound) {
		t.Fatalf("other org sees session: %v", err)
	}
	if _, err := f.svc.AgentToken(f.ctx, f.b.org, other, id); !errors.Is(err, remotesupport.ErrNotFound) {
		t.Fatalf("other org token: %v", err)
	}
	if _, err := f.svc.EndByAgent(f.ctx, f.b.org, other, id); !errors.Is(err, remotesupport.ErrNotFound) {
		t.Fatalf("other org end: %v", err)
	}
	if rows, err := f.svc.ForConversation(f.ctx, f.b.org, f.a.conv); err != nil || len(rows) != 0 {
		t.Fatalf("other org lists %d sessions: %v", len(rows), err)
	}
	// A member of another organization (even its owner) cannot subscribe to
	// the session channel.
	var bUser uuid.UUID
	if err := f.pool.QueryRow(f.ctx, "SELECT uuid FROM users WHERE id=$1", f.b.agent).Scan(&bUser); err != nil {
		t.Fatal(err)
	}
	authz := remotesupport.Authorizer{Resolver: staticResolver{f}, Queries: f.q}
	if ok, _ := authz.CanSubscribeChannel(f.ctx, bUser, false, remotesupport.Channel(id)); ok {
		t.Fatal("other org subscribed to the session channel")
	}
	var aUser uuid.UUID
	if err := f.pool.QueryRow(f.ctx, "SELECT uuid FROM users WHERE id=$1", f.a.agent).Scan(&aUser); err != nil {
		t.Fatal(err)
	}
	if ok, _ := authz.CanSubscribeChannel(f.ctx, aUser, false, remotesupport.Channel(id)); !ok {
		t.Fatal("session agent cannot subscribe")
	}
	// The client secret of one session does not open another.
	resA, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: created.Code})
	if err != nil {
		t.Fatal(err)
	}
	createdB := f.create(t, f.b, remotesupport.ModeView)
	resB, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: createdB.Code})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err = f.svc.Consent(f.ctx, resB.SessionUUID, remotesupport.ConsentInput{ClientSecret: resA.ClientSecret, View: &yes}); !errors.Is(err, remotesupport.ErrUnauthorized) {
		t.Fatalf("cross-session secret: %v", err)
	}
	if _, err = f.svc.EndByCustomer(f.ctx, resA.SessionUUID, remotesupport.ClientInput{ClientSecret: "x"}); !errors.Is(err, remotesupport.ErrUnauthorized) {
		t.Fatalf("bad secret: %v", err)
	}
	if resA.Organization.Name != "Örnek İşletme A" || resB.Organization.Name != "Örnek İşletme B" {
		t.Fatal("resolve shows the wrong organization")
	}
}

// staticResolver resolves membership scopes from the test database.
type staticResolver struct{ f *fixture }

func (r staticResolver) Resolve(ctx context.Context, orgRef string, userID int64) (orgctx.Scope, error) {
	id, err := uuid.Parse(orgRef)
	if err != nil {
		return orgctx.Scope{}, err
	}
	for _, tn := range []tenant{r.f.a, r.f.b} {
		if tn.orgUUID == id && tn.agent == userID {
			return orgctx.Scope{ID: tn.org, UUID: tn.orgUUID, Permissions: []string{"tenant.remote_support.use", "tenant.remote_support.settings"}}, nil
		}
	}
	return orgctx.Scope{}, errors.New("not a member")
}

func TestReportsSuspendAndKillSwitch(t *testing.T) {
	f := setup(t)
	for i := 0; i < remotesupport.ReportThreshold; i++ {
		c := f.create(t, f.a, remotesupport.ModeView)
		res, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: c.Code})
		if err != nil {
			t.Fatal(err)
		}
		v, err := f.svc.Report(f.ctx, res.SessionUUID, remotesupport.ReportInput{ClientSecret: res.ClientSecret, Reason: "Telefonla aradılar"})
		if err != nil {
			t.Fatal(err)
		}
		if !v.Reported || v.Status == remotesupport.StatusJoined {
			t.Fatalf("report %+v", v)
		}
	}
	set, err := f.svc.Settings(f.ctx, f.a.org)
	if err != nil {
		t.Fatal(err)
	}
	if set.PlatformState != remotesupport.PlatformSuspended || set.Ready {
		t.Fatalf("not suspended: %+v", set)
	}
	if _, err = f.svc.Create(f.ctx, f.a.org, remotesupport.Agent{ID: f.a.agent}, f.conv(t, f.a), remotesupport.CreateInput{}); !errors.Is(err, remotesupport.ErrSuspended) {
		t.Fatalf("create while suspended: %v", err)
	}
	// Organization B keeps working; the platform kill switch ends its open
	// sessions at once.
	open := f.create(t, f.b, remotesupport.ModeView)
	if _, err = f.svc.SetPlatformState(f.ctx, 0, f.b.orgUUID, remotesupport.PlatformStateInput{State: remotesupport.PlatformDisabled, Reason: "İnceleme"}); err != nil {
		t.Fatal(err)
	}
	if row := f.session(t, f.b, open.Session.UUID); row.Status != remotesupport.StatusCancelled || row.EndedReason != "platform" {
		t.Fatalf("kill switch left %s/%s", row.Status, row.EndedReason)
	}
	if _, err = f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: open.Code}); !errors.Is(err, remotesupport.ErrCodeInvalid) {
		t.Fatalf("resolve after kill switch: %v", err)
	}
	if _, err = f.svc.SetPlatformState(f.ctx, 0, f.b.orgUUID, remotesupport.PlatformStateInput{State: remotesupport.PlatformDisabled}); !errors.Is(err, remotesupport.ErrInvalid) {
		t.Fatalf("kill switch without reason: %v", err)
	}
}

func TestRecordingRetention(t *testing.T) {
	f := setup(t)
	c := f.create(t, f.a, remotesupport.ModeView)
	res, err := f.svc.Resolve(f.ctx, remotesupport.ResolveInput{Code: c.Code})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err = f.svc.Consent(f.ctx, res.SessionUUID, remotesupport.ConsentInput{ClientSecret: res.ClientSecret, View: &yes, Recording: &yes}); err != nil {
		t.Fatal(err)
	}
	row := f.session(t, f.a, res.SessionUUID)
	joined(f, t, row)
	row = f.session(t, f.a, res.SessionUUID)
	if row.EgressID == "" || row.RecordingExpiresAt.Time.Before(f.now.Add(29*24*time.Hour)) {
		t.Fatalf("recording %q expires %v", row.EgressID, row.RecordingExpiresAt.Time)
	}
	ev := livekit.Event{Event: "egress_ended", EgressInfo: &livekit.EgressInfo{EgressID: row.EgressID, RoomName: row.RoomName, Status: "EGRESS_COMPLETE",
		FileResults: []livekit.FileInfo{{Size: 2048}}}}
	if err = f.svc.HandleEvent(f.ctx, ev); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE remote_support_sessions SET recording_expires_at = now() - interval '1 minute' WHERE organization_id=$1 AND id=$2", row.OrganizationID, row.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.svc.PurgeRecordings(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.session(t, f.a, row.Uuid); got.RecordingStatus != "deleted" || got.RecordingSizeBytes != 2048 {
		t.Fatalf("recording %s %d", got.RecordingStatus, got.RecordingSizeBytes)
	}
}
