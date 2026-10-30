package usecase

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Normalization and serialized validation are ported from Takwim coupons.
var couponCodePattern = regexp.MustCompile(`^[A-Z0-9_-]{3,32}$`)

type CouponInput struct {
	Code               string     `json:"code"`
	PercentBasisPoints *int32     `json:"percent_basis_points"`
	AmountMinor        *int64     `json:"amount_minor"`
	Currency           string     `json:"currency"`
	StartsAt           time.Time  `json:"starts_at"`
	EndsAt             *time.Time `json:"ends_at"`
	MaxRedemptions     *int64     `json:"max_redemptions"`
	PerOrgLimit        int64      `json:"per_org_limit"`
	DurationMonths     *int32     `json:"duration_months"`
	Recurring          bool       `json:"recurring"`
	PlanUUID           *uuid.UUID `json:"plan_uuid"`
	Interval           string     `json:"interval"`
}

func (s *Service) CouponCatalog(ctx context.Context, limit, offset int32) ([]db.BillingCoupon, error) {
	return s.repo.Q.BillingCouponCatalog(ctx, db.BillingCouponCatalogParams{Limit: limit, Offset: offset})
}
func (s *Service) CreateCoupon(ctx context.Context, in CouponInput) (db.BillingCoupon, error) {
	if in.DurationMonths != nil {
		in.Recurring = true
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if in.StartsAt.IsZero() {
		in.StartsAt = time.Now().UTC()
	}
	if in.PerOrgLimit == 0 {
		in.PerOrgLimit = 1
	}
	var zero db.BillingCoupon
	if !couponCodePattern.MatchString(in.Code) || in.PerOrgLimit < 1 || (in.PercentBasisPoints == nil) == (in.AmountMinor == nil) || in.DurationMonths != nil && (*in.DurationMonths < 1 || *in.DurationMonths > 120) || in.EndsAt != nil && !in.EndsAt.After(in.StartsAt) || in.MaxRedemptions != nil && *in.MaxRedemptions < 1 {
		return zero, model.ErrInvalid
	}
	if in.PercentBasisPoints != nil && (*in.PercentBasisPoints < 1 || *in.PercentBasisPoints > 10000 || in.Currency != "") {
		return zero, model.ErrInvalid
	}
	if in.AmountMinor != nil && (*in.AmountMinor < 1 || *in.AmountMinor > money.MaxMinor || (in.Currency != "TRY")) {
		return zero, model.ErrInvalid
	}
	if in.Interval != "" && in.Interval != "monthly" && in.Interval != "yearly" {
		return zero, model.ErrInvalid
	}
	var plan pgtype.Int8
	if in.PlanUUID != nil {
		p, e := s.repo.Q.BillingGetPlan(ctx, *in.PlanUUID)
		if e != nil {
			return zero, e
		}
		plan = pgtype.Int8{Int64: p.ID, Valid: true}
	}
	percent := pgtype.Int4{}
	if in.PercentBasisPoints != nil {
		percent = pgtype.Int4{Int32: *in.PercentBasisPoints, Valid: true}
	}
	duration := pgtype.Int4{}
	if in.DurationMonths != nil {
		duration = pgtype.Int4{Int32: *in.DurationMonths, Valid: true}
	}
	return s.repo.Q.BillingNewCoupon(ctx, db.BillingNewCouponParams{Code: in.Code, PercentBasisPoints: percent, AmountMinor: repository.Int(in.AmountMinor), Currency: text(in.Currency), StartsAt: repository.TS(in.StartsAt), EndsAt: optionalTS(in.EndsAt), MaxRedemptions: repository.Int(in.MaxRedemptions), PerOrgLimit: in.PerOrgLimit, DurationMonths: duration, Recurring: in.Recurring, PlanID: plan, Interval: text(in.Interval)})
}
func (s *Service) couponDiscount(ctx context.Context, r *repository.Repository, org int64, c db.BillingCoupon, p db.BillingPrice, subtotal int64) (int64, error) {
	now := time.Now().UTC()
	if !c.Active || c.StartsAt.Time.After(now) || c.EndsAt.Valid && !c.EndsAt.Time.After(now) || c.Interval.Valid && c.Interval.String != p.Interval {
		return 0, model.ErrInvalid
	}
	if c.PlanID.Valid {
		v, e := r.Q.BillingGetVersionID(ctx, p.PlanVersionID)
		if e != nil {
			return 0, e
		}
		if v.PlanID != c.PlanID.Int64 {
			return 0, model.ErrInvalid
		}
	}
	uses, err := r.Q.BillingCouponUses(ctx, db.BillingCouponUsesParams{CouponID: c.ID, OrganizationID: org})
	if err != nil {
		return 0, err
	}
	if c.MaxRedemptions.Valid && uses.Total >= c.MaxRedemptions.Int64 {
		return 0, model.ErrConflict
	}
	if !c.Recurring && uses.OrgTotal >= c.PerOrgLimit {
		return 0, nil
	}
	if c.PercentBasisPoints.Valid {
		return money.New(subtotal).Ratio(int64(c.PercentBasisPoints.Int32), 10000).Minor(), nil
	}
	// Invoice amounts are settled in TRY. Fixed foreign currency discounts are
	// converted at the invoice's frozen rate by the builder, never mixed here.
	if c.Currency.String != "TRY" {
		return 0, model.ErrInvalid
	}
	return min(subtotal, c.AmountMinor.Int64), nil
}
func (s *Service) AttachCoupon(ctx context.Context, org int64, code string) error {
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		sub, e := r.Q.BillingGetSubscription(ctx, org)
		if e != nil {
			return e
		}
		c, e := r.Q.BillingLockCoupon(ctx, strings.ToUpper(strings.TrimSpace(code)))
		if e != nil {
			return e
		}
		p, e := r.Q.BillingGetPriceID(ctx, sub.PriceID)
		if e != nil {
			return e
		}
		uses, ue := r.Q.BillingCouponUses(ctx, db.BillingCouponUsesParams{CouponID: c.ID, OrganizationID: org})
		if ue != nil {
			return ue
		}
		if uses.OrgTotal >= c.PerOrgLimit {
			return model.ErrConflict
		}
		if _, e = s.couponDiscount(ctx, r, org, c, p, p.AmountMinor); e != nil {
			return e
		}
		var expiry *time.Time
		if c.DurationMonths.Valid {
			at := time.Now().AddDate(0, int(c.DurationMonths.Int32), 0)
			expiry = &at
		}
		if e = r.Q.BillingAttachCoupon(ctx, db.BillingAttachCouponParams{OrganizationID: org, SubscriptionID: sub.ID, CouponID: c.ID, ExpiresAt: optionalTS(expiry)}); e != nil {
			return e
		}
		return r.Emit(ctx, events.New("billing.coupon_attached").WithTenant(org).WithPayload(map[string]any{"coupon_uuid": c.Uuid, "subscription_uuid": sub.Uuid}))
	})
}
