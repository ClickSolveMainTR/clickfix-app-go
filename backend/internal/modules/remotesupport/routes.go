package remotesupport

import (
	"errors"
	"net/http"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/clientip"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Limiter is the shared Redis limiter in its fail-closed form
// (ratelimit.AllowStrict): public remote support routes refuse requests
// rather than run unlimited when Redis is down.
type Limiter func(r *http.Request, action, subject string, limit int, window time.Duration) bool

type routes struct {
	rt    supportops.Router
	s     *Service
	limit Limiter
}

// RegisterRoutes mounts the tenant, platform and public routes.
func RegisterRoutes(rt supportops.Router, s *Service, limit Limiter) {
	h := routes{rt: rt, s: s, limit: limit}
	h.tenant()
	h.platform()
	h.public()
}

type endpoint func(*http.Request) (any, error)

func (h routes) serve(fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		out, err := fn(r.WithContext(WithRequest(r.Context(), r)))
		if err != nil {
			fail(w, r, err)
			return
		}
		response.JSON(w, r, http.StatusOK, out)
	}
}

func (h routes) tenantRoute(pattern, perm string, fn endpoint) {
	if h.rt.Resolver == nil {
		return
	}
	h.rt.Mux.Handle(pattern, middleware.Chain(h.serve(fn), middleware.Authenticate(h.rt.Tokens, h.rt.Loader),
		middleware.RequireOrganization(h.rt.Resolver), middleware.RequireTenantPermission(perm)))
}

func org(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

func agent(r *http.Request) Agent {
	scope := orgctx.Must(r.Context())
	return Agent{ID: supportops.Actor(r.Context()), CanControl: scope.Has(rbac.PermTenantRemoteSupportControl),
		Manager: scope.Has(rbac.PermTenantRemoteSupportSettings)}
}

// visible: the session's agent, a remote support manager, or a member who
// can see the conversation under the inbox team rules.
func (h routes) visible(r *http.Request, row db.RemoteSupportSession) error {
	a := agent(r)
	if a.Manager || row.AgentUserID.Valid && row.AgentUserID.Int64 == a.ID {
		return nil
	}
	conv, err := h.s.Q.GetConversationByID(r.Context(), db.GetConversationByIDParams{OrganizationID: row.OrganizationID, ID: row.ConversationID})
	if err != nil {
		return err
	}
	if _, err = supportops.Conversation(r.Context(), h.s.Q, org(r), a.ID, conv.Uuid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (h routes) tenant() {
	use, settings := rbac.PermTenantRemoteSupportUse, rbac.PermTenantRemoteSupportSettings
	h.tenantRoute("POST /v1/tenant/conversations/{uuid}/remote-support", use, func(r *http.Request) (any, error) {
		in, err := supportops.ReadOptional[CreateInput](r)
		if err != nil {
			return nil, ErrInvalid
		}
		a := agent(r)
		conv, err := supportops.Conversation(r.Context(), h.s.Q, org(r), a.ID, supportops.ID(r))
		if err != nil {
			return nil, err
		}
		return h.s.Create(r.Context(), org(r), a, conv, in)
	})
	h.tenantRoute("GET /v1/tenant/conversations/{uuid}/remote-support", use, func(r *http.Request) (any, error) {
		conv, err := supportops.Conversation(r.Context(), h.s.Q, org(r), agent(r).ID, supportops.ID(r))
		if err != nil {
			return nil, err
		}
		rows, err := h.s.ForConversation(r.Context(), org(r), conv.ID)
		return map[string]any{"items": rows}, err
	})
	h.tenantRoute("GET /v1/tenant/remote-support/settings", settings, func(r *http.Request) (any, error) {
		return h.s.Settings(r.Context(), org(r))
	})
	h.tenantRoute("PUT /v1/tenant/remote-support/settings", settings, func(r *http.Request) (any, error) {
		in, err := supportops.Read[SettingsInput](r)
		if err != nil {
			return nil, ErrInvalid
		}
		out, err := h.s.SaveSettings(r.Context(), org(r), in)
		if err == nil && h.rt.Activity != nil {
			actor := supportops.Actor(r.Context())
			scope := orgctx.Must(r.Context())
			h.rt.Activity.Record(r.Context(), &actor, "remote_support.settings_updated", "remote_support_settings", &scope.UUID,
				map[string]any{"organization_id": scope.ID, "enabled": in.Enabled, "allow_control": in.AllowControl, "recording": out.Recording}, r)
		}
		return out, err
	})
	h.tenantRoute("GET /v1/tenant/remote-support/{uuid}", use, func(r *http.Request) (any, error) {
		row, err := h.s.Session(r.Context(), org(r), supportops.ID(r))
		if err != nil {
			return nil, err
		}
		if err = h.visible(r, row); err != nil {
			return nil, err
		}
		return h.s.View(r.Context(), row)
	})
	h.tenantRoute("POST /v1/tenant/remote-support/{uuid}/token", use, func(r *http.Request) (any, error) {
		return h.s.AgentToken(r.Context(), org(r), agent(r), supportops.ID(r))
	})
	h.tenantRoute("POST /v1/tenant/remote-support/{uuid}/end", use, func(r *http.Request) (any, error) {
		return h.s.EndByAgent(r.Context(), org(r), agent(r), supportops.ID(r))
	})
}

func (h routes) platformRoute(pattern string, fn endpoint) {
	h.rt.Mux.Handle(pattern, middleware.Chain(h.serve(fn), middleware.Authenticate(h.rt.Tokens, h.rt.Loader),
		middleware.RequirePermission(rbac.PermPlatformRemoteSupportManage)))
}

func (h routes) platform() {
	h.platformRoute("GET /v1/platform/remote-support/organizations", func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		rows, err := h.s.PlatformOrganizations(r.Context(), r.URL.Query().Get("state"), q.Limit, q.Offset)
		return map[string]any{"items": rows, "limit": q.Limit, "offset": q.Offset}, err
	})
	h.platformRoute("PUT /v1/platform/remote-support/organizations/{uuid}/state", func(r *http.Request) (any, error) {
		in, err := supportops.Read[PlatformStateInput](r)
		if err != nil {
			return nil, ErrInvalid
		}
		return h.s.SetPlatformState(r.Context(), supportops.Actor(r.Context()), supportops.ID(r), in)
	})
	h.platformRoute("GET /v1/platform/remote-support/reports", func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		rows, err := h.s.PlatformReports(r.Context(), q.Limit, q.Offset)
		return map[string]any{"items": rows, "limit": q.Limit, "offset": q.Offset}, err
	})
}

func (h routes) allow(r *http.Request, action, subject string, n int, window time.Duration) error {
	if h.limit == nil || !h.limit(r, action, subject, n, window) {
		return ErrRateLimited
	}
	return nil
}

// sessionLimits bound every customer call per address and per session.
func (h routes) sessionLimits(r *http.Request, action string) (uuid.UUID, error) {
	ip := clientip.FromRequest(r)
	if err := h.allow(r, "remote_support_"+action, ip, 60, time.Minute); err != nil {
		return uuid.Nil, err
	}
	id := supportops.ID(r)
	if id == uuid.Nil {
		return id, ErrUnauthorized
	}
	return id, h.allow(r, "remote_support_"+action+"_session", id.String(), 120, 10*time.Minute)
}

func (h routes) public() {
	h.rt.Mux.Handle("POST /v1/public/remote-support/resolve", h.serve(func(r *http.Request) (any, error) {
		ip := clientip.FromRequest(r)
		// Code guessing: per address per minute and per hour, then per code
		// prefix (the session a guess is charged to) and per join token.
		if err := h.allow(r, "remote_support_resolve", ip, 10, time.Minute); err != nil {
			return nil, err
		}
		if err := h.allow(r, "remote_support_resolve_hour", ip, 30, time.Hour); err != nil {
			return nil, err
		}
		in, err := supportops.Read[ResolveInput](r)
		if err != nil {
			return nil, ErrInvalid
		}
		if code, ok := NormalizeCode(in.Code); ok {
			if err := h.allow(r, "remote_support_resolve_code", h.s.Hasher.CodeLookup(code), 10, CodeTTL); err != nil {
				return nil, err
			}
		} else if in.Token != "" {
			if err := h.allow(r, "remote_support_resolve_token", h.s.Hasher.JoinToken(in.Token), 10, CodeTTL); err != nil {
				return nil, err
			}
		}
		return h.s.Resolve(r.Context(), in)
	}))
	h.rt.Mux.Handle("POST /v1/public/remote-support/{uuid}/consent", h.serve(func(r *http.Request) (any, error) {
		id, err := h.sessionLimits(r, "consent")
		if err != nil {
			return nil, err
		}
		in, err := supportops.Read[ConsentInput](r)
		if err != nil {
			return nil, ErrInvalid
		}
		return h.s.Consent(r.Context(), id, in)
	}))
	h.rt.Mux.Handle("POST /v1/public/remote-support/{uuid}/token", h.serve(func(r *http.Request) (any, error) {
		id, err := h.sessionLimits(r, "token")
		if err != nil {
			return nil, err
		}
		in, err := supportops.Read[ClientInput](r)
		if err != nil {
			return nil, ErrInvalid
		}
		return h.s.CustomerToken(r.Context(), id, in)
	}))
	h.rt.Mux.Handle("POST /v1/public/remote-support/{uuid}/end", h.serve(func(r *http.Request) (any, error) {
		id, err := h.sessionLimits(r, "end")
		if err != nil {
			return nil, err
		}
		in, err := supportops.Read[ClientInput](r)
		if err != nil {
			return nil, ErrInvalid
		}
		return h.s.EndByCustomer(r.Context(), id, in)
	}))
	h.rt.Mux.Handle("POST /v1/public/remote-support/{uuid}/report", h.serve(func(r *http.Request) (any, error) {
		id, err := h.sessionLimits(r, "report")
		if err != nil {
			return nil, err
		}
		in, err := supportops.Read[ReportInput](r)
		if err != nil {
			return nil, ErrInvalid
		}
		return h.s.Report(r.Context(), id, in)
	}))
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	var ent *entitlement.Error
	switch {
	case errors.As(err, &ent):
		if !entitlement.WriteError(w, r, err) {
			response.Forbidden(w, r, "Plan does not include remote support")
		}
	case errors.Is(err, ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		response.NotFound(w, r, "Record not found")
	case errors.Is(err, ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Message: "Invalid input"}})
	case errors.Is(err, ErrDenied):
		response.Forbidden(w, r, "Access denied")
	case errors.Is(err, ErrConflict):
		response.Conflict(w, r, "REMOTE_SUPPORT_CONFLICT", "The conversation already has an open remote support session or is closed")
	case errors.Is(err, ErrDisabled):
		response.Error(w, r, http.StatusForbidden, "REMOTE_SUPPORT_DISABLED", "Remote support is turned off for this organization")
	case errors.Is(err, ErrSuspended):
		response.Error(w, r, http.StatusForbidden, "REMOTE_SUPPORT_SUSPENDED", "Remote support is suspended for this organization")
	case errors.Is(err, ErrNotConfigured), errors.Is(err, livekit.ErrNotConfigured):
		response.Conflict(w, r, "REMOTE_SUPPORT_NOT_CONFIGURED", "Media server is not configured")
	case errors.Is(err, ErrNoMinutes):
		response.Error(w, r, http.StatusPaymentRequired, "REMOTE_SUPPORT_MINUTES_EXHAUSTED", "No remote support minutes left")
	case errors.Is(err, ErrCodeInvalid):
		response.Error(w, r, http.StatusNotFound, "REMOTE_SUPPORT_CODE_INVALID", "The code is invalid or has expired")
	case errors.Is(err, ErrUnauthorized):
		response.Unauthorized(w, r, "Invalid session credentials")
	case errors.Is(err, ErrConsent):
		response.Conflict(w, r, "REMOTE_SUPPORT_CONSENT_REQUIRED", "The customer has not approved viewing yet")
	case errors.Is(err, ErrEnded):
		response.Error(w, r, http.StatusGone, "REMOTE_SUPPORT_ENDED", "The session has ended")
	case errors.Is(err, ErrRateLimited):
		response.TooManyRequests(w, r, "Too many attempts")
	default:
		supportops.Fail(w, r, err)
	}
}
