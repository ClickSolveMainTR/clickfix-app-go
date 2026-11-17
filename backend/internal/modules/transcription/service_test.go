package transcription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	airepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/repository"
	aiusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	attachmentmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/billingtest"
	billingmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	billingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/transcription/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func fundedAI(t *testing.T, modes ...string) (*aiusecase.Service, int64, int64) {
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
	var actor, org int64
	var orgUUID uuid.UUID
	if err = pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'fake','AI','Test') RETURNING id", uuid.NewString()+"@example.test").Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'AI test') RETURNING id,uuid", uuid.NewString()).Scan(&org, &orgUUID); err != nil {
		t.Fatal(err)
	}
	repo := billingrepo.New(pool, db.New(pool))
	billing := billingusecase.New(repo)
	plan, err := billing.Create(ctx, billingmodel.PlanInput{Code: "ai-" + uuid.NewString(), Name: "AI", Visibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	grant := billingtest.Grant(1000000, map[string]int64{"ai.credits": 1000})
	if len(modes) > 0 && modes[0] == "unlimited" {
		base := grant
		grant = func(feature billingmodel.Feature) billingmodel.Grant {
			g := base(feature)
			if feature.Key == "ai.credits" {
				g.Limit = nil
				g.Unlimited = true
			}
			return g
		}
	}
	prices := billingmodel.VersionInput{Prices: []billingmodel.Price{{Interval: "monthly", Currency: "TRY", Amount: 10000, VAT: 2000}}}
	var version billingmodel.Version
	if len(modes) > 0 && modes[0] == "missing-tokens" {
		// Simulate an immutable legacy version created before the current
		// complete-catalog validator: save the draft, drop one grant and
		// publish it behind the validator's back.
		features, err := billing.Features(ctx)
		if err != nil {
			t.Fatal(err)
		}
		in := prices
		for _, feature := range features {
			in.Entitlements = append(in.Entitlements, grant(feature))
		}
		if version, err = billing.SaveVersion(ctx, plan.UUID, uuid.Nil, in); err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, "DELETE FROM billing_plan_entitlements WHERE plan_version_id=(SELECT id FROM billing_plan_versions WHERE uuid=$1) AND feature_id=(SELECT id FROM billing_features WHERE key='ai.tokens')", version.UUID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, "UPDATE billing_plan_versions SET status='published',published_at=now() WHERE uuid=$1", version.UUID); err != nil {
			t.Fatal(err)
		}
	} else if version, err = billingtest.PublishVersion(ctx, billing, plan.UUID, actor, prices, grant); err != nil {
		t.Fatal(err)
	}
	_, err = billing.CreateSubscription(ctx, billingmodel.CreateSubscription{OrganizationUUID: orgUUID, VersionUUID: version.UUID, PriceUUID: version.Prices[0].UUID, Status: "active", PeriodStart: time.Now().Add(-time.Hour), PeriodEnd: time.Now().AddDate(0, 1, 0)}, actor)
	if err != nil {
		t.Fatal(err)
	}
	ent := entitlements.New(repo, nil, "ai-test")
	store := airepo.New(repo, ent)
	_, err = store.SavePolicy(ctx, db.AISavePolicyParams{OrganizationID: org, Enabled: true, MaskPii: true, AllowUnmask: true, DailyCalls: pgtype.Int4{Int32: 100, Valid: true}, Concurrency: pgtype.Int4{Int32: 1, Valid: true}, DailyBudgetMicros: pgtype.Int8{Int64: 10000000, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}

	routes := map[string]ai.Route{}
	for _, task := range []string{"triage", "reply", "complex"} {
		routes[task] = ai.Route{Provider: nil, ProviderName: ai.ProviderAnthropic, Model: ai.ModelForTask(task), InputPrice: 1000000, OutputPrice: 2000000}
	}
	return &aiusecase.Service{Repo: store, Routes: routes}, org, actor
}

type fakeFiles struct{}

func (fakeFiles) Open(context.Context, int64, uuid.UUID, string) (attachmentmodel.OpenFile, error) {
	return attachmentmodel.OpenFile{Body: io.NopCloser(strings.NewReader("audio")), Size: 5}, nil
}

type fakeVideo struct{}

func (fakeVideo) OpenRecording(context.Context, int64, uuid.UUID) (io.ReadCloser, int64, string, error) {
	return io.NopCloser(strings.NewReader("audio")), 5, "room.mp4", nil
}

type fixture struct {
	svc                              *Service
	pool                             *pgxpool.Pool
	org, actor, conv, voice, message int64
	id, attachment, conversation     uuid.UUID
	calls                            atomic.Int32
	provider                         *httptest.Server
}

func setup(t *testing.T) *fixture {
	t.Helper()
	core, org, actor := fundedAI(t)
	pool := core.Repo.Billing.Pool
	f := &fixture{pool: pool, org: org, actor: actor}
	ctx := context.Background()
	var contact, instance int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Audio') RETURNING id", org).Scan(&contact))
	must(pool.QueryRow(ctx, "INSERT INTO channel_instances(organization_id,channel_type,name) VALUES($1,'voice','Audio') RETURNING id", org).Scan(&instance))
	must(pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,contact_id,channel_type,channel_instance_id,number) VALUES($1,$2,'voice',$3,1) RETURNING id,uuid", org, contact, instance).Scan(&f.conv, &f.conversation))
	var messageUUID uuid.UUID
	must(pool.QueryRow(ctx, "INSERT INTO messages(organization_id,conversation_id,channel_instance_id,direction,sender_type,kind,status) VALUES($1,$2,$3,'in','contact','media','received') RETURNING id,uuid", org, f.conv, instance).Scan(&f.message, &messageUUID))
	must(pool.QueryRow(ctx, "INSERT INTO attachments(organization_id,owner_type,owner_id,filename,content_type,size_bytes,sha256,object_key,scan_status) VALUES($1,'message',$2,'call.wav','audio/wav',5,'test','audio-test','clean') RETURNING uuid", org, messageUUID).Scan(&f.attachment))
	must(pool.QueryRow(ctx, "INSERT INTO channel_voice_calls(organization_id,channel_instance_id,external_id,provider,direction,status,duration_seconds,recording_message_id,last_event_at) VALUES($1,$2,$3,'twilio','inbound','completed',61,$4,now()) RETURNING id", org, instance, uuid.NewString(), f.message).Scan(&f.voice))
	must(pool.QueryRow(ctx, "SELECT uuid FROM recording_transcripts WHERE organization_id=$1 AND voice_call_id=$2", org, f.voice).Scan(&f.id))
	f.provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.URL.Path == "/audio/transcriptions" {
			_, _ = fmt.Fprint(w, `{"text":"Contact person@example.test","segments":[{"start":0,"end":1,"speaker":"A","text":"person@example.test"}]}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"summary":"Contact person@example.test","action_items":["Call back"]}`}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 10}})
	}))
	t.Cleanup(f.provider.Close)
	raw := ai.NewOpenAI("test", f.provider.URL, "")
	core.Routes["reply"] = ai.Route{Provider: raw, ProviderName: ai.ProviderOpenAI, Model: "test", InputPrice: 1, OutputPrice: 1}
	f.svc = &Service{Repo: &repository.Store{Queries: db.New(pool), Pool: pool}, AI: core, Speech: &aiusecase.MeteredTranscriber{Repo: core.Repo, Raw: raw, Model: "whisper-1", PricePerMinute: 6000}, Files: fakeFiles{}, Video: fakeVideo{}, Summarize: true}
	return f
}
func (f *fixture) get(t *testing.T) db.RecordingTranscript {
	t.Helper()
	r, e := f.svc.Repo.TranscriptGet(context.Background(), db.TranscriptGetParams{OrganizationID: f.org, Uuid: f.id})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestVoiceTranscriptSummaryIsolationAndErasure(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	pending := f.get(t)
	if pending.Status != "pending" || !pending.TimelineMessageID.Valid {
		t.Fatal(pending)
	}
	if err := f.svc.Process(ctx, f.org, f.id); err != nil {
		t.Fatal(err)
	}
	done := f.get(t)
	if done.Status != "completed" || done.SummaryStatus != "completed" || strings.Contains(done.Transcript, "person@example.test") || strings.Contains(string(done.Segments), "person@example.test") || strings.Contains(string(done.Summary), "person@example.test") {
		t.Fatalf("%+v", done)
	}
	if err := f.svc.Process(ctx, f.org, f.id); err != nil || f.calls.Load() != 2 {
		t.Fatalf("duplicate calls=%d err=%v", f.calls.Load(), err)
	}
	if _, err := f.svc.Repo.TranscriptGet(ctx, db.TranscriptGetParams{OrganizationID: f.org + 999999, Uuid: f.id}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	// Team visibility is checked even when the UUID is known.
	req := httptest.NewRequest("GET", "/", nil)
	req.SetPathValue("uuid", f.id.String())
	req = req.WithContext(orgctx.WithScope(ctx, orgctx.Scope{ID: f.org, Permissions: []string{rbac.PermTenantConversationsReadAll, rbac.PermTenantConversationsRead}}))
	req = req.WithContext(authctx.WithPrincipal(req.Context(), authctx.Principal{UserInternal: f.actor}))
	if _, err := f.svc.authorized(req); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE conversations SET assignee_user_id=$3 WHERE organization_id=$1 AND id=$2", f.org, f.conv, f.actor); err != nil {
		t.Fatal(err)
	}
	denied := req.WithContext(orgctx.WithScope(ctx, orgctx.Scope{ID: f.org, Permissions: []string{rbac.PermTenantConversationsRead}}))
	denied = denied.WithContext(authctx.WithPrincipal(denied.Context(), authctx.Principal{UserInternal: f.actor + 99999}))
	if _, err := f.svc.authorized(denied); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("team isolation: %v", err)
	}

	if _, err := f.pool.Exec(ctx, "UPDATE conversations SET anonymized_at=now() WHERE organization_id=$1 AND id=$2", f.org, f.conv); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Repo.TranscriptGet(ctx, db.TranscriptGetParams{OrganizationID: f.org, Uuid: f.id}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	var output string
	if err := f.pool.QueryRow(ctx, "SELECT output FROM ai_runs WHERE organization_id=$1 AND uuid=$2", f.org, done.RunUuid).Scan(&output); err != nil || output != "" {
		t.Fatalf("erased output=%q %v", output, err)
	}
	// Finalization of a concurrently erased run must not restore transcript text.
	_, err := f.pool.Exec(ctx, "UPDATE ai_runs SET status='running' WHERE organization_id=$1 AND uuid=$2", f.org, done.RunUuid)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.Repo.AIFinishRun(ctx, db.AIFinishRunParams{OrganizationID: f.org, Uuid: done.RunUuid, Status: "completed", Output: "must not return", ToolCalls: []byte(`[]`), Provider: "openai", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(ctx, "SELECT output FROM ai_runs WHERE organization_id=$1 AND uuid=$2", f.org, done.RunUuid).Scan(&output); err != nil || output != "" {
		t.Fatal("erased AI log was restored", err)
	}
}
func TestTranscriptRetentionAndDisabledProvider(t *testing.T) {
	for _, method := range []string{"attachment", "message", "expiry", "disabled"} {
		t.Run(method, func(t *testing.T) {
			f := setup(t)
			ctx := context.Background()
			if method == "disabled" {
				f.svc.Speech.PricePerMinute = 0
				if err := f.svc.Process(ctx, f.org, f.id); err != nil {
					t.Fatal(err)
				}
				if f.get(t).Status != "unavailable" || f.calls.Load() != 0 {
					t.Fatal("disabled provider invoked")
				}
				return
			}
			if err := f.svc.Process(ctx, f.org, f.id); err != nil {
				t.Fatal(err)
			}
			var err error
			switch method {
			case "attachment":
				_, err = f.pool.Exec(ctx, "UPDATE attachments SET deleted_at=now() WHERE organization_id=$1 AND uuid=$2", f.org, f.attachment)
			case "message":
				_, err = f.pool.Exec(ctx, "UPDATE messages SET deleted_at=now() WHERE organization_id=$1 AND id=$2", f.org, f.message)
			case "expiry":
				_, err = f.pool.Exec(ctx, "UPDATE attachments SET expires_at=now()-interval '1 second' WHERE organization_id=$1 AND uuid=$2", f.org, f.attachment)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.svc.Repo.TranscriptGet(ctx, db.TranscriptGetParams{OrganizationID: f.org, Uuid: f.id}); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatal(err)
			}
			if err = f.svc.Sweep(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestVideoRecordingCompletionAndRetention(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var room, recording int64
	var rid uuid.UUID
	if err := f.pool.QueryRow(ctx, "INSERT INTO video_rooms(organization_id,name,livekit_name,max_participants,max_duration_minutes,links_expire_at,conversation_id) VALUES($1,'Room',$2,2,60,now()+interval '1 day',$3) RETURNING id", f.org, uuid.NewString(), f.conv).Scan(&room); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO video_recordings(organization_id,room_id,storage,object_key) VALUES($1,$2,'platform','video') RETURNING id,uuid", f.org, room).Scan(&recording, &rid); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Repo.VideoFinishRecording(ctx, db.VideoFinishRecordingParams{OrganizationID: f.org, ID: recording, Status: "complete", SizeBytes: 5, DurationSeconds: 120, RetentionHours: 24}); err != nil {
		t.Fatal(err)
	}
	id, err := f.svc.Repo.TranscriptByVideo(ctx, db.TranscriptByVideoParams{OrganizationID: f.org, Uuid: rid})
	if err != nil {
		t.Fatal(err)
	}
	f.id = id
	if err = f.svc.Process(ctx, f.org, id); err != nil {
		t.Fatal(err)
	}
	if f.get(t).Status != "completed" {
		t.Fatal("video did not complete")
	}
	if err = f.svc.Repo.VideoMarkRecordingDeleted(ctx, db.VideoMarkRecordingDeletedParams{OrganizationID: f.org, ID: recording}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.Repo.TranscriptGet(ctx, db.TranscriptGetParams{OrganizationID: f.org, Uuid: id}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
}

type capturedJobs struct{ payloads []Payload }

func (j *capturedJobs) EnqueueContext(_ context.Context, t *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	if t.Type() == "app:recording:transcribe" {
		var p Payload
		_ = json.Unmarshal(t.Payload(), &p)
		j.payloads = append(j.payloads, p)
	}
	return &asynq.TaskInfo{}, nil
}
func TestPendingDispatchAndInterruptedJobNeverReplays(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	jobs := &capturedJobs{}
	f.svc.Jobs = jobs
	if err := f.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range jobs.payloads {
		if p.UUID == f.id && p.OrganizationID == f.org {
			found = true
		}
	}
	if !found {
		t.Fatal("pending recording was not dispatched")
	}
	if _, err := f.svc.Repo.TranscriptClaim(ctx, db.TranscriptClaimParams{OrganizationID: f.org, Uuid: f.id}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Process(ctx, f.org, f.id); err != nil || f.calls.Load() != 0 {
		t.Fatal("claimed job replayed", err)
	}
	// Insert a pre-existing interrupted lease without touching other jobs' clocks.
	if _, err := f.pool.Exec(ctx, "DELETE FROM recording_transcripts WHERE organization_id=$1 AND uuid=$2", f.org, f.id); err != nil {
		t.Fatal(err)
	}
	_, err := f.pool.Exec(ctx, "INSERT INTO recording_transcripts(organization_id,uuid,voice_call_id,source_message_id,attachment_uuid,status,updated_at) VALUES($1,$2,$3,$4,$5,'processing',now()-interval '16 minutes')", f.org, f.id, f.voice, f.message, f.attachment)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if f.get(t).Status != "failed" || f.calls.Load() != 0 {
		t.Fatal("interrupted job was replayed")
	}
}
func TestErasureDuringProviderCallCannotRestoreOutput(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	pending := f.get(t)
	started, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = fmt.Fprint(w, `{"text":"person@example.test"}`)
	}))
	defer srv.Close()
	f.svc.Speech.Raw = ai.NewOpenAI("test", srv.URL, "")
	done := make(chan error, 1)
	go func() { done <- f.svc.Process(ctx, f.org, f.id) }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("provider did not start")
	}
	_, err := f.pool.Exec(ctx, "UPDATE attachments SET deleted_at=now() WHERE organization_id=$1 AND uuid=$2", f.org, f.attachment)
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.Repo.TranscriptGet(ctx, db.TranscriptGetParams{OrganizationID: f.org, Uuid: f.id}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("erased transcript returned", err)
	}
	run, err := f.svc.AI.Repo.Get(ctx, f.org, pending.RunUuid)
	if err != nil || run.Output != "" || !run.PiiErasedAt.Valid {
		t.Fatalf("run resurrected: %+v %v", run, err)
	}
	if f.calls.Load() != 0 {
		t.Fatal("summary sent after erasure")
	}
}
