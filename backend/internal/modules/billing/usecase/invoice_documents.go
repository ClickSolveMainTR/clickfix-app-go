package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/ubl"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func ublParty(in ProfileInput) ubl.Party {
	p := ubl.Party{IdentityType: ubl.IdentityVKN, Identity: in.TaxID, Name: in.LegalName, TaxOffice: in.TaxOffice, Street: in.Address.Street, District: in.Address.District, City: in.Address.City, PostalCode: in.Address.PostalCode, CountryCode: in.CountryCode, Country: in.CountryCode}
	if len(in.Emails) > 0 {
		p.Email = in.Emails[0]
	}
	if in.Kind == "individual" {
		p.IdentityType = ubl.IdentityTCKN
		p.FirstName = in.LegalName
		p.LastName = "-"
	}
	if in.CountryCode != "TR" {
		p.IdentityType = ubl.IdentityTCKN
		p.Identity = ubl.AnonymousTCKN
	}
	return p
}

func (s *Service) InvoiceDetail(ctx context.Context, org int64, id uuid.UUID) (db.BillingInvoice, []db.BillingInvoiceLine, error) {
	i, err := s.repo.Q.BillingInvoice(ctx, db.BillingInvoiceParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return i, nil, err
	}
	lines, err := s.repo.Q.BillingInvoiceLines(ctx, db.BillingInvoiceLinesParams{OrganizationID: org, InvoiceID: i.ID})
	return i, lines, err
}

func (s *Service) PrepareInvoice(ctx context.Context, org int64, id uuid.UUID) (db.BillingInvoiceDocument, error) {
	var out db.BillingInvoiceDocument
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		i, e := r.Q.BillingInvoice(ctx, db.BillingInvoiceParams{OrganizationID: org, Uuid: id})
		if e != nil {
			return e
		}
		out, e = r.Q.BillingDocument(ctx, db.BillingDocumentParams{OrganizationID: org, InvoiceID: i.ID})
		if e == nil {
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if s.einvoice == nil {
			return model.ErrEInvoiceNotConfigured
		}
		if i.Status != "open" && i.Status != "paid" {
			return model.ErrConflict
		}
		var buyer, seller ProfileInput
		if e = json.Unmarshal(i.BuyerSnapshot, &buyer); e != nil {
			return e
		}
		if e = json.Unmarshal(i.SellerSnapshot, &seller); e != nil {
			return e
		}
		if e = buyer.Validate(); e != nil {
			return e
		}
		if e = seller.Validate(); e != nil {
			return e
		}
		taxable, alias, e := s.einvoice.Taxpayer(ctx, buyer.TaxID)
		if e != nil {
			return e
		}
		profile := ubl.ProfileEArsiv
		if taxable {
			if alias == "" {
				return model.ErrInvalid
			}
			profile = ubl.ProfileTemel
		}
		now := time.Now().In(time.FixedZone("Europe/Istanbul", 3*60*60))
		if !i.Number.Valid {
			series := s.invoiceSeries
			if series == "" {
				series = "CFX"
			}
			n, e := r.Q.BillingNextInvoiceNumber(ctx, db.BillingNextInvoiceNumberParams{Series: series, Year: safeconv.ToInt32(now.Year())})
			if e != nil {
				return e
			}
			i, e = r.Q.BillingNumberInvoice(ctx, db.BillingNumberInvoiceParams{OrganizationID: org, ID: i.ID, Number: text(fmt.Sprintf("%s%d%09d", series, now.Year(), n)), IssuedAt: repository.TS(now)})
			if e != nil {
				return e
			}
		}
		doc := ubl.Document{UUID: i.Uuid.String(), Number: i.Number.String, Profile: profile, Type: ubl.TypeSatis, IssueDate: i.IssuedAt.Time.In(now.Location()).Format("2006-01-02"), IssueTime: i.IssuedAt.Time.In(now.Location()).Format("15:04:05"), Currency: i.Currency, Supplier: ublParty(seller), Customer: ublParty(buyer), Notes: []string{"Payment reference: " + i.PaymentReference, "Exchange rate source: " + i.FxSource}}
		if buyer.ExportExemption {
			doc.Type = "ISTISNA"
		}
		if i.Kind == "credit_note" {
			doc.Type = ubl.TypeIade
			original, e := r.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: org, ID: i.OriginalInvoiceID.Int64})
			if e != nil {
				return e
			}
			if !original.Number.Valid || !original.IssuedAt.Valid {
				return model.ErrConflict
			}
			doc.Returns = &ubl.Reference{Number: original.Number.String, IssueDate: original.IssuedAt.Time.In(now.Location()).Format("2006-01-02")}
		}
		lines, e := r.Q.BillingInvoiceLines(ctx, db.BillingInvoiceLinesParams{OrganizationID: org, InvoiceID: i.ID})
		if e != nil {
			return e
		}
		dataset := ioengine.Dataset{Columns: []ioengine.Column{{Key: "description", LabelKey: "billing.description"}, {Key: "net", LabelKey: "billing.net"}, {Key: "tax", LabelKey: "billing.tax"}, {Key: "total", LabelKey: "billing.total"}}}
		for _, l := range lines {
			ul := ubl.Line{Name: l.Description, Net: money.New(l.SubtotalMinor - l.DiscountMinor), VAT: money.New(l.TaxMinor), Rate: money.Rate(l.VatBasisPoints)}
			if buyer.ExportExemption {
				ul.ExemptionCode = "302"
				ul.ExemptionReason = "Hizmet ihracatı"
			} else if l.VatBasisPoints == 0 {
				ul.ExemptionCode = "351"
				ul.ExemptionReason = "KDV istisna olmayan diğer"
			}
			doc.Lines = append(doc.Lines, ul)
			dataset.Rows = append(dataset.Rows, map[string]any{"description": l.Description, "net": ul.Net.String(), "tax": ul.VAT.String(), "total": money.New(l.TotalMinor).String()})
		}
		xml, totals, problems, e := ubl.Validate(ctx, doc)
		if e == nil && len(problems) > 0 {
			return fmt.Errorf("%w: UBL validation: %v", model.ErrInvalid, problems)
		}
		if e != nil {
			return e
		}
		if totals.Payable.Minor() != i.TotalMinor {
			return ubl.ErrTotalsMismatch
		}
		html, e := ubl.HTML(ctx, xml)
		if e != nil {
			return e
		}
		dataset.Rows = append(dataset.Rows, map[string]any{"description": "TOTAL TRY", "net": money.New(i.SubtotalMinor - i.DiscountMinor).String(), "tax": money.New(i.TaxMinor).String(), "total": money.New(i.TotalMinor).String()})
		pdf, e := ioengine.EncodePDF(dataset, "tr", &ioengine.Letterhead{CompanyName: seller.LegalName, Address: seller.Address.Street, Email: seller.Emails[0], FooterText: buyer.LegalName + " | " + buyer.TaxID}, i.Number.String)
		if e != nil {
			return e
		}
		if e = r.Q.BillingSaveDocument(ctx, db.BillingSaveDocumentParams{OrganizationID: org, InvoiceID: i.ID, Profile: profile, Xml: xml, Pdf: pdf, Html: html, Provider: s.einvoice.Code()}); e != nil {
			return e
		}
		out, e = r.Q.BillingDocument(ctx, db.BillingDocumentParams{OrganizationID: org, InvoiceID: i.ID})
		return e
	})
	return out, err
}

func (s *Service) IssueInvoice(ctx context.Context, org int64, id uuid.UUID) error {
	if s.einvoice == nil {
		return model.ErrEInvoiceNotConfigured
	}
	i, err := s.repo.Q.BillingInvoice(ctx, db.BillingInvoiceParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return err
	}
	if i.Status != "paid" && i.Kind != "credit_note" {
		return model.ErrConflict
	}
	doc, err := s.PrepareInvoice(ctx, org, id)
	if err != nil {
		return err
	}
	if doc.State == "issued" || doc.State == "void" {
		return nil
	}
	i, err = s.repo.Q.BillingInvoice(ctx, db.BillingInvoiceParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return err
	}
	if doc.Provider != s.einvoice.Code() {
		return model.ErrConflict
	}
	var buyer ProfileInput
	if err = json.Unmarshal(i.BuyerSnapshot, &buyer); err != nil {
		return err
	}
	_, alias, err := s.einvoice.Taxpayer(ctx, buyer.TaxID)
	if err != nil {
		return err
	}
	var status EInvoiceStatus
	if doc.State == "submitted" {
		status, err = s.einvoice.Status(ctx, i.Uuid.String())
	}
	if doc.State != "submitted" || errors.Is(err, ErrEInvoiceNotFound) {
		status, err = s.einvoice.Send(ctx, EInvoiceEnvelope{UUID: i.Uuid.String(), Number: i.Number.String, Profile: doc.Profile, ReceiverAlias: alias, XML: doc.Xml})
	}
	if err != nil {
		_ = s.repo.Q.BillingDocumentState(ctx, db.BillingDocumentStateParams{OrganizationID: org, InvoiceID: i.ID, State: "submitted", LastError: "provider response unavailable; retry same UUID"})
		return err
	}
	if status.State != "issued" && status.State != "submitted" && status.State != "failed" {
		return model.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		if e := r.Q.BillingDocumentState(ctx, db.BillingDocumentStateParams{OrganizationID: org, InvoiceID: i.ID, State: status.State, ProviderReference: status.Reference}); e != nil {
			return e
		}
		state := "pending"
		if status.State == "issued" || status.State == "failed" {
			state = status.State
		}
		if e := r.Q.BillingEInvoiceState(ctx, db.BillingEInvoiceStateParams{OrganizationID: org, ID: i.ID, EinvoiceStatus: state, EinvoiceReference: text(status.Reference)}); e != nil {
			return e
		}
		if state != "issued" {
			return nil
		}
		event := events.New("billing.invoice_issued").WithTenant(org).WithEntity("billing_invoices", &i.ID, &i.Uuid).WithPayload(map[string]any{"invoice_uuid": i.Uuid, "pdf_path": "/api/v1/tenant/billing/invoices/" + i.Uuid.String() + "/pdf", "invoice_url": s.paymentFrontend + "/billing", "billing_emails": buyer.Emails})
		event.EventID = uuid.NewSHA1(i.Uuid, []byte("invoice_issued"))
		return emitPaymentNotice(ctx, r, event)
	})
}

func (s *Service) InvoiceSweep(ctx context.Context) error {
	if s.einvoice == nil {
		return nil
	}
	var failures []error
	after := int64(0)
	for {
		invoices, err := s.repo.Q.BillingPaidDocuments(ctx, after)
		if err != nil {
			return err
		}
		if len(invoices) == 0 {
			break
		}
		for _, i := range invoices {
			after = i.ID
			if err = s.IssueInvoice(ctx, i.OrganizationID, i.Uuid); err != nil {
				failures = append(failures, fmt.Errorf("issue invoice %s: %w", i.Uuid, err))
			}
		}
	}
	return errors.Join(failures...)
}
