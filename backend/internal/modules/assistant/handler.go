package assistant

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// API error codes.
const (
	CodeNotFound           = "ASSISTANT_NOT_FOUND"
	CodeDisabled           = "ASSISTANT_DISABLED"
	CodeAIUnavailable      = "ASSISTANT_AI_UNAVAILABLE"
	CodeDailyLimit         = "ASSISTANT_DAILY_LIMIT"
	CodeActionState        = "ASSISTANT_ACTION_NOT_PENDING"
	CodeSupportUnavailable = "ASSISTANT_SUPPORT_UNAVAILABLE"
	CodeUnsafe             = "ASSISTANT_MESSAGE_REJECTED"
	CodeActionFailed       = "ASSISTANT_ACTION_FAILED"
)

// Handler serves /v1/tenant/assistant/* and /v1/platform/assistant/*.
type Handler struct {
	svc      *Service
	activity *activity.Recorder
	// limit is a short-window burst limiter (per organization); nil = none.
	limit func(r *http.Request, action, subject string, limit int, window time.Duration) bool
}

// NewHandler builds the handler.
func NewHandler(svc *Service, rec *activity.Recorder, limit func(r *http.Request, action, subject string, limit int, window time.Duration) bool) *Handler {
	return &Handler{svc: svc, activity: rec, limit: limit}
}

// RegisterRoutes mounts the routes. Every member with tenant.assistant.use
// (all template roles) gets the chat; action confirmation re-checks the
// action's own permission.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	authn := middleware.Authenticate(tokens, loader)
	if resolver != nil {
		org := middleware.RequireOrganization(resolver)
		use := middleware.RequireTenantPermission(rbac.PermTenantAssistantUse)
		tenant := func(pattern string, fn http.HandlerFunc) {
			mux.Handle(pattern, middleware.Chain(fn, authn, org, use))
		}
		tenant("GET /v1/tenant/assistant/status", h.Status)
		tenant("GET /v1/tenant/assistant/threads", h.Threads)
		tenant("POST /v1/tenant/assistant/threads", h.CreateThread)
		tenant("GET /v1/tenant/assistant/threads/{uuid}/messages", h.Messages)
		tenant("POST /v1/tenant/assistant/threads/{uuid}/messages", h.Send)
		tenant("POST /v1/tenant/assistant/threads/{uuid}/messages/stream", h.SendStream)
		tenant("POST /v1/tenant/assistant/threads/{uuid}/actions/{action_id}/confirm", h.Confirm)
		tenant("POST /v1/tenant/assistant/threads/{uuid}/actions/{action_id}/reject", h.Reject)
		tenant("POST /v1/tenant/assistant/threads/{uuid}/escalate", h.Escalate)
	}
	manage := middleware.RequirePermission(rbac.PermPlatformAssistantManage)
	mux.Handle("GET /v1/platform/assistant/settings", middleware.Chain(http.HandlerFunc(h.GetSettings), authn, manage))
	mux.Handle("PUT /v1/platform/assistant/settings", middleware.Chain(http.HandlerFunc(h.PutSettings), authn, manage))
	mux.Handle("GET /v1/platform/assistant/usage", middleware.Chain(http.HandlerFunc(h.Usage), authn, manage))
}

func decode(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil || dec.Decode(&struct{}{}) != io.EOF {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

// WriteError maps service errors to API responses.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *orgusecase.ValidationError
	switch {
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrActionInvalid), errors.As(err, &ve), errors.Is(err, usecase.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Code: response.CodeValidationError, Message: "invalid input"}})
	case errors.Is(err, ErrNotFound):
		response.Error(w, r, http.StatusNotFound, CodeNotFound, "Not found")
	case errors.Is(err, ErrForbidden):
		response.Forbidden(w, r, "You do not have permission for this action")
	case errors.Is(err, ErrDisabled):
		response.Error(w, r, http.StatusForbidden, CodeDisabled, "The assistant is turned off")
	case errors.Is(err, ErrAIUnavailable):
		// The assistant's AI route is not configured: a state the status
		// endpoint already reports, not a server fault.
		response.Conflict(w, r, CodeAIUnavailable, "AI is not available")
	case errors.Is(err, ErrDailyLimit):
		response.Error(w, r, http.StatusTooManyRequests, CodeDailyLimit, "Daily assistant limit reached")
	case errors.Is(err, ErrActionState):
		response.Conflict(w, r, CodeActionState, "This action is no longer pending")
	case errors.Is(err, ErrSupportUnavailable):
		response.Conflict(w, r, CodeSupportUnavailable, "Support escalation is not configured")
	case errors.Is(err, ErrUnsafe):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeUnsafe, "Message rejected by the safety filter")
	default:
		response.InternalErr(w, r, err, "")
	}
}

func caller(w http.ResponseWriter, r *http.Request) (Caller, bool) {
	sc, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("assistant: missing organization scope"), "")
		return Caller{}, false
	}
	p := authctx.MustPrincipal(r.Context())
	locale := r.URL.Query().Get("locale")
	if locale == "" {
		locale = r.Header.Get("Accept-Language")
	}
	return Caller{Scope: sc, UserID: p.UserInternal, Locale: locale}, true
}

func threadID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.Error(w, r, http.StatusNotFound, CodeNotFound, "Not found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) record(r *http.Request, c Caller, action string, id uuid.UUID, meta map[string]any) {
	if h.activity == nil {
		return
	}
	if meta == nil {
		meta = map[string]any{}
	}
	meta["organization_uuid"] = c.Scope.UUID
	h.activity.Record(r.Context(), &c.UserID, action, "assistant_thread", &id, meta, r)
}

// Status: GET /v1/tenant/assistant/status
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	st, _, err := h.svc.Status(r.Context(), c.Scope.ID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// Threads: GET /v1/tenant/assistant/threads
func (h *Handler) Threads(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Threads(r.Context(), c.Scope.ID, c.UserID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateThread: POST /v1/tenant/assistant/threads
func (h *Handler) CreateThread(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	var in struct {
		Context Context `json:"context"`
	}
	if !decode(w, r, &in, 8<<10) {
		return
	}
	t, err := h.svc.CreateThread(r.Context(), c.Scope.ID, c.UserID, in.Context)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, t)
}

// Messages: GET /v1/tenant/assistant/threads/{uuid}/messages
func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Messages(r.Context(), c.Scope.ID, c.UserID, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Send: POST /v1/tenant/assistant/threads/{uuid}/messages
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	var in struct {
		Content string  `json:"content"`
		Context Context `json:"context"`
	}
	if !decode(w, r, &in, 32<<10) {
		return
	}
	if h.limit != nil && !h.limit(r, "assistant_send", strconv.FormatInt(c.Scope.ID, 10), 20, time.Minute) {
		response.TooManyRequests(w, r, "Too many messages, slow down")
		return
	}
	out, err := h.svc.Send(r.Context(), c, id, in.Content, in.Context)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Confirm: POST /v1/tenant/assistant/threads/{uuid}/actions/{action_id}/confirm
func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	msg, err := h.svc.Confirm(r.Context(), c, id, r.PathValue("action_id"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	meta := map[string]any{"action_id": r.PathValue("action_id")}
	if len(msg.Actions) == 1 {
		meta["kind"], meta["status"] = msg.Actions[0].Kind, msg.Actions[0].Status
	}
	h.record(r, c, "assistant.action_confirmed", id, meta)
	response.JSON(w, r, http.StatusOK, msg)
}

// Reject: POST /v1/tenant/assistant/threads/{uuid}/actions/{action_id}/reject
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Reject(r.Context(), c, id, r.PathValue("action_id")); err != nil {
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Escalate: POST /v1/tenant/assistant/threads/{uuid}/escalate
func (h *Handler) Escalate(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	var in struct {
		Note string `json:"note"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in, 8<<10) {
		return
	}
	if h.limit != nil && !h.limit(r, "assistant_escalate", strconv.FormatInt(c.Scope.ID, 10), 10, time.Hour) {
		response.TooManyRequests(w, r, "Too many escalations, try again later")
		return
	}
	ref, err := h.svc.Escalate(r.Context(), c, id, in.Note)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	h.record(r, c, "assistant.escalated", id, map[string]any{"ticket_ref": ref})
	response.JSON(w, r, http.StatusOK, map[string]any{"ticket_ref": ref})
}

// GetSettings: GET /v1/platform/assistant/settings
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.GetSettings(r.Context())
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// PutSettings: PUT /v1/platform/assistant/settings
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if !decode(w, r, &in, 16<<10) {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	st, err := h.svc.SaveSettings(r.Context(), actor, in)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), &actor, "assistant.settings_updated", "assistant_settings", nil,
			map[string]any{"enabled": st.Enabled, "model_route": st.ModelRoute, "daily_limit_per_org": st.DailyLimitPerOrg}, r)
	}
	response.JSON(w, r, http.StatusOK, st)
}

// Usage: GET /v1/platform/assistant/usage?days=30
func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 || days > 365 {
		days = 30
	}
	out, err := h.svc.Usage(r.Context(), time.Now().AddDate(0, 0, -days))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
