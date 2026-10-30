package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// Handler serves realtime token endpoints.
type Handler struct {
	issuer   *TokenIssuer
	authz    ChannelAuthorizer
	connInfo ConnInfoFunc
}

// ConnInfoFunc returns the public connection info of a user (name), shown
// to other clients through presence and join/leave.
type ConnInfoFunc func(ctx context.Context, userInternal int64) map[string]any

// WithConnInfo sets the connection info source.
func (h *Handler) WithConnInfo(fn ConnInfoFunc) *Handler {
	h.connInfo = fn
	return h
}

// NewHandler creates HTTP handlers for Centrifugo token issuance.
func NewHandler(issuer *TokenIssuer, authz ChannelAuthorizer) *Handler {
	return &Handler{issuer: issuer, authz: authz}
}

type connectionTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	WSURL     string `json:"ws_url"`
}

type subscriptionTokenRequest struct {
	Channel string `json:"channel"`
}

type subscriptionTokenResponse struct {
	Token     string `json:"token"`
	Channel   string `json:"channel"`
	ExpiresAt string `json:"expires_at"`
}

// ConnectionToken issues a Centrifugo connection JWT.
func (h *Handler) ConnectionToken(w http.ResponseWriter, r *http.Request) {
	if h == nil || !h.issuer.Enabled() {
		response.ServiceUnavailable(w, r, response.CodeRealtimeDisabled, "Realtime is disabled")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	var info map[string]any
	if h.connInfo != nil {
		info = h.connInfo(r.Context(), p.UserInternal)
	}
	token, exp, err := h.issuer.ConnectionTokenWithInfo(p.UserID.String(), info)
	if err != nil {
		response.InternalErr(w, r, err, "Could not issue connection token")
		return
	}
	response.JSON(w, r, http.StatusOK, connectionTokenResponse{
		Token:     token,
		ExpiresAt: exp.UTC().Format(timeRFC3339),
		WSURL:     h.issuer.WSURL(),
	})
}

// SubscriptionToken issues a channel subscription JWT after authorization.
func (h *Handler) SubscriptionToken(w http.ResponseWriter, r *http.Request) {
	if h == nil || !h.issuer.Enabled() {
		response.ServiceUnavailable(w, r, response.CodeRealtimeDisabled, "Realtime is disabled")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	var req subscriptionTokenRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.Channel) == "" {
		response.BadRequest(w, r, response.CodeValidationError, "channel is required")
		return
	}
	req.Channel = strings.TrimSpace(req.Channel)
	if h.authz == nil {
		response.Forbidden(w, r, "Not allowed to subscribe to this channel")
		return
	}
	ok, err := h.authz.CanSubscribeChannel(r.Context(), p.UserID, p.IsSuperAdmin, req.Channel)
	if err != nil {
		response.InternalErr(w, r, err, "Could not authorize channel subscription")
		return
	}
	if !ok {
		response.Forbidden(w, r, "Not allowed to subscribe to this channel")
		return
	}
	token, exp, err := h.issuer.SubscriptionToken(p.UserID.String(), req.Channel)
	if err != nil {
		response.InternalErr(w, r, err, "Could not issue subscription token")
		return
	}
	response.JSON(w, r, http.StatusOK, subscriptionTokenResponse{
		Token:     token,
		Channel:   req.Channel,
		ExpiresAt: exp.UTC().Format(timeRFC3339),
	})
}

// RegisterRoutes mounts realtime token endpoints.
func RegisterRoutes(
	mux *http.ServeMux,
	h *Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	// Any signed-in user may connect: organization staff (self-registered
	// owners, invitees) carry no platform role, so gating on auth.session
	// cut every workspace member off realtime. Each channel is still
	// authorized on subscribe (ChannelAuthorizer: own user channel, inbox of
	// an organization the user reads, system channel for super admins).
	authn := middleware.Authenticate(tokens, loader)
	mux.Handle("POST /v1/realtime/connection-token", middleware.Chain(
		http.HandlerFunc(h.ConnectionToken), authn,
	))
	mux.Handle("POST /v1/realtime/subscription-token", middleware.Chain(
		http.HandlerFunc(h.SubscriptionToken), authn,
	))
}
