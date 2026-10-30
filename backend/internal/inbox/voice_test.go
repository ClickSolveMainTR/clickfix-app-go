package inbox_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	channelsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/voice"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Same opt-in DB fixture as the existing inbox integration tests.
func TestVoiceCallIdempotencyRecordingAndOrganizationIsolation(t *testing.T) {
	e := setup(t)
	other := setup(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		_, _ = fmt.Fprint(w,
			`{"code":"00","balance":"100,00"}`)
	}))
	defer server.Close()
	e.ib.Registry.Register(voice.New(voice.Config{NetgsmURL: server.URL}))
	view, err := e.ib.Channels.Create(ctx,
		channelsusecase.CreateInput{OrganizationID: e.org,
			ActorID: e.agent,
			Type:    channels.TypeVoice,
			Name:    "Santral",
			Credentials: channels.Credentials{"provider": "netgsm",
				"usercode":      "fixture",
				"password":      "fixture",
				"webhook_token": "test-voice-callback-secret-32-chars"}})
	if err != nil {
		t.Fatal(err)
	}
	inst, _, err := e.ib.Channels.Open(ctx, e.org, view.UUID)
	if err != nil {
		t.Fatal(err)
	}
	a := voice.New(voice.Config{})
	result, err := a.ParseWebhook(ctx,
		nil,
		[]byte(`{"uniqueid":"call-1","source":"05551112233","destination":"08501112233","direction":1,"duration":20}`),
		inst)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.ib.Receive(ctx, inst, result); err != nil {
		t.Fatal(err)
	}
	// Redelivery preserves one log and one conversation message.
	if err := e.ib.Receive(ctx, inst, result); err != nil {
		t.Fatal(err)
	}
	rows, total, err := e.ib.Channels.VoiceCalls(ctx, e.org, view.UUID, 20, 0)
	if err != nil || len(rows) != 1 || total != 1 || rows[0].DurationSeconds != 20 {
		t.Fatalf("logs %+v %d %v", rows, total, err)
	}
	if _, _, err := e.ib.Channels.VoiceCalls(ctx, other.org, view.UUID, 20, 0); err == nil {
		t.Fatal("other org read call logs")
	}
	// Link a recording only through a message belonging to this org + instance.
	event := result.Calls[0]
	event.RecordingMessageExternalID = "recording:1"
	event.Status = "recording"
	recording := channels.InboundMessage{ExternalID: "recording:1",
		From: channels.Sender{ExternalID: "+905551112233",
			Phone: "+905551112233"},
		Kind:     "text",
		Text:     "recording",
		ThreadID: "call-1",
		At:       time.Now()}
	if err := e.ib.Receive(ctx,
		inst,
		channels.WebhookResult{Calls: []channels.CallEvent{event},
			Messages: []channels.InboundMessage{recording}}); err != nil {
		t.Fatal(err)
	}
	rows, _, err = e.ib.Channels.VoiceCalls(ctx, e.org, view.UUID, 20, 0)
	if err != nil || !rows[0].RecordingMessageUuid.Valid || rows[0].Status != "completed" || rows[0].TranscriptStatus != "unavailable" {
		t.Fatalf("recording/final state %+v %v", rows, err)
	}
	q := db.New(e.pool)
	// The composite FK also rejects attaching a call to another org's inbox.
	err = q.UpsertChannelVoiceCall(ctx,
		db.UpsertChannelVoiceCallParams{OrganizationID: other.org,
			ChannelInstanceID: inst.ID,
			ExternalID:        uuid.NewString(),
			Provider:          "netgsm",
			Direction:         "inbound",
			Status:            "completed"})
	if err == nil {
		t.Fatal("cross-org voice foreign key accepted")
	}
}

func TestNetgsmLifecycleMemberSIPAndBrowserMerge(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		_, _ = fmt.Fprint(w,
			`{"code":"00","balance":"100,00"}`)
	}))
	defer server.Close()
	adapter := voice.New(voice.Config{NetgsmURL: server.URL})
	e.ib.Registry.Register(adapter)
	var role int64
	if err := e.pool.QueryRow(ctx,
		"SELECT id FROM roles WHERE slug='admin' AND scope='tenant' AND organization_id IS NULL LIMIT 1").Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx,
		"INSERT INTO organization_members(organization_id,user_id,role_id) VALUES($1,$2,$3)",
		e.org,
		e.agent,
		role); err != nil {
		t.Fatal(err)
	}
	var user uuid.UUID
	if err := e.pool.QueryRow(ctx, "SELECT uuid FROM users WHERE id=$1", e.agent).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := e.ib.Channels.SetMemberSIP(ctx,
		e.org,
		user,
		channelsusecase.SIPSettings{Extension: "101",
			Username: "agent-fixture",
			Password: "fixture-only"}); err != nil {
		t.Fatal(err)
	}
	meta, err := e.ib.Channels.MemberSIP(ctx, e.org, user)
	if err != nil || !meta.HasPassword || meta.Password != "" {
		t.Fatalf("SIP read leaked password: %+v %v", meta, err)
	}
	view, err := e.ib.Channels.Create(ctx,
		channelsusecase.CreateInput{OrganizationID: e.org,
			ActorID: e.agent,
			Type:    channels.TypeVoice,
			Name:    "Netgsm fixture",
			Credentials: channels.Credentials{"provider": "netgsm",
				"usercode":      "fixture",
				"password":      "fixture",
				"webhook_token": "test-voice-callback-secret-32-chars"},
			Settings: map[string]any{"ws_url": "wss://pbx.example.test/ws",
				"domain": "pbx.example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	inst, _, err := e.ib.Channels.Open(ctx, e.org, view.UUID)
	if err != nil {
		t.Fatal(err)
	}
	config, err := e.ib.Channels.Webphone(ctx, e.org, user)
	if err != nil || !config.Enabled || config.Password != "fixture-only" {
		t.Fatal("own SIP config unavailable", err)
	}
	started := time.Now().Add(-time.Minute).Truncate(time.Second)
	event := channelsusecase.WebphoneEvent{EventID: uuid.New(),
		Direction:   "inbound",
		Phone:       "05551112233",
		StartedAt:   started,
		EndedAt:     started.Add(30 * time.Second),
		Status:      "answered",
		TalkSeconds: 20}
	if _, err = e.pool.Exec(ctx, "INSERT INTO contacts(organization_id,name,phone) VALUES($1,'Fixture','0555 111 22 33')", e.org); err != nil {
		t.Fatal(err)
	}
	for _, order := range []string{"client-first", "provider-first", "sparse-provider-first"} {
		t.Run(order, func(t *testing.T) {
			id := uuid.NewString()
			body := fmt.Sprintf(`{"scenario":"cdr","unique_id":%q,"yon":"1","sondurum":"1","customer_num":"05551112233","internal_num":"101","callInitiatedTime":%q,"callEndedTime":%q,"talktime":20}`,
				id,
				started.Format(time.RFC3339),
				started.Add(30*time.Second).Format(time.RFC3339))
			result, err := adapter.ParseWebhook(ctx, nil, []byte(body), inst)
			if err != nil {
				t.Fatal(err)
			}
			if order == "sparse-provider-first" {
				early := result
				early.Messages = nil
				early.Calls = append([]channels.CallEvent(nil), result.Calls...)
				early.Calls[0].InternalNumber = ""
				early.Calls[0].InitiatedAt = time.Time{}
				early.Calls[0].ProviderFinal = false
				early.Calls[0].Status = "ringing"
				if err = e.ib.Receive(ctx, inst, early); err != nil {
					t.Fatal(err)
				}
			}
			if order == "provider-first" {
				if err = e.ib.Receive(ctx, inst, result); err != nil {
					t.Fatal(err)
				}
			}
			if err = e.ib.Channels.WebphoneEvent(ctx, e.org, user, event); err != nil {
				t.Fatal(err)
			}
			if err = e.ib.Receive(ctx, inst, result); err != nil {
				t.Fatal(err)
			}
			var count int
			if err = e.pool.QueryRow(ctx,
				"SELECT count(*) FROM channel_voice_calls WHERE organization_id=$1 AND channel_instance_id=$2",
				e.org,
				inst.ID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("merge kept %d calls: %v", count, err)
			}
			call, err := db.New(e.pool).GetChannelVoiceCall(ctx,
				db.GetChannelVoiceCallParams{OrganizationID: e.org,
					ChannelInstanceID: inst.ID,
					ExternalID:        id})
			if err != nil || !call.AgentUserID.Valid || !call.ClientEventID.Valid || call.Status != "completed" || call.InternalNumber != "101" {
				t.Fatalf("merged %+v %v", call, err)
			}
			if _, err = e.pool.Exec(ctx,
				"DELETE FROM channel_voice_calls WHERE organization_id=$1 AND channel_instance_id=$2",
				e.org,
				inst.ID); err != nil {
				t.Fatal(err)
			}
			event.EventID = uuid.New()
		})
	}
	var contacts int
	if err = e.pool.QueryRow(ctx, "SELECT count(*) FROM contacts WHERE organization_id=$1", e.org).Scan(&contacts); err != nil || contacts != 1 {
		t.Fatalf("last10 contact match: %d %v", contacts, err)
	}
	missed, err := adapter.ParseWebhook(ctx, nil, []byte(`{"scenario":"cdr","unique_id":"missed-fixture","customer_num":"05551112233","internal_num":"101","yon":"1","sondurum":"2"}`), inst)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.ib.Receive(ctx, inst, missed); err != nil {
		t.Fatal(err)
	}
	q := db.New(e.pool)
	claim := db.ClaimMissedNetgsmCallParams{OrganizationID: e.org, ChannelInstanceID: inst.ID, ExternalID: "missed-fixture"}
	recipients, err := q.ClaimMissedNetgsmCall(ctx, claim)
	if err != nil || len(recipients) != 1 || recipients[0] != e.agent {
		t.Fatalf("missed recipients: %+v %v", recipients, err)
	}
	recipients, err = q.ClaimMissedNetgsmCall(ctx, claim)
	if err != nil || len(recipients) != 0 {
		t.Fatal("duplicate missed notification")
	}
	for _, body := range []string{"broken", `{"scenario":"unknown"}`, `{"scenario":"Answer"}`} {
		r := httptest.NewRequest("POST",
			view.WebhookPath+"?token=test-voice-callback-secret-32-chars",
			strings.NewReader(body))
		w := httptest.NewRecorder()
		e.mux.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("ack=%d", w.Code)
		}
	}
	r := httptest.NewRequest("POST", view.WebhookPath+"?token=wrong", strings.NewReader("broken"))
	w := httptest.NewRecorder()
	e.mux.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("bad token accepted")
	}
}

type recordingHTTP func(*http.Request) (*http.Response, error)

func (f recordingHTTP) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type recordingStore struct {
	pool   *pgxpool.Pool
	fail   bool
	writes int
}

func (s *recordingStore) StoreInbound(ctx context.Context, org int64, message uuid.UUID, name, mime string, body io.Reader) error {
	if s.fail {
		s.fail = false
		return fmt.Errorf("fixture temporary storage failure")
	}
	b, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.writes++
	_, err = s.pool.Exec(ctx, "INSERT INTO attachments(uuid,organization_id,owner_type,owner_id,filename,content_type,size_bytes,sha256,object_key,scan_status) VALUES($1,$2,'message',$3,$4,$5,$6,'fixture-checksum','fixture-recording','clean')", uuid.New(), org, message, name, mime, len(b))
	return err
}
func TestNetgsmRecordingRetryAndCF122Bridge(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `{"code":"00","balance":"100"}`) }))
	defer server.Close()
	client := &http.Client{Transport: recordingHTTP(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: 4, Header: http.Header{"Content-Type": {"audio/wav"}}, Body: io.NopCloser(strings.NewReader("RIFF")), Request: r}, nil
	})}
	adapter := voice.New(voice.Config{NetgsmURL: server.URL, RecordingClient: client})
	e.ib.Registry.Register(adapter)
	files := &recordingStore{pool: e.pool, fail: true}
	e.ib.Conversations.WithAttachments(files)
	view, err := e.ib.Channels.Create(ctx, channelsusecase.CreateInput{OrganizationID: e.org, ActorID: e.agent, Type: channels.TypeVoice, Name: "Recording fixture", Credentials: channels.Credentials{"provider": "netgsm", "usercode": "fixture", "password": "fixture", "webhook_token": "test-voice-callback-secret-32-chars"}})
	if err != nil {
		t.Fatal(err)
	}
	inst, _, err := e.ib.Channels.Open(ctx, e.org, view.UUID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ParseWebhook(ctx, nil, []byte(`{"scenario":"cdr","unique_id":"archive-fixture","customer_num":"05551112233","internal_num":"101","yon":"1","sondurum":"1","talktime":15,"seskaydi":"https://record.netgsm.com.tr/fixture.wav"}`), inst)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.ib.Receive(ctx, inst, result); err != nil {
		t.Fatal(err)
	}
	if err = e.ib.ArchiveNetgsm(ctx, e.org, inst.ID, "archive-fixture"); err == nil {
		t.Fatal("storage failure was swallowed")
	}
	call, err := db.New(e.pool).GetChannelVoiceCall(ctx, db.GetChannelVoiceCallParams{OrganizationID: e.org, ChannelInstanceID: inst.ID, ExternalID: "archive-fixture"})
	if err != nil || call.RecordingMessageID.Valid {
		t.Fatal("recording linked before upload")
	}
	if err = e.ib.ArchiveNetgsm(ctx, e.org, inst.ID, "archive-fixture"); err != nil {
		t.Fatal(err)
	}
	if err = e.ib.ArchiveNetgsm(ctx, e.org, inst.ID, "archive-fixture"); err != nil {
		t.Fatal(err)
	}
	if files.writes != 1 {
		t.Fatal("duplicate recording upload")
	}
	var transcripts int
	err = e.pool.QueryRow(ctx, "SELECT count(*) FROM recording_transcripts WHERE organization_id=$1 AND voice_call_id=$2 AND status='pending'", e.org, call.ID).Scan(&transcripts)
	if err != nil || transcripts != 1 {
		t.Fatalf("CF122 trigger: %d %v", transcripts, err)
	}
}
