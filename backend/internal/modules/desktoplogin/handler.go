package desktoplogin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	authusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/clientip"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

const (
	rateWindow          = 15 * time.Minute
	codeUserLimit       = 20
	exchangeIPLimit     = 30
	webSessionUserLimit = 30
	auditResource       = "auth.desktop_login"
)

// Audit actions (activity log). Payloads never carry codes, verifiers or tokens.
const (
	ActionCodeCreated      = "desktop_login.code_created"
	ActionExchanged        = "desktop_login.exchanged"
	ActionCodeReused       = "desktop_login.code_reused"
	ActionVerifierMismatch = "desktop_login.verifier_mismatch"
	ActionWebSession       = "desktop_login.web_session"
)

// Auditor writes activity rows (activity.Recorder).
type Auditor interface {
	Record(ctx context.Context, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, r *http.Request)
}

// Handler serves /v1/auth/desktop/*.
type Handler struct {
	svc     *Service
	limiter *ratelimit.Limiter
	audit   Auditor
}

// NewHandler builds the HTTP handler. limiter and audit may be nil.
func NewHandler(svc *Service, limiter *ratelimit.Limiter, audit Auditor) *Handler {
	return &Handler{svc: svc, limiter: limiter, audit: audit}
}

// CreateCode is POST /v1/auth/desktop/codes (Bearer; the signed-in browser).
func (h *Handler) CreateCode(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in CodeInput
	if !decode(w, r, &in) {
		return
	}
	if !h.allow(w, r, "desktop_code_user", p.UserID.String(), codeUserLimit) {
		return
	}
	out, err := h.svc.CreateCode(r.Context(), Requester{
		UserUUID: p.UserID, UserID: p.UserInternal, Impersonating: p.ImpersonatorUserID != nil,
	}, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, p.UserInternal, ActionCodeCreated, nil)
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusCreated, out)
}

// Exchange is POST /v1/auth/desktop/exchange (public; the desktop app).
func (h *Handler) Exchange(w http.ResponseWriter, r *http.Request) {
	var in ExchangeInput
	if !decode(w, r, &in) {
		return
	}
	ip := clientip.FromRequest(r)
	if !h.allow(w, r, "desktop_exchange_ip", ip, exchangeIPLimit) {
		return
	}
	out, redeemed, err := h.svc.Exchange(r.Context(), in, r.UserAgent(), ip)
	if err != nil {
		switch {
		case errors.Is(err, ErrCodeUsed):
			h.record(r, redeemed.UserID, ActionCodeReused, map[string]any{"client_os": in.ClientOS})
		case errors.Is(err, ErrVerifierMismatch):
			h.record(r, redeemed.UserID, ActionVerifierMismatch, map[string]any{"client_os": in.ClientOS})
		}
		writeError(w, r, err)
		return
	}
	h.record(r, redeemed.UserID, ActionExchanged, map[string]any{"client_os": in.ClientOS})
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// WebSession is POST /v1/auth/desktop/web-session (Bearer of a desktop app
// session; called by the web server's /desktop/session route only).
func (h *Handler) WebSession(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if !h.allow(w, r, "desktop_web_session_user", p.UserID.String(), webSessionUserLimit) {
		return
	}
	if p.ImpersonatorUserID != nil {
		writeError(w, r, authusecase.ErrNotDesktopSession)
		return
	}
	out, err := h.svc.WebSession(r.Context(), p.UserInternal, p.SessionID, r.UserAgent(), clientip.FromRequest(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, p.UserInternal, ActionWebSession, nil)
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) record(r *http.Request, userID int64, action string, payload map[string]any) {
	if h.audit == nil || userID == 0 {
		return
	}
	id := userID
	h.audit.Record(r.Context(), &id, action, auditResource, nil, payload, r)
}

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int) bool {
	if h.limiter == nil || strings.TrimSpace(subject) == "" {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), action, subject, limit, rateWindow)
	if ok {
		return true
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "")
	return false
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
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, "Invalid desktop sign-in request")
	case errors.Is(err, ErrInvalidCode), errors.Is(err, ErrCodeUsed), errors.Is(err, ErrVerifierMismatch):
		// One answer for all: the app starts over either way.
		response.BadRequest(w, r, response.CodeDesktopLoginInvalid, "Desktop sign-in code is invalid, expired or already used")
	case errors.Is(err, ErrImpersonating):
		response.Forbidden(w, r, "Stop impersonating before signing in to the desktop app")
	case errors.Is(err, authusecase.ErrNotDesktopSession):
		response.Forbidden(w, r, "This call needs a desktop app session")
	case errors.Is(err, authusecase.ErrAccountDeactivated):
		response.Error(w, r, http.StatusForbidden, response.CodeAccountDeactivated, "This account has been deleted")
	case errors.Is(err, authusecase.ErrUserDisabled):
		response.Forbidden(w, r, "User is disabled")
	case errors.Is(err, authusecase.ErrInvalidCredentials):
		response.Unauthorized(w, r, "Account is no longer available")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
