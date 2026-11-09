package assistant

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
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
	CodeDocumentType       = "ASSISTANT_DOCUMENT_TYPE"
	CodeDocumentTooLarge   = "ASSISTANT_DOCUMENT_TOO_LARGE"
	CodeDocumentLimit      = "ASSISTANT_DOCUMENT_LIMIT"
	CodeDocumentEmpty      = "ASSISTANT_DOCUMENT_EMPTY"
	CodeDocumentFetch      = "ASSISTANT_DOCUMENT_FETCH"
	CodeDocumentPDF        = "ASSISTANT_DOCUMENT_PDF"
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
		tenant("POST /v1/tenant/assistant/threads/{uuid}/actions/{action_id}/dry-run", h.DryRun)
		tenant("POST /v1/tenant/assistant/threads/{uuid}/actions/{action_id}/reject", h.Reject)
		tenant("POST /v1/tenant/assistant/threads/{uuid}/escalate", h.Escalate)
		tenant("GET /v1/tenant/assistant/threads/{uuid}/documents", h.Documents)
		tenant("POST /v1/tenant/assistant/threads/{uuid}/documents", h.UploadDocument)
		tenant("POST /v1/tenant/assistant/threads/{uuid}/documents/link", h.LinkDocument)
		tenant("DELETE /v1/tenant/assistant/threads/{uuid}/documents/{document_uuid}", h.DeleteDocument)
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
	var pe *capability.ParamError
	var fe *FetchError
	switch {
	case errors.As(err, &fe):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeDocumentFetch, fe.Message)
	case errors.Is(err, ErrDocumentType):
		response.Error(w, r, http.StatusUnsupportedMediaType, CodeDocumentType, "Only PDF, text, Markdown, CSV, JSON, YAML and HTML documents can be read")
	case errors.Is(err, ErrDocumentPDF):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeDocumentPDF, "The PDF text cannot be read (encrypted, damaged or scanned)")
	case errors.Is(err, ErrDocumentTooLarge):
		response.Error(w, r, http.StatusRequestEntityTooLarge, CodeDocumentTooLarge, "The document is larger than 2 MB")
	case errors.Is(err, ErrDocumentLimit):
		response.Conflict(w, r, CodeDocumentLimit, "A chat can hold 5 documents; remove one first")
	case errors.Is(err, ErrDocumentEmpty):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeDocumentEmpty, "The document has no readable text")
	case errors.As(err, &pe):
		response.ValidationError(w, r, []response.Detail{{Code: response.CodeValidationError, Message: pe.Message}})
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
	case errors.Is(err, ErrAPIUnavailable):
		response.Error(w, r, http.StatusServiceUnavailable, CodeActionFailed, "The action cannot run right now; try again")
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

type actionRequest struct {
	Params capability.Params `json:"params"`
}

// actionInput reads the optional {params} body of confirm / dry-run.
func actionInput(w http.ResponseWriter, r *http.Request) (capability.Params, bool) {
	// An empty body (any transfer encoding) means "as proposed".
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<10))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return nil, false
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, true
	}
	var in actionRequest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || dec.Decode(&struct{}{}) != io.EOF {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return nil, false
	}
	return in.Params, true
}

// actionAllowed is the per-organization limit of dry runs and confirms.
func (h *Handler) actionAllowed(w http.ResponseWriter, r *http.Request, c Caller) bool {
	if h.limit != nil && !h.limit(r, "assistant_action", strconv.FormatInt(c.Scope.ID, 10), 30, time.Minute) {
		response.TooManyRequests(w, r, "Too many actions, slow down")
		return false
	}
	return true
}

// auditAction records an action event; params never carry secrets
// (capabilities strip them) and pass through activity.Scrub.
func (h *Handler) auditAction(r *http.Request, c Caller, event string, id uuid.UUID, a *ProposedAction, extra map[string]any) {
	meta := map[string]any{"action_id": r.PathValue("action_id")}
	if a != nil {
		meta["kind"], meta["status"] = a.Kind, a.Status
		if a.Capability != "" {
			meta["capability"], meta["risk"] = a.Capability, a.Risk
			if cp, ok := h.svc.Caps.Get(a.Capability); ok {
				meta["params"] = map[string]any(cp.WithoutSecrets(a.Params))
			}
		} else if len(a.Params) > 0 {
			// Older actions: secret-looking values are masked.
			meta["params"] = redactParams(a.Params)
		}
		if a.Verification != nil {
			meta["verification"] = a.Verification.Status
		}
	}
	maps.Copy(meta, extra)
	h.record(r, c, event, id, meta)
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
	params, ok := actionInput(w, r)
	if !ok || !h.actionAllowed(w, r, c) {
		return
	}
	out, err := h.svc.Confirm(r.Context(), c, id, r.PathValue("action_id"), params)
	if errors.Is(err, ErrForbidden) {
		h.auditAction(r, c, "assistant.action_forbidden", id, nil, nil)
	}
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var a *ProposedAction
	if len(out.Actions) == 1 {
		a = &out.Actions[0]
	}
	h.auditAction(r, c, "assistant.action_confirmed", id, a, map[string]any{"edited": len(params) > 0})
	response.JSON(w, r, http.StatusOK, out)
}

// DryRun: POST /v1/tenant/assistant/threads/{uuid}/actions/{action_id}/dry-run
func (h *Handler) DryRun(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	params, ok := actionInput(w, r)
	if !ok || !h.actionAllowed(w, r, c) {
		return
	}
	a, err := h.svc.DryRun(r.Context(), c, id, r.PathValue("action_id"), params)
	if errors.Is(err, ErrForbidden) {
		h.auditAction(r, c, "assistant.action_forbidden", id, nil, map[string]any{"dry_run": true})
	}
	if err != nil {
		WriteError(w, r, err)
		return
	}
	h.auditAction(r, c, "assistant.action_dry_run", id, &a, map[string]any{"ok": a.Preview != nil && a.Preview.OK})
	response.JSON(w, r, http.StatusOK, a)
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
	h.auditAction(r, c, "assistant.action_rejected", id, nil, nil)
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

// documentAllowed limits document uploads and fetches per organization.
func (h *Handler) documentAllowed(w http.ResponseWriter, r *http.Request, c Caller) bool {
	if h.limit != nil && !h.limit(r, "assistant_document", strconv.FormatInt(c.Scope.ID, 10), 20, time.Hour) {
		response.TooManyRequests(w, r, "Too many documents, try again later")
		return false
	}
	return true
}

func (h *Handler) documentAdded(r *http.Request, c Caller, id uuid.UUID, d Document) {
	h.record(r, c, "assistant.document_added", id, map[string]any{"document_uuid": d.UUID, "kind": d.Kind,
		"content_type": d.ContentType, "size_bytes": d.SizeBytes, "masked": d.Masked})
}

// Documents: GET /v1/tenant/assistant/threads/{uuid}/documents
func (h *Handler) Documents(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Documents(r.Context(), c, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// UploadDocument: POST /v1/tenant/assistant/threads/{uuid}/documents
// (multipart/form-data, field "file", at most 2 MB).
func (h *Handler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	if !h.documentAllowed(w, r, c) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxDocumentBytes+64<<10)
	if err := r.ParseMultipartForm(MaxDocumentBytes + 64<<10); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			WriteError(w, r, ErrDocumentTooLarge)
			return
		}
		response.BadRequest(w, r, response.CodeValidationError, "A multipart form with a file field is required")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "A multipart form with a file field is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxDocumentBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "The file could not be read")
		return
	}
	d, err := h.svc.AddFile(r.Context(), c, id, header.Filename, header.Header.Get("Content-Type"), data)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	h.documentAdded(r, c, id, d)
	response.JSON(w, r, http.StatusCreated, d)
}

// LinkDocument: POST /v1/tenant/assistant/threads/{uuid}/documents/link
func (h *Handler) LinkDocument(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	var in struct {
		URL string `json:"url"`
	}
	if !decode(w, r, &in, 4<<10) {
		return
	}
	if !h.documentAllowed(w, r, c) {
		return
	}
	d, err := h.svc.AddLink(r.Context(), c, id, in.URL)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	h.documentAdded(r, c, id, d)
	response.JSON(w, r, http.StatusCreated, d)
}

// DeleteDocument: DELETE /v1/tenant/assistant/threads/{uuid}/documents/{document_uuid}
func (h *Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := threadID(w, r)
	if !ok {
		return
	}
	doc, err := uuid.Parse(r.PathValue("document_uuid"))
	if err != nil {
		WriteError(w, r, ErrNotFound)
		return
	}
	if err := h.svc.DeleteDocument(r.Context(), c, id, doc); err != nil {
		WriteError(w, r, err)
		return
	}
	h.record(r, c, "assistant.document_removed", id, map[string]any{"document_uuid": doc})
	w.WriteHeader(http.StatusNoContent)
}

// redactParams masks secret-looking strings of older actions' params.
func redactParams(p map[string]any) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		if str, ok := v.(string); ok {
			v = capability.RedactSecrets(str)
		}
		out[k] = v
	}
	return out
}
