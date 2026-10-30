package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AddonInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Feature     string `json:"feature"`
	Units       int64  `json:"units"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

func (s *Service) AddonCatalog(ctx context.Context, limit, offset int32) ([]db.BillingAddonCatalogRow, error) {
	return s.repo.Q.BillingAddonCatalog(ctx, db.BillingAddonCatalogParams{Limit: limit, Offset: offset})
}
func (s *Service) CreateAddon(ctx context.Context, in AddonInput) (db.BillingAddon, error) {
	if !couponCodePattern.MatchString(in.Code) || in.Name == "" || in.Units < 1 || in.Units > 1000000000 || in.AmountMinor < 0 || in.AmountMinor > money.MaxMinor || (in.Currency != "TRY" && in.Currency != "USD" && in.Currency != "EUR") {
		return db.BillingAddon{}, model.ErrInvalid
	}
	return s.repo.Q.BillingSaveAddon(ctx, db.BillingSaveAddonParams{Code: in.Code, Name: in.Name, Key: in.Feature, Units: in.Units, AmountMinor: in.AmountMinor, Currency: in.Currency})
}

type AddonChangeInput struct {
	AddonUUID      uuid.UUID `json:"addon_uuid"`
	Quantity       int64     `json:"quantity"`
	IdempotencyKey string    `json:"idempotency_key"`
}

func (s *Service) ChangeAddon(ctx context.Context, org int64, in AddonChangeInput) (*db.BillingInvoice, error) {
	var out *db.BillingInvoice
	if in.Quantity < 0 || in.Quantity > 10000 || len(in.IdempotencyKey) == 0 || len(in.IdempotencyKey) > 120 {
		return out, model.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		sub, e := r.Q.BillingGetSubscription(ctx, org)
		if e != nil {
			return e
		}
		if sub.Status != "active" || !sub.PeriodEnd.Time.After(time.Now()) {
			return model.ErrConflict
		}
		addon, e := r.Q.BillingAddon(ctx, in.AddonUUID)
		if e != nil {
			return e
		}
		items, e := r.Q.BillingSubscriptionItems(ctx, db.BillingSubscriptionItemsParams{OrganizationID: org, SubscriptionID: sub.ID})
		if e != nil {
			return e
		}
		old := int64(0)
		for _, item := range items {
			if item.AddonID == addon.ID {
				old = item.Quantity
			}
		}
		key := "addon:" + in.IdempotencyKey
		existing, e := r.Q.BillingInvoiceKey(ctx, db.BillingInvoiceKeyParams{OrganizationID: org, IdempotencyKey: key})
		if e == nil {
			item, e := r.Q.BillingItemChangeInvoice(ctx, db.BillingItemChangeInvoiceParams{OrganizationID: org, InvoiceID: existing.ID})
			if e != nil {
				return e
			}
			if item.AddonID != addon.ID || item.Quantity != in.Quantity {
				return model.ErrConflict
			}
			out = &existing
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if in.Quantity == old {
			return model.ErrInvalid
		}
		if in.Quantity < old {
			return r.Q.BillingScheduleItem(ctx, db.BillingScheduleItemParams{OrganizationID: org, SubscriptionID: sub.ID, AddonID: addon.ID, PendingQuantity: pgtype.Int8{Int64: in.Quantity, Valid: true}})
		}
		if addon.AmountMinor > money.MaxMinor/max(in.Quantity-old, 1) {
			return model.ErrInvalid
		}
		now := time.Now().UTC()
		amount := money.New(addon.AmountMinor*(in.Quantity-old)).Ratio(sub.PeriodEnd.Time.Sub(now).Nanoseconds(), sub.PeriodEnd.Time.Sub(sub.PeriodStart.Time).Nanoseconds()).Minor()
		price, e := r.Q.BillingGetPriceID(ctx, sub.PriceID)
		if e != nil {
			return e
		}
		if price.Currency != addon.Currency {
			return payments.ErrUnsupported
		}
		invoice, e := s.buildInvoice(ctx, r, sub, InvoiceInput{Kind: "addon", IdempotencyKey: key, Currency: addon.Currency, PeriodStart: now, PeriodEnd: sub.PeriodEnd.Time, DueAt: now, Lines: []InvoiceLineInput{{Description: addon.Name, Quantity: 1, UnitAmountMinor: amount, VATBasisPoints: price.VatBasisPoints}}})
		if e != nil {
			return e
		}
		out = &invoice
		if e = r.Q.BillingNewItemChange(ctx, db.BillingNewItemChangeParams{OrganizationID: org, InvoiceID: invoice.ID, SubscriptionID: sub.ID, AddonID: addon.ID, Quantity: in.Quantity, UnitAmountMinor: addon.AmountMinor, Currency: addon.Currency}); e != nil {
			return e
		}
		if invoice.TotalMinor == 0 {
			return s.settleCommerce(ctx, r, invoice, sub)
		}
		return nil
	})
	return out, err
}
