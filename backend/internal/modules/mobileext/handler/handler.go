// Package handler serves /v1/mobile/* (CF-152): widget tokens, the widget
// authentication middleware, Live Activity tokens and settings.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// Limiter is ratelimit.Limiter (nil = no limits).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Rate limits per user.
const (
	widgetTokenLimit        = 30
	widgetTokenWindow       = time.Hour
	liveActivityTokenLimit  = 60
	liveActivityTokenWindow = time.Hour
	widgetAPILimit          = 60
	widgetAPIWindow         = time.Minute
)

// Handler serves the mobile surface routes.
type Handler struct {
	svc     *usecase.Service
	limiter Limiter
}

// New builds the handler; limiter may be nil.
func New(svc *usecase.Service, limiter Limiter) *Handler {
	return &Handler{svc: svc, limiter: limiter}
}

// IssueWidgetToken handles POST /v1/mobile/widget-token.
func (h *Handler) IssueWidgetToken(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("mobileext: missing organization scope"), "")
		return
	}
	// An impersonation session must not mint a long-lived credential.
	if p.ImpersonatorUserID != nil {
		response.Forbidden(w, r, "Widget tokens cannot be issued while impersonating")
		return
	}
	if h.rateLimited(w, r, "widget_token", p.UserInternal, widgetTokenLimit, widgetTokenWindow) {
		return
	}
	var body struct {
		DeviceID string `json:"device_id"`
	}
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.IssueWidgetToken(r.Context(), usecase.IssueInput{
		UserID: p.UserInternal, OrganizationID: scope.ID, SessionID: p.SessionID, DeviceID: body.DeviceID,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusCreated, out)
}

// RevokeWidgetToken handles DELETE /v1/mobile/widget-token?device_id=...
func (h *Handler) RevokeWidgetToken(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if err := h.svc.RevokeWidgetToken(r.Context(), p.UserInternal, r.URL.Query().Get("device_id")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RegisterLiveActivityToken handles POST /v1/mobile/live-activity-tokens.
func (h *Handler) RegisterLiveActivityToken(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if h.rateLimited(w, r, "live_activity_token", p.UserInternal, liveActivityTokenLimit, liveActivityTokenWindow) {
		return
	}
	var body usecase.LiveActivityTokenInput
	if !decode(w, r, &body) {
		return
	}
	if err := h.svc.RegisterLiveActivityToken(r.Context(), p.UserInternal, body); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteLiveActivityToken handles DELETE /v1/mobile/live-activity-tokens/{token}.
func (h *Handler) DeleteLiveActivityToken(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if err := h.svc.DeleteLiveActivityToken(r.Context(), p.UserInternal, r.PathValue("token")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetLiveActivitySettings handles GET /v1/mobile/live-activity-settings.
func (h *Handler) GetLiveActivitySettings(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.GetLiveActivitySettings(r.Context(), p.UserInternal)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PutLiveActivitySettings handles PUT /v1/mobile/live-activity-settings.
func (h *Handler) PutLiveActivitySettings(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		response.ValidationError(w, r, []response.Detail{{Field: "enabled", Message: "is required", Code: "required"}})
		return
	}
	out, err := h.svc.SetLiveActivitySettings(r.Context(), p.UserInternal, usecase.LiveActivitySettings{Enabled: *body.Enabled})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RateLimitWidget limits widget API calls per user (after AuthenticateWidget).
func (h *Handler) RateLimitWidget(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := authctx.PrincipalFrom(r.Context())
		if ok && h.rateLimited(w, r, "widget_api", p.UserInternal, widgetAPILimit, widgetAPIWindow) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) rateLimited(w http.ResponseWriter, r *http.Request, action string, userID int64, limit int, window time.Duration) bool {
	if h.limiter == nil || userID <= 0 {
		return false
	}
	allowed, retry := h.limiter.Allow(r.Context(), "mobile_"+action, strconv.FormatInt(userID, 10), limit, window)
	if allowed {
		return false
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "Too many requests. Try again in a moment.")
	return true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		fields := make([]string, 0, len(ve))
		for f := range ve {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		details := make([]response.Detail, 0, len(fields))
		for _, f := range fields {
			details = append(details, response.Detail{Field: f, Message: ve[f], Code: "invalid"})
		}
		response.ValidationError(w, r, details)
	case errors.Is(err, usecase.ErrSessionRequired):
		response.Forbidden(w, r, "A signed-in session is required")
	default:
		response.InternalErr(w, r, err, "")
	}
}
