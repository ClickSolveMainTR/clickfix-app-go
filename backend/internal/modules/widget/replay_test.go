package widget_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/retention"
	uc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
)

type replayStorage struct {
	storage.Driver
	captureStore
}

func (s *replayStorage) Upload(c context.Context, f storage.File, k string) error {
	return s.captureStore.Upload(c, f, k)
}
func (s *replayStorage) Download(c context.Context, k string) (io.ReadCloser, int64, error) {
	return s.captureStore.Download(c, k)
}
func (s *replayStorage) Delete(c context.Context, k string) error { return s.captureStore.Delete(c, k) }
func (s *replayStorage) ListVersions(context.Context, string) ([]storage.VersionInfo, error) {
	return nil, nil
}

type replayEnt struct {
	deny  bool
	bytes int64
}

func (e *replayEnt) Require(context.Context, int64, string) error {
	if e.deny {
		return entitlement.ErrDenied
	}
	return nil
}
func (e *replayEnt) RequireWritable(context.Context, int64) error { return nil }
func (e *replayEnt) CheckLimit(_ context.Context, _ int64, key string, used, delta int64) (entitlement.Decision, error) {
	limit := int64(10000)
	if key == "replay.bytes" {
		limit = e.bytes
	}
	return entitlement.Check(entitlement.Value{Kind: "limit", Feature: key, Limit: limit}, used, delta)
}
func gzdata(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, e := w.Write([]byte(s)); e != nil {
		t.Fatal(e)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestReplayMaskAndDecompressionBounds(t *testing.T) {
	clean, err := uc.ValidateReplay(gzdata(t, `[{"type":2,"timestamp":123,"data":{"node":{"type":2,"tagName":"div","attributes":{"title":"secret@example.test","onclick":"alert(1)"},"childNodes":[{"type":3,"textContent":"secret@example.test"}]}},"href":"https://x.test?token=secret"}]`))
	if err != nil || bytes.Contains(clean, []byte("secret")) || bytes.Contains(clean, []byte("alert")) {
		t.Fatalf("mask: %s %v", clean, err)
	}
	for _, b := range [][]byte{[]byte("bad"), gzdata(t, strings.Repeat("x", uc.MaxReplayExpanded+1)), gzdata(t, `[{"type":88,"timestamp":1}]`), gzdata(t, `[]`)} {
		if _, err := uc.ValidateReplay(b); err == nil {
			t.Fatal("accepted malformed/unbounded replay")
		}
	}
}
func TestReplayConsentQuotaSigningRevocationAndErasure(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	q := db.New(e.pool)
	store := &replayStorage{captureStore: captureStore{objects: map[string][]byte{}}}
	ent := &replayEnt{bytes: 4 << 20}
	issuer, err := realtime.NewTokenIssuer(config.CentrifugoConfig{Enabled: true, TokenHMAC: "0123456789abcdef0123456789abcdef", WSURL: "wss://realtime.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	svc := uc.New(uc.Deps{Queries: q, Store: convrepo.New(e.pool, q), Instances: e.ib.Channels, Pipeline: e.ib.Conversations, Signer: widget.NewSigner("0123456789abcdef0123456789abcdef"), Publisher: e.pub, Issuer: issuer}).WithReplay(store, ent, "0123456789abcdef0123456789abcdef")
	wd, err := svc.Open(ctx, e.inst.UUID, siteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	token := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	claims, err := svc.Authenticate(wd, token)
	if err != nil {
		t.Fatal(err)
	}
	created := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "Replay", "client_id": uuid.New(), "name": "Visitor"})
	if created.Code != 201 {
		t.Fatalf("create %+v", created)
	}
	convID := uuid.MustParse(str(created.Data, "conversation_uuid"))
	if _, err := svc.StartReplay(ctx, wd, claims, convID, true); !errors.Is(err, uc.ErrUploadDisabled) {
		t.Fatalf("opt in: %v", err)
	}
	wd.Instance.Settings["replay"] = map[string]any{"replay_enabled": true, "cobrowse_enabled": true}
	if _, err := svc.StartReplay(ctx, wd, claims, convID, false); !errors.Is(err, uc.ErrInvalid) {
		t.Fatalf("consent: %v", err)
	}
	ent.deny = true
	if _, err := svc.StartReplay(ctx, wd, claims, convID, true); !errors.Is(err, entitlement.ErrDenied) {
		t.Fatalf("plan: %v", err)
	}
	ent.deny = false
	session, err := svc.StartReplay(ctx, wd, claims, convID, true)
	if err != nil {
		t.Fatal(err)
	}
	row, err := svc.VisitorSession(ctx, wd, claims, convID, session.UUID)
	if err != nil {
		t.Fatal(err)
	}
	data := gzdata(t, `[{"type":2,"timestamp":123,"data":{"node":{"tagName":"div","textContent":"PII email"}}}]`)
	ent.bytes = 1
	if err := svc.UploadReplay(ctx, wd, row, 0, data); !errors.Is(err, entitlement.ErrDenied) {
		t.Fatalf("quota %v", err)
	}
	ent.bytes = 4 << 20
	if err := svc.UploadReplay(ctx, wd, row, 1, data); !errors.Is(err, uc.ErrInvalid) {
		t.Fatalf("sequence %v", err)
	}
	if err := svc.UploadReplay(ctx, wd, row, 0, data); err != nil {
		t.Fatal(err)
	}
	if err := svc.UploadReplay(ctx, wd, row, 0, data); !errors.Is(err, uc.ErrInvalid) {
		t.Fatalf("duplicate %v", err)
	}
	for _, b := range store.objects {
		z, er := gzip.NewReader(bytes.NewReader(b))
		if er != nil {
			t.Fatal(er)
		}
		raw, _ := io.ReadAll(z)
		_ = z.Close()
		if bytes.Contains(raw, []byte("email")) {
			t.Fatal("stored unmasked data")
		}
	}
	manifest, err := svc.Manifest(ctx, row)
	if err != nil || len(manifest.Chunks) != 1 {
		t.Fatalf("manifest %+v %v", manifest, err)
	}
	signed := strings.TrimPrefix(manifest.Chunks[0].URL, "/api/v1/public/widget-replays/")
	body, _, err := svc.ReplayDownload(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
	if _, _, err := svc.ReplayDownload(ctx, signed+"x"); err == nil {
		t.Fatal("accepted tampered signature")
	}
	other := claims
	other.Visitor = uuid.NewString()
	if _, err := svc.VisitorSession(ctx, wd, other, convID, row.Uuid); !errors.Is(err, uc.ErrNotFound) {
		t.Fatalf("visitor isolation %v", err)
	}
	if _, err := svc.ChangeSession(ctx, wd, row, "revoke"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ReplayDownload(ctx, signed); err == nil {
		t.Fatal("signed replay survived revocation")
	}
	if err := (retention.Service{Q: q, Store: store}).Purge(ctx); err != nil {
		t.Fatal(err)
	}
	if len(store.objects) != 0 {
		t.Fatal("retention left bytes")
	}
	// A separate session verifies immediate invalidation by the KVKK trigger.
	session, err = svc.StartReplay(ctx, wd, claims, convID, true)
	if err != nil {
		t.Fatal(err)
	}
	row, err = svc.VisitorSession(ctx, wd, claims, convID, session.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UploadReplay(ctx, wd, row, 0, data); err != nil {
		t.Fatal(err)
	}
	manifest, err = svc.Manifest(ctx, row)
	if err != nil {
		t.Fatal(err)
	}
	signed = strings.TrimPrefix(manifest.Chunks[0].URL, "/api/v1/public/widget-replays/")
	if _, err := e.pool.Exec(ctx, "UPDATE conversations SET anonymized_at=now() WHERE organization_id=$1 AND uuid=$2", e.org, convID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ReplayDownload(ctx, signed); err == nil {
		t.Fatal("signed replay survived erasure")
	}
	if err := (retention.Service{Q: q, Store: store}).Purge(ctx); err != nil {
		t.Fatal(err)
	}
	if len(store.objects) != 0 {
		t.Fatal("erasure left bytes")
	}
}
func TestCobrowseAcceptanceLiveStreamAndTimeout(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	q := db.New(e.pool)
	store := &replayStorage{captureStore: captureStore{objects: map[string][]byte{}}}
	issuer, _ := realtime.NewTokenIssuer(config.CentrifugoConfig{Enabled: true, TokenHMAC: "0123456789abcdef0123456789abcdef"})
	svc := uc.New(uc.Deps{Queries: q, Store: convrepo.New(e.pool, q), Instances: e.ib.Channels, Pipeline: e.ib.Conversations, Signer: widget.NewSigner("0123456789abcdef0123456789abcdef"), Publisher: e.pub, Issuer: issuer}).WithReplay(store, &replayEnt{bytes: 4 << 20}, "0123456789abcdef0123456789abcdef")
	if _, err := e.pool.Exec(ctx, `UPDATE channel_instances SET settings=settings || '{"replay":{"replay_enabled":true,"cobrowse_enabled":true}}'::jsonb WHERE organization_id=$1 AND uuid=$2`, e.org, e.inst.UUID); err != nil {
		t.Fatal(err)
	}
	wd, err := svc.Open(ctx, e.inst.UUID, siteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	token := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	claims, _ := svc.Authenticate(wd, token)
	created := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "Co-browse", "client_id": uuid.New(), "name": "Visitor"})
	if created.Code != 201 {
		t.Fatalf("create %+v", created)
	}
	convID := uuid.MustParse(str(created.Data, "conversation_uuid"))
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: e.org, Permissions: []string{rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReadAll, rbac.PermTenantCobrowseUse}})
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: e.agent})
	session, err := svc.RequestCobrowse(ctx, convID, false)
	if err != nil {
		t.Fatal(err)
	}
	row, err := svc.VisitorSession(ctx, wd, claims, convID, session.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LiveTokens(row); err == nil {
		t.Fatal("token before consent")
	}
	data := gzdata(t, `[{"type":2,"timestamp":123}]`)
	if err := svc.UploadReplay(ctx, wd, row, 0, data); !errors.Is(err, uc.ErrConversationClosed) {
		t.Fatalf("stream before consent %v", err)
	}
	if _, err := svc.ChangeSession(ctx, wd, row, "accept"); err != nil {
		t.Fatal(err)
	}
	row, err = svc.VisitorSession(ctx, wd, claims, convID, session.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LiveTokens(row); err != nil {
		t.Fatal(err)
	}
	if err := svc.UploadReplay(ctx, wd, row, 0, data); err != nil {
		t.Fatal(err)
	}
	if len(store.objects) != 0 {
		t.Fatal("persisted live DOM")
	}
	if err := svc.AgentPointer(ctx, wd, row, uc.Pointer{Type: "control_click", X: 10, Y: 10}); !errors.Is(err, uc.ErrUnauthorized) {
		t.Fatalf("control default %v", err)
	}
	if err := svc.AgentPointer(ctx, wd, row, uc.Pointer{Type: "pointer", X: 10, Y: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChangeSession(ctx, wd, row, "accept"); !errors.Is(err, uc.ErrInvalid) {
		t.Fatalf("accept twice %v", err)
	}
	foreign := orgctx.WithScope(ctx, orgctx.Scope{ID: e.org + 1000000000, Permissions: []string{rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReadAll}})
	if _, _, err := svc.AgentSession(foreign, convID, row.Uuid); !errors.Is(err, uc.ErrNotFound) {
		t.Fatalf("cross org session %v", err)
	}
	svc.WithClock(func() time.Time { return time.Now().Add(16 * time.Minute) })
	if _, err := svc.LiveTokens(row); err == nil {
		t.Fatal("token after timeout")
	}
	if err := svc.UploadReplay(ctx, wd, row, 1, data); !errors.Is(err, uc.ErrConversationClosed) {
		t.Fatalf("upload after timeout %v", err)
	}
	svc.WithClock(time.Now)
	if _, err := svc.ChangeSession(ctx, wd, row, "revoke"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AgentPointer(ctx, wd, row, uc.Pointer{Type: "pointer", X: 10, Y: 10}); !errors.Is(err, uc.ErrInvalid) {
		t.Fatalf("stale pointer after revocation %v", err)
	}

}
