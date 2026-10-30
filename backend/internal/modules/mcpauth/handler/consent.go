package handler

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// ConsentService is the consent / connected apps usecase.
type ConsentService interface {
	Authorize(ctx context.Context, ip string, in model.AuthorizeInput) (string, error)
	Consent(ctx context.Context, a usecase.Actor, id uuid.UUID) (model.Consent, error)
	Decide(ctx context.Context, a usecase.Actor, id uuid.UUID, in model.DecideInput) (model.Decision, error)
	ListGrants(ctx context.Context, a usecase.Actor, f usecase.ListFilter) ([]model.Grant, int64, error)
	RevokeGrant(ctx context.Context, a usecase.Actor, id uuid.UUID) error
	ListClients(ctx context.Context, f usecase.ListFilter) ([]model.ClientSummary, int64, error)
	RevokeClient(ctx context.Context, a usecase.Actor, id uuid.UUID) error
}

// ActorFunc builds the usecase actor of a session request (staff or
// customer panel); false answers 401.
type ActorFunc func(r *http.Request) (usecase.Actor, bool)

// Consent serves the authorize endpoint and the session endpoints.
type Consent struct {
	svc ConsentService
	h   *Handler
}

// NewConsent builds the consent handler.
func NewConsent(svc ConsentService, h *Handler) *Consent {
	return &Consent{svc: svc, h: h}
}

// Authorize serves GET /oauth/authorize: 302 to the consent screen of the
// layer (or to the client with an error once client and redirect_uri are
// verified); an unknown client, an unregistered redirect_uri or the rate
// limit renders an error page and never redirects.
func (c *Consent) Authorize(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	to, err := c.svc.Authorize(r.Context(), ClientIP(r), model.AuthorizeInput{
		ClientID: v.Get("client_id"), RedirectURI: v.Get("redirect_uri"), ResponseType: v.Get("response_type"),
		CodeChallenge: v.Get("code_challenge"), CodeChallengeMethod: v.Get("code_challenge_method"),
		State: v.Get("state"), Scope: v.Get("scope"), Resource: v.Get("resource"),
	})
	if err != nil {
		c.errorPage(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, to, http.StatusFound)
}

func (c *Consent) errorPage(w http.ResponseWriter, r *http.Request, err error) {
	var oe *model.Error
	if !errors.As(err, &oe) {
		oe = &model.Error{Status: http.StatusInternalServerError, Code: model.ErrServerError, Description: "internal server error", Cause: err}
	}
	if oe.Status >= http.StatusInternalServerError {
		c.h.log.ErrorContext(r.Context(), "mcp_oauth_authorize_failed", "error", oe.Cause)
	}
	if oe.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(oe.RetryAfter.Seconds())+1))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(oe.Status)
	_, _ = w.Write([]byte("<!doctype html><html><head><meta charset=\"utf-8\"><title>Yetkilendirme hatası / Authorization error</title></head><body>" +
		"<h1>Yetkilendirme hatası / Authorization error</h1><p><code>" + html.EscapeString(oe.Code) + "</code></p><p>" +
		html.EscapeString(oe.Description) + "</p></body></html>"))
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return uuid.Nil, false
	}
	return id, true
}

func (c *Consent) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *apiquery.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	var fe *usecase.ForbiddenError
	switch {
	case errors.As(err, &fe):
		response.Forbidden(w, r, fe.Message)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	default:
		response.InternalErr(w, r, err, "MCP authorization request failed")
	}
}

// ConsentInfo serves GET {prefix}/consent/{uuid}.
func (c *Consent) ConsentInfo(actor ActorFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := actor(r)
		if !ok {
			response.Unauthorized(w, r, "Authentication is required")
			return
		}
		id, ok := pathUUID(w, r)
		if !ok {
			return
		}
		out, err := c.svc.Consent(r.Context(), a, id)
		if err != nil {
			c.writeError(w, r, err)
			return
		}
		response.JSON(w, r, http.StatusOK, out)
	}
}

// Decide serves POST {prefix}/consent/{uuid}/decide.
func (c *Consent) Decide(actor ActorFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := actor(r)
		if !ok {
			response.Unauthorized(w, r, "Authentication is required")
			return
		}
		id, ok := pathUUID(w, r)
		if !ok {
			return
		}
		var in model.DecideInput
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
			return
		}
		out, err := c.svc.Decide(r.Context(), a, id, in)
		if err != nil {
			c.writeError(w, r, err)
			return
		}
		response.JSON(w, r, http.StatusOK, out)
	}
}

// ListGrants serves GET {prefix}/grants.
func (c *Consent) ListGrants(actor ActorFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := actor(r)
		if !ok {
			response.Unauthorized(w, r, "Authentication is required")
			return
		}
		f, err := usecase.ParseGrantListFilter(r.URL.Query())
		if err != nil {
			c.writeError(w, r, err)
			return
		}
		items, total, err := c.svc.ListGrants(r.Context(), a, f)
		if err != nil {
			c.writeError(w, r, err)
			return
		}
		response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
	}
}

// RevokeGrant serves DELETE {prefix}/grants/{uuid}.
func (c *Consent) RevokeGrant(actor ActorFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := actor(r)
		if !ok {
			response.Unauthorized(w, r, "Authentication is required")
			return
		}
		id, ok := pathUUID(w, r)
		if !ok {
			return
		}
		if err := c.svc.RevokeGrant(r.Context(), a, id); err != nil {
			c.writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ListClients serves GET /v1/platform/mcp/clients.
func (c *Consent) ListClients(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseClientListFilter(r.URL.Query())
	if err != nil {
		c.writeError(w, r, err)
		return
	}
	items, total, err := c.svc.ListClients(r.Context(), f)
	if err != nil {
		c.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// RevokeClient serves DELETE /v1/platform/mcp/clients/{uuid}.
func (c *Consent) RevokeClient(actor ActorFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, _ := actor(r)
		id, ok := pathUUID(w, r)
		if !ok {
			return
		}
		if err := c.svc.RevokeClient(r.Context(), a, id); err != nil {
			c.writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
