package autoreply

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

const maxBodyBytes = 64 << 10

// Handler serves GET/PUT /v1/tenant/auto-replies.
type Handler struct {
	svc      *Service
	activity *activity.Recorder
}

// NewHandler builds the handler.
func NewHandler(svc *Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// RegisterRoutes mounts the endpoints (tenant settings permissions). A nil
// resolver (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantSettingsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantSettingsWrite)
	mux.Handle("GET /v1/tenant/auto-replies", middleware.Chain(http.HandlerFunc(h.Get), authn, org, read))
	mux.Handle("PUT /v1/tenant/auto-replies", middleware.Chain(http.HandlerFunc(h.Put), authn, org, write))
}

// Get returns the settings.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("autoreply: missing organization scope"), "")
		return
	}
	out, err := h.svc.Get(r.Context(), scope.ID)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Put replaces the settings.
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("autoreply: missing organization scope"), "")
		return
	}
	var in Settings
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return
	}
	out, err := h.svc.Put(r.Context(), scope.ID, in)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			response.ValidationError(w, r, []response.Detail{{Message: strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")}})
			return
		}
		response.InternalErr(w, r, err, "")
		return
	}
	if h.activity != nil {
		p := authctx.MustPrincipal(r.Context())
		h.activity.Record(r.Context(), &p.UserInternal, "auto_replies.updated", "auto_replies", nil, map[string]any{
			"organization_uuid": scope.UUID, "ack": out.Ack.Enabled, "offline": out.Offline.Enabled, "handoff": out.Handoff.Enabled,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, out)
}
