package entitlements

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
)

// RegisterMetering consumes transactional billing.usage_recorded source events.
// Producer emits feature, quantity, event_key, occurred_at in the outbox transaction
// of its successful operation. Preflight enforcement uses Consume/WithinLimit.
func (s *Service) RegisterMetering(bus interface{ SubscribeReliable(string, events.Handler) }) {
	bus.SubscribeReliable(events.BillingUsageRecorded, func(ctx context.Context, event events.Event) error {
		if event.TenantID == nil {
			return fmt.Errorf("metering: organization required")
		}
		raw, err := json.Marshal(event.Payload)
		if err != nil {
			return err
		}
		var in MeterEvent
		if err = json.Unmarshal(raw, &in); err != nil {
			return err
		}
		if in.EventKey == "" {
			in.EventKey = event.EventID.String()
		}
		if in.OccurredAt.IsZero() {
			in.OccurredAt = event.OccurredAt
		}
		_, err = s.Meter(ctx, *event.TenantID, in)
		if errors.Is(err, entitlement.ErrDenied) {
			return nil // Meter committed the deduplicated denial signal.
		}
		return err
	})
}

func (s *Service) recordDenial(ctx context.Context, org int64, in MeterEvent, cause error) error {
	var denied *entitlement.Error
	if !errors.As(cause, &denied) {
		return cause
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		n, err := r.Q.BillingUsageDenial(ctx, db.BillingUsageDenialParams{OrganizationID: org, EventKey: in.EventKey, Feature: in.Feature, Quantity: in.Quantity, Code: denied.Code})
		if err != nil || n == 0 {
			return err
		}
		return r.Emit(ctx, events.New(events.BillingUsageDenied).WithTenant(org).WithPayload(map[string]any{"event_key": in.EventKey, "feature": in.Feature, "quantity": in.Quantity, "code": denied.Code}))
	})
}
