package supportaccess

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Resource is the activity resource of grants.
const Resource = "tenant.support_access"

// Handler serves the support access routes.
type Handler struct {
	svc      *Service
	activity *activity.Recorder
}

// NewHandler builds the handler.
func NewHandler(svc *Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// RegisterRoutes mounts the tenant and platform routes. A nil resolver
// (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantSupportAccessRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantSupportAccessManage)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	route("GET /v1/tenant/support-access", h.List, read)
	route("POST /v1/tenant/support-access", h.Create, read, manage)
	route("POST /v1/tenant/support-access/{uuid}/revoke", h.Revoke, read, manage)
	mux.Handle("GET /v1/platform/support-access", middleware.Chain(
		http.HandlerFunc(h.Platform), authn, middleware.RequirePermission(rbac.PermPlatformSupportAccess),
	))
}

// List serves GET /v1/tenant/support-access.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, total, active, err := h.svc.List(r.Context(), orgctx.Must(r.Context()).ID, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	page := apiquery.NewPage(items, total, q.Limit, q.Offset)
	response.JSON(w, r, http.StatusOK, map[string]any{
		"items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "active": active,
	})
}

// Create serves POST /v1/tenant/support-access. Support staff themselves
// cannot extend their own access.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	if scope.Support || p.ImpersonatorUserID != nil {
		response.Forbidden(w, r, "support access is granted by the organization's own members")
		return
	}
	var in GrantInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	g, err := h.svc.Grant(r.Context(), scope.ID, p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "support_access.granted", g.UUID, map[string]any{"mode": g.Mode, "expires_at": g.ExpiresAt})
	response.JSON(w, r, http.StatusCreated, g)
}

// Revoke serves POST /v1/tenant/support-access/{uuid}/revoke.
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	g, err := h.svc.Revoke(r.Context(), scope.ID, p.UserInternal, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "support_access.revoked", g.UUID, map[string]any{"mode": g.Mode})
	response.JSON(w, r, http.StatusOK, g)
}

// Platform serves GET /v1/platform/support-access.
func (h *Handler) Platform(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Platform(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) record(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, Resource, &id, payload, r)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	if errors.Is(err, ErrNotFound) {
		response.NotFound(w, r, err.Error())
		return
	}
	response.InternalErr(w, r, err, "")
}
