package videorooms_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/videorooms"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const lkSecret = "test-livekit-secret-0123456789abcdef"

type fixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	q       *db.Queries
	billing *usecase.Service
	svc     *videorooms.Service
	org     int64
	actor   int64
}

func setup(t *testing.T, minutes int64) *fixture {
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
	f := &fixture{ctx: ctx, pool: pool, q: db.New(pool)}
	repo := repository.New(pool, f.q)
	f.billing = usecase.New(repo)
	var orgUUID uuid.UUID
	if err = pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname) VALUES($1,'test-hash','Video','Host') RETURNING id", uuid.NewString()+"@example.test").Scan(&f.actor); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Video test') RETURNING id,uuid", uuid.NewString()).Scan(&f.org, &orgUUID); err != nil {
		t.Fatal(err)
	}
	plan, err := f.billing.Create(ctx, model.PlanInput{Code: "video-" + uuid.NewString(), Name: "Video", Visibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	features, err := f.billing.Features(ctx)
	if err != nil {
		t.Fatal(err)
	}
	in := model.VersionInput{Prices: []model.Price{{Interval: "monthly", Currency: "TRY", Amount: 10000, VAT: 2000}}}
	for _, feature := range features {
		g := model.Grant{Feature: feature.Key, Enforcement: "hard", WarnPercent: 80}
		if feature.Kind == "boolean" {
			v := true
			g.Enabled = &v
		} else {
			v := int64(10)
			switch feature.Key {
			case "ai.credits":
				v = 0
			case "video.minutes":
				v = minutes
			}
			g.Limit = &v
		}
		in.Entitlements = append(in.Entitlements, g)
	}
	v, err := f.billing.SaveVersion(ctx, plan.UUID, uuid.Nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if v, err = f.billing.Publish(ctx, plan.UUID, v.UUID, f.actor); err != nil {
		t.Fatal(err)
	}
	if _, err = f.billing.CreateSubscription(ctx, model.CreateSubscription{OrganizationUUID: orgUUID, VersionUUID: v.UUID, PriceUUID: v.Prices[0].UUID, Status: "active",
		PeriodStart: time.Now().Add(-time.Hour), PeriodEnd: time.Now().Add(24 * time.Hour)}, f.actor); err != nil {
		t.Fatal(err)
	}
	lk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	t.Cleanup(lk.Close)
	box, err := crypto.NewSecretBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	f.svc = &videorooms.Service{
		Q: f.q, Box: box, Signer: videorooms.NewSigner("test"), Wallet: f.billing, Entitlements: entitlements.New(repo, nil, "video-test-"+uuid.NewString()),
		LiveKit:     livekit.New(livekit.Config{URL: "wss://video.example.test", APIURL: lk.URL, APIKey: "key", APISecret: lkSecret}),
		FrontendURL: "https://app.example.test", Log: slog.New(slog.DiscardHandler),
	}
	p, err := f.svc.PlatformPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Enabled {
		p.Enabled = true
		if _, err = f.svc.SavePlatformPolicy(ctx, f.actor, p); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *fixture) webhook(t *testing.T, body string) {
	t.Helper()
	sum := sha256.Sum256([]byte(body))
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": "key", "exp": time.Now().Add(time.Minute).Unix(), "sha256": base64.StdEncoding.EncodeToString(sum[:])}).SignedString([]byte(lkSecret))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/v1/public/video/webhook", strings.NewReader(body))
	r.Header.Set("Authorization", tok)
	if err = f.svc.Webhook(f.ctx, r); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) connection(t *testing.T, room, identity string, minutes int) {
	t.Helper()
	sid := "PA_" + uuid.NewString()
	// One clock read: a second boundary between join and leave would add a
	// second and round the bill up a whole minute.
	end := time.Now().Unix()
	start := end - int64(minutes)*60
	f.webhook(t, fmt.Sprintf(`{"id":"EV_%s","event":"participant_joined","createdAt":"%d","room":{"name":%q},"participant":{"sid":%q,"identity":%q,"joinedAt":"%d"}}`, uuid.NewString(), start, room, sid, identity, start))
	left := fmt.Sprintf(`{"id":"EV_%s","event":"participant_left","createdAt":"%d","room":{"name":%q},"participant":{"sid":%q,"identity":%q}}`, uuid.NewString(), end, room, sid, identity)
	f.webhook(t, left)
	f.webhook(t, left) // LiveKit retries: the replay must not bill twice.
}

func (f *fixture) balance(t *testing.T, want int64) {
	t.Helper()
	got, err := f.billing.MeteredBalance(f.ctx, f.org, videorooms.UnitVideoMinutes)
	if err != nil || got != want {
		t.Fatalf("video minutes balance=%d want=%d err=%v", got, want, err)
	}
}

func TestGuestLobbyJoinAndMinuteMetering(t *testing.T) {
	f := setup(t, 10)
	f.balance(t, 10)
	room, err := f.svc.CreateRoom(f.ctx, f.org, f.actor, videorooms.CreateRoomInput{Name: "Destek görüşmesi", PIN: "4321"})
	if err != nil {
		t.Fatal(err)
	}
	if !room.E2EE || !room.Lobby || room.Recording || room.GuestLink == "" || room.PIN != "4321" {
		t.Fatalf("room defaults: %+v", room)
	}
	code := room.GuestLink[strings.LastIndex(room.GuestLink, "/")+1:]
	if _, err = f.svc.GuestJoin(f.ctx, code, videorooms.GuestJoinInput{Name: "Müşteri", PIN: "0000"}); !errors.Is(err, videorooms.ErrPINInvalid) {
		t.Fatalf("wrong pin: %v", err)
	}
	g, err := f.svc.GuestJoin(f.ctx, code, videorooms.GuestJoinInput{Name: "Müşteri", PIN: "4321"})
	if err != nil || g.Status != videorooms.StatusWaiting || g.Token != "" || g.E2EEKey != "" || g.Ticket == "" || g.Room.OrganizationName != "Video test" {
		t.Fatalf("lobby join leaked a grant or failed: %+v %v", g, err)
	}
	if _, err = f.svc.SetParticipant(f.ctx, f.org, f.actor, room.UUID, g.ParticipantUUID, "admit"); err != nil {
		t.Fatal(err)
	}
	admitted, err := f.svc.Ticket(f.ctx, g.Ticket)
	if err != nil || admitted.Status != videorooms.StatusAdmitted || admitted.Token == "" || admitted.E2EEKey == "" || admitted.ServerURL != "wss://video.example.test" {
		t.Fatalf("admitted grant: %+v %v", admitted, err)
	}
	var lkName string
	if err = f.pool.QueryRow(f.ctx, "SELECT livekit_name FROM video_rooms WHERE uuid=$1", room.UUID).Scan(&lkName); err != nil {
		t.Fatal(err)
	}
	// 2.5 minutes rounds up to 3 participant-minutes.
	f.connection(t, lkName, "g:a", 3)
	f.balance(t, 7)
	// 9 more minutes with 7 left: 7 billed, 2 recorded as unbilled, no debt.
	f.connection(t, lkName, "u:b", 9)
	f.balance(t, 0)
	got, err := f.svc.Room(f.ctx, f.org, room.UUID)
	if err != nil || got.BilledMinutes != 10 || got.UnbilledMinutes != 2 || got.Status != "live" {
		t.Fatalf("room billing: %+v %v", got, err)
	}
	if _, err = f.svc.CreateRoom(f.ctx, f.org, f.actor, videorooms.CreateRoomInput{Name: "Second"}); !errors.Is(err, videorooms.ErrNoMinutes) {
		t.Fatalf("empty wallet must block new rooms: %v", err)
	}
	if _, err = f.svc.Ticket(f.ctx, g.Ticket); !errors.Is(err, videorooms.ErrNoMinutes) {
		t.Fatalf("empty wallet must stop new grants: %v", err)
	}
	if err = f.svc.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ = f.svc.Room(f.ctx, f.org, room.UUID); got.Status != "ended" {
		t.Fatalf("sweep must end a live room without minutes, status=%s", got.Status)
	}
}

func TestRemovedGuestCannotWalkBackIn(t *testing.T) {
	f := setup(t, 30)
	no := false
	// The platform default requires a lobby; this plan's override lifts it.
	var plan uuid.UUID
	if err := f.pool.QueryRow(f.ctx, "SELECT p.uuid FROM billing_subscriptions s JOIN billing_plan_versions v ON v.id=s.plan_version_id JOIN billing_plans p ON p.id=v.plan_id WHERE s.organization_id=$1", f.org).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SavePlanPolicy(f.ctx, plan, videorooms.Partial{RequireLobby: &no}); err != nil {
		t.Fatal(err)
	}
	room, err := f.svc.CreateRoom(f.ctx, f.org, f.actor, videorooms.CreateRoomInput{Name: "Açık oda", Lobby: &no})
	if err != nil || room.Lobby {
		t.Fatalf("room without lobby: %+v %v", room, err)
	}
	code := room.GuestLink[strings.LastIndex(room.GuestLink, "/")+1:]
	g, err := f.svc.GuestJoin(f.ctx, code, videorooms.GuestJoinInput{Name: "Misafir"})
	if err != nil || g.Status != videorooms.StatusAdmitted || g.Token == "" {
		t.Fatalf("no-lobby join: %+v %v", g, err)
	}
	if _, err = f.svc.SetParticipant(f.ctx, f.org, f.actor, room.UUID, g.ParticipantUUID, "remove"); err != nil {
		t.Fatal(err)
	}
	again, err := f.svc.GuestJoin(f.ctx, code, videorooms.GuestJoinInput{Name: "Misafir"})
	if err != nil || again.Status != videorooms.StatusWaiting || again.Token != "" {
		t.Fatalf("a removed guest must wait for the host on the next join: %+v %v", again, err)
	}
}

func TestRecordableRoomIsNotEncryptedAndAgentLinksNeedLogin(t *testing.T) {
	f := setup(t, 30)
	if _, err := f.svc.CreateRoom(f.ctx, f.org, f.actor, videorooms.CreateRoomInput{Name: "Kayıt", Record: true}); !errors.Is(err, videorooms.ErrRecordingNotAllowed) {
		t.Fatalf("platform default forbids recording: %v", err)
	}
	room, err := f.svc.CreateRoom(f.ctx, f.org, f.actor, videorooms.CreateRoomInput{Name: "Ajan"})
	if err != nil {
		t.Fatal(err)
	}
	link, err := f.svc.Link(f.ctx, f.org, room.UUID, videorooms.LinkInput{Role: videorooms.LinkAgent, TTLMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.GuestJoin(f.ctx, link.Code, videorooms.GuestJoinInput{Name: "x"}); !errors.Is(err, videorooms.ErrLoginRequired) {
		t.Fatalf("agent link joined as guest: %v", err)
	}
	if _, err = f.svc.SaveOrgSettings(f.ctx, f.org, videorooms.OrgSettingsInput{Enabled: true, Storage: videorooms.StorageInput{Mode: "custom", Endpoint: "https://10.0.0.1", Bucket: "rec", AccessKey: "a", SecretKey: "b"}}); !errors.Is(err, videorooms.ErrInvalid) {
		t.Fatalf("private S3 endpoint accepted: %v", err)
	}
	view, err := f.svc.SaveOrgSettings(f.ctx, f.org, videorooms.OrgSettingsInput{Enabled: true, Storage: videorooms.StorageInput{Mode: "custom", Endpoint: "https://s3.eu-central-1.amazonaws.com", Region: "eu-central-1", Bucket: "acme-recordings", AccessKey: "AKIA", SecretKey: "secret"}})
	if err != nil || !view.Storage.HasSecretKey || view.Storage.Bucket != "acme-recordings" {
		t.Fatalf("custom storage: %+v %v", view.Storage, err)
	}
	var stored string
	if err = f.pool.QueryRow(f.ctx, "SELECT s3_secret_key_enc FROM video_org_settings WHERE organization_id=$1", f.org).Scan(&stored); err != nil || stored == "" || stored == "secret" {
		t.Fatalf("secret must be sealed at rest: %q %v", stored, err)
	}
}
