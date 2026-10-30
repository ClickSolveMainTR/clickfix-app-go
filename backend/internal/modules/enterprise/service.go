// Package enterprise implements the sales quote lifecycle and custom billing versions.
package enterprise

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	billingmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	billingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jung-kurt/gofpdf/v2"
	"strings"
	"time"
)

type Service struct {
	Repo    *repository.Repository
	Billing *billingusecase.Service
}
type Line struct {
	Description string `json:"description"`
	Quantity    int64  `json:"quantity"`
	UnitAmount  int64  `json:"unit_amount_minor"`
}
type Input struct {
	Organization   uuid.UUID            `json:"organization_uuid"`
	BaseVersion    uuid.UUID            `json:"base_version_uuid"`
	Title          string               `json:"title"`
	Currency       string               `json:"currency"`
	Interval       string               `json:"interval"`
	Lines          []Line               `json:"lines"`
	Grants         []billingmodel.Grant `json:"grants"`
	VAT            int32                `json:"vat_basis_points"`
	ContractMonths int32                `json:"contract_months"`
	PaymentTerms   int32                `json:"payment_terms_days"`
	PO             string               `json:"po_number"`
	Terms          string               `json:"terms"`
	ValidUntil     time.Time            `json:"valid_until"`
}

func Total(lines []Line) (int64, error) {
	if len(lines) == 0 || len(lines) > 100 {
		return 0, supportops.ErrInvalid
	}
	var total int64
	for _, l := range lines {
		if strings.TrimSpace(l.Description) == "" || len(l.Description) > 500 || l.Quantity < 1 || l.Quantity > 100000 || l.UnitAmount < 0 || l.UnitAmount > 1000000000000 {
			return 0, supportops.ErrInvalid
		}
		amount := l.Quantity * l.UnitAmount
		if total > 100000000000000-amount {
			return 0, supportops.ErrInvalid
		}
		total += amount
	}
	return total, nil
}
func (s *Service) Create(ctx context.Context, actor int64, in Input) (db.EnterpriseQuote, error) {
	var out db.EnterpriseQuote
	total, err := Total(in.Lines)
	if err != nil {
		return out, err
	}
	if strings.TrimSpace(in.Title) == "" || len(in.Title) > 200 || in.Currency != "TRY" && in.Currency != "USD" && in.Currency != "EUR" || in.Interval != "monthly" && in.Interval != "yearly" || in.ContractMonths < 1 || in.ContractMonths > 120 || in.Interval == "yearly" && in.ContractMonths%12 != 0 || in.VAT < 0 || in.VAT > 10000 || len(in.PO) > 200 || len(in.Terms) > 16000 || !in.ValidUntil.After(time.Now()) || in.ValidUntil.After(time.Now().AddDate(1, 0, 0)) || in.PaymentTerms != 0 && in.PaymentTerms != 15 && in.PaymentTerms != 30 && in.PaymentTerms != 60 && in.PaymentTerms != 90 {
		return out, supportops.ErrInvalid
	}
	org, err := s.Repo.Q.BillingGetOrganizationID(ctx, in.Organization)
	if err != nil {
		return out, err
	}
	v, err := s.Repo.Q.BillingGetVersion(ctx, in.BaseVersion)
	if err != nil {
		return out, err
	}
	if v.Status != "published" {
		return out, supportops.ErrConflict
	}
	fs, err := s.Repo.Features(ctx)
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	if len(in.Grants) > 100 {
		return out, supportops.ErrInvalid
	}
	for _, g := range in.Grants {
		f, ok := fs[g.Feature]
		if !ok || seen[g.Feature] || f.Kind == "boolean" && (g.Enabled == nil || g.Limit != nil || g.Unlimited) || f.Kind == "limit" && (g.Enabled != nil || (g.Limit == nil) == !g.Unlimited || g.Limit != nil && *g.Limit < 0) {
			return out, supportops.ErrInvalid
		}
		seen[g.Feature] = true
	}
	lines, _ := json.Marshal(in.Lines)
	if in.Grants == nil {
		in.Grants = []billingmodel.Grant{}
	}
	grants, _ := json.Marshal(in.Grants)
	return s.Repo.Q.QuoteCreate(ctx, db.QuoteCreateParams{OrganizationID: org, BaseVersionID: v.ID, Title: in.Title, Currency: in.Currency, Interval: in.Interval, AmountMinor: total, VatBasisPoints: in.VAT, Lines: lines, Grants: grants, ContractMonths: in.ContractMonths, PaymentTermsDays: in.PaymentTerms, PoNumber: in.PO, Terms: in.Terms, ValidUntil: repository.TS(in.ValidUntil), CreatedBy: actor})
}
func (s *Service) Decide(ctx context.Context, token uuid.UUID, accept bool, ip string) (db.EnterpriseQuote, error) {
	var out db.EnterpriseQuote
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		q, err := r.Q.QuotePublicLock(ctx, token)
		if err != nil {
			return err
		}
		if q.Status != "sent" || !q.ValidUntil.Time.After(time.Now()) {
			return supportops.ErrConflict
		}
		status := "rejected"
		if accept {
			status = "accepted"
		}
		out, err = r.Q.QuoteDecide(ctx, db.QuoteDecideParams{OrganizationID: q.OrganizationID, Uuid: q.Uuid, Status: status, DecisionIp: ip})
		if err != nil {
			return err
		}
		return r.Emit(ctx, events.New("enterprise.quote_decided").WithOrganization(q.OrganizationID).WithEntity("enterprise_quote", &q.ID, &q.Uuid).WithPayload(map[string]any{"status": status}))
	})
	return out, err
}
func (s *Service) Convert(ctx context.Context, org, actor int64, id uuid.UUID) (db.EnterpriseQuote, error) {
	var out db.EnterpriseQuote
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		q, err := r.Q.QuoteLock(ctx, db.QuoteLockParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if q.Status == "converted" {
			out = q
			return nil
		}
		if q.Status != "accepted" {
			return supportops.ErrConflict
		}
		// Require a legal billing profile before granting commercial access.
		if _, err = r.Q.BillingProfile(ctx, org); err != nil {
			return err
		}
		plan, err := r.Q.BillingCreatePlan(ctx, db.BillingCreatePlanParams{Code: "enterprise-" + q.Uuid.String(), Name: q.Title, Description: "Organization-specific enterprise contract", Visibility: "custom"})
		if err != nil {
			return err
		}
		version, err := r.Q.BillingCreateVersion(ctx, db.BillingCreateVersionParams{PlanID: plan.ID})
		if err != nil {
			return err
		}
		if err = r.Q.QuoteCopyEntitlements(ctx, db.QuoteCopyEntitlementsParams{PlanVersionID: q.BaseVersionID, PlanVersionID_2: version.ID}); err != nil {
			return err
		}
		var grants []billingmodel.Grant
		if err = json.Unmarshal(q.Grants, &grants); err != nil {
			return err
		}
		fs, err := r.Features(ctx)
		if err != nil {
			return err
		}
		for _, g := range grants {
			f, ok := fs[g.Feature]
			if !ok {
				return supportops.ErrInvalid
			}
			n, err := r.Q.QuoteOverrideEntitlement(ctx, db.QuoteOverrideEntitlementParams{PlanVersionID: version.ID, FeatureID: f.ID, ValueBool: repository.Bool(g.Enabled), ValueLimit: repository.Int(g.Limit), Unlimited: g.Unlimited})
			if err != nil {
				return err
			}
			if n == 0 {
				if err = r.Q.BillingCreateEntitlement(ctx, db.BillingCreateEntitlementParams{PlanVersionID: version.ID, FeatureID: f.ID, Kind: f.Kind, ValueBool: repository.Bool(g.Enabled), ValueLimit: repository.Int(g.Limit), Unlimited: g.Unlimited, Enforcement: "hard", WarnPercent: 80}); err != nil {
					return err
				}
			}
		}
		price, err := r.Q.BillingCreatePrice(ctx, db.BillingCreatePriceParams{PlanVersionID: version.ID, Interval: q.Interval, Currency: q.Currency, AmountMinor: q.AmountMinor, VatBasisPoints: q.VatBasisPoints})
		if err != nil {
			return err
		}
		basePrices, e := r.Q.BillingListPrices(ctx, q.BaseVersionID)
		if e != nil {
			return e
		}
		matched := len(basePrices) == 0
		for _, base := range basePrices {
			if base.Currency == q.Currency && base.Interval == q.Interval {
				matched = true
				if e = r.Q.QuoteCopyTiers(ctx, db.QuoteCopyTiersParams{PriceID: base.ID, PriceID_2: price.ID}); e != nil {
					return e
				}
				break
			}
		}
		if !matched {
			return supportops.ErrInvalid
		}
		if _, err = r.Q.BillingPublishVersion(ctx, version.ID); err != nil {
			return err
		}
		newSubscription := false
		sub, err := r.Q.BillingGetSubscription(ctx, org)
		if errors.Is(err, pgx.ErrNoRows) {
			newSubscription = true
			now := time.Now().UTC()
			status := "pending_payment"
			if q.PaymentTermsDays > 0 {
				status = "active"
			}
			sub, err = r.Q.BillingCreateSubscription(ctx, db.BillingCreateSubscriptionParams{OrganizationID: org, PlanVersionID: version.ID, PriceID: price.ID, Status: status, PeriodStart: repository.TS(now), PeriodEnd: repository.TS(billingusecase.NextPeriod(now, q.Interval == "yearly"))})
			if err != nil {
				return err
			}
			if err = r.Q.BillingRecordTransition(ctx, db.BillingRecordTransitionParams{OrganizationID: org, SubscriptionID: sub.ID, ToStatus: status, Reason: "Enterprise quote " + q.Uuid.String(), ActorUserID: repository.Int(&actor)}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			// Existing contracts keep their current paid period. New pricing applies at renewal.
			if err = r.Q.BillingSchedulePlan(ctx, db.BillingSchedulePlanParams{OrganizationID: org, ID: sub.ID, PendingPlanVersionID: repository.Int(&version.ID), PendingPriceID: repository.Int(&price.ID)}); err != nil {
				return err
			}
		}
		if err = r.Q.QuoteAttachTerms(ctx, db.QuoteAttachTermsParams{OrganizationID: org, PaymentTermsDays: q.PaymentTermsDays}); err != nil {
			return err
		}
		var initialStart, initialEnd pgtype.Timestamptz
		if newSubscription {
			initialStart = sub.PeriodStart
			initialEnd = sub.PeriodEnd
		}
		out, err = r.Q.QuoteConvert(ctx, db.QuoteConvertParams{OrganizationID: org, Uuid: id, SubscriptionID: repository.Int(&sub.ID), RequiresInitialInvoice: newSubscription, InitialPeriodStart: initialStart, InitialPeriodEnd: initialEnd})
		if err != nil {
			return err
		}
		return r.Emit(ctx, events.New("enterprise.quote_converted").WithOrganization(org).WithActor(actor).WithEntity("enterprise_quote", &q.ID, &q.Uuid).WithPayload(map[string]any{"subscription_uuid": sub.Uuid}))
	})
	if err != nil {
		return out, err
	}

	// Only a newly created subscription needs an initial invoice. Scheduled plan changes
	// never create another invoice when a conversion is retried after their renewal.
	if !out.RequiresInitialInvoice || out.InitialInvoiceID.Valid || s.Billing == nil {
		return out, nil
	}
	key := "enterprise:" + out.Uuid.String()
	invoice, lookupErr := s.Repo.Q.BillingInvoiceKey(ctx, db.BillingInvoiceKeyParams{OrganizationID: org, IdempotencyKey: key})
	if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
		return out, lookupErr
	}
	if errors.Is(lookupErr, pgx.ErrNoRows) {
		sub, err := s.Repo.Q.BillingGetSubscription(ctx, org)
		if err != nil {
			return out, err
		}
		if sub.ID != out.SubscriptionID.Int64 {
			return out, supportops.ErrConflict
		}
		var lines []Line
		if err = json.Unmarshal(out.Lines, &lines); err != nil {
			return out, err
		}
		invoiceLines := []billingusecase.InvoiceLineInput{}
		for _, l := range lines {
			invoiceLines = append(invoiceLines, billingusecase.InvoiceLineInput{Description: l.Description, Quantity: l.Quantity, UnitAmountMinor: l.UnitAmount, VATBasisPoints: out.VatBasisPoints})
		}
		invoice, err = s.Billing.GenerateInvoice(ctx, org, billingusecase.InvoiceInput{Kind: "manual", IdempotencyKey: key, Currency: out.Currency, PeriodStart: out.InitialPeriodStart.Time, PeriodEnd: out.InitialPeriodEnd.Time, DueAt: out.InitialPeriodStart.Time.AddDate(0, 0, int(out.PaymentTermsDays)), Lines: invoiceLines})
		if err != nil {
			return out, err
		}
	}
	return s.Repo.Q.QuoteLinkInitialInvoice(ctx, db.QuoteLinkInitialInvoiceParams{OrganizationID: org, Uuid: id, InitialInvoiceID: repository.Int(&invoice.ID)})
}

// PublicView deliberately omits internal organization, plan, actor and decision evidence fields.
func PublicView(q db.EnterpriseQuote) map[string]any {
	return map[string]any{"uuid": q.Uuid, "title": q.Title, "status": q.Status, "currency": q.Currency, "interval": q.Interval, "amount_minor": q.AmountMinor, "vat_basis_points": q.VatBasisPoints, "lines": json.RawMessage(q.Lines), "contract_months": q.ContractMonths, "payment_terms_days": q.PaymentTermsDays, "po_number": q.PoNumber, "terms": q.Terms, "valid_until": q.ValidUntil}
}

// PDF renders Unicode text with automatic page breaks, including the complete agreed terms.
func PDF(q db.EnterpriseQuote) ([]byte, error) {
	var lines []Line
	if err := json.Unmarshal(q.Lines, &lines); err != nil {
		return nil, err
	}
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 20)
	if err := ioengine.RegisterPDFFonts(pdf); err != nil {
		return nil, err
	}
	pdf.SetTitle(q.Title, true)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("DejaVu", "", 8)
		pdf.CellFormat(190, 5, fmt.Sprintf("ClickFix | %s | %d", q.Uuid, pdf.PageNo()), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()
	pdf.SetFont("DejaVu", "B", 16)
	pdf.MultiCell(190, 8, q.Title, "", "L", false)
	pdf.SetFont("DejaVu", "", 10)
	pdf.MultiCell(190, 6, fmt.Sprintf("Teklif / Quote: %s\nDurum / Status: %s\nGeçerlilik / Valid until: %s\nDönem / Interval: %s\nSözleşme / Contract: %d ay / months\nÖdeme / Payment: Net %d\nPO: %s", q.Uuid, q.Status, q.ValidUntil.Time.Format(time.RFC3339), q.Interval, q.ContractMonths, q.PaymentTermsDays, q.PoNumber), "", "L", false)
	pdf.Ln(4)
	money := func(v int64) string { return fmt.Sprintf("%d.%02d %s", v/100, v%100, q.Currency) }
	for i, l := range lines {
		pdf.SetFont("DejaVu", "B", 10)
		pdf.MultiCell(190, 6, fmt.Sprintf("%d. %s", i+1, l.Description), "", "L", false)
		pdf.SetFont("DejaVu", "", 10)
		pdf.MultiCell(190, 6, fmt.Sprintf("%d × %s = %s", l.Quantity, money(l.UnitAmount), money(l.Quantity*l.UnitAmount)), "", "R", false)
		pdf.Ln(2)
	}
	tax := (q.AmountMinor*int64(q.VatBasisPoints) + 5000) / 10000
	pdf.SetFont("DejaVu", "B", 11)
	pdf.MultiCell(190, 7, fmt.Sprintf("Ara toplam / Subtotal: %s\nKDV / VAT (%0.2f%%): %s\nToplam / Total: %s", money(q.AmountMinor), float64(q.VatBasisPoints)/100, money(tax), money(q.AmountMinor+tax)), "", "R", false)
	if q.Terms != "" {
		pdf.Ln(5)
		pdf.SetFont("DejaVu", "B", 11)
		pdf.MultiCell(190, 7, "Koşullar / Terms", "", "L", false)
		pdf.SetFont("DejaVu", "", 10)
		pdf.MultiCell(190, 6, q.Terms, "", "L", false)
	}
	var b bytes.Buffer
	if err := pdf.Output(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// PrivateView includes the approval capability only on authenticated platform routes.
func PrivateView(q db.EnterpriseQuote) map[string]any {
	v := PublicView(q)
	v["share_token"] = q.ShareToken
	v["organization_id"] = q.OrganizationID
	v["subscription_id"] = q.SubscriptionID
	v["grants"] = json.RawMessage(q.Grants)
	return v
}
