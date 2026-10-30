// Package repository implements billing persistence using sqlc and explicit transactions.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is transaction-bound when passed to a use case's Within callback.
type Repository struct {
	Pool   *pgxpool.Pool
	Q      *db.Queries
	tx     pgx.Tx
	outbox outbox.Enqueuer
}

func New(pool *pgxpool.Pool, q *db.Queries) *Repository {
	return &Repository{Pool: pool, Q: q, outbox: outbox.NewStore(pool, q)}
}
func (r *Repository) Within(ctx context.Context, fn func(*Repository) error) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	child := &Repository{Pool: r.Pool, Q: r.Q.WithTx(tx), tx: tx, outbox: r.outbox}
	if err = fn(child); err != nil {
		return Normalize(err)
	}
	return Normalize(tx.Commit(ctx))
}
func (r *Repository) Emit(ctx context.Context, ev events.Event) error {
	return r.outbox.Enqueue(ctx, r.tx, ev)
}
func Normalize(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505", "23514", "23503":
			return model.ErrConflict
		}
	}
	return err
}
func TS(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()} }
func TimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
func Int(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
func Bool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}
func (r *Repository) Features(ctx context.Context) (map[string]db.BillingFeature, error) {
	rows, err := r.Q.BillingListFeatures(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]db.BillingFeature{}
	for _, f := range rows {
		out[f.Key] = f
	}
	return out, nil
}
func FeatureKinds(fs map[string]db.BillingFeature) map[string]string {
	out := map[string]string{}
	for k, v := range fs {
		out[k] = v.Kind
	}
	return out
}
func (r *Repository) Version(ctx context.Context, v db.BillingPlanVersion) (model.Version, error) {
	out := model.Version{UUID: v.Uuid, Number: v.Version, Status: v.Status, PublishedAt: TimePtr(v.PublishedAt), VersionInput: model.VersionInput{TrialDays: v.TrialDays, Prices: []model.Price{}, Entitlements: []model.Grant{}}}
	ps, err := r.Q.BillingListPrices(ctx, v.ID)
	if err != nil {
		return out, err
	}
	for _, p := range ps {
		price := model.Price{UUID: p.Uuid, Interval: p.Interval, Currency: p.Currency, Amount: p.AmountMinor, SeatAmount: p.SeatAmountMinor, TaxInclusive: p.TaxInclusive, VAT: p.VatBasisPoints, Tiers: []model.Tier{}}
		tiers, err := r.Q.BillingListTiers(ctx, p.ID)
		if err != nil {
			return out, err
		}
		for _, t := range tiers {
			var upper *int64
			if t.UpperBound.Valid {
				upper = &t.UpperBound.Int64
			}
			price.Tiers = append(price.Tiers, model.Tier{Feature: t.FeatureKey, Lower: t.LowerBound, Upper: upper, UnitAmount: t.UnitAmountMinor})
		}
		out.Prices = append(out.Prices, price)
	}
	es, err := r.Q.BillingListPlanEntitlements(ctx, v.ID)
	if err != nil {
		return out, err
	}
	for _, e := range es {
		g := model.Grant{Feature: e.FeatureKey, Unlimited: e.Unlimited, Enforcement: e.Enforcement, WarnPercent: e.WarnPercent}
		if e.ValueBool.Valid {
			g.Enabled = &e.ValueBool.Bool
		}
		if e.ValueLimit.Valid {
			g.Limit = &e.ValueLimit.Int64
		}
		out.Entitlements = append(out.Entitlements, g)
	}
	return out, nil
}
func (r *Repository) Plan(ctx context.Context, p db.BillingPlan, public bool) (model.Plan, error) {
	out := model.Plan{UUID: p.Uuid, PlanInput: model.PlanInput{Code: p.Code, Name: p.Name, Description: p.Description, Visibility: p.Visibility, SortOrder: p.SortOrder}, ArchivedAt: TimePtr(p.ArchivedAt), Versions: []model.Version{}}
	vs, err := r.Q.BillingListVersions(ctx, p.ID)
	if err != nil {
		return out, err
	}
	for _, v := range vs {
		if public && v.Status != "published" {
			continue
		}
		detail, err := r.Version(ctx, v)
		if err != nil {
			return out, err
		}
		out.Versions = append(out.Versions, detail)
		if public {
			break
		}
	}
	return out, nil
}
func (r *Repository) ReplaceVersion(ctx context.Context, id int64, in model.VersionInput, fs map[string]db.BillingFeature) error {
	if err := r.Q.BillingClearTiers(ctx, id); err != nil {
		return err
	}
	if err := r.Q.BillingClearPrices(ctx, id); err != nil {
		return err
	}
	if err := r.Q.BillingClearEntitlements(ctx, id); err != nil {
		return err
	}
	for _, p := range in.Prices {
		row, err := r.Q.BillingCreatePrice(ctx, db.BillingCreatePriceParams{PlanVersionID: id, Interval: p.Interval, Currency: p.Currency, AmountMinor: p.Amount, SeatAmountMinor: p.SeatAmount, TaxInclusive: p.TaxInclusive, VatBasisPoints: p.VAT})
		if err != nil {
			return err
		}
		for _, t := range p.Tiers {
			if err := r.Q.BillingCreateTier(ctx, db.BillingCreateTierParams{PriceID: row.ID, FeatureID: fs[t.Feature].ID, LowerBound: t.Lower, UpperBound: Int(t.Upper), UnitAmountMinor: t.UnitAmount}); err != nil {
				return err
			}
		}
	}
	for _, g := range in.Entitlements {
		if err := r.Q.BillingCreateEntitlement(ctx, db.BillingCreateEntitlementParams{PlanVersionID: id, FeatureID: fs[g.Feature].ID, Kind: fs[g.Feature].Kind, ValueBool: Bool(g.Enabled), ValueLimit: Int(g.Limit), Unlimited: g.Unlimited, Enforcement: g.Enforcement, WarnPercent: g.WarnPercent}); err != nil {
			return err
		}
	}
	return nil
}

// WithTx binds billing writes and outbox to the signup transaction.
func (r *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{Pool: r.Pool, Q: r.Q.WithTx(tx), tx: tx, outbox: r.outbox}
}
