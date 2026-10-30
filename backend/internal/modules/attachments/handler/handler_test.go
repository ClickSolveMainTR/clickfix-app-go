package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type env struct {
	mux    *http.ServeMux
	store  *storage.Memory
	q      *db.Queries
	orgA   orgctx.Scope
	orgB   orgctx.Scope
	userID int64
}

// newEnv wires the real Postgres repository, a memory object store and the
// handlers behind a mux that injects the organization scope from the
// X-Test-Org header. It skips without TEST_DATABASE_URL (CI sets it).
func newEnv(t *testing.T, cfg config.AttachmentConfig) *env {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)

	suffix := uuid.NewString()[:8]
	mkOrg := func(name string) orgctx.Scope {
		org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: name + "-" + suffix, Name: name})
		if err != nil {
			t.Fatalf("create org: %v", err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, org.ID) })
		return orgctx.Scope{
			ID: org.ID, UUID: org.Uuid, Slug: org.Slug, Status: orgctx.StatusActive,
			Permissions: []string{"tenant.attachments.read", "tenant.attachments.write"},
		}
	}
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Test', 'User', 'active') RETURNING id`,
		"att-"+suffix+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) })

	e := &env{store: storage.NewMemory(), q: q, orgA: mkOrg("alpha"), orgB: mkOrg("bravo"), userID: userID}
	svc := usecase.New(repository.NewPostgres(pool, q), e.store, "handler-test-secret", cfg, nil)
	h := handler.New(svc, activity.NewRecorder(q, nil))

	scopes := map[string]orgctx.Scope{"a": e.orgA, "b": e.orgB}
	tenant := func(next http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := scopes[r.Header.Get("X-Test-Org")]
			if !ok {
				http.Error(w, "no scope", http.StatusForbidden)
				return
			}
			ctx := orgctx.WithScope(r.Context(), scope)
			ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: userID})
			next(w, r.WithContext(ctx))
		})
	}
	e.mux = http.NewServeMux()
	e.mux.Handle("GET /v1/tenant/attachments", tenant(h.List))
	e.mux.Handle("GET /v1/tenant/attachments/usage", tenant(h.Usage))
	e.mux.Handle("POST /v1/tenant/attachments", tenant(h.Upload))
	e.mux.Handle("GET /v1/tenant/attachments/{uuid}", tenant(h.Get))
	e.mux.Handle("GET /v1/tenant/attachments/{uuid}/download", tenant(h.Download))
	e.mux.Handle("POST /v1/tenant/attachments/{uuid}/signed-url", tenant(h.SignedURL))
	e.mux.Handle("DELETE /v1/tenant/attachments/{uuid}", tenant(h.Delete))
	e.mux.Handle("GET /v1/public/attachments/{token}", http.HandlerFunc(h.PublicDownload))
	return e
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (e *env) do(t *testing.T, org, method, path string, body io.Reader, contentType string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if org != "" {
		req.Header.Set("X-Test-Org", org)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	var env envelope
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
	}
	return rec, env
}

func multipartBody(t *testing.T, filename, partType string, content []byte, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	hdr.Set("Content-Type", partType)
	part, err := mw.CreatePart(hdr)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(content)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

type attachmentDTO struct {
	UUID         uuid.UUID  `json:"uuid"`
	Filename     string     `json:"filename"`
	ContentType  string     `json:"content_type"`
	SizeBytes    int64      `json:"size_bytes"`
	OwnerType    *string    `json:"owner_type"`
	OwnerID      *uuid.UUID `json:"owner_id"`
	UploadedBy   *uuid.UUID `json:"uploaded_by"`
	HasThumbnail bool       `json:"has_thumbnail"`
	DownloadURL  string     `json:"download_url"`
	ThumbnailURL *string    `json:"thumbnail_url"`
}

func (e *env) upload(t *testing.T, org, filename, partType string, content []byte, fields map[string]string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	body, ct := multipartBody(t, filename, partType, content, fields)
	return e.do(t, org, http.MethodPost, "/v1/tenant/attachments", body, ct)
}

func mustDecode[T any](t *testing.T, env envelope) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatalf("decode %s: %v", env.Data, err)
	}
	return v
}

func testPNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 640, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

var cfg = config.AttachmentConfig{MaxFileBytes: 1 << 20, OrgQuotaBytes: 0}

func TestUploadListDownloadDeleteFlow(t *testing.T) {
	e := newEnv(t, cfg)
	owner := uuid.New()

	rec, env := e.upload(t, "a", "foto.png", "image/png", testPNG(), map[string]string{
		"owner_type": "ticket", "owner_id": owner.String(),
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status %d: %s", rec.Code, rec.Body)
	}
	att := mustDecode[attachmentDTO](t, env)
	if att.Filename != "foto.png" || att.ContentType != "image/png" || !att.HasThumbnail {
		t.Fatalf("attachment = %+v", att)
	}
	if att.OwnerType == nil || *att.OwnerType != "ticket" || att.OwnerID == nil || *att.OwnerID != owner {
		t.Fatalf("owner = %v %v", att.OwnerType, att.OwnerID)
	}
	if att.UploadedBy == nil {
		t.Fatal("uploaded_by uuid missing")
	}
	// The response never leaks storage keys or bucket URLs.
	if strings.Contains(rec.Body.String(), "orgs/") || strings.Contains(rec.Body.String(), "object_key") {
		t.Fatalf("response leaks the object key: %s", rec.Body)
	}

	// Second file for another owner, then filter by owner.
	if rec, _ := e.upload(t, "a", "doc.pdf", "application/pdf", []byte("%PDF-1.4 x"), map[string]string{
		"owner_type": "ticket", "owner_id": uuid.NewString(),
	}); rec.Code != http.StatusCreated {
		t.Fatalf("second upload %d: %s", rec.Code, rec.Body)
	}
	rec, env = e.do(t, "a", http.MethodGet, "/v1/tenant/attachments?owner_type=ticket&owner_id="+owner.String(), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", rec.Code, rec.Body)
	}
	page := mustDecode[struct {
		Items []attachmentDTO `json:"items"`
		Total int64           `json:"total"`
	}](t, env)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].UUID != att.UUID {
		t.Fatalf("owner filter page = %+v", page)
	}
	if rec, env = e.do(t, "a", http.MethodGet, "/v1/tenant/attachments", nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("list all %d", rec.Code)
	}
	if all := mustDecode[struct {
		Total int64 `json:"total"`
	}](t, env); all.Total != 2 {
		t.Fatalf("total = %d, want 2", all.Total)
	}

	// Original download: inline image with hardened headers.
	rec, _ = e.do(t, "a", http.MethodGet, att.DownloadURL, nil, "")
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), testPNG()) {
		t.Fatalf("download status %d, %d bytes", rec.Code, rec.Body.Len())
	}
	if h := rec.Header(); !strings.HasPrefix(h.Get("Content-Disposition"), "inline") ||
		h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") ||
		h.Get("Content-Type") != "image/png" {
		t.Fatalf("unexpected headers: %v", h)
	}
	// ?download=1 forces an attachment.
	rec, _ = e.do(t, "a", http.MethodGet, att.DownloadURL+"?download=1", nil, "")
	if !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download=1 disposition = %q", rec.Header().Get("Content-Disposition"))
	}
	// Thumbnail.
	rec, _ = e.do(t, "a", http.MethodGet, *att.ThumbnailURL, nil, "")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumb status %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	// Delete: gone for everyone, bytes removed.
	if rec, _ = e.do(t, "a", http.MethodDelete, "/v1/tenant/attachments/"+att.UUID.String(), nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete status %d: %s", rec.Code, rec.Body)
	}
	if rec, _ = e.do(t, "a", http.MethodGet, att.DownloadURL, nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("download after delete = %d", rec.Code)
	}
	if rec, _ = e.do(t, "a", http.MethodDelete, "/v1/tenant/attachments/"+att.UUID.String(), nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", rec.Code)
	}
	for _, k := range e.store.Keys() {
		if strings.Contains(k, att.UUID.String()) {
			t.Fatalf("object %q survived delete", k)
		}
	}
}

func TestOrganizationScopingOverHTTP(t *testing.T) {
	e := newEnv(t, cfg)
	rec, env := e.upload(t, "a", "gizli.txt", "text/plain", []byte("sadece alpha"), nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload %d: %s", rec.Code, rec.Body)
	}
	att := mustDecode[attachmentDTO](t, env)
	id := att.UUID.String()

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/tenant/attachments/" + id},
		{http.MethodGet, "/v1/tenant/attachments/" + id + "/download"},
		{http.MethodPost, "/v1/tenant/attachments/" + id + "/signed-url"},
		{http.MethodDelete, "/v1/tenant/attachments/" + id},
	} {
		if rec, _ := e.do(t, "b", tc.method, tc.path, nil, ""); rec.Code != http.StatusNotFound {
			t.Errorf("org B %s %s = %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
	rec, env = e.do(t, "b", http.MethodGet, "/v1/tenant/attachments", nil, "")
	if rec.Code != http.StatusOK || mustDecode[struct {
		Total int64 `json:"total"`
	}](t, env).Total != 0 {
		t.Fatalf("org B list leaked rows: %s", rec.Body)
	}
	// Still intact for A.
	if rec, _ := e.do(t, "a", http.MethodGet, "/v1/tenant/attachments/"+id, nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("org A get = %d", rec.Code)
	}
}

func TestSignedURLPublicDownload(t *testing.T) {
	e := newEnv(t, cfg)
	_, env := e.upload(t, "a", "paket.zip", "application/zip", []byte("PK\x03\x04data"), nil)
	att := mustDecode[attachmentDTO](t, env)

	rec, env := e.do(t, "a", http.MethodPost, "/v1/tenant/attachments/"+att.UUID.String()+"/signed-url", strings.NewReader(`{"ttl_seconds":60}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("signed-url %d: %s", rec.Code, rec.Body)
	}
	signed := mustDecode[struct {
		URL       string `json:"url"`
		ExpiresAt string `json:"expires_at"`
	}](t, env)
	if !strings.HasPrefix(signed.URL, "/v1/public/attachments/") || signed.ExpiresAt == "" {
		t.Fatalf("signed = %+v", signed)
	}

	// No session, no org header: the token alone authorizes.
	rec, _ = e.do(t, "", http.MethodGet, signed.URL, nil, "")
	if rec.Code != http.StatusOK || rec.Body.String() != "PK\x03\x04data" {
		t.Fatalf("public download %d %q", rec.Code, rec.Body)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("zip must be a download: %q", rec.Header().Get("Content-Disposition"))
	}
	if rec, _ := e.do(t, "", http.MethodGet, signed.URL+"x", nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("tampered token = %d", rec.Code)
	}
	if rec, _ := e.do(t, "a", http.MethodPost, "/v1/tenant/attachments/"+att.UUID.String()+"/signed-url", strings.NewReader(`{"ttl_seconds":-5}`), "application/json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("negative ttl = %d", rec.Code)
	}

	// Deleting the file revokes the link.
	e.do(t, "a", http.MethodDelete, "/v1/tenant/attachments/"+att.UUID.String(), nil, "")
	if rec, _ := e.do(t, "", http.MethodGet, signed.URL, nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("link after delete = %d", rec.Code)
	}
}

func TestUploadRejections(t *testing.T) {
	e := newEnv(t, config.AttachmentConfig{MaxFileBytes: 64})
	cases := []struct {
		name     string
		file     string
		partType string
		content  []byte
		fields   map[string]string
		status   int
		code     string
	}{
		{"too large", "a.txt", "text/plain", bytes.Repeat([]byte("a"), 65), nil, http.StatusRequestEntityTooLarge, handler.CodeFileTooLarge},
		{"executable", "a.exe", "application/octet-stream", []byte("MZ\x90\x00x"), nil, http.StatusUnsupportedMediaType, handler.CodeUnsupportedType},
		{"html", "a.html", "text/html", []byte("<script>1</script>"), nil, http.StatusUnsupportedMediaType, handler.CodeUnsupportedType},
		{"bad owner type", "a.txt", "text/plain", []byte("x"), map[string]string{"owner_type": "invoice", "owner_id": uuid.NewString()}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"bad owner id", "a.txt", "text/plain", []byte("x"), map[string]string{"owner_type": "ticket", "owner_id": "nope"}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"owner type alone", "a.txt", "text/plain", []byte("x"), map[string]string{"owner_type": "ticket"}, http.StatusBadRequest, "VALIDATION_ERROR"},
	}
	for _, tc := range cases {
		rec, env := e.upload(t, "a", tc.file, tc.partType, tc.content, tc.fields)
		if rec.Code != tc.status || env.Error == nil || env.Error.Code != tc.code {
			t.Errorf("%s: status %d body %s, want %d %s", tc.name, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
	if keys := e.store.Keys(); len(keys) != 0 {
		t.Fatalf("rejected uploads left objects: %v", keys)
	}
	// Missing file part.
	rec, _ := e.do(t, "a", http.MethodPost, "/v1/tenant/attachments", strings.NewReader("--x--"), "multipart/form-data; boundary=x")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing file = %d", rec.Code)
	}
}

func TestQuotaIsAtomicUnderConcurrentUploads(t *testing.T) {
	// Room for exactly 3 of the 10 byte files.
	e := newEnv(t, config.AttachmentConfig{MaxFileBytes: 1 << 20, OrgQuotaBytes: 35})
	var created, rejected int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec, env := e.upload(t, "a", "f.txt", "text/plain", []byte("0123456789"), nil)
			switch {
			case rec.Code == http.StatusCreated:
				atomic.AddInt32(&created, 1)
			case rec.Code == http.StatusRequestEntityTooLarge && env.Error != nil && env.Error.Code == handler.CodeQuotaExceeded:
				atomic.AddInt32(&rejected, 1)
			default:
				t.Errorf("unexpected response %d: %s", rec.Code, rec.Body)
			}
		}()
	}
	wg.Wait()
	if created != 3 || rejected != 7 {
		t.Fatalf("created=%d rejected=%d, want 3/7", created, rejected)
	}
	if got := len(e.store.Keys()); got != 3 {
		t.Fatalf("objects = %d, want 3 (rejected uploads cleaned up)", got)
	}

	rec, env := e.do(t, "a", http.MethodGet, "/v1/tenant/attachments/usage", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("usage %d", rec.Code)
	}
	u := mustDecode[struct {
		UsedBytes  int64 `json:"used_bytes"`
		FileCount  int64 `json:"file_count"`
		QuotaBytes int64 `json:"quota_bytes"`
	}](t, env)
	if u.UsedBytes != 30 || u.FileCount != 3 || u.QuotaBytes != 35 {
		t.Fatalf("usage = %+v", u)
	}
	// Org B is unaffected by A's full quota.
	if rec, _ := e.upload(t, "b", "f.txt", "text/plain", []byte("0123456789"), nil); rec.Code != http.StatusCreated {
		t.Fatalf("org B upload = %d", rec.Code)
	}
}
