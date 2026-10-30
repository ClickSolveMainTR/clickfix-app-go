package marketing

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// Resource is the activity resource of settings changes.
const Resource = "platform.marketing"

// Handler serves the settings, the public read and the report.
type Handler struct {
	svc      *Service
	storage  storage.Driver
	activity *activity.Recorder
}

// NewHandler builds the handler.
func NewHandler(svc *Service, store storage.Driver, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, storage: store, activity: rec}
}

func actorID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func (h *Handler) record(r *http.Request, action string, payload map[string]any) {
	if h.activity == nil {
		return
	}
	actor := actorID(r)
	h.activity.Record(r.Context(), &actor, action, Resource, nil, payload, r)
}

// Get returns the settings for the admin page.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Get(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "failed to load marketing settings")
		return
	}
	response.JSON(w, r, http.StatusOK, s)
}

// Put replaces the whole config.
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	var in Config
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 3*MaxSnippetBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return
	}
	var details []response.Detail
	if len(in.CustomHead) > MaxSnippetBytes {
		details = append(details, response.Detail{Field: "custom_head", Message: "at most 20 KB"})
	}
	if len(in.CustomBody) > MaxSnippetBytes {
		details = append(details, response.Detail{Field: "custom_body", Message: "at most 20 KB"})
	}
	cfg := in.Normalize()
	details = append(details, cfg.Validate()...)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	s, err := h.svc.Save(r.Context(), cfg, actorID(r))
	if err != nil {
		response.InternalErr(w, r, err, "failed to save marketing settings")
		return
	}
	h.record(r, "platform.marketing.updated", map[string]any{
		"custom_head_changed": cfg.CustomHead != "", "custom_body_changed": cfg.CustomBody != "",
	})
	response.JSON(w, r, http.StatusOK, s)
}

// UploadOGImage stores the default share image (PNG / JPEG / WebP, 2 MiB).
func (h *Handler) UploadOGImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, (3<<20)+(1<<20))
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "image is required")
		return
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime, err := storage.DetectLogoMIME(header.Header.Get("Content-Type"), head[:n])
	if err == nil {
		err = storage.ValidateLogoSize(header.Size)
	}
	var ext string
	if err == nil {
		ext, err = storage.LogoExtForMIME(mime)
	}
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, strings.ReplaceAll(err.Error(), "logo", "image"))
		return
	}
	previous, _ := h.svc.OGImageKey(r.Context())
	key := "app/marketing/og-image." + ext
	body := io.MultiReader(strings.NewReader(string(head[:n])), file)
	if err := h.storage.Upload(r.Context(), storage.File{
		Body: body, Size: header.Size, ContentType: mime, Filename: header.Filename,
	}, key); err != nil {
		response.InternalErr(w, r, err, "image upload failed")
		return
	}
	if previous != "" && previous != key {
		_ = h.storage.Delete(r.Context(), previous)
	}
	s, err := h.svc.SetOGImage(r.Context(), key, actorID(r))
	if err != nil {
		response.InternalErr(w, r, err, "failed to save image")
		return
	}
	h.record(r, "platform.marketing.og_image_updated", nil)
	response.JSON(w, r, http.StatusOK, s)
}

// DeleteOGImage removes the share image (pages fall back to the generated card).
func (h *Handler) DeleteOGImage(w http.ResponseWriter, r *http.Request) {
	if key, err := h.svc.OGImageKey(r.Context()); err == nil {
		_ = h.storage.Delete(r.Context(), key)
	}
	s, err := h.svc.SetOGImage(r.Context(), "", actorID(r))
	if err != nil {
		response.InternalErr(w, r, err, "failed to clear image")
		return
	}
	h.record(r, "platform.marketing.og_image_removed", nil)
	response.JSON(w, r, http.StatusOK, s)
}

// Public is the unauthenticated read for the marketing site (cached briefly).
func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Get(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "failed to load marketing settings")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	response.JSON(w, r, http.StatusOK, s)
}

// StreamOGImage serves the share image; its URL is versioned, so it caches long.
func (h *Handler) StreamOGImage(w http.ResponseWriter, r *http.Request) {
	key, err := h.svc.OGImageKey(r.Context())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.NotFound(w, r, "image not set")
			return
		}
		response.InternalErr(w, r, err, "failed to load image")
		return
	}
	rc, _, err := h.storage.Download(r.Context(), key)
	if err != nil {
		response.NotFound(w, r, "image not found")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", storage.MIMEFromLogoKey(key))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, rc)
}

// OrganizationAttribution returns where an organization signed up from.
func (h *Handler) OrganizationAttribution(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "organization uuid is invalid")
		return
	}
	a, err := h.svc.ForOrganization(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		response.NotFound(w, r, "no attribution recorded")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "failed to load attribution")
		return
	}
	response.JSON(w, r, http.StatusOK, a)
}

// Sources is the "Kaynaklar" report: organizations by sign-up source in a
// date range (`from` / `to`, YYYY-MM-DD, `to` inclusive; default 90 days).
func (h *Handler) Sources(w http.ResponseWriter, r *http.Request) {
	from, to, ok := reportRange(r, time.Now().UTC())
	if !ok {
		response.BadRequest(w, r, response.CodeValidationError, "from / to must be YYYY-MM-DD, from <= to, at most 2 years")
		return
	}
	rep, err := h.svc.Sources(r.Context(), from, to)
	if err != nil {
		response.InternalErr(w, r, err, "failed to build report")
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

func reportRange(r *http.Request, now time.Time) (time.Time, time.Time, bool) {
	day := func(name string, def time.Time) (time.Time, bool) {
		v := r.URL.Query().Get(name)
		if v == "" {
			return def, true
		}
		t, err := time.Parse(time.DateOnly, v)
		return t, err == nil
	}
	today := now.Truncate(24 * time.Hour)
	from, ok1 := day("from", today.AddDate(0, 0, -89))
	to, ok2 := day("to", today)
	to = to.AddDate(0, 0, 1)
	if !ok1 || !ok2 || !from.Before(to) || to.Sub(from) > 731*24*time.Hour {
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}
