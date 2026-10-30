// Package handler exposes the engineering bridge HTTP contract.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type Handler struct {
	Service  *usecase.Service
	Activity *activity.Recorder
}

func (h *Handler) WithViewer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope := orgctx.Must(r.Context())
		p := authctx.MustPrincipal(r.Context())
		viewer, err := access.For(r.Context(), h.Service.Store.Q, scope, p.UserInternal)
		if err != nil {
			writeError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(access.WithViewer(r.Context(), viewer)))
	})
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Expected one JSON value")
		return false
	}
	return true
}
func id(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	v, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return uuid.Nil, false
	}
	return v, true
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var pe *pgconn.PgError
	switch {
	case errors.Is(err, usecase.ErrNotFound), errors.Is(err, conv.ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrConflict):
		response.Conflict(w, r, "REVISION_CONFLICT", "Refresh and retry")
	case errors.Is(err, usecase.ErrSignature):
		response.Unauthorized(w, r, "Invalid webhook signature")
	case errors.Is(err, usecase.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Message: err.Error(), Code: "invalid"}})
	case errors.As(err, &pe) && pe.Code == "23505":
		response.Conflict(w, r, "ALREADY_EXISTS", "Already exists")
	case errors.As(err, &pe) && (pe.Code == "23503" || pe.Code == "23514"):
		response.ValidationError(w, r, []response.Detail{{Message: "Invalid resource reference", Code: "invalid"}})
	case errors.Is(err, usecase.ErrUnavailable):
		response.BadRequest(w, r, "INTEGRATION_UNAVAILABLE", "Integration unavailable")
	default:
		response.InternalErr(w, r, err, "")
	}
}
func (h *Handler) respond(w http.ResponseWriter, r *http.Request, status int, out any, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, status, out)
}
func (h *Handler) record(r *http.Request, id uuid.UUID, operation string) {
	if h.Activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	scope := orgctx.Must(r.Context())
	// The payload preserves the exact operation for the audit log.
	h.Activity.Record(r.Context(), &p.UserInternal, "engineering.changed", "engineering", &id, map[string]any{"organization_uuid": scope.UUID, "engineering_operation": operation}, r)
}
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, resourcemeta.ResourceMeta{Resource: "engineering.issues", DefaultSort: "-updated_at", DefaultFields: []string{"uuid", "title", "status", "priority"}, Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true, Search: true, Filter: true}, SearchableFields: []string{"title", "description"}, SortableFields: []string{"updated_at"}, FilterableFields: []string{"status", "priority"}, Columns: []resourcemeta.Column{}, Filters: []resourcemeta.Filter{}, Includes: []string{}})
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.Service.List(r.Context(), scope.ID, apiquery.Parse(r.URL.Query()), r.URL.Query().Get("status"), r.URL.Query().Get("priority"))
	h.respond(w, r, 200, out, err)
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	out, err := h.Service.Get(r.Context(), orgctx.Must(r.Context()).ID, v)
	h.respond(w, r, 200, out, err)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.CreateInput
	if !decode(w, r, &in) {
		return
	}
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	if in.AISummary && !scope.Has(rbac.PermTenantConversationsReply) {
		response.Forbidden(w, r, "AI summary requires conversation reply permission")
		return
	}
	out, err := h.Service.Create(r.Context(), scope.ID, p.UserInternal, in)
	if err == nil {
		h.record(r, out.UUID, "issue_created")
	}
	h.respond(w, r, 201, out, err)
}
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	var in model.PatchInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Service.Patch(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v, in)
	if err == nil {
		h.record(r, v, "issue_updated")
	}
	h.respond(w, r, 200, out, err)
}
func (h *Handler) Links(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	out, err := h.Service.Links(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) Link(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	var in model.LinkInput
	if !decode(w, r, &in) {
		return
	}
	err := h.Service.Link(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v, in, false)
	if err == nil {
		h.record(r, v, "conversation_linked")
	}
	h.respond(w, r, 200, map[string]bool{"linked": true}, err)
}
func (h *Handler) Unlink(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	c, ok := id(w, r, "conversation")
	if !ok {
		return
	}
	err := h.Service.Link(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v, model.LinkInput{Conversation: c}, true)
	if err == nil {
		h.record(r, v, "conversation_unlinked")
	}
	h.respond(w, r, 200, map[string]bool{"deleted": true}, err)
}
func (h *Handler) Suggestions(w http.ResponseWriter, r *http.Request) {
	c, ok := id(w, r, "conversation")
	if !ok {
		return
	}
	out, err := h.Service.Suggestions(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, c)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) Comments(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	out, err := h.Service.Comments(r.Context(), orgctx.Must(r.Context()).ID, v)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) Comment(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Service.Comment(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v, in.Body)
	if err == nil {
		h.record(r, v, "issue_commented")
	}
	h.respond(w, r, 201, usecase.CommentDTO(out), err)
}
func (h *Handler) Impact(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	out, err := h.Service.Impact(r.Context(), orgctx.Must(r.Context()).ID, v)
	h.respond(w, r, 200, out, err)
}
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	out, err := h.Service.Preview(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) Deliveries(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	out, err := h.Service.Deliveries(r.Context(), orgctx.Must(r.Context()).ID, v)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) Projects(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.Projects(r.Context(), orgctx.Must(r.Context()).ID)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Service.CreateProject(r.Context(), orgctx.Must(r.Context()).ID, in.Name)
	if err == nil {
		h.record(r, uuid.Nil, "project_created")
	}
	h.respond(w, r, 201, out, err)
}
func (h *Handler) Releases(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.Releases(r.Context(), orgctx.Must(r.Context()).ID)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) CreateRelease(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version   string `json:"version"`
		Changelog string `json:"changelog_url"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Service.CreateRelease(r.Context(), orgctx.Must(r.Context()).ID, in.Version, in.Changelog)
	if err == nil {
		h.record(r, out.Uuid, "release_created")
	}
	h.respond(w, r, 201, usecase.ReleaseDTO(out), err)
}
func (h *Handler) PublishRelease(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "release")
	if !ok {
		return
	}
	out, err := h.Service.PublishRelease(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v)
	if err == nil {
		h.record(r, v, "release_published")
	}
	h.respond(w, r, 200, usecase.ReleaseDTO(out), err)
}
func (h *Handler) Escalations(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "conversation")
	if !ok {
		return
	}
	out, err := h.Service.Escalations(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) Escalate(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "conversation")
	if !ok {
		return
	}
	var in model.EscalationInput
	if !decode(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.Service.Escalate(r.Context(), orgctx.Must(r.Context()).ID, conv.Actor{ID: p.UserInternal, UUID: p.UserID}, v, in)
	if err == nil {
		h.record(r, v, "escalated")
	}
	h.respond(w, r, 201, usecase.EscalationDTO(out), err)
}
func (h *Handler) Integrations(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.Integrations(r.Context(), orgctx.Must(r.Context()).ID)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) SaveIntegration(w http.ResponseWriter, r *http.Request) {
	var in usecase.IntegrationInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Service.SaveIntegration(r.Context(), orgctx.Must(r.Context()).ID, r.PathValue("provider"), in)
	if err == nil {
		h.record(r, out.UUID, "integration_updated")
	}
	h.respond(w, r, 200, out, err)
}
func (h *Handler) StartOAuth(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.StartOAuth(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, r.PathValue("provider"))
	h.respond(w, r, 200, map[string]string{"authorize_url": out}, err)
}
func (h *Handler) CompleteOAuth(w http.ResponseWriter, r *http.Request) {
	var in struct {
		State string `json:"state"`
		Code  string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Service.CompleteOAuth(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, r.PathValue("provider"), in.State, in.Code)
	if err == nil {
		h.record(r, out.UUID, "oauth_connected")
	}
	h.respond(w, r, 200, out, err)
}
func (h *Handler) ExternalLinks(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	out, err := h.Service.ExternalLinks(r.Context(), orgctx.Must(r.Context()).ID, v)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) ConnectIssue(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "issue")
	if !ok {
		return
	}
	var in struct {
		Provider   string `json:"provider"`
		ExternalID string `json:"external_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Service.ConnectIssue(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v, in.Provider, in.ExternalID)
	if err == nil {
		h.record(r, v, "sync_requested")
	}
	h.respond(w, r, 202, usecase.ExternalDTO(out), err)
}
func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "integration")
	if !ok {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Payload too large")
		return
	}
	err = h.Service.Webhook(r.Context(), v, r.Header, raw)
	h.respond(w, r, 200, map[string]bool{"accepted": err == nil}, err)
}

func (h *Handler) Errors(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "conversation")
	if !ok {
		return
	}
	out, err := h.Service.Errors(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v)
	h.respond(w, r, 200, map[string]any{"items": out}, err)
}
func (h *Handler) AttachError(w http.ResponseWriter, r *http.Request) {
	v, ok := id(w, r, "conversation")
	if !ok {
		return
	}
	var in usecase.ErrorInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.Service.AttachError(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, v, in)
	if err == nil {
		h.record(r, v, "sentry_error_attached")
	}
	h.respond(w, r, 201, out, err)
}
