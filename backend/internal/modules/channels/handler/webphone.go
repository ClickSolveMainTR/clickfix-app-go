package handler

import (
	"encoding/json"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) WebphoneConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	if scope.Support {
		response.Forbidden(w, r, "SIP credentials require membership")
		return
	}
	c, e := h.svc.Webphone(r.Context(), scope.ID, authctx.MustPrincipal(r.Context()).UserID)
	if e != nil {
		writeError(w, r, e)
		return
	}
	response.JSON(w, r, 200, c)
}
func (h *Handler) MemberSIP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	admin := scope.MemberRole == orgctx.RoleOwner || scope.MemberRole == orgctx.RoleAdmin
	for _, role := range scope.MemberRoles {
		if role == orgctx.RoleOwner || role == orgctx.RoleAdmin {
			admin = true
		}
	}
	if !admin {
		response.Forbidden(w, r, "Admin role required")
		return
	}
	user, e := uuid.Parse(r.PathValue("user"))
	if e != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid member")
		return
	}
	if r.Method == http.MethodPut {
		var in usecase.SIPSettings
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
			response.BadRequest(w, r, response.CodeValidationError, "Invalid SIP settings")
			return
		}
		if e = h.svc.SetMemberSIP(r.Context(), scope.ID, user, in); e != nil {
			writeError(w, r, e)
			return
		}
		if h.activity != nil {
			actor := authctx.MustPrincipal(r.Context()).UserInternal
			h.activity.Record(r.Context(), &actor, "webphone.sip_updated", "organization_members", &user, map[string]any{"organization_uuid": scope.UUID}, r)
		}
	}
	out, e := h.svc.MemberSIP(r.Context(), scope.ID, user)
	if e != nil {
		writeError(w, r, e)
		return
	}
	response.JSON(w, r, 200, out)
}
func (h *Handler) WebphoneEvents(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var in usecase.WebphoneEvent
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid call event")
		return
	}
	if err := h.svc.WebphoneEvent(r.Context(), scope.ID, authctx.MustPrincipal(r.Context()).UserID, in); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, 200, map[string]bool{"received": true})
}
