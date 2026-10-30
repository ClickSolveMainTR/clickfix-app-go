package usecase

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/repository"
	platstorage "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeRepo is an in-memory Repository that mirrors the SQL semantics: live
// rows are scoped by organization and the quota is enforced on insert.
type fakeRepo struct {
	mu   sync.Mutex
	rows []db.Attachment
}

func (f *fakeRepo) InsertWithQuota(_ context.Context, p db.InsertAttachmentParams, quota int64) (db.Attachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if quota > 0 {
		var used int64
		for _, r := range f.rows {
			if r.OrganizationID == p.OrganizationID && !r.DeletedAt.Valid {
				used += r.SizeBytes
			}
		}
		if used+p.SizeBytes > quota {
			return db.Attachment{}, repository.ErrQuotaExceeded
		}
	}
	row := db.Attachment{
		ID: int64(len(f.rows) + 1), Uuid: p.Uuid, OrganizationID: p.OrganizationID,
		UploadedBy: p.UploadedBy, OwnerType: p.OwnerType, OwnerID: p.OwnerID,
		Filename: p.Filename, ContentType: p.ContentType, SizeBytes: p.SizeBytes,
		Sha256: p.Sha256, ObjectKey: p.ObjectKey, ThumbKey: p.ThumbKey,
		ScanStatus: p.ScanStatus, ScanDetail: p.ScanDetail, ScannedAt: p.ScannedAt,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	f.rows = append(f.rows, row)
	return row, nil
}

func (f *fakeRepo) Get(_ context.Context, orgID int64, id uuid.UUID) (db.Attachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.OrganizationID == orgID && r.Uuid == id && !r.DeletedAt.Valid {
			return r, nil
		}
	}
	return db.Attachment{}, repository.ErrNotFound
}

func (f *fakeRepo) List(_ context.Context, l db.ListAttachmentsParams, _ db.CountAttachmentsParams) ([]db.Attachment, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []db.Attachment
	for _, r := range f.rows {
		if r.OrganizationID != l.OrganizationID || r.DeletedAt.Valid {
			continue
		}
		if l.OwnerType.Valid && r.OwnerType != l.OwnerType {
			continue
		}
		if l.OwnerID.Valid && r.OwnerID != l.OwnerID {
			continue
		}
		out = append(out, r)
	}
	return out, int64(len(out)), nil
}

func (f *fakeRepo) SoftDelete(_ context.Context, orgID int64, id uuid.UUID) (db.Attachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.rows {
		if r.OrganizationID == orgID && r.Uuid == id && !r.DeletedAt.Valid {
			f.rows[i].DeletedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
			return f.rows[i], nil
		}
	}
	return db.Attachment{}, repository.ErrNotFound
}

func (f *fakeRepo) Usage(_ context.Context, orgID int64) (db.SumAttachmentUsageRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var u db.SumAttachmentUsageRow
	for _, r := range f.rows {
		if r.OrganizationID == orgID && !r.DeletedAt.Valid {
			u.UsedBytes += r.SizeBytes
			u.FileCount++
		}
	}
	return u, nil
}

func (f *fakeRepo) UserUUIDs(_ context.Context, ids []int64) (map[int64]uuid.UUID, error) {
	out := map[int64]uuid.UUID{}
	for _, id := range ids {
		out[id] = uuid.NewSHA1(uuid.NameSpaceOID, []byte{byte(id)})
	}
	return out, nil
}

type stubScanner struct {
	res ScanResult
	err error
	got ScanInput
}

func (s *stubScanner) Scan(_ context.Context, in ScanInput) (ScanResult, error) {
	s.got = in
	return s.res, s.err
}

type fixture struct {
	svc   *Service
	repo  *fakeRepo
	store *platstorage.Memory
	org   model.UploadInput
	now   time.Time
}

func newFixture(t *testing.T, cfg config.AttachmentConfig) *fixture {
	t.Helper()
	repo := &fakeRepo{}
	store := platstorage.NewMemory()
	svc := New(repo, store, "test-secret", cfg, nil)
	f := &fixture{svc: svc, repo: repo, store: store, now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}
	svc.now = func() time.Time { return f.now }
	f.org = model.UploadInput{OrganizationID: 1, OrganizationUUID: uuid.New(), UploaderID: 7}
	return f
}

func (f *fixture) upload(t *testing.T, name, declared string, body []byte) (model.Attachment, error) {
	t.Helper()
	in := f.org
	in.Filename, in.DeclaredType, in.Body = name, declared, bytes.NewReader(body)
	return f.svc.Upload(context.Background(), in)
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

var defaultCfg = config.AttachmentConfig{MaxFileBytes: 1 << 20, OrgQuotaBytes: 0, SignedURLTTL: time.Minute}

func TestUploadStoresObjectAndMetadata(t *testing.T) {
	t.Parallel()
	f := newFixture(t, defaultCfg)
	owner := uuid.New()
	f.org.OwnerType, f.org.OwnerID = model.OwnerTicket, &owner

	att, err := f.upload(t, "../../etc/rapor.pdf", "application/pdf", []byte("%PDF-1.4 test"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if att.Filename != "rapor.pdf" {
		t.Fatalf("filename = %q, want path stripped", att.Filename)
	}
	if att.ContentType != "application/pdf" || att.SizeBytes != 13 || att.ScanStatus != model.ScanSkipped {
		t.Fatalf("unexpected metadata: %+v", att)
	}
	if att.OwnerType == nil || *att.OwnerType != model.OwnerTicket || att.OwnerID == nil || *att.OwnerID != owner {
		t.Fatalf("owner = %v/%v", att.OwnerType, att.OwnerID)
	}
	if att.UploadedBy == nil {
		t.Fatal("uploader uuid missing")
	}
	if !strings.HasPrefix(att.DownloadURL, "/v1/tenant/attachments/") {
		t.Fatalf("download url = %q", att.DownloadURL)
	}
	key := platstorage.AttachmentObjectKey(f.org.OrganizationUUID.String(), att.UUID.String())
	if ok, _ := f.store.Exists(context.Background(), key); !ok {
		t.Fatalf("object %q was not stored", key)
	}
}

func TestUploadValidation(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	cases := []struct {
		name    string
		mutate  func(*model.UploadInput)
		file    string
		declare string
		body    []byte
		want    error
	}{
		{name: "empty file", file: "a.txt", declare: "text/plain", body: nil, want: ErrInvalidRequest},
		{name: "no filename", file: "  ", declare: "text/plain", body: []byte("x"), want: ErrInvalidRequest},
		{name: "owner type without id", mutate: func(in *model.UploadInput) { in.OwnerType = model.OwnerTicket }, file: "a.txt", declare: "text/plain", body: []byte("x"), want: ErrInvalidRequest},
		{name: "owner id without type", mutate: func(in *model.UploadInput) { in.OwnerID = &owner }, file: "a.txt", declare: "text/plain", body: []byte("x"), want: ErrInvalidRequest},
		{name: "unknown owner type", mutate: func(in *model.UploadInput) { in.OwnerType, in.OwnerID = "invoice", &owner }, file: "a.txt", declare: "text/plain", body: []byte("x"), want: ErrInvalidRequest},
		{name: "blocked extension", file: "setup.EXE", declare: "application/pdf", body: []byte("hello"), want: ErrUnsupportedType},
		{name: "executable magic", file: "photo.png", declare: "image/png", body: append([]byte("MZ"), make([]byte, 64)...), want: ErrUnsupportedType},
		{name: "html is not allowed", file: "page.html", declare: "text/html", body: []byte("<html><script>alert(1)</script></html>"), want: ErrUnsupportedType},
		{name: "svg sniffed as xml", file: "logo.svg", declare: "image/svg+xml", body: []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), want: ErrUnsupportedType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, defaultCfg)
			if tc.mutate != nil {
				tc.mutate(&f.org)
			}
			_, err := f.upload(t, tc.file, tc.declare, tc.body)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestUploadDeclaredTypeCannotHideContent(t *testing.T) {
	t.Parallel()
	f := newFixture(t, defaultCfg)
	// PNG bytes declared as PDF: the sniffed type wins.
	att, err := f.upload(t, "scan.pdf", "application/pdf", pngBytes(t, 4, 4))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if att.ContentType != "image/png" {
		t.Fatalf("content type = %q, want image/png", att.ContentType)
	}
}

func TestUploadSizeLimitAndQuota(t *testing.T) {
	t.Parallel()
	f := newFixture(t, config.AttachmentConfig{MaxFileBytes: 100, OrgQuotaBytes: 150})
	big := bytes.Repeat([]byte("a"), 101)
	if _, err := f.upload(t, "big.txt", "text/plain", big); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize err = %v", err)
	}
	ok := bytes.Repeat([]byte("a"), 100)
	if _, err := f.upload(t, "one.txt", "text/plain", ok); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := f.upload(t, "two.txt", "text/plain", ok); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("quota err = %v", err)
	}
	if got := len(f.store.Keys()); got != 1 {
		t.Fatalf("objects after rejected upload = %d, want 1 (rollback)", got)
	}
	// A second organization has its own quota.
	other := f.org
	other.OrganizationID, other.OrganizationUUID = 2, uuid.New()
	other.Filename, other.DeclaredType, other.Body = "x.txt", "text/plain", bytes.NewReader(ok)
	if _, err := f.svc.Upload(context.Background(), other); err != nil {
		t.Fatalf("other org upload: %v", err)
	}
	u, err := f.svc.Usage(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if u.UsedBytes != 100 || u.FileCount != 1 || u.QuotaBytes != 150 || u.MaxFileBytes != 100 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestPlanLimitsProviderOverridesDefaults(t *testing.T) {
	t.Parallel()
	f := newFixture(t, defaultCfg)
	f.svc.WithLimits(StaticLimits{MaxFileBytes: 4, MaxTotalBytes: 0})
	if _, err := f.upload(t, "a.txt", "text/plain", []byte("12345")); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge from plan limit", err)
	}
}

func TestScannerHook(t *testing.T) {
	t.Parallel()
	t.Run("infected is rejected and nothing stored", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, defaultCfg)
		f.svc.WithScanner(&stubScanner{res: ScanResult{Status: model.ScanInfected, Detail: "Eicar-Test"}})
		if _, err := f.upload(t, "a.txt", "text/plain", []byte("x")); !errors.Is(err, ErrInfected) {
			t.Fatalf("err = %v", err)
		}
		if len(f.store.Keys()) != 0 || len(f.repo.rows) != 0 {
			t.Fatal("infected upload must not be stored")
		}
	})
	t.Run("scanner error fails closed", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, defaultCfg)
		f.svc.WithScanner(&stubScanner{err: errors.New("clamd down")})
		if _, err := f.upload(t, "a.txt", "text/plain", []byte("x")); !errors.Is(err, ErrScanUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if len(f.store.Keys()) != 0 {
			t.Fatal("nothing may be stored when the verdict is unknown")
		}
	})
	t.Run("clean verdict is recorded with the scan input", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t, defaultCfg)
		sc := &stubScanner{res: ScanResult{Status: model.ScanClean}}
		f.svc.WithScanner(sc)
		att, err := f.upload(t, "a.txt", "text/plain", []byte("hello"))
		if err != nil {
			t.Fatal(err)
		}
		if att.ScanStatus != model.ScanClean || string(sc.got.Data) != "hello" || sc.got.SHA256 != att.SHA256 {
			t.Fatalf("status=%q scan input=%+v", att.ScanStatus, sc.got)
		}
		if !f.repo.rows[0].ScannedAt.Valid {
			t.Fatal("scanned_at not set for a clean verdict")
		}
	})
}

func TestThumbnailGeneratedForImages(t *testing.T) {
	t.Parallel()
	f := newFixture(t, defaultCfg)
	att, err := f.upload(t, "big.png", "image/png", pngBytes(t, 800, 400))
	if err != nil {
		t.Fatal(err)
	}
	if !att.HasThumbnail || att.ThumbnailURL == nil {
		t.Fatalf("expected a thumbnail: %+v", att)
	}
	file, err := f.svc.Open(context.Background(), 1, att.UUID, model.VariantThumbnail)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Body.Close() }()
	if file.ContentType != "image/jpeg" {
		t.Fatalf("thumb type = %q", file.ContentType)
	}
	raw, _ := io.ReadAll(file.Body)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("thumbnail is not an image: %v", err)
	}
	if cfg.Width != thumbMaxEdge || cfg.Height != thumbMaxEdge/2 {
		t.Fatalf("thumb size = %dx%d, want %dx%d", cfg.Width, cfg.Height, thumbMaxEdge, thumbMaxEdge/2)
	}

	pdf, err := f.upload(t, "a.pdf", "application/pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	if pdf.HasThumbnail {
		t.Fatal("pdf must not get a thumbnail")
	}
	if _, err := f.svc.Open(context.Background(), 1, pdf.UUID, model.VariantThumbnail); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing thumb err = %v", err)
	}
}

func TestOrganizationIsolation(t *testing.T) {
	t.Parallel()
	f := newFixture(t, defaultCfg)
	att, err := f.upload(t, "a.txt", "text/plain", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := f.svc.Get(ctx, 2, att.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get other org err = %v", err)
	}
	if _, err := f.svc.Open(ctx, 2, att.UUID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open other org err = %v", err)
	}
	if _, err := f.svc.SignedURL(ctx, 2, att.UUID, "", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SignedURL other org err = %v", err)
	}
	if _, err := f.svc.Delete(ctx, 2, att.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete other org err = %v", err)
	}
	items, total, err := f.svc.List(ctx, model.ListFilter{OrganizationID: 2, Limit: 10})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("List other org = %v/%d/%v", items, total, err)
	}
	if _, err := f.svc.Get(ctx, 1, att.UUID); err != nil {
		t.Fatalf("owner org Get: %v", err)
	}
}

func TestListByOwner(t *testing.T) {
	t.Parallel()
	f := newFixture(t, defaultCfg)
	ticket, other := uuid.New(), uuid.New()
	for _, o := range []uuid.UUID{ticket, ticket, other} {
		o := o
		f.org.OwnerType, f.org.OwnerID = model.OwnerTicket, &o
		if _, err := f.upload(t, "a.txt", "text/plain", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := f.svc.List(context.Background(), model.ListFilter{OrganizationID: 1, OwnerType: model.OwnerTicket, OwnerID: &ticket, Limit: 10})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("list = %d/%d/%v", len(items), total, err)
	}
	if _, _, err := f.svc.List(context.Background(), model.ListFilter{OrganizationID: 1, OwnerID: &ticket}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("owner_id without type err = %v", err)
	}
}

func TestDeleteRemovesObjectsAndFreesQuota(t *testing.T) {
	t.Parallel()
	f := newFixture(t, config.AttachmentConfig{MaxFileBytes: 1 << 20, OrgQuotaBytes: 200})
	att, err := f.upload(t, "p.png", "image/png", pngBytes(t, 8, 8))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.store.Keys()) != 2 {
		t.Fatalf("keys = %v, want original + thumb", f.store.Keys())
	}
	if _, err := f.svc.Delete(context.Background(), 1, att.UUID); err != nil {
		t.Fatal(err)
	}
	if len(f.store.Keys()) != 0 {
		t.Fatalf("objects left after delete: %v", f.store.Keys())
	}
	if _, err := f.svc.Get(context.Background(), 1, att.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete err = %v", err)
	}
	if _, err := f.svc.Delete(context.Background(), 1, att.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
	u, _ := f.svc.Usage(context.Background(), 1)
	if u.UsedBytes != 0 {
		t.Fatalf("used bytes after delete = %d", u.UsedBytes)
	}
}

func TestSignedURLLifecycle(t *testing.T) {
	t.Parallel()
	f := newFixture(t, defaultCfg)
	att, err := f.upload(t, "a.txt", "text/plain", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	signed, err := f.svc.SignedURL(ctx, 1, att.UUID, "", 90*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "/v1/public/attachments/"
	if !strings.HasPrefix(signed.URL, prefix) || !signed.ExpiresAt.Equal(f.now.Add(90*time.Second)) {
		t.Fatalf("signed = %+v", signed)
	}
	token := strings.TrimPrefix(signed.URL, prefix)

	file, err := f.svc.OpenSigned(ctx, token)
	if err != nil {
		t.Fatalf("OpenSigned: %v", err)
	}
	body, _ := io.ReadAll(file.Body)
	_ = file.Body.Close()
	if string(body) != "hello" {
		t.Fatalf("body = %q", body)
	}

	// Tampered token and wrong organization claims are not found.
	if _, err := f.svc.OpenSigned(ctx, token+"x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tampered err = %v", err)
	}
	forged := f.svc.signer.sign(tokenClaims{Org: 2, Item: att.UUID, Variant: model.VariantOriginal, Exp: f.now.Add(time.Minute).Unix()})
	if _, err := f.svc.OpenSigned(ctx, forged); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other-org token err = %v", err)
	}

	// Expiry.
	f.now = f.now.Add(91 * time.Second)
	if _, err := f.svc.OpenSigned(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired err = %v", err)
	}

	// Deleting the attachment kills outstanding links.
	f.now = f.now.Add(-time.Hour)
	fresh, _ := f.svc.SignedURL(ctx, 1, att.UUID, "", time.Minute)
	if _, err := f.svc.Delete(ctx, 1, att.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.OpenSigned(ctx, strings.TrimPrefix(fresh.URL, prefix)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted err = %v", err)
	}
}

func TestSignedURLTTLIsClamped(t *testing.T) {
	t.Parallel()
	f := newFixture(t, defaultCfg)
	att, err := f.upload(t, "a.txt", "text/plain", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	long, _ := f.svc.SignedURL(context.Background(), 1, att.UUID, "", 48*time.Hour)
	if !long.ExpiresAt.Equal(f.now.Add(maxSignedTTL)) {
		t.Fatalf("long ttl expiry = %v", long.ExpiresAt)
	}
	def, _ := f.svc.SignedURL(context.Background(), 1, att.UUID, "", 0)
	if !def.ExpiresAt.Equal(f.now.Add(time.Minute)) {
		t.Fatalf("default ttl expiry = %v", def.ExpiresAt)
	}
	if _, err := f.svc.SignedURL(context.Background(), 1, att.UUID, "weird", 0); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("bad variant err = %v", err)
	}
}
