package widget_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	attachmentrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/repository"
	attachmentuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/usecase"
	convrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/retention"
	widgetuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

func TestDiagnosticVisitorIsolationMaskingQuotaAndExpiry(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	token := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	created := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "Support", "client_id": uuid.New(), "name": "Visitor"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create=%+v", created)
	}
	id := str(created.Data, "conversation_uuid")
	path := "/conversations/" + id + "/diagnostics"
	in := map[string]any{"consent": true, "context": map[string]any{"url": "https://shop.test/account/123?token=secret", "console_errors": []string{"ada@example.com password=secret"}, "metadata": map[string]any{"email": "ada@example.com"}}}
	if r := e.call(http.MethodPost, path, token, siteOrigin, map[string]any{"consent": false}); r.Code != http.StatusBadRequest {
		t.Fatalf("no consent=%d %v", r.Code, r.Err)
	}
	other := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	if r := e.call(http.MethodPost, path, other, siteOrigin, in); r.Code != http.StatusNotFound {
		t.Fatalf("foreign visitor=%d", r.Code)
	}
	captured := e.call(http.MethodPost, path, token, siteOrigin, in)
	if captured.Code != http.StatusCreated {
		t.Fatalf("capture=%+v", captured)
	}
	raw, _ := json.Marshal(captured.Data)
	if strings.Contains(string(raw), "ada@example.com") || strings.Contains(string(raw), "password=secret") {
		t.Fatalf("PII=%s", raw)
	}
	convID, _ := uuid.Parse(id)
	visible, err := e.ib.Conversations.Diagnostics(ctx, e.org, convID)
	if err != nil || len(visible) != 1 {
		t.Fatalf("agent read=%v %v", visible, err)
	}
	// An explicitly restricted viewer from another team gets no diagnostic access.
	restricted := access.WithViewer(ctx, access.Viewer{UserID: -1, All: false})
	if _, err := e.pool.Exec(ctx, "UPDATE conversations SET assignee_user_id=$1 WHERE uuid=$2", e.agent, convID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ib.Conversations.Diagnostics(restricted, e.org, convID); err == nil {
		t.Fatal("restricted viewer accessed diagnostic")
	}
	foreignOrg := orgctx.WithScope(ctx, orgctx.Scope{ID: -1})
	if _, err := e.ib.Conversations.Diagnostics(foreignOrg, -1, convID); err == nil {
		t.Fatal("foreign organization accessed diagnostic")
	}
	// Both the listing and byte access expire at 30 days; the worker subsequently purges storage.
	var expiry time.Time
	if err := e.pool.QueryRow(ctx, "SELECT expires_at FROM widget_diagnostics WHERE organization_id=$1 AND uuid=$2", e.org, captured.Data["uuid"]).Scan(&expiry); err != nil || time.Until(expiry) < 29*24*time.Hour {
		t.Fatalf("expiry=%v %v", expiry, err)
	}
	if _, err := e.pool.Exec(ctx, "UPDATE widget_diagnostics SET expires_at=now()-interval '1 second' WHERE organization_id=$1", e.org); err != nil {
		t.Fatal(err)
	}
	visible, err = e.ib.Conversations.Diagnostics(ctx, e.org, convID)
	if err != nil || len(visible) != 0 {
		t.Fatalf("expired read=%v %v", visible, err)
	}
	// Seed at the conversation cap; next API submission is rate/quota limited.
	q := db.New(e.pool)
	conv, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: e.org, Uuid: convID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, "INSERT INTO widget_diagnostics(organization_id,conversation_id,kind) SELECT $1,$2,'technical_context' FROM generate_series(1,100)", e.org, conv.ID); err != nil {
		t.Fatal(err)
	}
	if r := e.call(http.MethodPost, path, token, siteOrigin, in); r.Code != http.StatusTooManyRequests {
		t.Fatalf("quota=%d %v", r.Code, r.Err)
	}
}

type captureStore struct{ objects map[string][]byte }

func (s *captureStore) Upload(_ context.Context, f storage.File, key string) error {
	b, err := io.ReadAll(f.Body)
	if err == nil {
		s.objects[key] = b
	}
	return err
}
func (s *captureStore) Download(_ context.Context, key string) (io.ReadCloser, int64, error) {
	b, ok := s.objects[key]
	if !ok {
		return nil, 0, errors.New("missing")
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}
func (s *captureStore) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}
func TestScreenshotAttachmentQuotaAndRetention(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	q := db.New(e.pool)
	store := &captureStore{objects: map[string][]byte{}}
	files := attachmentuc.New(attachmentrepo.NewPostgres(e.pool, q), store, "capture-test-signing-key", config.AttachmentConfig{MaxFileBytes: 10 << 20, OrgQuotaBytes: 10 << 20}, nil)
	svc := widgetuc.New(widgetuc.Deps{Queries: q, Store: convrepo.New(e.pool, q), Instances: e.ib.Channels, Pipeline: e.ib.Conversations, Files: files, Signer: widget.NewSigner("0123456789abcdef0123456789abcdef"), Publisher: e.pub})
	wd, err := svc.Open(ctx, e.inst.UUID, siteOrigin)
	if err != nil {
		t.Fatal(err)
	}
	token := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	claims, err := svc.Authenticate(wd, token)
	if err != nil {
		t.Fatal(err)
	}
	created := e.call(http.MethodPost, "/conversations", token, siteOrigin, map[string]any{"text": "Capture", "client_id": uuid.New(), "name": "Visitor"})
	if created.Code != 201 {
		t.Fatalf("create=%+v", created)
	}
	id, err := uuid.Parse(str(created.Data, "conversation_uuid"))
	if err != nil {
		t.Fatal(err)
	}
	var pngBody bytes.Buffer
	if err := png.Encode(&pngBody, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	in := widgetuc.DiagnosticInput{Kind: "screenshot", Consent: true, PIIMasked: true, Body: bytes.NewReader(pngBody.Bytes())}
	capture, err := svc.Capture(ctx, wd, claims, id, in)
	if err != nil || capture.AttachmentID == nil {
		t.Fatalf("capture=%+v err=%v", capture, err)
	}
	if _, err := files.Get(ctx, e.org, *capture.AttachmentID); err != nil {
		t.Fatal(err)
	}
	signed, err := files.SignedURL(ctx, e.org, *capture.AttachmentID, "original", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Both attachment lookup and an already-issued public URL stop working after retention.
	if _, err := e.pool.Exec(ctx, "UPDATE attachments SET expires_at=now()-interval '1 second' WHERE organization_id=$1 AND uuid=$2", e.org, *capture.AttachmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Get(ctx, e.org, *capture.AttachmentID); err == nil {
		t.Fatal("expired attachment accessible")
	}
	if _, err := files.OpenSigned(ctx, strings.TrimPrefix(signed.URL, "/v1/public/attachments/")); err == nil {
		t.Fatal("expired signed URL accessible")
	}
	if _, err := e.pool.Exec(ctx, "UPDATE widget_diagnostics SET expires_at=now()-interval '1 second' WHERE organization_id=$1 AND uuid=$2", e.org, capture.UUID); err != nil {
		t.Fatal(err)
	}
	if err := (retention.Service{Q: q, Store: store}).Purge(ctx); err != nil {
		t.Fatal(err)
	}
	if len(store.objects) != 0 {
		t.Fatalf("retention left %d objects", len(store.objects))
	}
	var count int
	if err := e.pool.QueryRow(ctx, "SELECT count(*) FROM widget_diagnostics WHERE organization_id=$1 AND uuid=$2", e.org, capture.UUID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("context retained: %d %v", count, err)
	}
	in.Body = bytes.NewReader(pngBody.Bytes())
	files.WithLimits(attachmentuc.StaticLimits{MaxFileBytes: 10 << 20, MaxTotalBytes: 1})
	if _, err := svc.Capture(ctx, wd, claims, id, in); !errors.Is(err, widgetuc.ErrDiagnosticQuota) {
		t.Fatalf("attachment quota: %v", err)
	}
	in.Body = strings.NewReader(strings.Repeat("a", widgetuc.MaxScreenshotBytes+1))
	if _, err := svc.Capture(ctx, wd, claims, id, in); !errors.Is(err, widgetuc.ErrFileTooLarge) {
		t.Fatalf("size cap: %v", err)
	}
	in.Kind = "recording"
	in.Body = strings.NewReader("not video")
	if _, err := svc.Capture(ctx, wd, claims, id, in); !errors.Is(err, widgetuc.ErrInvalid) {
		t.Fatalf("recording validation: %v", err)
	}
}
