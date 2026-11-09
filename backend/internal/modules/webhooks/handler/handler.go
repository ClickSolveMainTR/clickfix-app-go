package handler

import (
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/integrationhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type Handler struct {
	Service  *usecase.Service
	Activity *activity.Recorder
}

func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, 200, events.WebhookEvents())
}
func (h *Handler) CRUD(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	id := uuid.Nil
	var err error
	if raw := r.PathValue("uuid"); raw != "" {
		id, err = uuid.Parse(raw)
		if err != nil {
			h.fail(w, r, usecase.ErrInvalid)
			return
		}
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut:
		var in model.Input
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in) != nil {
			h.fail(w, r, usecase.ErrInvalid)
			return
		}
		out, err := h.Service.Save(r.Context(), org.ID, id, in)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		savedID := out["uuid"].(uuid.UUID)
		action := "updated"
		if id == uuid.Nil {
			action = "created"
		}
		h.Activity.Record(r.Context(), actorID(r), action, "outbound_webhooks", &savedID, map[string]any{"name": in.Name}, r)
		w.Header().Set("Cache-Control", "no-store")
		status := 200
		if id == uuid.Nil {
			status = 201
		}
		response.JSON(w, r, status, out)
	case http.MethodDelete:
		n, err := h.Service.Store.DeleteOutboundWebhook(r.Context(), db.DeleteOutboundWebhookParams{OrganizationID: org.ID, Uuid: id})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if n == 0 {
			h.fail(w, r, pgx.ErrNoRows)
			return
		}
		h.Activity.Record(r.Context(), actorID(r), "deleted", "outbound_webhooks", &id, map[string]any{}, r)
		response.JSON(w, r, 200, map[string]bool{"deleted": true})
	default:
		q, ok := integrationhttp.Query(w, r, false)
		if !ok {
			return
		}
		rows, err := h.Service.Store.ListOutboundWebhooks(r.Context(), db.ListOutboundWebhooksParams{OrganizationID: org.ID, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		n, err := h.Service.Store.CountOutboundWebhooks(r.Context(), org.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, apiquery.NewPage(rows, n, q.Limit, q.Offset))
	}
}
func (h *Handler) Deliveries(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	if r.Method == http.MethodPost {
		id, err := uuid.Parse(r.PathValue("uuid"))
		if err != nil {
			h.fail(w, r, usecase.ErrInvalid)
			return
		}
		n, err := h.Service.Store.ReplayOutboundDelivery(r.Context(), db.ReplayOutboundDeliveryParams{OrganizationID: org.ID, Uuid: id})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if n == 0 {
			response.Conflict(w, r, "DELIVERY_UNAVAILABLE", "Delivery is missing, pending or leased")
			return
		}
		response.JSON(w, r, 202, map[string]bool{"queued": true})
		return
	}
	q, ok := integrationhttp.Query(w, r, false)
	if !ok {
		return
	}
	rows, err := h.Service.Store.ListOutboundDeliveries(r.Context(), db.ListOutboundDeliveriesParams{OrganizationID: org.ID, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	n, err := h.Service.Store.CountOutboundDeliveries(r.Context(), org.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, 200, apiquery.NewPage(rows, n, q.Limit, q.Offset))
}
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalid), errors.Is(err, usecase.ErrWebhookTarget):
		response.BadRequest(w, r, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(w, r, "Webhook not found")
	default:
		response.InternalErr(w, r, err, "")
	}
}

func actorID(r *http.Request) *int64 { p := authctx.MustPrincipal(r.Context()); return &p.UserInternal }

// TestEvent is the event name of a test delivery.
const TestEvent = "webhook.test"

// Test queues a signed test event for one subscription (CF-187): it is
// delivered like any event (signature, retries, delivery log) and its
// event_id finds it in /v1/tenant/webhook-deliveries.
func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	org := orgctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		h.fail(w, r, usecase.ErrInvalid)
		return
	}
	eventID := uuid.New()
	payload := map[string]any{"id": eventID, "event": TestEvent, "test": true, "sent_at": time.Now().UTC()}
	if err := h.Service.Enqueue(r.Context(), org.ID, id, eventID, TestEvent, payload); err != nil {
		h.fail(w, r, err)
		return
	}
	h.Activity.Record(r.Context(), actorID(r), "webhooks.test_queued", "outbound_webhooks", &id, map[string]any{"event_id": eventID}, r)
	response.JSON(w, r, http.StatusAccepted, map[string]any{"queued": true, "event_id": eventID})
}
