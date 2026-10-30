package handler

import (
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/integrationhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	Service  *usecase.Service
	Activity *activity.Recorder
}

func (h *Handler) Keys(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	switch r.Method {
	case http.MethodPost:
		var in model.KeyInput
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in) != nil {
			response.BadRequest(w, r, "VALIDATION_ERROR", "Invalid JSON")
			return
		}
		out, err := h.Service.Create(r.Context(), org.ID, p.UserInternal, in)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		keyID := out["uuid"].(uuid.UUID)
		h.Activity.Record(r.Context(), &p.UserInternal, "created", "public_api_keys", &keyID, map[string]any{"name": in.Name}, r)
		w.Header().Set("Cache-Control", "no-store")
		response.JSON(w, r, 201, out)
	case http.MethodDelete:
		id, err := uuid.Parse(r.PathValue("uuid"))
		if err != nil {
			h.fail(w, r, usecase.ErrInvalid)
			return
		}
		n, err := h.Service.Store.RevokeIntegrationKey(r.Context(), db.RevokeIntegrationKeyParams{OrganizationID: org.ID, Uuid: id})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if n == 0 {
			response.NotFound(w, r, "Key not found")
			return
		}
		h.Activity.Record(r.Context(), &p.UserInternal, "revoked", "public_api_keys", &id, nil, r)
		response.JSON(w, r, 200, map[string]bool{"revoked": true})
	default:
		q, ok := integrationhttp.Query(w, r, false)
		if !ok {
			return
		}
		rows, err := h.Service.Store.ListIntegrationKeys(r.Context(), db.ListIntegrationKeysParams{OrganizationID: org.ID, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		n, err := h.Service.Store.CountIntegrationKeys(r.Context(), org.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, apiquery.NewPage(rows, n, q.Limit, q.Offset))
	}
}
func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		response.Unauthorized(w, r, "API key required")
		return
	}
	k, err := h.Service.Authenticate(r.Context(), raw)
	if err != nil {
		response.Unauthorized(w, r, "Invalid API key")
		return
	}
	if h.Service.Limiter != nil {
		allowed, retry := h.Service.Limiter.Allow(r.Context(), "public_api_key", k.Uuid.String(), int(k.RateLimit), time.Minute)
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			response.Error(w, r, 429, "RATE_LIMITED", "API key rate limit exceeded")
			return
		}
	}
	target, scope, ok := usecase.Route(r.Method, r.PathValue("path"))
	if !ok {
		response.NotFound(w, r, "Unknown public API route")
		return
	}
	if !slices.Contains(k.Scopes, scope) {
		response.Forbidden(w, r, "Missing key scope: "+scope)
		return
	}
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<10))
	if err != nil {
		h.fail(w, r, usecase.ErrInvalid)
		return
	}
	out, err := h.Service.Execute(r.Context(), k, r.Method, target, r.Header.Get("Idempotency-Key"), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if out.Truncated {
		response.Error(w, r, 502, "RESULT_TOO_LARGE", "Narrow the request")
		return
	}
	_ = h.Service.Store.TouchIntegrationKey(r.Context(), db.TouchIntegrationKeyParams{OrganizationID: k.OrganizationID, ID: k.ID})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(out.Status)
	_, _ = w.Write(out.Body)
}
func (h *Handler) Contacts(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	q, ok := integrationhttp.Query(w, r, true)
	if !ok {
		return
	}
	rows, err := h.Service.Store.ListIntegrationContacts(r.Context(), db.ListIntegrationContactsParams{OrganizationID: org.ID, Q: q.Q, RowLimit: q.Limit, RowOffset: q.Offset})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	n, err := h.Service.Store.CountIntegrationContacts(r.Context(), db.CountIntegrationContactsParams{OrganizationID: org.ID, Q: q.Q})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, apiquery.NewPage(rows, n, q.Limit, q.Offset))
}
func (h *Handler) Companies(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	q, ok := integrationhttp.Query(w, r, false)
	if !ok {
		return
	}
	rows, err := h.Service.Store.ListIntegrationCompanies(r.Context(), db.ListIntegrationCompaniesParams{OrganizationID: org.ID, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	n, err := h.Service.Store.CountIntegrationCompanies(r.Context(), org.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, apiquery.NewPage(rows, n, q.Limit, q.Offset))
}
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, "VALIDATION_ERROR", "Invalid input; writes require Idempotency-Key")
	case errors.Is(err, usecase.ErrConflict):
		response.Conflict(w, r, "IDEMPOTENCY_CONFLICT", err.Error())
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(w, r, "Not found")
	default:
		response.InternalErr(w, r, err, "")
	}
}
