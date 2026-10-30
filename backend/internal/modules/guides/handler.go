package guides

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// Resource is the activity log resource name.
const Resource = "guide"

// Error codes.
const (
	CodeGuideNotFound   = "GUIDE_NOT_FOUND"
	CodeGuideConflict   = "GUIDE_KEY_EXISTS"
	CodeAIUnavailable   = "ASSISTANT_AI_UNAVAILABLE"
	CodeAttachment      = "FEEDBACK_ATTACHMENT_INVALID"
	maxBodyBytes        = 256 << 10
	maxGenerateBodySize = 64 << 10
)

// Handler serves the guides and product feedback endpoints.
type Handler struct {
	svc      *Service
	activity *activity.Recorder
}

// NewHandler builds the handler.
func NewHandler(svc *Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// RegisterRoutes mounts /v1/tenant/guides*, /v1/tenant/product-feedback
// (any member of the organization) and the platform editor routes.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	authn := middleware.Authenticate(tokens, loader)
	if resolver != nil {
		org := middleware.RequireOrganization(resolver)
		tenant := func(pattern string, fn http.HandlerFunc) {
			mux.Handle(pattern, middleware.Chain(fn, authn, org))
		}
		tenant("GET /v1/tenant/guides", h.List)
		tenant("GET /v1/tenant/guides/setup", h.Setup)
		tenant("GET /v1/tenant/guides/preferences", h.Preferences)
		tenant("PUT /v1/tenant/guides/preferences", h.SavePreferences)
		tenant("POST /v1/tenant/guides/{key}/auto-start", h.AutoStart)
		tenant("PUT /v1/tenant/guides/{key}/progress", h.Progress)
		tenant("POST /v1/tenant/guides/reset", h.Reset)
		tenant("POST /v1/tenant/product-feedback", h.SubmitFeedback)
	}
	platform := func(pattern, perm string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, authn, middleware.RequirePermission(perm)))
	}
	platform("GET /v1/platform/guides", rbac.PermPlatformGuidesManage, h.AdminList)
	platform("POST /v1/platform/guides", rbac.PermPlatformGuidesManage, h.AdminCreate)
	platform("POST /v1/platform/guides/generate", rbac.PermPlatformGuidesManage, h.AdminGenerate)
	platform("GET /v1/platform/guides/{uuid}", rbac.PermPlatformGuidesManage, h.AdminGet)
	platform("PUT /v1/platform/guides/{uuid}", rbac.PermPlatformGuidesManage, h.AdminUpdate)
	platform("DELETE /v1/platform/guides/{uuid}", rbac.PermPlatformGuidesManage, h.AdminDelete)
	platform("GET /v1/platform/product-feedback", rbac.PermPlatformProductFeedbackRead, h.AdminFeedback)
	platform("PATCH /v1/platform/product-feedback/{uuid}", rbac.PermPlatformProductFeedbackRead, h.AdminFeedbackStatus)
}

func decode(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Code: response.CodeValidationError, Message: "invalid input"}})
	case errors.Is(err, ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		response.Error(w, r, http.StatusNotFound, CodeGuideNotFound, "Guide not found")
	case errors.Is(err, ErrConflict):
		response.Conflict(w, r, CodeGuideConflict, "A guide with this key already exists for the locale")
	case errors.Is(err, ErrAttachment):
		response.ValidationError(w, r, []response.Detail{{Field: "attachment_uuids", Code: CodeAttachment, Message: "unknown attachment"}})
	case errors.Is(err, ErrAIUnavailable):
		response.ServiceUnavailable(w, r, CodeAIUnavailable, "AI is not available")
	default:
		response.InternalErr(w, r, err, "")
	}
}

func scope(w http.ResponseWriter, r *http.Request) (orgctx.Scope, int64, bool) {
	sc, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("guides: missing organization scope"), "")
		return orgctx.Scope{}, 0, false
	}
	return sc, authctx.MustPrincipal(r.Context()).UserInternal, true
}

// List: GET /v1/tenant/guides?surface=&route=&locale=
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	sc, user, ok := scope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	items, err := h.svc.ForRoute(r.Context(), sc.ID, user, q.Get("surface"), q.Get("route"), q.Get("locale"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Setup: GET /v1/tenant/guides/setup
func (h *Handler) Setup(w http.ResponseWriter, r *http.Request) {
	sc, user, ok := scope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	tour, err := h.svc.Setup(r.Context(), sc.ID, user, q.Get("surface"), q.Get("locale"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tour)
}

// Progress: PUT /v1/tenant/guides/{key}/progress
func (h *Handler) Progress(w http.ResponseWriter, r *http.Request) {
	sc, user, ok := scope(w, r)
	if !ok {
		return
	}
	var in Progress
	if !decode(w, r, &in, 4<<10) {
		return
	}
	if err := h.svc.SaveProgress(r.Context(), sc.ID, user, r.PathValue("key"), in); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Reset: POST /v1/tenant/guides/reset
func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	sc, user, ok := scope(w, r)
	if !ok {
		return
	}
	if err := h.svc.Reset(r.Context(), sc.ID, user); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SubmitFeedback: POST /v1/tenant/product-feedback
func (h *Handler) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	sc, user, ok := scope(w, r)
	if !ok {
		return
	}
	var in FeedbackInput
	if !decode(w, r, &in, 32<<10) {
		return
	}
	id, err := h.svc.SubmitFeedback(r.Context(), sc.ID, user, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), &user, "product_feedback.created", "product_feedback", &id,
			map[string]any{"organization_uuid": sc.UUID, "kind": in.Kind, "surface": in.Surface}, r)
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"uuid": id})
}

func intParam(r *http.Request, key string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(key))
	return n
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.Error(w, r, http.StatusNotFound, CodeGuideNotFound, "Not found")
		return uuid.Nil, false
	}
	return id, true
}

func actor(r *http.Request) int64 { return authctx.MustPrincipal(r.Context()).UserInternal }

func (h *Handler) record(r *http.Request, action string, g AdminGuide) {
	if h.activity == nil {
		return
	}
	a := actor(r)
	h.activity.Record(r.Context(), &a, action, Resource, &g.UUID, map[string]any{"key": g.Key, "locale": g.Locale}, r)
}

// AdminList: GET /v1/platform/guides
func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.svc.AdminList(r.Context(), q.Get("surface"), q.Get("locale"), q.Get("q"), intParam(r, "limit"), intParam(r, "offset"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

// AdminGet: GET /v1/platform/guides/{uuid}
func (h *Handler) AdminGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	g, err := h.svc.AdminGet(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, g)
}

// AdminCreate: POST /v1/platform/guides
func (h *Handler) AdminCreate(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !decode(w, r, &in, maxBodyBytes) {
		return
	}
	g, err := h.svc.Create(r.Context(), actor(r), in, "custom")
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "guide.created", g)
	response.JSON(w, r, http.StatusCreated, g)
}

// AdminUpdate: PUT /v1/platform/guides/{uuid}
func (h *Handler) AdminUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in Input
	if !decode(w, r, &in, maxBodyBytes) {
		return
	}
	g, err := h.svc.Update(r.Context(), actor(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "guide.updated", g)
	response.JSON(w, r, http.StatusOK, g)
}

// AdminDelete: DELETE /v1/platform/guides/{uuid}
func (h *Handler) AdminDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	g, err := h.svc.AdminGet(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "guide.deleted", g)
	w.WriteHeader(http.StatusNoContent)
}

// AdminGenerate: POST /v1/platform/guides/generate
func (h *Handler) AdminGenerate(w http.ResponseWriter, r *http.Request) {
	var in GenerateInput
	if !decode(w, r, &in, maxGenerateBodySize) {
		return
	}
	draft, saved, err := h.svc.Generate(r.Context(), actor(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if saved != nil {
		h.record(r, "guide.generated", *saved)
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"draft": draft, "guide": saved})
}

// AdminFeedback: GET /v1/platform/product-feedback
func (h *Handler) AdminFeedback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.svc.ListFeedback(r.Context(), q.Get("kind"), q.Get("status"), intParam(r, "limit"), intParam(r, "offset"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

// AdminFeedbackStatus: PATCH /v1/platform/product-feedback/{uuid}
func (h *Handler) AdminFeedbackStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &in, 4<<10) {
		return
	}
	if err := h.svc.SetFeedbackStatus(r.Context(), id, in.Status); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Preferences is shared by every device of the current user.
func (h *Handler) Preferences(w http.ResponseWriter, r *http.Request) {
	_, user, ok := scope(w, r)
	if !ok {
		return
	}
	prefs, err := h.svc.Preferences(r.Context(), user)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, prefs)
}

func (h *Handler) SavePreferences(w http.ResponseWriter, r *http.Request) {
	_, user, ok := scope(w, r)
	if !ok {
		return
	}
	var in struct {
		AutoStart *bool `json:"auto_start"`
	}
	if !decode(w, r, &in, 1024) {
		return
	}
	if in.AutoStart == nil {
		writeError(w, r, ErrInvalid)
		return
	}
	if err := h.svc.SavePreferences(r.Context(), user, *in.AutoStart); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"auto_start": *in.AutoStart})
}

func (h *Handler) AutoStart(w http.ResponseWriter, r *http.Request) {
	sc, user, ok := scope(w, r)
	if !ok {
		return
	}
	started, err := h.svc.ClaimAutoStart(r.Context(), sc.ID, user, r.PathValue("key"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"started": started})
}
