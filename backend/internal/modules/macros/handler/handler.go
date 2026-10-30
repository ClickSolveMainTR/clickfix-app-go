// Package handler exposes canned responses and macros
// (/v1/tenant/canned-responses, /v1/tenant/macros).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/macros/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes.
const (
	CodeShortcutExists = "CANNED_SHORTCUT_EXISTS"
	maxBodyBytes       = 64 << 10
)

// Handler serves canned responses and macros.
type Handler struct {
	svc *usecase.Service
}

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func viewer(w http.ResponseWriter, r *http.Request) (usecase.Viewer, orgctx.Scope, bool) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("macros: missing organization scope"), "")
		return usecase.Viewer{}, scope, false
	}
	p := authctx.MustPrincipal(r.Context())
	return usecase.Viewer{OrganizationID: scope.ID, UserID: p.UserInternal, UserUUID: p.UserID, CanWrite: scope.Has(rbac.PermTenantMacrosWrite)}, scope, true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return uuid.Nil, false
	}
	return id, true
}

// ListCanned lists usable canned responses (?q filters by title, shortcut, text).
func (h *Handler) ListCanned(w http.ResponseWriter, r *http.Request) {
	v, _, ok := viewer(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListCanned(r.Context(), v, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type cannedRequest struct {
	Scope    string `json:"scope"`
	Title    string `json:"title"`
	Shortcut string `json:"shortcut"`
	Category string `json:"category"`
	Content  string `json:"content"`
}

func (c cannedRequest) input() usecase.CannedInput {
	return usecase.CannedInput{Scope: c.Scope, Title: c.Title, Shortcut: c.Shortcut, Category: c.Category, Content: c.Content}
}

// CreateCanned creates a canned response.
func (h *Handler) CreateCanned(w http.ResponseWriter, r *http.Request) {
	v, _, ok := viewer(w, r)
	if !ok {
		return
	}
	var req cannedRequest
	if !decode(w, r, &req) {
		return
	}
	c, err := h.svc.CreateCanned(r.Context(), v, req.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, c)
}

// UpdateCanned edits a canned response.
func (h *Handler) UpdateCanned(w http.ResponseWriter, r *http.Request) {
	v, _, ok := viewer(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	var req cannedRequest
	if !decode(w, r, &req) {
		return
	}
	c, err := h.svc.UpdateCanned(r.Context(), v, id, req.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

// DeleteCanned deletes a canned response.
func (h *Handler) DeleteCanned(w http.ResponseWriter, r *http.Request) {
	v, _, ok := viewer(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteCanned(r.Context(), v, id); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// RenderCanned renders a canned response for a conversation.
func (h *Handler) RenderCanned(w http.ResponseWriter, r *http.Request) {
	v, _, ok := viewer(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	var req struct {
		Conversation *uuid.UUID `json:"conversation"`
	}
	if !decode(w, r, &req) {
		return
	}
	conv := uuid.Nil
	if req.Conversation != nil {
		conv = *req.Conversation
	}
	out, err := h.svc.Render(r.Context(), v, id, conv)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListMacros lists runnable macros.
func (h *Handler) ListMacros(w http.ResponseWriter, r *http.Request) {
	v, _, ok := viewer(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListMacros(r.Context(), v)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type macroRequest struct {
	Scope       string           `json:"scope"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Actions     []usecase.Action `json:"actions"`
}

func (m macroRequest) input() usecase.MacroInput {
	return usecase.MacroInput{Scope: m.Scope, Name: m.Name, Description: m.Description, Actions: m.Actions}
}

// CreateMacro creates a macro.
func (h *Handler) CreateMacro(w http.ResponseWriter, r *http.Request) {
	v, _, ok := viewer(w, r)
	if !ok {
		return
	}
	var req macroRequest
	if !decode(w, r, &req) {
		return
	}
	m, err := h.svc.CreateMacro(r.Context(), v, req.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, m)
}

// UpdateMacro edits a macro.
func (h *Handler) UpdateMacro(w http.ResponseWriter, r *http.Request) {
	v, _, ok := viewer(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	var req macroRequest
	if !decode(w, r, &req) {
		return
	}
	m, err := h.svc.UpdateMacro(r.Context(), v, id, req.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, m)
}

// DeleteMacro deletes a macro.
func (h *Handler) DeleteMacro(w http.ResponseWriter, r *http.Request) {
	v, _, ok := viewer(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteMacro(r.Context(), v, id); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// ApplyMacro runs a macro on a conversation.
func (h *Handler) ApplyMacro(w http.ResponseWriter, r *http.Request) {
	v, scope, ok := viewer(w, r)
	if !ok {
		return
	}
	conv, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	macro, ok := pathID(w, r, "macro")
	if !ok {
		return
	}
	perms := usecase.Permissions{
		Reply:  scope.Has(rbac.PermTenantConversationsReply),
		Note:   scope.Has(rbac.PermTenantConversationsNote),
		Manage: scope.Has(rbac.PermTenantConversationsManage),
		Assign: scope.Has(rbac.PermTenantConversationsAssign),
	}
	res, err := h.svc.Apply(r.Context(), v, perms, macro, conv)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, strings.TrimPrefix(err.Error(), usecase.ErrForbidden.Error()+": "))
	case errors.Is(err, usecase.ErrShortcutExists):
		response.Conflict(w, r, CodeShortcutExists, "This shortcut is already used")
	case errors.Is(err, usecase.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Message: strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": ")}})
	default:
		response.InternalErr(w, r, err, "")
	}
}
