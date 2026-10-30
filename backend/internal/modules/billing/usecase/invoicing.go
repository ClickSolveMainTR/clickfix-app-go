package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
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

type InvoiceLineInput struct {
	Description     string `json:"description"`
	Quantity        int64  `json:"quantity"`
	UnitAmountMinor int64  `json:"unit_amount_minor"`
	VATBasisPoints  int32  `json:"vat_basis_points"`
	TaxInclusive    bool   `json:"tax_inclusive"`
}
type InvoiceInput struct {
	IdempotencyKey    string             `json:"idempotency_key"`
	Kind              string             `json:"kind"`
	Currency          string             `json:"currency"`
	PeriodStart       time.Time          `json:"period_start"`
	PeriodEnd         time.Time          `json:"period_end"`
	DueAt             time.Time          `json:"due_at"`
	Lines             []InvoiceLineInput `json:"lines"`
	OriginalInvoiceID *int64             `json:"-"`
}

func (s *Service) ConfigureInvoicing(provider EInvoiceProvider, seller ProfileInput, series, env string) error {
	if provider != nil && provider.Code() == "fake" && env != "development" && env != "test" {
		return payments.ErrUnsupported
	}
	if series == "" {
		series = "CFX"
	}
	if len(series) != 3 || strings.Trim(series, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return model.ErrInvalid
	}
	s.einvoice = provider
	s.seller = seller
	s.invoiceSeries = series
	return nil
}
func (s *Service) InvoicingFromEnvironment(env string) error {
	var seller ProfileInput
	raw := os.Getenv("BILLING_SELLER_JSON")
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &seller); err != nil {
			return err
		}
		if err := seller.Validate(); err != nil {
			return fmt.Errorf("billing seller: %w", err)
		}
	}
	var provider EInvoiceProvider
	switch os.Getenv("BILLING_EINVOICE_PROVIDER") {
	case "":
	case "fake":
		provider = &FakeEInvoiceProvider{}
	default:
		return fmt.Errorf("live e-invoice integrator is not configured")
	}
	return s.ConfigureInvoicing(provider, seller, os.Getenv("BILLING_INVOICE_SERIES"), env)
}
func (s *Service) GenerateInvoice(ctx context.Context, org int64, in InvoiceInput) (db.BillingInvoice, error) {
	var out db.BillingInvoice
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		sub, e := r.Q.BillingGetSubscription(ctx, org)
		if e != nil {
			return e
		}
		out, e = s.buildInvoice(ctx, r, sub, in)
		return e
	})
	return out, err
}

// invoiceSeller is BILLING_SELLER_JSON, else the seller of the
// subscription's previous invoice (kept across a restart with an empty env).
func (s *Service) invoiceSeller(ctx context.Context, r *repository.Repository, sub db.BillingSubscription) (ProfileInput, error) {
	seller := s.seller
	if seller.LegalName == "" {
		previous, e := r.Q.BillingPreviousInvoice(ctx, db.BillingPreviousInvoiceParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID})
		if errors.Is(e, pgx.ErrNoRows) {
			return seller, model.ErrSellerNotConfigured
		}
		if e != nil {
			return seller, e
		}
		if e = json.Unmarshal(previous.SellerSnapshot, &seller); e != nil {
			return seller, model.ErrSellerNotConfigured
		}
	}
	if err := seller.Validate(); err != nil {
		return seller, fmt.Errorf("%w: %v", model.ErrSellerNotConfigured, err)
	}
	return seller, nil
}

func (s *Service) buildInvoice(ctx context.Context, r *repository.Repository, sub db.BillingSubscription, in InvoiceInput) (db.BillingInvoice, error) {
	var zero db.BillingInvoice
	if len(in.IdempotencyKey) == 0 || len(in.IdempotencyKey) > 160 || !in.PeriodEnd.After(in.PeriodStart) || in.DueAt.IsZero() || len(in.Lines) == 0 || len(in.Lines) > 100 {
		return zero, model.ErrInvalid
	}
	if in.Kind == "" {
		in.Kind = "renewal"
	}
	if in.Currency == "" {
		in.Currency = "TRY"
	}
	requestBytes, err := json.Marshal(in)
	if err != nil {
		return zero, err
	}
	hash := sha256.Sum256(requestBytes)
	digest := hex.EncodeToString(hash[:])
	existing, err := r.Q.BillingInvoiceKey(ctx, db.BillingInvoiceKeyParams{OrganizationID: sub.OrganizationID, IdempotencyKey: in.IdempotencyKey})
	if err == nil {
		if existing.RequestDigest != "" && existing.RequestDigest != digest {
			return zero, model.ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return zero, err
	}
	profile, err := r.Q.BillingProfile(ctx, sub.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, model.ErrProfileIncomplete
	}
	if err != nil {
		return zero, err
	}
	buyer := profileInput(profile)
	if err = buyer.Validate(); err != nil {
		return zero, fmt.Errorf("%w: %v", model.ErrProfileIncomplete, err)
	}
	// A missing seller identity is an operator configuration gap, not a
	// payment gateway outage (CF-154: it answered 503 on credit purchases).
	seller, err := s.invoiceSeller(ctx, r, sub)
	if err != nil {
		return zero, err
	}
	buyerBytes, err := json.Marshal(buyer)
	if err != nil {
		return zero, err
	}
	sellerBytes, err := json.Marshal(seller)
	if err != nil {
		return zero, err
	}
	rate := pgtype.Numeric{}
	_ = rate.Scan("1")
	source := "native"
	observed := time.Now().UTC()
	if in.Currency != "TRY" {
		fx, e := r.Q.BillingLatestFX(ctx, db.BillingLatestFXParams{Currency: in.Currency, AtDate: pgtype.Date{Time: observed, Valid: true}})
		if e != nil {
			return zero, e
		}
		rate = fx.Rate
		source = fx.Source
		observed = fx.ObservedOn.Time
	}
	convert := func(amount int64) (int64, error) {
		v := new(big.Rat).SetInt64(amount)
		n := new(big.Rat).SetInt(rate.Int)
		if rate.Exp < 0 {
			n.Quo(n, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(-int64(rate.Exp)), nil)))
		} else {
			n.Mul(n, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(rate.Exp)), nil)))
		}
		v.Mul(v, n)
		q, rem := new(big.Int).QuoRem(v.Num(), v.Denom(), new(big.Int))
		if rem.Mul(rem, big.NewInt(2)).Cmp(v.Denom()) >= 0 {
			q.Add(q, big.NewInt(1))
		}
		if !q.IsInt64() || q.Int64() > money.MaxMinor {
			return 0, model.ErrInvalid
		}
		return q.Int64(), nil
	}
	lines := make([]db.BillingCreateInvoiceLineParams, 0, len(in.Lines))
	subtotal := int64(0)
	for _, l := range in.Lines {
		if l.Quantity <= 0 || l.UnitAmountMinor < 0 || l.UnitAmountMinor > money.MaxMinor/l.Quantity || l.VATBasisPoints < 0 || l.VATBasisPoints > 10000 || l.Description == "" {
			return zero, model.ErrInvalid
		}
		gross, e := convert(l.UnitAmountMinor * l.Quantity)
		if e != nil {
			return zero, e
		}
		vat := l.VATBasisPoints
		if buyer.ExportExemption {
			vat = 0
		}
		net := gross
		if l.TaxInclusive {
			net = money.New(gross).Ratio(10000, 10000+int64(vat)).Minor()
		}
		subtotal += net
		if subtotal > money.MaxMinor {
			return zero, model.ErrInvalid
		}
		lines = append(lines, db.BillingCreateInvoiceLineParams{OrganizationID: sub.OrganizationID, Description: l.Description, Quantity: l.Quantity, UnitAmountMinor: money.New(net).Ratio(1, l.Quantity).Minor(), SubtotalMinor: net, VatBasisPoints: vat})
	}
	discount := int64(0)
	var coupon db.BillingCoupon
	if in.Kind == "renewal" {
		coupon, err = r.Q.BillingAttachedCoupon(ctx, db.BillingAttachedCouponParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, AtTime: repository.TS(time.Now().UTC())})
		if err == nil {
			price, e := r.Q.BillingGetPriceID(ctx, sub.PriceID)
			if e != nil {
				return zero, e
			}
			discount, err = s.couponDiscount(ctx, r, sub.OrganizationID, coupon, price, subtotal)
			if errors.Is(err, model.ErrInvalid) || errors.Is(err, model.ErrConflict) {
				discount = 0
				err = nil
			}
		} else if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return zero, err
		}
	}
	remaining := discount
	tax := int64(0)
	for i := range lines {
		d := int64(0)
		if subtotal > 0 {
			d = money.New(discount).Ratio(lines[i].SubtotalMinor, subtotal).Minor()
		}
		d = min(d, remaining, lines[i].SubtotalMinor)
		if i == len(lines)-1 {
			d = min(remaining, lines[i].SubtotalMinor)
		}
		remaining -= d
		lines[i].DiscountMinor = d
		lines[i].TaxMinor = money.New(lines[i].SubtotalMinor-d).Ratio(int64(lines[i].VatBasisPoints), 10000).Minor()
		lines[i].TotalMinor = lines[i].SubtotalMinor - d + lines[i].TaxMinor
		tax += lines[i].TaxMinor
	}
	total := subtotal - discount + tax
	if total > money.MaxMinor {
		return zero, model.ErrInvalid
	}
	out, err := r.Q.BillingCreateInvoice(ctx, db.BillingCreateInvoiceParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, PeriodStart: repository.TS(in.PeriodStart), PeriodEnd: repository.TS(in.PeriodEnd), Kind: in.Kind, IdempotencyKey: in.IdempotencyKey, SourceCurrency: in.Currency, FxRate: rate, FxSource: source, FxObservedAt: repository.TS(observed), BuyerSnapshot: buyerBytes, SellerSnapshot: sellerBytes, SubtotalMinor: subtotal, DiscountMinor: discount, TaxMinor: tax, TotalMinor: total, DueAt: repository.TS(in.DueAt), OriginalInvoiceID: repository.Int(in.OriginalInvoiceID), RequestDigest: digest})
	if err != nil {
		return zero, err
	}
	for _, l := range lines {
		l.InvoiceID = out.ID
		if err = r.Q.BillingCreateInvoiceLine(ctx, l); err != nil {
			return zero, err
		}
	}
	if discount > 0 {
		if err = r.Q.BillingRedeemCoupon(ctx, db.BillingRedeemCouponParams{OrganizationID: sub.OrganizationID, CouponID: coupon.ID, InvoiceID: out.ID, DiscountMinor: discount}); err != nil {
			return zero, err
		}
	}
	if total == 0 {
		if err = r.Q.BillingPayInvoice(ctx, db.BillingPayInvoiceParams{OrganizationID: sub.OrganizationID, ID: out.ID}); err != nil {
			return zero, err
		}
		out, err = r.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: sub.OrganizationID, ID: out.ID})
		if err != nil {
			return zero, err
		}
		if err = s.settleCommerce(ctx, r, out, sub); err != nil {
			return zero, err
		}
		if in.Kind == "renewal" && !in.PeriodStart.Before(sub.PeriodEnd.Time) {
			if _, e := applyTransition(ctx, r, sub, model.Transition{To: "active", Reason: "Zero balance invoice settled", PeriodStart: &in.PeriodStart, PeriodEnd: &in.PeriodEnd}, 0, time.Now().UTC()); e != nil {
				return zero, e
			}
		}

	}
	event := events.New("billing.invoice_created").WithTenant(sub.OrganizationID).WithEntity("billing_invoices", &out.ID, &out.Uuid).WithPayload(map[string]any{"invoice_uuid": out.Uuid, "total_minor": total})
	event.EventID = uuid.NewSHA1(out.Uuid, []byte("invoice_created"))
	return out, r.Emit(ctx, event)
}

// periodInvoice includes frozen plan, seats, add-ons and closed usage tiers.
func (s *Service) periodInvoice(ctx context.Context, r *repository.Repository, sub db.BillingSubscription, now time.Time) (db.BillingInvoice, error) {
	key := "renewal:" + sub.Uuid.String() + ":" + sub.PeriodEnd.Time.Format(time.RFC3339Nano)
	if existing, e := r.Q.BillingInvoiceKey(ctx, db.BillingInvoiceKeyParams{OrganizationID: sub.OrganizationID, IdempotencyKey: key}); e == nil {
		return existing, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return db.BillingInvoice{}, e
	}
	priceID := sub.PriceID
	if sub.PendingPriceID.Valid {
		priceID = sub.PendingPriceID.Int64
	}
	p, err := r.Q.BillingGetPriceID(ctx, priceID)
	if err != nil {
		return db.BillingInvoice{}, err
	}
	seats, err := r.Q.BillingInvoiceSeats(ctx, sub.OrganizationID)
	if err != nil {
		return db.BillingInvoice{}, err
	}
	contracted := sub.ContractedSeats
	if change, e := r.Q.BillingScheduledChange(ctx, db.BillingScheduledChangeParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID}); e == nil {
		contracted = change.Seats
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return db.BillingInvoice{}, e
	}
	seats = max(seats, contracted)
	lines := []InvoiceLineInput{{Description: "ClickFix plan", Quantity: 1, UnitAmountMinor: p.AmountMinor, VATBasisPoints: p.VatBasisPoints, TaxInclusive: p.TaxInclusive}}
	if p.SeatAmountMinor > 0 {
		lines = append(lines, InvoiceLineInput{Description: "Seats", Quantity: seats, UnitAmountMinor: p.SeatAmountMinor, VATBasisPoints: p.VatBasisPoints, TaxInclusive: p.TaxInclusive})
	}
	items, err := r.Q.BillingSubscriptionItems(ctx, db.BillingSubscriptionItemsParams{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID})
	if err != nil {
		return db.BillingInvoice{}, err
	}
	for _, item := range items {
		qty := item.Quantity
		if item.PendingQuantity.Valid {
			qty = item.PendingQuantity.Int64
		}
		if qty == 0 {
			continue
		}
		if item.Currency != p.Currency {
			return db.BillingInvoice{}, payments.ErrUnsupported
		}
		lines = append(lines, InvoiceLineInput{Description: item.Name, Quantity: qty, UnitAmountMinor: item.UnitAmountMinor, VATBasisPoints: p.VatBasisPoints, TaxInclusive: p.TaxInclusive})
	}
	usage, err := r.Q.BillingInvoiceUsage(ctx, db.BillingInvoiceUsageParams{OrganizationID: sub.OrganizationID, PeriodEnd: sub.PeriodEnd})
	if err != nil {
		return db.BillingInvoice{}, err
	}
	previousPrice, e := r.Q.BillingGetPriceID(ctx, sub.PriceID)
	if e != nil {
		return db.BillingInvoice{}, e
	}
	if previousPrice.Currency != p.Currency {
		return db.BillingInvoice{}, payments.ErrUnsupported
	}
	tiers, err := r.Q.BillingListTiers(ctx, previousPrice.ID)
	if err != nil {
		return db.BillingInvoice{}, err
	}
	for _, u := range usage {
		for _, tier := range tiers {
			if u.Key != tier.FeatureKey {
				continue
			}
			upper := u.Quantity
			if tier.UpperBound.Valid {
				upper = min(upper, tier.UpperBound.Int64)
			}
			qty := max(upper-tier.LowerBound, 0)
			if qty > 0 {
				lines = append(lines, InvoiceLineInput{Description: fmt.Sprintf("Usage: %s period %d", tier.FeatureKey, u.UsagePeriodID), Quantity: qty, UnitAmountMinor: tier.UnitAmountMinor, VATBasisPoints: previousPrice.VatBasisPoints, TaxInclusive: previousPrice.TaxInclusive})
			}
		}
	}
	start := sub.PeriodEnd.Time
	end := NextPeriod(start, p.Interval == "yearly")
	// Preserve the contractual period even when the scheduler was delayed.
	sub.PriceID = priceID
	profile, err := r.Q.BillingProfile(ctx, sub.OrganizationID)
	if err != nil {
		return db.BillingInvoice{}, err
	}
	due := start.AddDate(0, 0, int(profile.PaymentTermsDays))
	invoice, err := s.buildInvoice(ctx, r, sub, InvoiceInput{Kind: "renewal", IdempotencyKey: "renewal:" + sub.Uuid.String() + ":" + start.Format(time.RFC3339Nano), Currency: p.Currency, PeriodStart: start, PeriodEnd: end, DueAt: due, Lines: lines})
	if err != nil {
		return invoice, err
	}
	seen := map[int64]bool{}
	for _, u := range usage {
		if seen[u.UsagePeriodID] {
			continue
		}
		seen[u.UsagePeriodID] = true
		if err = r.Q.BillingClaimInvoiceUsage(ctx, db.BillingClaimInvoiceUsageParams{OrganizationID: sub.OrganizationID, UsagePeriodID: u.UsagePeriodID, InvoiceID: invoice.ID}); err != nil {
			return invoice, err
		}
	}
	return invoice, nil
}

func (s *Service) CommerceCounts(ctx context.Context, org int64) (db.BillingCommerceCountsRow, error) {
	return s.repo.Q.BillingCommerceCounts(ctx, org)
}

// CommerceSweep is called every five minutes. A broken provider never blocks
// document issuance, refund notes or unrelated tenant renewal deadlines.
func (s *Service) CommerceSweep(ctx context.Context, now time.Time) error {
	return errors.Join(s.RenewalSweep(ctx, now), s.ReconcileStatements(ctx), s.CreditNoteSweep(ctx), s.WalletSweep(ctx, now), s.InvoiceSweep(ctx), s.ReferralSweep(ctx))
}

// ProviderSync reconciles scheduled provider calls independently of renewal charges.
func (s *Service) ProviderSync(ctx context.Context) error {
	return errors.Join(s.ReconcilePending(ctx), s.ReconcileRefunds(ctx))
}

func (s *Service) NightlyCommerce(ctx context.Context) error {
	var statementErr error
	if s.statements != nil {
		rows, e := s.statements.Fetch(ctx, time.Now().UTC().AddDate(0, 0, -1))
		statementErr = e
		if e == nil && len(rows) > 0 {
			for start := 0; start < len(rows); start += 1000 {
				if e = s.ImportStatement(ctx, rows[start:min(start+1000, len(rows))]); e != nil {
					statementErr = e
					break
				}
			}
		}
	}
	return errors.Join(s.FetchTCMB(ctx), statementErr, s.NightlyReconcile(ctx), s.ReconcileStatements(ctx), s.ReconcileRefunds(ctx))
}
