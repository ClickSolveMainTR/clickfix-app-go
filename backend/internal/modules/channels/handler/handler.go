// Package handler exposes channel instances (/v1/tenant/channels) and the
// shared channel webhook endpoint (/v1/webhooks/channels/{type}/{instance}).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the channel endpoints.
const (
	CodeUnknownChannelType = "UNKNOWN_CHANNEL_TYPE"
	CodeChannelLinked      = "CHANNEL_ALREADY_CONNECTED"
	CodeChannelRejected    = "CHANNEL_CREDENTIALS_REJECTED"
	CodeWebhookSignature   = "WEBHOOK_SIGNATURE_INVALID"
)

const (
	maxBodyBytes    = 64 << 10
	maxWebhookBytes = 5 << 20
)

// Resource is the activity resource of channel instances.
const Resource = "channels"

// Handler serves the channel endpoints.
type Handler struct {
	svc      *usecase.Service
	receiver func(ctx context.Context, inst channels.Instance, res channels.WebhookResult) error
	activity *activity.Recorder
	log      *slog.Logger
	// providers and oauthKey serve the connect wizards (providers.go).
	providers Providers
	oauthKey  []byte
}

// New builds the handler. receive runs the inbound pipeline for a verified
// webhook delivery.
func New(svc *usecase.Service, receive func(ctx context.Context, inst channels.Instance, res channels.WebhookResult) error, rec *activity.Recorder, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, receiver: receive, activity: rec, log: log}
}

// Types lists the channel types that can be connected.
func (h *Handler) Types(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"items": h.svc.Types()})
}

// List returns the organization's channel instances.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	items, err := h.svc.List(r.Context(), scope.ID, strings.TrimSpace(r.URL.Query().Get("type")))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Get returns one instance.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type createRequest struct {
	BrandID     *uuid.UUID           `json:"brand_id"`
	Type        string               `json:"type"`
	Name        string               `json:"name"`
	Credentials channels.Credentials `json:"credentials"`
	Settings    map[string]any       `json:"settings"`
}

// Create connects a channel instance.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var req createRequest
	if !decode(w, r, &req) {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	v, err := h.svc.Create(r.Context(), usecase.CreateInput{
		BrandID: req.BrandID, OrganizationID: scope.ID, ActorID: actor, Type: strings.TrimSpace(req.Type), Name: req.Name,
		Credentials: req.Credentials, Settings: req.Settings,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "channels.connected", v)
	response.JSON(w, r, http.StatusCreated, v)
}

type updateRequest struct {
	BrandID     json.RawMessage      `json:"brand_id"`
	Name        *string              `json:"name"`
	Status      *string              `json:"status"`
	Credentials channels.Credentials `json:"credentials"`
	Settings    map[string]any       `json:"settings"`
}

// Update edits an instance.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	var req updateRequest
	if !decode(w, r, &req) {
		return
	}
	var brandID *uuid.UUID
	if len(req.BrandID) > 0 && string(req.BrandID) != "null" {
		var v uuid.UUID
		if json.Unmarshal(req.BrandID, &v) != nil {
			response.BadRequest(w, r, response.CodeValidationError, "Invalid brand_id")
			return
		}
		brandID = &v
	}
	v, err := h.svc.Update(r.Context(), scope.ID, id, usecase.UpdateInput{
		BrandID: brandID, BrandSet: len(req.BrandID) > 0,
		Name: req.Name, Status: req.Status, Credentials: req.Credentials, Settings: req.Settings,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "channels.updated", v)
	response.JSON(w, r, http.StatusOK, v)
}

// Delete disconnects an instance.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Delete(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "channels.disconnected", v)
	response.JSON(w, r, http.StatusOK, v)
}

// Check asks the provider about an instance's connection now.
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.CheckHealth(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// TestDelivery sends a test event to the channel's own endpoint.
func (h *Handler) TestDelivery(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.TestDelivery(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		actor := authctx.MustPrincipal(r.Context()).UserInternal
		h.activity.Record(r.Context(), &actor, "channels.test_delivery", Resource, &id,
			map[string]any{"organization_uuid": scope.UUID, "ok": out.OK, "status_code": out.StatusCode}, r)
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Reconnect runs the provider connection again with the stored credentials.
func (h *Handler) Reconnect(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Reconnect(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "channels.reconnected", v)
	response.JSON(w, r, http.StatusOK, v)
}

// Challenge answers a provider's GET subscription challenge for an instance.
func (h *Handler) Challenge(w http.ResponseWriter, r *http.Request) {
	inst, adapter, ok := h.webhookInstance(w, r)
	if !ok {
		return
	}
	c, isChallenger := adapter.(channels.Challenger)
	if !isChallenger {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	answer, accepted := c.Challenge(r, inst)
	if !accepted {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, answer)
}

// SharedChallenge answers the GET subscription challenge of a type-wide
// webhook URL (one Meta app for every organization).
func (h *Handler) SharedChallenge(w http.ResponseWriter, r *http.Request) {
	adapter, found := h.svc.Registry().Get(r.PathValue("type"))
	c, ok := adapter.(channels.SharedChallenger)
	if !found || !ok {
		response.NotFound(w, r, "Unknown channel webhook")
		return
	}
	answer, accepted := c.ChallengeShared(r)
	if !accepted {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, answer)
}

// Webhook receives a provider delivery for one instance. Answers: 200 when
// stored (or nothing to store), 401 for a bad signature, 400 for a body the
// adapter cannot read, 500 when storing failed (the provider retries;
// stored messages are skipped by their ids).
func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	inst, adapter, ok := h.webhookInstance(w, r)
	if !ok {
		return
	}
	if inst.Type == channels.TypeVoice && inst.Credentials["provider"] == "netgsm" {
		if adapter.VerifyWebhook(r, nil, inst) != nil {
			response.Unauthorized(w, r, "Webhook signature invalid")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
		if err != nil {
			response.JSON(w, r, 200, map[string]bool{"received": true})
			return
		}
		h.deliver(w, r, inst, adapter, body)
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	h.deliver(w, r, inst, adapter, body)
}

// SharedWebhook receives a provider delivery on the type-wide URL of
// channels implementing channels.Router (one app webhook for every account).
func (h *Handler) SharedWebhook(w http.ResponseWriter, r *http.Request) {
	channelType := r.PathValue("type")
	adapter, found := h.svc.Registry().Get(channelType)
	router, isRouter := adapter.(channels.Router)
	if !found || !isRouter {
		response.NotFound(w, r, "Unknown channel webhook")
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	if responder, ok := adapter.(channels.SubscriptionResponder); ok {
		if h.subscriptionResponse(w, r, responder, body, channels.Instance{}) {
			return
		}
	}
	ids, err := router.Route(r, body)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Unreadable webhook")
		return
	}
	for _, ext := range ids {
		inst, a, err := h.svc.ForExternal(r.Context(), channelType, ext)
		if errors.Is(err, usecase.ErrNotFound) {
			h.log.Info("channel_webhook_unrouted", "type", channelType, "external_id", ext)
			continue
		}
		if err != nil {
			response.InternalErr(w, r, err, "")
			return
		}
		if !h.process(w, r, inst, a, body) {
			return
		}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"received": true})
}

func (h *Handler) deliver(w http.ResponseWriter, r *http.Request, inst channels.Instance, adapter channels.Adapter, body []byte) {
	if responder, ok := adapter.(channels.SubscriptionResponder); ok {
		if h.subscriptionResponse(w, r, responder, body, inst) {
			return
		}
	}
	if h.process(w, r, inst, adapter, body) {
		if ack, ok := adapter.(channels.WebhookAcknowledger); ok {
			contentType, answer := ack.AcknowledgeWebhook(r, body, inst)
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, answer)
			return
		}
		response.JSON(w, r, http.StatusOK, map[string]any{"received": true})
	}
}

// process verifies, parses and stores one body for one instance; it writes
// the error response and returns false on failure.
func (h *Handler) process(w http.ResponseWriter, r *http.Request, inst channels.Instance, adapter channels.Adapter, body []byte) bool {
	if err := adapter.VerifyWebhook(r, body, inst); err != nil {
		h.log.Warn("channel_webhook_rejected", "type", inst.Type, "instance", inst.UUID, "error", err)
		response.Unauthorized(w, r, "Webhook signature invalid")
		return false
	}
	h.svc.TouchWebhook(r.Context(), inst.OrganizationID, inst.ID)
	if inst.Status == usecase.StatusPaused || inst.Status == usecase.StatusDisconnected {
		// Acknowledge so the provider does not retry; a paused inbox drops traffic.
		return true
	}
	res, err := adapter.ParseWebhook(r.Context(), r, body, inst)
	if err != nil {
		h.log.Warn("channel_webhook_unreadable", "type", inst.Type, "instance", inst.UUID, "error", err)
		if inst.Type == channels.TypeVoice && inst.Credentials["provider"] == "netgsm" {
			return true
		}
		response.BadRequest(w, r, response.CodeValidationError, "Unreadable webhook")
		return false
	}
	if err := h.receiver(r.Context(), inst, res); err != nil {
		if inst.Type == channels.TypeVoice && inst.Credentials["provider"] == "netgsm" {
			h.log.Error("netgsm_webhook_apply_failed", "instance", inst.UUID)
			return true
		}
		response.InternalErr(w, r, err, "")
		return false
	}
	return true
}

func (h *Handler) webhookInstance(w http.ResponseWriter, r *http.Request) (channels.Instance, channels.Adapter, bool) {
	id, err := uuid.Parse(r.PathValue("instance"))
	if err != nil {
		response.NotFound(w, r, "Unknown channel webhook")
		return channels.Instance{}, nil, false
	}
	inst, adapter, err := h.svc.ForWebhook(r.Context(), r.PathValue("type"), id)
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrUnknownType) {
			response.NotFound(w, r, "Unknown channel webhook")
		} else {
			response.InternalErr(w, r, err, "")
		}
		return channels.Instance{}, nil, false
	}
	return inst, adapter, true
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		response.Error(w, r, http.StatusRequestEntityTooLarge, response.CodeValidationError, "Webhook body too large")
		return nil, false
	}
	return body, true
}

func (h *Handler) record(r *http.Request, scope orgctx.Scope, action string, v usecase.InstanceView) {
	if h.activity == nil {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	h.activity.Record(r.Context(), &actor, action, Resource, &v.UUID, map[string]any{
		"organization_uuid": scope.UUID, "type": v.Type, "name": v.Name, "status": v.Status,
	}, r)
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
	if entitlement.WriteError(w, r, err) {
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Channel not found")
	case errors.Is(err, usecase.ErrUnknownType):
		response.BadRequest(w, r, CodeUnknownChannelType, "Unknown channel type")
	case errors.Is(err, usecase.ErrAlreadyLinked):
		response.Conflict(w, r, CodeChannelLinked, "This account is already connected")
	case errors.Is(err, usecase.ErrConnectFailed):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeChannelRejected, "The provider rejected the credentials",
			[]response.Detail{{Message: err.Error()}})
	case errors.Is(err, usecase.ErrInvalid):
		response.ValidationError(w, r, []response.Detail{{Message: strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": ")}})
	default:
		response.InternalErr(w, r, err, "")
	}
}

func scopeOf(w http.ResponseWriter, r *http.Request) (orgctx.Scope, bool) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("channels: missing organization scope"), "")
		return orgctx.Scope{}, false
	}
	return scope, true
}

func scopeAndID(w http.ResponseWriter, r *http.Request) (orgctx.Scope, uuid.UUID, bool) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return orgctx.Scope{}, uuid.Nil, false
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Channel not found")
		return orgctx.Scope{}, uuid.Nil, false
	}
	return scope, id, true
}

func (h *Handler) subscriptionResponse(w http.ResponseWriter, r *http.Request, responder channels.SubscriptionResponder, body []byte, inst channels.Instance) bool {
	contentType, answer, handled, err := responder.RespondWebhook(r, body, inst)
	if !handled {
		return false
	}
	if err != nil {
		response.Unauthorized(w, r, "Webhook subscription verification failed")
		return true
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, answer)
	return true
}
