package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TrialInvoice returns the invoice of the first paid period of a trialing
// subscription, creating it when missing (CF-33 / CF-42: "pay now" during the
// trial and the self-serve checkout funnel). The paid period starts when the
// trial ends, so paying early never shortens the trial; settlement activates
// the subscription for [trial end, trial end + interval). It reuses the same
// renewal invoice the trial sweep would create, so the sweep and an early
// payment can never bill twice.
func (s *Service) TrialInvoice(ctx context.Context, org int64, id uuid.UUID) (InvoiceSummary, error) {
	var invoice db.BillingInvoice
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		sub, e := r.Q.BillingLockSubscription(ctx, db.BillingLockSubscriptionParams{OrganizationID: org, Uuid: id})
		if e != nil {
			return e
		}
		if (sub.Status != "trialing" && sub.Status != "pending_payment") || sub.CancelAtPeriodEnd {
			return model.ErrConflict
		}
		invoice, e = r.Q.BillingOpenTrialInvoice(ctx, db.BillingOpenTrialInvoiceParams{OrganizationID: org, SubscriptionID: sub.ID, PeriodStart: sub.PeriodEnd})
		if errors.Is(e, pgx.ErrNoRows) {
			invoice, e = s.periodInvoice(ctx, r, sub, time.Now().UTC())
		}
		return e
	})
	if err != nil {
		return InvoiceSummary{}, err
	}
	return invoiceSummary(invoice), nil
}

func invoiceSummary(r db.BillingInvoice) InvoiceSummary {
	var number *string
	pdfURL, xmlURL := "", ""
	if r.Number.Valid {
		v := r.Number.String
		number = &v
		pdfURL = "/api/v1/tenant/billing/invoices/" + r.Uuid.String() + "/pdf"
		xmlURL = "/api/v1/tenant/billing/invoices/" + r.Uuid.String() + "/xml"
	}
	return InvoiceSummary{Number: number, EInvoiceStatus: r.EinvoiceStatus, PDFURL: pdfURL, XMLURL: xmlURL, UUID: r.Uuid, PaymentReference: r.PaymentReference, Status: r.Status, Currency: r.Currency, TotalMinor: r.TotalMinor, DueAt: r.DueAt.Time, PeriodStart: r.PeriodStart.Time, PeriodEnd: r.PeriodEnd.Time}
}
