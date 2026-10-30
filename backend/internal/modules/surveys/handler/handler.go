package handler

import (
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"time"
)

type Limiter func(*http.Request) bool
type Handler struct {
	svc   *usecase.Service
	rec   *activity.Recorder
	limit Limiter
}

func New(s *usecase.Service, rec *activity.Recorder, limit Limiter) *Handler {
	return &Handler{s, rec, limit}
}
func fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, model.ErrInvalid):
		response.Error(w, r, 400, "VALIDATION_ERROR", "Invalid survey input")
	case errors.Is(err, model.ErrExpired):
		response.Error(w, r, 410, "SURVEY_EXPIRED", "Survey expired")
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(w, r, "Survey not found")
	default:
		response.Internal(w, r, "Survey unavailable")
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		response.Error(w, r, 400, "VALIDATION_ERROR", "Invalid JSON")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		response.Error(w, r, 400, "VALIDATION_ERROR", "Expected one JSON value")
		return false
	}
	return true
}
func (h *Handler) Templates(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Templates(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"items": v})
}
func (h *Handler) SaveTemplate(w http.ResponseWriter, r *http.Request) {
	var in model.TemplateInput
	if !decode(w, r, &in) {
		return
	}
	var id uuid.UUID
	if v := r.PathValue("uuid"); v != "" {
		var err error
		id, err = uuid.Parse(v)
		if err != nil {
			fail(w, r, model.ErrInvalid)
			return
		}
	}
	v, err := h.svc.SaveTemplate(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	h.rec.Record(r.Context(), &actor, "settings.updated", "settings", &id, map[string]any{"module": "surveys", "operation": "save_template"}, r)
	response.JSON(w, r, 200, v)
}
func (h *Handler) Invite(w http.ResponseWriter, r *http.Request) {
	var in model.InviteInput
	if !decode(w, r, &in) {
		return
	}
	id, err := h.svc.Invite(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	h.rec.Record(r.Context(), &actor, "settings.updated", "settings", &id, map[string]any{"module": "surveys", "operation": "invite"}, r)
	response.JSON(w, r, 202, map[string]any{"uuid": id})
}
func (h *Handler) Results(w http.ResponseWriter, r *http.Request) {
	from, _ := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, _ := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	v, err := h.svc.Results(r.Context(), orgctx.Must(r.Context()).ID, from, to)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]any{"items": v})
}
func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if h.limit != nil && !h.limit(r) {
		response.Error(w, r, 429, "RATE_LIMITED", "Too many requests")
		return
	}
	token := r.PathValue("token")
	if r.Method == http.MethodGet {
		v, err := h.svc.Public(r.Context(), token)
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, v)
		return
	}
	var in model.Answer
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.Answer(r.Context(), token, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, v)
}
