package desktopreleases_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/desktopreleases"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeQ keeps releases in memory with the same semantics as the SQL.
type fakeQ struct {
	mu   sync.Mutex
	rows []db.DesktopRelease
}

func (f *fakeQ) CreateDesktopRelease(_ context.Context, a db.CreateDesktopReleaseParams) (db.DesktopRelease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.App == a.App && r.Channel == a.Channel && r.Target == a.Target && r.Version == a.Version {
			return db.DesktopRelease{}, &pgconn.PgError{Code: "23505"}
		}
	}
	row := db.DesktopRelease{ID: int64(len(f.rows) + 1), Uuid: uuid.New(), App: a.App, Channel: a.Channel, Version: a.Version,
		Target: a.Target, Status: "published", Notes: a.Notes, Signature: a.Signature, OsSigned: a.OsSigned,
		UpdaterKey: a.UpdaterKey, UpdaterName: a.UpdaterName, UpdaterSize: a.UpdaterSize, UpdaterSha256: a.UpdaterSha256,
		InstallerKey: a.InstallerKey, InstallerName: a.InstallerName, InstallerSize: a.InstallerSize, InstallerSha256: a.InstallerSha256,
		PublishedByUserID: a.PublishedByUserID, PublishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	f.rows = append(f.rows, row)
	return row, nil
}

func (f *fakeQ) GetDesktopReleaseByUUID(_ context.Context, id uuid.UUID) (db.DesktopRelease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.Uuid == id {
			return r, nil
		}
	}
	return db.DesktopRelease{}, pgx.ErrNoRows
}

func in(s string, list []string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (f *fakeQ) ListPublishedDesktopReleases(_ context.Context, a db.ListPublishedDesktopReleasesParams) ([]db.DesktopRelease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []db.DesktopRelease
	for _, r := range f.rows {
		if r.App == a.App && r.Target == a.Target && r.Status == "published" && in(r.Channel, a.Channels) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeQ) ListAllPublishedDesktopReleases(_ context.Context, channels []string) ([]db.DesktopRelease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []db.DesktopRelease
	for _, r := range f.rows {
		if r.Status == "published" && in(r.Channel, channels) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeQ) ListDesktopReleases(_ context.Context, a db.ListDesktopReleasesParams) ([]db.DesktopRelease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []db.DesktopRelease
	for i := len(f.rows) - 1; i >= 0; i-- {
		r := f.rows[i]
		if (!a.App.Valid || r.App == a.App.String) && (!a.Status.Valid || r.Status == a.Status.String) && (!a.Channel.Valid || r.Channel == a.Channel.String) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeQ) CountDesktopReleases(ctx context.Context, a db.CountDesktopReleasesParams) (int64, error) {
	rows, _ := f.ListDesktopReleases(ctx, db.ListDesktopReleasesParams{App: a.App, Channel: a.Channel, Status: a.Status})
	return int64(len(rows)), nil
}

func (f *fakeQ) WithdrawDesktopRelease(_ context.Context, a db.WithdrawDesktopReleaseParams) (db.DesktopRelease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.rows {
		if r.Uuid == a.Uuid && r.Status == "published" {
			r.Status, r.WithdrawnReason = "withdrawn", a.Reason
			r.WithdrawnAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
			f.rows[i] = r
			return r, nil
		}
	}
	return db.DesktopRelease{}, pgx.ErrNoRows
}

type loader map[uuid.UUID][]string

func (l loader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: 7, Permissions: l[id]}, nil
}

type env struct {
	mux     *http.ServeMux
	q       *fakeQ
	store   *storage.Memory
	admin   string
	nobody  string
	baseURL string
}

func setupEnv(t *testing.T) *env {
	t.Helper()
	tokens, err := jwt.NewManager("test-secret-0123456789abcdef0123456789", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	adminID, nobodyID := uuid.New(), uuid.New()
	ld := loader{adminID: {rbac.PermPlatformDesktopReleasesManage}, nobodyID: {"platform.users.read"}}
	issue := func(id uuid.UUID) string {
		tok, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: id})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	e := &env{mux: http.NewServeMux(), q: &fakeQ{}, store: storage.NewMemory(), baseURL: "https://app.example.test"}
	e.admin, e.nobody = issue(adminID), issue(nobodyID)
	svc := &desktopreleases.Service{Q: e.q, Store: e.store, BaseURL: e.baseURL + "/"}
	desktopreleases.RegisterRoutes(e.mux, svc, desktopreleases.Deps{Tokens: tokens, Loader: ld})
	return e
}

func (e *env) do(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	e.mux.ServeHTTP(w, r)
	return w
}

func sig(t string) string {
	return base64.StdEncoding.EncodeToString([]byte("untrusted comment: signature from tauri secret key\n" + t + "\n"))
}

type pub struct {
	app, channel, version, target string
	updater, installer            string // file names
	osSigned                      string
	token                         string
}

func (e *env) publish(t *testing.T, p pub) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fields := map[string]string{"app": p.app, "channel": p.channel, "version": p.version, "target": p.target,
		"notes": "Notlar " + p.version, "signature": sig(p.version), "os_signed": p.osSigned}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if p.updater != "" {
		fw, _ := mw.CreateFormFile("updater", p.updater)
		_, _ = io.WriteString(fw, "updater-bytes-"+p.version)
	}
	if p.installer != "" {
		fw, _ := mw.CreateFormFile("installer", p.installer)
		_, _ = io.WriteString(fw, "installer-bytes-"+p.version)
	}
	_ = mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/v1/platform/desktop/releases", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if p.token != "" {
		r.Header.Set("Authorization", "Bearer "+p.token)
	}
	return e.do(r)
}

func mac(version, channel, token string) pub {
	return pub{app: "support", channel: channel, version: version, target: "darwin-aarch64",
		updater: "ClickFix.Destek.app.tar.gz", installer: "ClickFix.Destek_" + version + "_aarch64.dmg", osSigned: "true", token: token}
}

func get(e *env, path string) *httptest.ResponseRecorder {
	return e.do(httptest.NewRequest(http.MethodGet, path, nil))
}

func TestPlatformRoutesRequirePermission(t *testing.T) {
	e := setupEnv(t)
	if w := e.publish(t, mac("0.2.0", "stable", "")); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous publish: %d", w.Code)
	}
	if w := e.publish(t, mac("0.2.0", "stable", e.nobody)); w.Code != http.StatusForbidden {
		t.Fatalf("publish without permission: %d", w.Code)
	}
	if w := get(e, "/v1/platform/desktop/releases"); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodPost, "/v1/platform/desktop/releases/"+uuid.NewString()+"/withdraw", strings.NewReader(`{"reason":"x"}`))
	r.Header.Set("Authorization", "Bearer "+e.nobody)
	if w := e.do(r); w.Code != http.StatusForbidden {
		t.Fatalf("withdraw without permission: %d", w.Code)
	}
	if len(e.q.rows) != 0 {
		t.Fatal("rows written without permission")
	}
}

func TestUpdaterManifestAndWithdraw(t *testing.T) {
	e := setupEnv(t)
	// No release yet: up to date (204), no organization header or login needed.
	if w := get(e, "/v1/public/desktop/releases/support/darwin-aarch64/0.1.0"); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("empty: %d %q", w.Code, w.Body.String())
	}
	for _, v := range []string{"0.9.0", "0.10.0"} {
		if w := e.publish(t, mac(v, "stable", e.admin)); w.Code != http.StatusCreated {
			t.Fatalf("publish %s: %d %s", v, w.Code, w.Body.String())
		}
	}
	if w := e.publish(t, mac("0.11.0-beta.1", "beta", e.admin)); w.Code != http.StatusCreated {
		t.Fatalf("publish beta: %d %s", w.Code, w.Body.String())
	}
	// Same version again: conflict, and the uploaded objects are removed.
	before := len(e.store.Keys())
	if w := e.publish(t, mac("0.10.0", "stable", e.admin)); w.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d", w.Code)
	}
	if after := len(e.store.Keys()); after != before {
		t.Fatalf("duplicate left objects: %d -> %d", before, after)
	}

	manifest := func(path string) desktopreleases.Manifest {
		t.Helper()
		w := get(e, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var m desktopreleases.Manifest
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	// 0.10.0 beats 0.9.0 (semver, not text order); stable never sees beta.
	m := manifest("/v1/public/desktop/releases/support/darwin-aarch64/0.1.0")
	if m.Version != "0.10.0" || m.Signature != sig("0.10.0") || m.Notes != "Notlar 0.10.0" {
		t.Fatalf("stable manifest: %+v", m)
	}
	if !strings.HasPrefix(m.URL, e.baseURL+"/api/v1/public/desktop/files/") || !strings.HasSuffix(m.URL, "/updater/ClickFix.Destek.app.tar.gz") {
		t.Fatalf("manifest url: %s", m.URL)
	}
	if _, err := time.Parse(time.RFC3339, m.PubDate); err != nil {
		t.Fatalf("pub_date: %v", err)
	}
	if m := manifest("/v1/public/desktop/releases/support/darwin-aarch64/0.10.0?channel=beta"); m.Version != "0.11.0-beta.1" {
		t.Fatalf("beta manifest: %+v", m)
	}
	if w := get(e, "/v1/public/desktop/releases/support/darwin-aarch64/0.10.0"); w.Code != http.StatusNoContent {
		t.Fatalf("up to date: %d", w.Code)
	}
	// Other app and other target are separate.
	for _, p := range []string{"/v1/public/desktop/releases/agent/darwin-aarch64/0.1.0", "/v1/public/desktop/releases/support/windows-x86_64/0.1.0"} {
		if w := get(e, p); w.Code != http.StatusNoContent {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
	// The package streams from storage, byte for byte.
	path := strings.TrimPrefix(m.URL, e.baseURL+"/api")
	w := get(e, path)
	if w.Code != http.StatusOK || w.Body.String() != "updater-bytes-0.10.0" || !strings.Contains(w.Header().Get("Content-Disposition"), "ClickFix.Destek.app.tar.gz") {
		t.Fatalf("file: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if w := get(e, strings.Replace(path, "ClickFix.Destek.app.tar.gz", "other.app.tar.gz", 1)); w.Code != http.StatusNotFound {
		t.Fatalf("wrong file name: %d", w.Code)
	}

	// Withdraw 0.10.0: the updater falls back to 0.9.0 and the file is gone.
	var id uuid.UUID
	for _, r := range e.q.rows {
		if r.Version == "0.10.0" {
			id = r.Uuid
		}
	}
	withdraw := func(reason string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/platform/desktop/releases/"+id.String()+"/withdraw", strings.NewReader(`{"reason":"`+reason+`"}`))
		r.Header.Set("Authorization", "Bearer "+e.admin)
		return e.do(r)
	}
	if w := withdraw(""); w.Code != http.StatusBadRequest {
		t.Fatalf("withdraw without reason: %d", w.Code)
	}
	if w := withdraw("bozuk paket"); w.Code != http.StatusOK {
		t.Fatalf("withdraw: %d %s", w.Code, w.Body.String())
	}
	if w := withdraw("tekrar"); w.Code != http.StatusConflict {
		t.Fatalf("withdraw twice: %d", w.Code)
	}
	if m := manifest("/v1/public/desktop/releases/support/darwin-aarch64/0.1.0"); m.Version != "0.9.0" {
		t.Fatalf("after withdraw: %+v", m)
	}
	if w := get(e, path); w.Code != http.StatusNotFound {
		t.Fatalf("withdrawn file: %d", w.Code)
	}
}

func TestPublicRoutesRejectBadInput(t *testing.T) {
	e := setupEnv(t)
	for _, p := range []string{
		"/v1/public/desktop/releases/other/darwin-aarch64/0.1.0",
		"/v1/public/desktop/releases/support/darwin-universal/0.1.0",
		"/v1/public/desktop/releases/support/darwin-aarch64/latest",
		"/v1/public/desktop/releases/support/darwin-aarch64/0.1.0?channel=nightly",
		"/v1/public/desktop/downloads?channel=nightly",
	} {
		if w := get(e, p); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	if w := get(e, "/v1/public/desktop/files/not-a-uuid/updater/x.exe"); w.Code != http.StatusNotFound {
		t.Errorf("bad uuid: %d", w.Code)
	}
}

func TestPublishValidation(t *testing.T) {
	e := setupEnv(t)
	cases := map[string]pub{
		"beta version on stable":  mac("0.2.0-beta.1", "stable", e.admin),
		"release version on beta": mac("0.2.0", "beta", e.admin),
		"v prefix":                mac("v0.2.0", "stable", e.admin),
		"bad target":              {app: "support", channel: "stable", version: "0.2.0", target: "macos", updater: "a.app.tar.gz", osSigned: "true", token: e.admin},
		"bad updater ext":         {app: "support", channel: "stable", version: "0.2.0", target: "darwin-aarch64", updater: "a.zip", osSigned: "true", token: e.admin},
		"unsafe name":             {app: "support", channel: "stable", version: "0.2.0", target: "darwin-aarch64", updater: "a\"b.app.tar.gz", osSigned: "true", token: e.admin},
		"no updater":              {app: "support", channel: "stable", version: "0.2.0", target: "darwin-aarch64", osSigned: "true", token: e.admin},
		"bad os_signed":           {app: "support", channel: "stable", version: "0.2.0", target: "darwin-aarch64", updater: "a.app.tar.gz", osSigned: "maybe", token: e.admin},
	}
	for name, p := range cases {
		if w := e.publish(t, p); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if len(e.q.rows) != 0 || len(e.store.Keys()) != 0 {
		t.Fatal("invalid publish wrote data")
	}
}

func TestDownloadsList(t *testing.T) {
	e := setupEnv(t)
	win := func(v string, signed string) pub {
		return pub{app: "agent", channel: "stable", version: v, target: "windows-x86_64", updater: "ClickFix_" + v + "_x64-setup.exe", osSigned: signed, token: e.admin}
	}
	for _, p := range []pub{mac("0.2.0", "stable", e.admin), mac("0.3.0-beta.1", "beta", e.admin), win("0.2.0", "false"), win("0.2.1", "false")} {
		if w := e.publish(t, p); w.Code != http.StatusCreated {
			t.Fatalf("publish: %d %s", w.Code, w.Body.String())
		}
	}
	w := get(e, "/v1/public/desktop/downloads")
	if w.Code != http.StatusOK {
		t.Fatalf("downloads: %d", w.Code)
	}
	var env struct {
		Data struct {
			Items []desktopreleases.Download `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	items := env.Data.Items
	if len(items) != 2 {
		t.Fatalf("items: %+v", items)
	}
	// Sorted by app then target: agent/windows first.
	a, s := items[0], items[1]
	if a.App != "agent" || a.Version != "0.2.1" || a.OS != "windows" || a.Arch != "x86_64" || a.OSSigned || a.File.Name != "ClickFix_0.2.1_x64-setup.exe" {
		t.Fatalf("agent item: %+v", a)
	}
	sum := sha256.Sum256([]byte("updater-bytes-0.2.1"))
	if a.File.SHA256 != hex.EncodeToString(sum[:]) || !strings.HasPrefix(a.File.URL, "/api/v1/public/desktop/files/") {
		t.Fatalf("agent file: %+v", a.File)
	}
	// macOS offers the .dmg installer, stable only.
	if s.App != "support" || s.Version != "0.2.0" || !s.OSSigned || !strings.HasSuffix(s.File.Name, ".dmg") || !strings.Contains(s.File.URL, "/installer/") {
		t.Fatalf("support item: %+v", s)
	}
}
