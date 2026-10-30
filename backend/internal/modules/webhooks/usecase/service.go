// Package usecase delivers durable organization webhook subscriptions.
// Signing, retry schedule and connection-time network checks are ported from Rentopia publicapi.
package usecase

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	publicuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrInvalid = errors.New("invalid webhook input")

type Service struct {
	Store  *repository.Store
	Box    *crypto.SecretBox
	Client *http.Client
}

func New(q *db.Queries, box *crypto.SecretBox) *Service {
	return &Service{Store: repository.New(q), Box: box, Client: &http.Client{Timeout: 10 * time.Second, Transport: webhookTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (s *Service) Save(ctx context.Context, org int64, id uuid.UUID, in model.Input) (map[string]any, error) {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 120 || len(in.Events) == 0 {
		return nil, ErrInvalid
	}
	if err := ValidateURL(in.URL); err != nil {
		return nil, err
	}
	for _, e := range in.Events {
		if !slices.Contains(events.WebhookEvents(), e) {
			return nil, ErrInvalid
		}
	}
	var row db.OutboundWebhook
	var secret string
	var err error
	if id == uuid.Nil {
		secret, err = publicuc.RandomToken(32)
		if err != nil {
			return nil, err
		}
		var enc string
		enc, err = s.Box.Encrypt(secret)
		if err != nil {
			return nil, err
		}
		row, err = s.Store.CreateOutboundWebhook(ctx, db.CreateOutboundWebhookParams{OrganizationID: org, Name: in.Name, Url: in.URL, Events: in.Events, Enabled: in.Enabled, SecretEnc: enc})
	} else {
		row, err = s.Store.UpdateOutboundWebhook(ctx, db.UpdateOutboundWebhookParams{OrganizationID: org, Uuid: id, Name: in.Name, Url: in.URL, Events: in.Events, Enabled: in.Enabled})
	}
	if err != nil {
		return nil, err
	}
	out := map[string]any{"uuid": row.Uuid, "name": row.Name, "url": row.Url, "events": row.Events, "enabled": row.Enabled}
	if secret != "" {
		out["secret"] = secret
	}
	return out, nil
}

func (s *Service) Register(bus events.ReliableBus) {
	for _, name := range events.WebhookEvents() {
		bus.SubscribeReliable(name, s.Consume)
	}
	for old := range events.DomainAliases() {
		bus.SubscribeReliable(old, func(ctx context.Context, e events.Event) error {
			for _, expanded := range events.ExpandDomainAlias(e) {
				if err := s.Consume(ctx, expanded); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

func (s *Service) Consume(ctx context.Context, ev events.Event) error {
	if ev.TenantID == nil {
		return nil
	}
	raw, err := json.Marshal(map[string]any{"id": ev.EventID, "event": ev.Name, "version": ev.Version, "occurred_at": ev.OccurredAt, "data": ev.Payload})
	if err != nil {
		return err
	}
	if slices.Contains([]string{"conversation.created", "conversation.resolved", "contact.created", "csat.submitted", "conversation.tag_added"}, ev.Name) {
		// Polling gets public references only; the resource API rechecks visibility for details.
		b, err := json.Marshal(map[string]any{"id": ev.EventID, "event": ev.Name, "entity_uuid": ev.EntityUUID, "occurred_at": ev.OccurredAt})
		if err != nil {
			return err
		}
		if err = s.Store.IntegrationRecordTrigger(ctx, db.IntegrationRecordTriggerParams{OrganizationID: *ev.TenantID, EventID: ev.EventID, EventName: ev.Name, Payload: b}); err != nil {
			return err
		}
	}
	return s.Store.EnqueueOutboundWebhooks(ctx, db.EnqueueOutboundWebhooksParams{OrganizationID: *ev.TenantID, EventID: ev.EventID, EventName: ev.Name, Payload: raw})
}

func (s *Service) Enqueue(ctx context.Context, org int64, webhook, eventID uuid.UUID, event string, payload any) error {
	if _, err := s.Store.GetOutboundWebhook(ctx, db.GetOutboundWebhookParams{OrganizationID: org, Uuid: webhook}); err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.Store.EnqueueSingleOutboundWebhook(ctx, db.EnqueueSingleOutboundWebhookParams{OrganizationID: org, WebhookUuid: webhook, EventID: eventID, EventName: event, Payload: raw})
}

// SignWebhook signs timestamp.body, allowing receivers to reject stale deliveries.
func SignWebhook(secret string, stamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strconv.FormatInt(stamp, 10)))
	_, _ = mac.Write([]byte{'.'})
	_, _ = mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

var backoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour}

func (s *Service) Sweep(ctx context.Context) error {
	// Bounded work per scan; SKIP LOCKED and leases coordinate all replicas.
	for range 4 {
		rows, err := s.Store.ClaimOutboundDeliveries(ctx)
		if err != nil {
			return err
		}
		for _, d := range rows {
			if err := s.deliver(ctx, d); err != nil {
				return err
			}
		}
		if len(rows) < 50 {
			break
		}
	}
	return nil
}

func (s *Service) deliver(ctx context.Context, d db.OutboundWebhookDelivery) error {
	ep, err := s.Store.GetOutboundWebhookByID(ctx, db.GetOutboundWebhookByIDParams{OrganizationID: d.OrganizationID, ID: d.WebhookID})
	if err != nil {
		return err
	}
	status := "pending"
	code := 0
	msg := ""
	var delivered pgtype.Timestamptz
	if !ep.Enabled {
		msg = "endpoint disabled"
		status = "failed"
	} else if d.Attempts > 8 {
		msg = "attempt limit reached"
		status = "failed"
	} else {
		code, err = s.post(ctx, ep, d)
		if err == nil && code >= 200 && code < 300 {
			status = "succeeded"
			delivered = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		} else {
			msg = "Delivery failed"
			if code > 0 {
				msg = fmt.Sprintf("HTTP %d", code)
			}
			if d.Attempts >= 8 {
				status = "failed"
			}
		}
	}
	next := time.Now().Add(backoff[min(max(int(d.Attempts)-1, 0), len(backoff)-1)])
	return s.Store.FinishOutboundDelivery(ctx, db.FinishOutboundDeliveryParams{OrganizationID: d.OrganizationID, ID: d.ID, Status: status, LastStatus: safeconv.ToInt32(code), LastError: msg, NextAttemptAt: pgtype.Timestamptz{Time: next, Valid: true}, DeliveredAt: delivered, Attempts: d.Attempts})
}

func (s *Service) post(ctx context.Context, ep db.OutboundWebhook, d db.OutboundWebhookDelivery) (int, error) {
	if err := ValidateURL(ep.Url); err != nil {
		return 0, err
	}
	secret, err := s.Box.Decrypt(ep.SecretEnc)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.Url, bytes.NewReader(d.Payload))
	if err != nil {
		return 0, err
	}
	stamp := time.Now().Unix()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ClickFix-Webhooks/1")
	req.Header.Set("ClickFix-Webhook-Id", d.Uuid.String())
	req.Header.Set("ClickFix-Webhook-Event", d.EventName)
	req.Header.Set("ClickFix-Webhook-Timestamp", strconv.FormatInt(stamp, 10))
	req.Header.Set("ClickFix-Webhook-Signature", SignWebhook(secret, stamp, d.Payload))
	res, err := s.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	return res.StatusCode, nil
}
