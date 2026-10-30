package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WalletInput is an append-only ledger command. Negative AI credits return a
// machine-readable handoff action when funds are exhausted; no debt is allowed.
type WalletInput struct {
	Unit           string     `json:"unit"`
	Delta          int64      `json:"delta"`
	Reason         string     `json:"reason"`
	IdempotencyKey string     `json:"idempotency_key"`
	ExpiresAt      *time.Time `json:"expires_at"`
	// Category classifies a manual adjustment (goodwill, correction,
	// promotion, sales, migration, other); Actor is the platform admin. Both
	// are stored on the ledger row (CF-143).
	Category string `json:"category"`
	Actor    int64  `json:"-"`
}

var adjustmentCategories = map[string]bool{"goodwill": true, "correction": true, "promotion": true, "sales": true, "migration": true, "other": true}

type WalletSummary struct {
	TRYMinor     int64                    `json:"try_minor"`
	AICredits    int64                    `json:"ai_credits"`
	AIAction     string                   `json:"ai_action"`
	VideoMinutes int64                    `json:"video_minutes"`
	Entries      []db.BillingCreditLedger `json:"entries"`
}

// Metered credit units: lots are consumed FIFO (soonest expiry first) and
// expire through the same sweep.
const (
	UnitAICredits    = "ai_credits"
	UnitVideoMinutes = "video_minutes"
)

var meteredUnits = []string{UnitAICredits, UnitVideoMinutes}

func validCreditUnit(unit string) bool { return unit == UnitAICredits || unit == UnitVideoMinutes }

func (s *Service) Wallet(ctx context.Context, org int64, limit, offset int32) (WalletSummary, error) {
	var out WalletSummary
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		if e := s.expireWallet(ctx, r, org, time.Now().UTC()); e != nil {
			return e
		}
		var e error
		out.TRYMinor, e = r.Q.BillingWalletBalance(ctx, db.BillingWalletBalanceParams{OrganizationID: org, Unit: "TRY"})
		if e != nil {
			return e
		}
		out.AICredits, e = r.Q.BillingWalletBalance(ctx, db.BillingWalletBalanceParams{OrganizationID: org, Unit: "ai_credits"})
		if e != nil {
			return e
		}
		out.VideoMinutes, e = r.Q.BillingWalletBalance(ctx, db.BillingWalletBalanceParams{OrganizationID: org, Unit: UnitVideoMinutes})
		if e != nil {
			return e
		}
		out.AIAction = "continue"
		if out.AICredits <= 0 {
			out.AIAction = "stop_and_handoff"
		}
		out.Entries, e = r.Q.BillingWalletEntries(ctx, db.BillingWalletEntriesParams{OrganizationID: org, Limit: limit, Offset: offset})
		return e
	})
	return out, err
}
func (s *Service) WalletCommand(ctx context.Context, org int64, in WalletInput) (db.BillingCreditLedger, error) {
	var out db.BillingCreditLedger
	if in.Category != "" && !adjustmentCategories[in.Category] {
		return out, model.ErrInvalid
	}
	if in.Unit != "TRY" && !validCreditUnit(in.Unit) || in.Delta == 0 || in.Delta > money.MaxMinor || in.Delta < -money.MaxMinor || in.Reason == "" || len(in.Reason) > 500 || len(in.IdempotencyKey) == 0 || len(in.IdempotencyKey) > 120 || in.ExpiresAt != nil && (in.Delta < 0 || !in.ExpiresAt.After(time.Now())) {
		return out, model.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		old, e := r.Q.BillingWalletKey(ctx, db.BillingWalletKeyParams{OrganizationID: org, Unit: in.Unit, IdempotencyKey: in.IdempotencyKey})
		if e == nil {
			if old.Delta != in.Delta || old.Reason != in.Reason || !sameOptionalTime(old.ExpiresAt, in.ExpiresAt) {
				return model.ErrConflict
			}
			out = old
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e = s.expireWallet(ctx, r, org, time.Now().UTC()); e != nil {
			return e
		}
		balance, e := r.Q.BillingWalletBalance(ctx, db.BillingWalletBalanceParams{OrganizationID: org, Unit: in.Unit})
		if e != nil {
			return e
		}
		if balance+in.Delta < 0 {
			return model.ErrConflict
		}
		if balance+in.Delta > money.MaxMinor {
			return model.ErrInvalid
		}
		sub, e := r.Q.BillingGetSubscription(ctx, org)
		if e != nil {
			return e
		}
		if in.Category != "" || in.Actor > 0 {
			var actor pgtype.Int8
			if in.Actor > 0 {
				actor = pgtype.Int8{Int64: in.Actor, Valid: true}
			}
			out, e = r.Q.BillingManualWalletEntry(ctx, db.BillingManualWalletEntryParams{OrganizationID: org, SubscriptionID: sub.ID, Unit: in.Unit, Delta: in.Delta, Reason: in.Reason, IdempotencyKey: in.IdempotencyKey, ExpiresAt: optionalTS(in.ExpiresAt), Category: pgtype.Text{String: in.Category, Valid: in.Category != ""}, ActorUserID: actor})
		} else {
			out, e = r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: org, SubscriptionID: sub.ID, Unit: in.Unit, Delta: in.Delta, Reason: in.Reason, IdempotencyKey: in.IdempotencyKey, ExpiresAt: optionalTS(in.ExpiresAt)})
		}
		if e != nil {
			return e
		}
		if in.Delta < 0 {
			lots, e := r.Q.BillingCreditLots(ctx, db.BillingCreditLotsParams{OrganizationID: org, Unit: in.Unit})
			if e != nil {
				return e
			}
			remaining := -in.Delta
			for _, lot := range lots {
				amount := min(remaining, lot.Remaining)
				if amount <= 0 {
					continue
				}
				if e = r.Q.BillingAllocateCredits(ctx, db.BillingAllocateCreditsParams{OrganizationID: org, DebitID: out.ID, CreditID: lot.ID, Amount: amount}); e != nil {
					return e
				}
				remaining -= amount
				if remaining == 0 {
					break
				}
			}
			if remaining > 0 {
				return model.ErrConflict
			}
		}
		return r.Emit(ctx, events.New("billing.wallet_changed").WithTenant(org).WithEntity("billing_credit_ledger", &out.ID, &out.Uuid).WithPayload(map[string]any{"unit": in.Unit, "delta": in.Delta, "balance": balance + in.Delta, "ai_action": map[bool]string{true: "stop_and_handoff", false: "continue"}[balance+in.Delta == 0]}))
	})
	return out, err
}
func sameOptionalTime(stored pgtype.Timestamptz, in *time.Time) bool {
	return !stored.Valid && in == nil || stored.Valid && in != nil && stored.Time.Equal(*in)
}
func (s *Service) expireWallet(ctx context.Context, r *repository.Repository, org int64, now time.Time) error {
	for _, unit := range meteredUnits {
		lots, err := r.Q.BillingCreditLots(ctx, db.BillingCreditLotsParams{OrganizationID: org, Unit: unit})
		if err != nil {
			return err
		}
		for _, lot := range lots {
			if !lot.ExpiresAt.Valid || lot.ExpiresAt.Time.After(now) || lot.Remaining <= 0 {
				continue
			}
			if _, err = r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: org, SubscriptionID: lot.SubscriptionID, Unit: lot.Unit, Delta: -lot.Remaining, Reason: "Credit expiry", IdempotencyKey: fmt.Sprintf("expiry:%d", lot.ID), ReversesID: pgtype.Int8{Int64: lot.ID, Valid: true}}); err != nil {
				return err
			}
		}
	}
	return nil
}

type CreditPackageInput struct {
	// Unit is the metered unit the package sells; empty means ai_credits.
	Unit        string `json:"unit"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Credits     int64  `json:"credits"`
	AmountMinor int64  `json:"amount_minor"`
	ExpiresDays *int32 `json:"expires_days"`
	// PlanUUIDs limits the pack to subscribers of these plans; empty = all.
	PlanUUIDs []uuid.UUID `json:"plan_uuids"`
	SortOrder int32       `json:"sort_order"`
	// Active is used by updates; a new package is always active.
	Active *bool `json:"active"`
}

// CreditPackage is a credit product with its plan visibility resolved to
// plan UUIDs for the API.
type CreditPackage struct {
	UUID        uuid.UUID   `json:"uuid"`
	Code        string      `json:"code"`
	Unit        string      `json:"unit"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Credits     int64       `json:"credits"`
	AmountMinor int64       `json:"amount_minor"`
	ExpiresDays *int32      `json:"expires_days"`
	Active      bool        `json:"active"`
	SortOrder   int32       `json:"sort_order"`
	PlanUUIDs   []uuid.UUID `json:"plan_uuids"`
	CreatedAt   time.Time   `json:"created_at"`
}

func (in *CreditPackageInput) validate() error {
	if in.Unit == "" {
		in.Unit = UnitAICredits
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if !validCreditUnit(in.Unit) || in.Name == "" || len(in.Name) > 200 || len(in.Description) > 500 || in.Credits < 1 || in.Credits > money.MaxMinor || in.AmountMinor < 1 || in.AmountMinor > money.MaxMinor || in.ExpiresDays != nil && (*in.ExpiresDays < 1 || *in.ExpiresDays > 3650) || len(in.PlanUUIDs) > 50 || in.SortOrder < 0 || in.SortOrder > 10000 {
		return model.ErrInvalid
	}
	return nil
}

func (s *Service) planIDs(ctx context.Context, ids []uuid.UUID) ([]int64, error) {
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		p, err := s.repo.Q.BillingGetPlan(ctx, id)
		if err != nil {
			return nil, repository.Normalize(err)
		}
		if !seen[p.ID] {
			seen[p.ID] = true
			out = append(out, p.ID)
		}
	}
	return out, nil
}

func (s *Service) creditPackages(ctx context.Context, rows []db.BillingCreditPackage) ([]CreditPackage, error) {
	plans := map[int64]uuid.UUID{}
	out := make([]CreditPackage, 0, len(rows))
	for _, row := range rows {
		p := CreditPackage{UUID: row.Uuid, Code: row.Code, Unit: row.Unit, Name: row.Name, Description: row.Description, Credits: row.Credits, AmountMinor: row.AmountMinor, Active: row.Active, SortOrder: row.SortOrder, PlanUUIDs: []uuid.UUID{}, CreatedAt: row.CreatedAt.Time}
		if row.ExpiresDays.Valid {
			days := row.ExpiresDays.Int32
			p.ExpiresDays = &days
		}
		for _, id := range row.PlanIds {
			if _, ok := plans[id]; !ok {
				plan, err := s.repo.Q.BillingGetPlanID(ctx, id)
				if err != nil {
					return nil, err
				}
				plans[id] = plan.Uuid
			}
			p.PlanUUIDs = append(p.PlanUUIDs, plans[id])
		}
		out = append(out, p)
	}
	return out, nil
}

// CreditPackages lists every credit product for the platform catalog,
// archived ones last.
func (s *Service) CreditPackages(ctx context.Context) ([]CreditPackage, error) {
	rows, err := s.repo.Q.BillingAllCreditPackages(ctx)
	if err != nil {
		return nil, err
	}
	return s.creditPackages(ctx, rows)
}

// OrganizationCreditPackages lists the packs the organization may buy.
func (s *Service) OrganizationCreditPackages(ctx context.Context, org int64) ([]CreditPackage, error) {
	rows, err := s.repo.Q.BillingOrganizationCreditPackages(ctx, org)
	if err != nil {
		return nil, err
	}
	return s.creditPackages(ctx, rows)
}

func (s *Service) CreateCreditPackage(ctx context.Context, in CreditPackageInput) (CreditPackage, error) {
	if err := in.validate(); err != nil || !couponCodePattern.MatchString(in.Code) {
		return CreditPackage{}, model.ErrInvalid
	}
	plans, err := s.planIDs(ctx, in.PlanUUIDs)
	if err != nil {
		return CreditPackage{}, err
	}
	row, err := s.repo.Q.BillingNewCreditPackage(ctx, db.BillingNewCreditPackageParams{Code: in.Code, Name: in.Name, Credits: in.Credits, AmountMinor: in.AmountMinor, ExpiresDays: optionalDays(in.ExpiresDays), Unit: in.Unit, Description: in.Description, PlanIds: plans, SortOrder: in.SortOrder})
	if err != nil {
		return CreditPackage{}, repository.Normalize(err)
	}
	out, err := s.creditPackages(ctx, []db.BillingCreditPackage{row})
	if err != nil {
		return CreditPackage{}, err
	}
	return out[0], nil
}

// UpdateCreditPackage edits a credit product. The code and unit are fixed;
// past purchases keep the credits and price they were invoiced with.
func (s *Service) UpdateCreditPackage(ctx context.Context, id uuid.UUID, in CreditPackageInput) (CreditPackage, error) {
	current, err := s.repo.Q.BillingCreditPackageAny(ctx, id)
	if err != nil {
		return CreditPackage{}, repository.Normalize(err)
	}
	in.Unit = current.Unit
	if err = in.validate(); err != nil {
		return CreditPackage{}, err
	}
	plans, err := s.planIDs(ctx, in.PlanUUIDs)
	if err != nil {
		return CreditPackage{}, err
	}
	active := current.Active
	if in.Active != nil {
		active = *in.Active
	}
	row, err := s.repo.Q.BillingUpdateCreditPackage(ctx, db.BillingUpdateCreditPackageParams{Uuid: id, Name: in.Name, Description: in.Description, Credits: in.Credits, AmountMinor: in.AmountMinor, ExpiresDays: optionalDays(in.ExpiresDays), PlanIds: plans, SortOrder: in.SortOrder, Active: active})
	if err != nil {
		return CreditPackage{}, repository.Normalize(err)
	}
	out, err := s.creditPackages(ctx, []db.BillingCreditPackage{row})
	if err != nil {
		return CreditPackage{}, err
	}
	return out[0], nil
}

func optionalDays(days *int32) pgtype.Int4 {
	if days == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *days, Valid: true}
}

// organizationPackage returns the package when the organization may buy it.
func organizationPackage(ctx context.Context, r *repository.Repository, org int64, id uuid.UUID) (db.BillingCreditPackage, error) {
	rows, err := r.Q.BillingOrganizationCreditPackages(ctx, org)
	if err != nil {
		return db.BillingCreditPackage{}, err
	}
	for _, p := range rows {
		if p.Uuid == id {
			return p, nil
		}
	}
	return db.BillingCreditPackage{}, model.ErrNotFound
}

type CreditPurchaseInput struct {
	PackageUUID    uuid.UUID `json:"package_uuid"`
	IdempotencyKey string    `json:"idempotency_key"`
}

func (s *Service) PurchaseCredits(ctx context.Context, org int64, in CreditPurchaseInput) (db.BillingInvoice, error) {
	var out db.BillingInvoice
	if in.IdempotencyKey == "" || len(in.IdempotencyKey) > 120 {
		return out, model.ErrInvalid
	}
	// Resolved before the transaction: provider lookups use their own pool
	// connections and must not wait while this one holds the organization lock.
	payable, err := s.invoicePayable(ctx)
	if err != nil {
		return out, err
	}
	err = s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		key := "credits:" + in.IdempotencyKey
		old, e := r.Q.BillingInvoiceKey(ctx, db.BillingInvoiceKeyParams{OrganizationID: org, IdempotencyKey: key})
		if e == nil {
			purchase, pe := r.Q.BillingPurchase(ctx, db.BillingPurchaseParams{OrganizationID: org, InvoiceID: old.ID})
			if pe != nil {
				return pe
			}
			p, pe := r.Q.BillingCreditPackage(ctx, in.PackageUUID)
			if errors.Is(pe, pgx.ErrNoRows) {
				return model.ErrPackageUnavailable
			}
			if pe != nil {
				return pe
			}
			if purchase.PackageID != p.ID {
				return model.ErrConflict
			}
			out = old
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		sub, e := r.Q.BillingGetSubscription(ctx, org)
		if errors.Is(e, pgx.ErrNoRows) {
			return model.ErrAccountMissing
		}
		if e != nil {
			return e
		}
		p, e := organizationPackage(ctx, r, org, in.PackageUUID)
		if errors.Is(e, model.ErrNotFound) {
			return model.ErrPackageUnavailable
		}
		if e != nil {
			return e
		}
		// An invoice nobody can pay is a dead end: refuse before opening it.
		if !payable {
			return model.ErrNoPaymentMethod
		}
		price, e := r.Q.BillingGetPriceID(ctx, sub.PriceID)
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		out, e = s.buildInvoice(ctx, r, sub, InvoiceInput{Kind: "wallet", IdempotencyKey: key, Currency: "TRY", PeriodStart: now, PeriodEnd: now.Add(time.Second), DueAt: now, Lines: []InvoiceLineInput{{Description: p.Name, Quantity: 1, UnitAmountMinor: p.AmountMinor, VATBasisPoints: price.VatBasisPoints}}})
		if e != nil {
			return e
		}
		return r.Q.BillingNewPurchase(ctx, db.BillingNewPurchaseParams{OrganizationID: org, InvoiceID: out.ID, PackageID: p.ID, Credits: p.Credits, ExpiresDays: p.ExpiresDays, Unit: p.Unit})
	})
	return out, err
}

type WalletSettingsInput struct {
	AutoTopup   bool      `json:"auto_topup"`
	Threshold   int64     `json:"threshold"`
	PackageUUID uuid.UUID `json:"package_uuid"`
	Consent     bool      `json:"consent"`
	// AlertThreshold warns owners when AI credits drop under it; nil keeps
	// the default (a fifth of the plan allowance).
	AlertThreshold *int64 `json:"alert_threshold"`
}

// WalletSettings is the saved auto top-up and low balance alert setup.
type WalletSettings struct {
	AutoTopup      bool       `json:"auto_topup"`
	Threshold      int64      `json:"threshold"`
	PackageUUID    *uuid.UUID `json:"package_uuid"`
	AlertThreshold *int64     `json:"alert_threshold"`
	// AutoTopupAvailable: auto top-up charges the saved card, so it needs
	// auto-pay with a card on file.
	AutoTopupAvailable bool `json:"auto_topup_available"`
}

func (s *Service) WalletSettings(ctx context.Context, org int64) (WalletSettings, error) {
	out := WalletSettings{}
	row, err := s.repo.Q.BillingWalletSettings(ctx, org)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if err == nil {
		out.AutoTopup, out.Threshold = row.AutoTopup, row.Threshold
		if row.PackageUuid.Valid {
			id := uuid.UUID(row.PackageUuid.Bytes)
			out.PackageUUID = &id
		}
		if row.AlertThreshold.Valid {
			v := row.AlertThreshold.Int64
			out.AlertThreshold = &v
		}
	}
	sub, err := s.repo.Q.BillingGetSubscription(ctx, org)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	out.AutoTopupAvailable = err == nil && sub.AutoPay && sub.PaymentMethodID.Valid
	return out, nil
}

func (s *Service) SaveWalletSettings(ctx context.Context, org int64, in WalletSettingsInput) error {
	if in.Threshold < 0 || in.Threshold > money.MaxMinor || in.AutoTopup && !in.Consent || in.AlertThreshold != nil && (*in.AlertThreshold < 0 || *in.AlertThreshold > money.MaxMinor) {
		return model.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		var packageID pgtype.Int8
		var consent pgtype.Timestamptz
		if in.AutoTopup {
			sub, e := r.Q.BillingGetSubscription(ctx, org)
			if e != nil {
				return e
			}
			if !sub.AutoPay || !sub.PaymentMethodID.Valid {
				return model.ErrConflict
			}
			p, e := organizationPackage(ctx, r, org, in.PackageUUID)
			if e != nil {
				return e
			}
			packageID = pgtype.Int8{Int64: p.ID, Valid: true}
			consent = repository.TS(time.Now().UTC())
		}
		if e := r.Q.BillingSaveWalletSettings(ctx, db.BillingSaveWalletSettingsParams{OrganizationID: org, AutoTopup: in.AutoTopup, Threshold: in.Threshold, PackageID: packageID, ConsentedAt: consent}); e != nil {
			return e
		}
		alert := pgtype.Int8{}
		if in.AlertThreshold != nil {
			alert = pgtype.Int8{Int64: *in.AlertThreshold, Valid: true}
		}
		return r.Q.BillingSetAlertThreshold(ctx, db.BillingSetAlertThresholdParams{OrganizationID: org, AlertThreshold: alert})
	})
}

// ExpireWallet expires one organization's unconsumed lots under its wallet lock.
func (s *Service) ExpireWallet(ctx context.Context, org int64, now time.Time) error {
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		return s.expireWallet(ctx, r, org, now)
	})
}

func (s *Service) WalletSweep(ctx context.Context, now time.Time) error {
	orgs, err := s.repo.Q.BillingExpiredWalletOrganizations(ctx, repository.TS(now))
	if err != nil {
		return err
	}
	for _, org := range orgs {
		if err = s.ExpireWallet(ctx, org, now); err != nil {
			return err
		}
	}
	var failures []error
	if err = s.lowBalanceAlerts(ctx); err != nil {
		failures = append(failures, err)
	}
	// Auto top-up charges a saved card; without a gateway there is nothing
	// to charge, so no invoice is opened either.
	if s.gateway == nil {
		return errors.Join(failures...)
	}
	candidates, err := s.repo.Q.BillingTopupCandidates(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		invoice, e := s.repo.Q.BillingUnsettledWalletInvoice(ctx, candidate.OrganizationID)
		if errors.Is(e, pgx.ErrNoRows) {
			invoice, e = s.PurchaseCredits(ctx, candidate.OrganizationID, CreditPurchaseInput{PackageUUID: candidate.PackageUuid, IdempotencyKey: "auto:" + now.UTC().Format("2006-01-02")})
		}
		if e == nil {
			e = s.chargeRenewal(ctx, invoice, now)
		}
		if e != nil {
			failures = append(failures, e)
		}
	}
	return errors.Join(failures...)
}

type WalletPaymentInput struct {
	InvoiceUUID    uuid.UUID `json:"invoice_uuid"`
	IdempotencyKey string    `json:"idempotency_key"`
}

func (s *Service) PayFromWallet(ctx context.Context, org int64, in WalletPaymentInput) (uuid.UUID, error) {
	var id uuid.UUID
	if in.IdempotencyKey == "" || len(in.IdempotencyKey) > 120 {
		return id, model.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		key := "wallet:" + in.IdempotencyKey
		invoice, e := r.Q.BillingInvoice(ctx, db.BillingInvoiceParams{OrganizationID: org, Uuid: in.InvoiceUUID})
		if e != nil {
			return e
		}
		old, e := r.Q.BillingPaymentKey(ctx, db.BillingPaymentKeyParams{OrganizationID: org, IdempotencyKey: key})
		if e == nil {
			if old.InvoiceID != invoice.ID {
				return model.ErrConflict
			}
			id = old.Uuid
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if invoice.Status != "open" || invoice.Kind == "credit_note" {
			return model.ErrConflict
		}
		paid, e := r.Q.BillingPaidTotal(ctx, db.BillingPaidTotalParams{OrganizationID: org, InvoiceID: invoice.ID})
		if e != nil {
			return e
		}
		balance, e := r.Q.BillingWalletBalance(ctx, db.BillingWalletBalanceParams{OrganizationID: org, Unit: "TRY"})
		if e != nil {
			return e
		}
		amount := min(balance, invoice.TotalMinor-paid)
		if amount <= 0 {
			return model.ErrConflict
		}
		p, e := r.Q.BillingNewPayment(ctx, db.BillingNewPaymentParams{OrganizationID: org, InvoiceID: invoice.ID, Provider: "wallet", Kind: "wallet", IdempotencyKey: key, AmountMinor: amount, OrderReference: text(OrderReference(org, key))})
		if e != nil {
			return e
		}
		id = p.Uuid
		debit, e := r.Q.BillingWalletEntry(ctx, db.BillingWalletEntryParams{OrganizationID: org, SubscriptionID: invoice.SubscriptionID, InvoiceID: pgtype.Int8{Int64: invoice.ID, Valid: true}, Unit: "TRY", Delta: -amount, Reason: "Invoice payment", IdempotencyKey: key})
		if e != nil {
			return e
		}
		lots, e := r.Q.BillingCreditLots(ctx, db.BillingCreditLotsParams{OrganizationID: org, Unit: "TRY"})
		if e != nil {
			return e
		}
		remaining := amount
		for _, lot := range lots {
			applied := min(lot.Remaining, remaining)
			if applied <= 0 {
				continue
			}
			if e = r.Q.BillingAllocateCredits(ctx, db.BillingAllocateCreditsParams{OrganizationID: org, DebitID: debit.ID, CreditID: lot.ID, Amount: applied}); e != nil {
				return e
			}
			remaining -= applied
			if remaining == 0 {
				break
			}
		}
		if remaining > 0 {
			return model.ErrConflict
		}
		return s.settle(ctx, r, p, payments.Notification{OrderRef: p.OrderReference.String, Status: payments.StatusSucceeded, Amount: amount, PaidAmount: amount, Currency: "TRY", ProviderPaymentID: "wallet:" + id.String()}, time.Now().UTC())
	})
	return id, err
}

// lowBalanceAlerts tells owners once a day when AI credits drop under the
// organization's alert threshold (CF-143). Video minutes have their own
// alert in videorooms.
func (s *Service) lowBalanceAlerts(ctx context.Context) error {
	rows, err := s.repo.Q.BillingLowAICreditCandidates(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, row := range rows {
		err = s.repo.Within(ctx, func(r *repository.Repository) error {
			if _, e := r.Q.BillingLockOrganizationID(ctx, row.OrganizationID); e != nil {
				return e
			}
			n, e := r.Q.BillingMarkLowBalance(ctx, row.OrganizationID)
			if e != nil || n == 0 {
				return e
			}
			day := time.Now().UTC().Format("2006-01-02")
			return r.Emit(ctx, events.New(events.BillingWalletLow).WithTenant(row.OrganizationID).WithPayload(map[string]any{"unit": UnitAICredits, "balance": row.Balance}).WithIdempotencyKey(fmt.Sprintf("wallet-low:%d:%s", row.OrganizationID, day)))
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
