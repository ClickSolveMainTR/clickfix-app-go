package entitlements

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
)

// MeterEvent is the event-spine contract. Event keys are stable at the source.
type MeterEvent struct {
	Feature    string    `json:"feature"`
	Quantity   int64     `json:"quantity"`
	EventKey   string    `json:"event_key"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (s *Service) Meter(ctx context.Context, org int64, in MeterEvent) (entitlement.Decision, error) {
	var out entitlement.Decision
	var err error
	if in.Feature == "seats" || in.Feature == "storage.bytes" {
		out, err = s.gauge(ctx, org, in)
	} else {
		out, err = s.ConsumeAt(ctx, org, in.Feature, in.Quantity, in.EventKey, in.OccurredAt)
	}
	if errors.Is(err, entitlement.ErrDenied) {
		if recordErr := s.recordDenial(ctx, org, in, err); recordErr != nil {
			return out, recordErr
		}
	}
	return out, err
}
func (s *Service) gauge(ctx context.Context, org int64, in MeterEvent) (entitlement.Decision, error) {
	var out entitlement.Decision
	if in.Quantity < 0 || strings.TrimSpace(in.EventKey) == "" || len(in.EventKey) > 200 || in.OccurredAt.IsZero() || in.OccurredAt.After(time.Now().Add(time.Minute)) {
		return out, model.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		snap, err := resolve(ctx, r, org)
		if err != nil {
			return err
		}
		v, ok := snap.Values[in.Feature]
		if !ok || v.Aggregation != "gauge" {
			return model.ErrInvalid
		}
		exists, err := r.Q.BillingUsageExists(ctx, db.BillingUsageExistsParams{OrganizationID: org, IdempotencyKey: in.EventKey})
		if err != nil {
			return err
		}
		if exists {
			old, e := r.Q.BillingGetUsageEvent(ctx, db.BillingGetUsageEventParams{OrganizationID: org, IdempotencyKey: in.EventKey})
			if e != nil {
				return e
			}
			if old.Key != in.Feature || old.Quantity != in.Quantity {
				return model.ErrConflict
			}
			out, _ = entitlement.Check(v, in.Quantity, 0)
			return nil
		}
		if !snap.Writable {
			return &entitlement.Error{Code: entitlement.CodeReadOnly, Feature: in.Feature}
		}
		out, err = entitlement.Check(v, 0, in.Quantity)
		if err != nil {
			return err
		}
		fs, err := r.Features(ctx)
		if err != nil {
			return err
		}
		at := in.OccurredAt.In(time.FixedZone("TRT", 10800))
		start := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, at.Location())
		period, err := r.Q.BillingEnsureUsagePeriod(ctx, db.BillingEnsureUsagePeriodParams{OrganizationID: org, StartsAt: repository.TS(start), EndsAt: repository.TS(start.AddDate(0, 1, 0))})
		if err != nil {
			return err
		}
		if period.ClosedAt.Valid {
			return model.ErrConflict
		}
		if err = r.Q.BillingRecordUsageAt(ctx, db.BillingRecordUsageAtParams{OrganizationID: org, UsagePeriodID: period.ID, FeatureID: fs[in.Feature].ID, IdempotencyKey: in.EventKey, Quantity: in.Quantity, OccurredAt: repository.TS(in.OccurredAt)}); err != nil {
			return err
		}
		before, err := r.Q.BillingUsageInPeriod(ctx, db.BillingUsageInPeriodParams{OrganizationID: org, UsagePeriodID: period.ID, FeatureID: fs[in.Feature].ID})
		if err != nil {
			return err
		}
		changed, err := r.Q.BillingSetGauge(ctx, db.BillingSetGaugeParams{OrganizationID: org, UsagePeriodID: period.ID, FeatureID: fs[in.Feature].ID, Quantity: in.Quantity, LastOccurredAt: repository.TS(in.OccurredAt)})
		if err != nil {
			return err
		}
		out.Used = in.Quantity
		if changed == 0 {
			out.Used = before
			return nil
		}
		previous, _ := entitlement.Check(v, before, 0)
		if out.WarningPercent > previous.WarningPercent {
			return r.Emit(ctx, events.New(events.EntitlementsQuotaWarning).WithTenant(org).WithPayload(map[string]any{"organization_id": org, "feature": in.Feature, "percent": out.WarningPercent, "used": in.Quantity, "limit": v.Limit}))
		}
		return nil
	})
	return out, err
}

// Usage lists the stored usage per period, newest first.
func (s *Service) Usage(ctx context.Context, org int64) ([]db.BillingUsageSummaryRow, error) {
	return s.repo.Q.BillingUsageSummary(ctx, org)
}

// UsageView is Usage for display: gauges no event has metered in the
// current period (seats, channels, contacts, storage) are filled with live
// counts, and the current period comes first even before it has a stored
// row. A metered gauge keeps its stored value.
func (s *Service) UsageView(ctx context.Context, org int64) ([]db.BillingUsageSummaryRow, error) {
	rows, err := s.repo.Q.BillingUsageSummary(ctx, org)
	if err != nil {
		return nil, err
	}
	live, err := s.repo.Q.BillingLiveGauges(ctx, org)
	if err != nil {
		return nil, err
	}
	return withLiveGauges(rows, live, time.Now()), nil
}

// withLiveGauges adds the live gauge counts missing from the current period
// (the open one containing now, or a new month period in Turkish time) and
// returns that period's rows first.
func withLiveGauges(rows []db.BillingUsageSummaryRow, live []db.BillingLiveGaugesRow, now time.Time) []db.BillingUsageSummaryRow {
	at := now.In(time.FixedZone("TRT", 10800))
	start := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, at.Location())
	cur := db.BillingUsageSummaryRow{StartsAt: repository.TS(start), EndsAt: repository.TS(start.AddDate(0, 1, 0))}
	for _, r := range rows {
		if !r.ClosedAt.Valid && !r.StartsAt.Time.After(now) && r.EndsAt.Time.After(now) {
			cur = db.BillingUsageSummaryRow{PeriodUuid: r.PeriodUuid, StartsAt: r.StartsAt, EndsAt: r.EndsAt}
			break
		}
	}
	gauges := make(map[string]int64, len(live))
	for _, g := range live {
		gauges[g.Key] = g.Quantity
	}
	current := make([]db.BillingUsageSummaryRow, 0, len(live))
	rest := make([]db.BillingUsageSummaryRow, 0, len(rows))
	for _, r := range rows {
		if cur.PeriodUuid == uuid.Nil || r.PeriodUuid != cur.PeriodUuid {
			rest = append(rest, r)
			continue
		}
		delete(gauges, r.Key)
		current = append(current, r)
	}
	for _, g := range live {
		if q, ok := gauges[g.Key]; ok {
			row := cur
			row.Key, row.Aggregation, row.Quantity = g.Key, "gauge", q
			current = append(current, row)
		}
	}
	return append(current, rest...)
}
func (s *Service) Daily(ctx context.Context, org int64, start, end time.Time) ([]db.BillingUsageDailyRow, error) {
	if !end.After(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, model.ErrInvalid
	}
	return s.repo.Q.BillingUsageDaily(ctx, db.BillingUsageDailyParams{OrganizationID: org, StartsAt: repository.TS(start), EndsAt: repository.TS(end)})
}

// ClosePeriod seals the period and emits invoice-ready immutable usage quantities.
// Invoice pricing/tax calculation belongs to the invoice engine consuming this event.
func (s *Service) ClosePeriod(ctx context.Context, org int64, id uuid.UUID, now time.Time) error {
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		p, err := r.Q.BillingUsagePeriod(ctx, db.BillingUsagePeriodParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if p.ClosedAt.Valid {
			return nil
		}
		if now.Before(p.EndsAt.Time) {
			return model.ErrConflict
		}
		rows, err := r.Q.BillingSettleUsage(ctx, db.BillingSettleUsageParams{OrganizationID: org, UsagePeriodID: p.ID})
		if err != nil {
			return err
		}
		if err = r.Q.BillingCloseUsagePeriod(ctx, db.BillingCloseUsagePeriodParams{OrganizationID: org, ID: p.ID}); err != nil {
			return err
		}
		return r.Emit(ctx, events.New("billing.usage_closed").WithTenant(org).WithEntity("billing_usage_periods", &p.ID, &p.Uuid).WithPayload(map[string]any{"organization_id": org, "period_uuid": p.Uuid, "starts_at": p.StartsAt.Time, "ends_at": p.EndsAt.Time, "lines": rows}))
	})
}

// Sweep closes completed periods before the invoice engine consumes their lines.
func (s *Service) Sweep(ctx context.Context, now time.Time) error {
	rows, err := s.repo.Q.BillingDueUsagePeriods(ctx, repository.TS(now))
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err = s.ClosePeriod(ctx, row.OrganizationID, row.Uuid, now); err != nil {
			return err
		}
	}
	return nil
}
