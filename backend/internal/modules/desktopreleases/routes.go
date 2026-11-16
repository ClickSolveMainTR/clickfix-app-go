package desktopreleases

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// MaxPublishBytes caps a publish request (both package files together).
const MaxPublishBytes = 1 << 30

// transferTimeout bounds a package upload or download.
const transferTimeout = 30 * time.Minute

// Deps are the route dependencies. Tokens may be nil in tests that only
// exercise the public routes; the platform routes then are not mounted.
type Deps struct {
	Tokens   *jwt.Manager
	Loader   middleware.IdentityLoader
	Activity *activity.Recorder
	Log      *slog.Logger
}

type routes struct {
	s *Service
	d Deps
}

// RegisterRoutes mounts the public updater/download routes and the platform
// publish/withdraw routes.
func RegisterRoutes(mux *http.ServeMux, s *Service, d Deps) {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	h := routes{s: s, d: d}

	// Public: no login, no organization. They expose only published,
	// platform-wide release data.
	mux.HandleFunc("GET /v1/public/desktop/releases/{app}/{target}/{current_version}", h.update)
	mux.HandleFunc("GET /v1/public/desktop/downloads", h.downloads)
	mux.HandleFunc("GET /v1/public/desktop/files/{uuid}/{kind}/{name}", h.file)

	if d.Tokens == nil {
		return
	}
	platform := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, middleware.Authenticate(d.Tokens, d.Loader),
			middleware.RequirePermission(rbac.PermPlatformDesktopReleasesManage)))
	}
	platform("GET /v1/platform/desktop/releases", h.list)
	platform("POST /v1/platform/desktop/releases", h.publish)
	platform("POST /v1/platform/desktop/releases/{uuid}/withdraw", h.withdraw)
}

func (h routes) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Message: "Invalid input"}})
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Release not found")
	case errors.Is(err, ErrConflict):
		response.Conflict(w, r, "DESKTOP_RELEASE_EXISTS", "This version is already published for this app, channel and target")
	case errors.Is(err, ErrWithdrawn):
		response.Conflict(w, r, "DESKTOP_RELEASE_WITHDRAWN", "Release is already withdrawn")
	default:
		response.InternalErr(w, r, err, "Internal server error")
	}
}

// update answers the Tauri updater in its own format (not the envelope):
// 200 with the manifest, or 204 when there is nothing newer.
func (h routes) update(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	m, err := h.s.Update(r.Context(), r.PathValue("app"), r.PathValue("target"), r.PathValue("current_version"), r.URL.Query().Get("channel"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if m == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m)
}

func (h routes) downloads(w http.ResponseWriter, r *http.Request) {
	items, err := h.s.Downloads(r.Context(), r.URL.Query().Get("channel"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h routes) file(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Release not found")
		return
	}
	rc, size, name, err := h.s.Open(r.Context(), id, r.PathValue("kind"), r.PathValue("name"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	// Packages are tens of MB; the server's write timeout (HTTP_WRITE_TIMEOUT)
	// is sized for API calls, so this response gets its own deadline.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(transferTimeout))
	w.Header().Set("Content-Type", contentType(name))
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The bytes behind a release uuid never change, but a withdrawn release
	// must stop being served soon: short shared cache only.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, rc); err != nil {
		h.d.Log.Warn("desktop_release_stream_failed", "release", id, "error", err)
	}
}

func actor(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func (h routes) list(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	v := r.URL.Query()
	items, total, err := h.s.List(r.Context(), ListFilter{App: v.Get("app"), Channel: v.Get("channel"), Status: v.Get("status"),
		Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "total": total, "limit": q.Limit, "offset": q.Offset})
}

func formFile(form *multipart.Form, field string) (*Upload, func(), error) {
	files := form.File[field]
	if len(files) == 0 {
		return nil, func() {}, nil
	}
	if len(files) > 1 {
		return nil, func() {}, ErrInvalid
	}
	fh := files[0]
	f, err := fh.Open()
	if err != nil {
		return nil, func() {}, err
	}
	return &Upload{Name: fh.Filename, Size: fh.Size, Body: f}, func() { _ = f.Close() }, nil
}

// publish takes a multipart form: app, channel, version, target, notes,
// signature (the .sig file content), os_signed, and the files updater
// (required) and installer (optional).
func (h routes) publish(w http.ResponseWriter, r *http.Request) {
	// Large upload: lift the server's read/write timeouts for this request.
	ctl := http.NewResponseController(w)
	_ = ctl.SetReadDeadline(time.Now().Add(transferTimeout))
	_ = ctl.SetWriteDeadline(time.Now().Add(transferTimeout))
	r.Body = http.MaxBytesReader(w, r.Body, MaxPublishBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.fail(w, r, ErrInvalid)
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	updater, closeU, err := formFile(r.MultipartForm, "updater")
	defer closeU()
	if err != nil || updater == nil {
		h.fail(w, r, ErrInvalid)
		return
	}
	installer, closeI, err := formFile(r.MultipartForm, "installer")
	defer closeI()
	if err != nil {
		h.fail(w, r, ErrInvalid)
		return
	}
	signed, err := strconv.ParseBool(strings.TrimSpace(r.FormValue("os_signed")))
	if err != nil {
		h.fail(w, r, ErrInvalid)
		return
	}
	in := PublishInput{
		App: r.FormValue("app"), Channel: r.FormValue("channel"), Version: r.FormValue("version"),
		Target: r.FormValue("target"), Notes: r.FormValue("notes"), Signature: r.FormValue("signature"),
		OSSigned: signed, Updater: *updater, Installer: installer,
	}
	rel, err := h.s.Publish(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	a := actor(r)
	h.d.Activity.Record(r.Context(), &a, "desktop_release.published", "desktop_release", &rel.UUID,
		map[string]any{"app": rel.App, "channel": rel.Channel, "version": rel.Version, "target": rel.Target, "os_signed": rel.OSSigned}, r)
	response.JSON(w, r, http.StatusCreated, rel)
}

func (h routes) withdraw(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		h.fail(w, r, ErrNotFound)
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		h.fail(w, r, ErrInvalid)
		return
	}
	rel, err := h.s.Withdraw(r.Context(), actor(r), id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	a := actor(r)
	h.d.Activity.Record(r.Context(), &a, "desktop_release.withdrawn", "desktop_release", &rel.UUID,
		map[string]any{"app": rel.App, "channel": rel.Channel, "version": rel.Version, "target": rel.Target, "reason": rel.WithdrawnReason}, r)
	response.JSON(w, r, http.StatusOK, rel)
}
