package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	aiapierror "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/apierror"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// profileAccess maps the member's permissions to what the contact page may
// show (CF-221). Conversation rows follow the inbox team visibility rule.
func (h *Handler) profileAccess(r *http.Request, scope orgctx.Scope) (usecase.ProfileAccess, error) {
	user := authctx.MustPrincipal(r.Context()).UserInternal
	return h.svc.ProfileAccessFor(r.Context(), scope, user)
}

// GetContactOverview returns the contact page: the contact, its company,
// health stats and every section the member may see.
func (h *Handler) GetContactOverview(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	a, err := h.profileAccess(r, scope)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.svc.Profile(r.Context(), scope.ID, id, a)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// GetContactTimeline returns one page of the contact's unified timeline:
// ?kinds=note,csat&q=fatura&from=<RFC 3339>&to=<RFC 3339>&before=<cursor>&limit=30.
func (h *Handler) GetContactTimeline(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	q := usecase.ContactTimelineQuery{Limit: atoi(v.Get("limit")), Q: v.Get("q"), Before: strings.TrimSpace(v.Get("before"))}
	for _, k := range strings.Split(v.Get("kinds"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			q.Kinds = append(q.Kinds, k)
		}
	}
	for key, dst := range map[string]**time.Time{"from": &q.From, "to": &q.To} {
		raw := strings.TrimSpace(v.Get(key))
		if raw == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			invalid(w, r, key+" must be an RFC 3339 time")
			return
		}
		*dst = &t
	}
	a, err := h.profileAccess(r, scope)
	if err != nil {
		writeError(w, r, err)
		return
	}
	page, err := h.svc.ContactTimeline(r.Context(), scope.ID, id, a, q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

// CreateContactNote adds an agent note to a contact.
func (h *Handler) CreateContactNote(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &req) {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	note, err := h.svc.AddContactNote(r.Context(), scope.ID, id, actor, req.Body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "contacts.note_added", "contacts", id, map[string]any{"note": note.UUID})
	response.JSON(w, r, http.StatusCreated, note)
}

// DeleteContactNote removes a contact note: its author, or anyone holding
// the contacts delete permission.
func (h *Handler) DeleteContactNote(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	err := h.svc.DeleteContactNote(r.Context(), scope.ID, id, actor, scope.Has(rbac.PermTenantContactsDelete))
	if errors.Is(err, usecase.ErrForbidden) {
		response.Forbidden(w, r, "Only the author can delete this note")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "contacts.note_deleted", "contact_notes", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// GenerateContactSummary asks the AI engine for a fresh summary of the
// contact (tenant.ai.use) and caches it: ?locale=tr|en.
func (h *Handler) GenerateContactSummary(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	a, err := h.profileAccess(r, scope)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.svc.GenerateSummary(r.Context(), scope.ID, id, a, r.URL.Query().Get("locale"))
	if errors.Is(err, usecase.ErrAIUnavailable) {
		response.Conflict(w, r, aiapierror.CodeNotConfigured, "AI is not configured on this platform")
		return
	}
	if aiapierror.Write(w, r, err) {
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
