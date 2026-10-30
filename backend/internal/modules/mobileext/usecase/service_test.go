package usecase

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apns"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestHashTokenIsSHA256Hex(t *testing.T) {
	// sha256("abc")
	if got := HashToken("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("hash %s", got)
	}
}

func TestIssueWidgetToken(t *testing.T) {
	store := newFakeStore()
	svc := New(store, nil, nil)
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	session := uuid.New()

	out, err := svc.IssueWidgetToken(context.Background(), IssueInput{UserID: 7, OrganizationID: 3, SessionID: session, DeviceID: "  install-1  "})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(out.Token)
	if err != nil || len(raw) != 32 || len(out.Token) != widgetTokenLen {
		t.Fatalf("token %q (%d bytes, err %v)", out.Token, len(raw), err)
	}
	if !out.ExpiresAt.Equal(now.Add(72 * time.Hour)) {
		t.Fatalf("expires %v", out.ExpiresAt)
	}
	got := store.issued[0]
	if got.TokenHash != HashToken(out.Token) || got.TokenHash == out.Token {
		t.Fatal("only the hash must be stored")
	}
	if got.DeviceID != "install-1" || got.UserID != 7 || got.OrganizationID != 3 {
		t.Fatalf("params %+v", got)
	}
	if !got.SessionUuid.Valid || uuid.UUID(got.SessionUuid.Bytes) != session {
		t.Fatalf("session %+v", got.SessionUuid)
	}

	second, _ := svc.IssueWidgetToken(context.Background(), IssueInput{UserID: 7, OrganizationID: 3, SessionID: session, DeviceID: "install-1"})
	if second.Token == out.Token {
		t.Fatal("tokens must be random")
	}
}

func TestIssueWidgetTokenValidation(t *testing.T) {
	svc := New(newFakeStore(), nil, nil)
	ctx := context.Background()
	var ve ValidationError
	for _, device := range []string{"", "   ", strings.Repeat("x", 129), "bad\x00id"} {
		_, err := svc.IssueWidgetToken(ctx, IssueInput{UserID: 1, OrganizationID: 1, SessionID: uuid.New(), DeviceID: device})
		if !errors.As(err, &ve) || ve["device_id"] == "" {
			t.Fatalf("device %q: %v", device, err)
		}
	}
	if _, err := svc.IssueWidgetToken(ctx, IssueInput{UserID: 1, OrganizationID: 1, DeviceID: "d"}); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("no session: %v", err)
	}
}

func TestAuthenticateWidget(t *testing.T) {
	store := newFakeStore()
	svc := New(store, nil, nil)
	ctx := context.Background()
	out, _ := svc.IssueWidgetToken(ctx, IssueInput{UserID: 9, OrganizationID: 4, SessionID: uuid.New(), DeviceID: "d"})
	user, session := uuid.New(), uuid.New()
	store.valid[HashToken(out.Token)] = db.GetValidWidgetTokenRow{
		ID: 55, UserID: 9, OrganizationID: 4, UserUuid: user,
		SessionUuid: pgtype.UUID{Bytes: session, Valid: true},
	}

	id, err := svc.AuthenticateWidget(ctx, out.Token)
	if err != nil {
		t.Fatal(err)
	}
	if id.UserID != 9 || id.OrganizationID != 4 || id.UserUUID != user || id.SessionID != session || id.TokenID != 55 {
		t.Fatalf("identity %+v", id)
	}
	if len(store.touched) != 1 {
		t.Fatalf("touched %v", store.touched)
	}

	// Recently used: no extra write.
	row := store.valid[HashToken(out.Token)]
	row.LastUsedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	store.valid[HashToken(out.Token)] = row
	if _, err := svc.AuthenticateWidget(ctx, out.Token); err != nil || len(store.touched) != 1 {
		t.Fatalf("touch throttling: %v %v", err, store.touched)
	}

	for _, raw := range []string{"", "short", strings.Repeat("A", 43), out.Token + "x", strings.Repeat("!", 43)} {
		if _, err := svc.AuthenticateWidget(ctx, raw); !errors.Is(err, ErrInvalidWidgetToken) {
			t.Fatalf("%q: %v", raw, err)
		}
	}
}

func TestSessionRotatedRebinds(t *testing.T) {
	store := newFakeStore()
	svc := New(store, nil, nil)
	from, to := uuid.New(), uuid.New()
	if err := svc.SessionRotated(context.Background(), 1, from, to); err != nil {
		t.Fatal(err)
	}
	_ = svc.SessionRotated(context.Background(), 1, uuid.Nil, to)
	_ = svc.SessionRotated(context.Background(), 1, from, from)
	if len(store.rebinds) != 1 || store.rebinds[0] != [2]uuid.UUID{from, to} {
		t.Fatalf("rebinds %v", store.rebinds)
	}
}

func TestValidateLiveActivityToken(t *testing.T) {
	ok := LiveActivityTokenInput{
		DeviceID: "dev", Kind: KindPushToStart, ActivityType: ActivityUrgentConversation,
		Token: strings.Repeat("AB", 32), Environment: "sandbox", Ref: " r ",
	}
	p, errs := ValidateLiveActivityToken(ok)
	if errs != nil {
		t.Fatal(errs)
	}
	if p.Token != strings.Repeat("ab", 32) || p.Ref != "r" {
		t.Fatalf("normalized %+v", p)
	}
	_, errs = ValidateLiveActivityToken(LiveActivityTokenInput{
		Kind: "x", ActivityType: "y", Token: "zz", Environment: "dev",
		ActivityID: strings.Repeat("a", 129), Ref: strings.Repeat("r", 65),
	})
	for _, f := range []string{"device_id", "kind", "activity_type", "token", "environment", "activity_id", "ref"} {
		if errs[f] == "" {
			t.Errorf("missing error for %s", f)
		}
	}
	if _, errs := ValidateLiveActivityToken(LiveActivityTokenInput{DeviceID: "d", Kind: KindUpdate, ActivityType: ActivityCall,
		Token: strings.Repeat("a", 31), Environment: "production"}); errs["token"] == "" {
		t.Error("31-char token must fail")
	}
}

func TestRegisterLiveActivityTokenAndSettings(t *testing.T) {
	store := newFakeStore()
	svc := New(store, nil, nil)
	ctx := context.Background()
	err := svc.RegisterLiveActivityToken(ctx, 5, LiveActivityTokenInput{DeviceID: "d", Kind: KindUpdate,
		ActivityType: ActivityUrgentConversation, Token: strings.Repeat("0f", 40), Environment: "production"})
	if err != nil || len(store.upserts) != 1 || store.upserts[0].UserID != 5 {
		t.Fatalf("register: %v %+v", err, store.upserts)
	}
	got, _ := svc.GetLiveActivitySettings(ctx, 5)
	if !got.Enabled {
		t.Fatal("default must be enabled")
	}
	if _, err := svc.SetLiveActivitySettings(ctx, 5, LiveActivitySettings{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetLiveActivitySettings(ctx, 5); got.Enabled {
		t.Fatal("must be off")
	}
}

func TestNewTreatsTypedNilSenderAsOff(t *testing.T) {
	var c *apns.Client
	if New(newFakeStore(), c, nil).PushConfigured() {
		t.Fatal("typed nil sender must be off")
	}
	if !New(newFakeStore(), &fakeSender{}, nil).PushConfigured() {
		t.Fatal("sender must be on")
	}
}
