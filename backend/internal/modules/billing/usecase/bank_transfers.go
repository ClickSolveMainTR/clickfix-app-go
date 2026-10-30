package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/iban"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type BankAccountInput struct {
	UUID        uuid.UUID `json:"uuid"`
	BankName    string    `json:"bank_name"`
	AccountName string    `json:"account_name"`
	IBAN        string    `json:"iban"`
	Active      bool      `json:"active"`
	Branch      string    `json:"branch"`
	Description string    `json:"description"`
}

type BankAccountView struct {
	BankAccountInput
	Currency string `json:"currency"`
}

func bankAccountView(r db.BillingBankAccount) BankAccountView {
	return BankAccountView{BankAccountInput: BankAccountInput{UUID: r.Uuid, BankName: r.BankName, AccountName: r.AccountName, IBAN: r.Iban, Active: r.Active, Branch: r.Branch, Description: r.Description}, Currency: r.Currency}
}
func (s *Service) BankAccounts(ctx context.Context, admin bool, paging ...int32) ([]BankAccountView, error) {
	limit, offset := paymentPaging(paging)

	rows, err := s.repo.Q.BillingBankAccounts(ctx, db.BillingBankAccountsParams{IncludeInactive: admin, RowLimit: limit, RowOffset: offset})
	out := make([]BankAccountView, 0, len(rows))
	for _, row := range rows {
		out = append(out, bankAccountView(row))
	}
	return out, err
}
func (s *Service) SaveBankAccount(ctx context.Context, in BankAccountInput) (BankAccountView, error) {
	var out BankAccountView
	norm := iban.Normalize(in.IBAN)
	if iban.Validate(norm) != nil || !strings.HasPrefix(norm, "TR") || strings.TrimSpace(in.BankName) == "" || strings.TrimSpace(in.AccountName) == "" || len(in.BankName) > 200 || len(in.AccountName) > 200 || len(in.Description) > 1000 || len(in.Branch) > 200 {
		return out, model.ErrInvalid
	}
	if in.UUID == uuid.Nil {
		in.UUID = uuid.New()
	}
	row, err := s.repo.Q.BillingSaveBankAccount(ctx, db.BillingSaveBankAccountParams{Uuid: in.UUID, BankName: in.BankName, AccountName: in.AccountName, Iban: norm, Active: in.Active, Branch: in.Branch, Description: in.Description})
	return bankAccountView(row), err
}

type TransferInput struct {
	InvoiceUUID     uuid.UUID `json:"invoice_uuid"`
	BankAccountUUID uuid.UUID `json:"bank_account_uuid"`
	AttachmentUUID  uuid.UUID `json:"attachment_uuid"`
	AmountMinor     int64     `json:"amount_minor"`
	Note            string    `json:"note"`
	SenderIBAN      string    `json:"sender_iban"`
}

func (s *Service) ReportTransfer(ctx context.Context, org int64, in TransferInput) (uuid.UUID, error) {
	var id uuid.UUID
	in.SenderIBAN = iban.Normalize(in.SenderIBAN)
	if in.SenderIBAN != "" && iban.Validate(in.SenderIBAN) != nil {
		return id, model.ErrInvalid
	}
	if in.AmountMinor <= 0 || in.AmountMinor > 99_999_999_999_999 || len(in.Note) > 1000 {
		return id, model.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		invoice, e := r.Q.BillingInvoice(ctx, db.BillingInvoiceParams{OrganizationID: org, Uuid: in.InvoiceUUID})
		if e != nil {
			return e
		}
		if invoice.Status != "open" || invoice.Kind == "credit_note" {
			return model.ErrConflict
		}
		bank, e := r.Q.BillingBankAccount(ctx, in.BankAccountUUID)
		if e != nil {
			return e
		}
		attachment, e := r.Q.BillingReceiptAttachment(ctx, db.BillingReceiptAttachmentParams{OrganizationID: org, Uuid: in.AttachmentUUID})
		if e != nil {
			return e
		}
		if attachment.OwnerID.Valid && (attachment.OwnerType.String != "billing_invoice" || attachment.OwnerID.Bytes != invoice.Uuid) {
			return model.ErrConflict
		}
		if attachment.ContentType != "application/pdf" && !strings.HasPrefix(attachment.ContentType, "image/") {
			return model.ErrInvalid
		}
		transfer, e := r.Q.BillingNewTransfer(ctx, db.BillingNewTransferParams{OrganizationID: org, InvoiceID: invoice.ID, BankAccountID: bank.ID, AmountMinor: in.AmountMinor, ReceiptObjectKey: attachment.ObjectKey, AttachmentUuid: puuid(in.AttachmentUUID.String()), Note: in.Note})
		if e != nil {
			return e
		}
		id = transfer.Uuid
		if e = r.Q.BillingSetTransferSenderIBAN(ctx, db.BillingSetTransferSenderIBANParams{OrganizationID: org, Uuid: id, SenderIban: in.SenderIBAN}); e != nil {
			return e
		}
		if e = r.Q.BillingLinkReceipt(ctx, db.BillingLinkReceiptParams{OrganizationID: org, Uuid: attachment.Uuid, OwnerID: puuid(invoice.Uuid.String())}); e != nil {
			return e
		}
		return r.Emit(ctx, events.New("billing.transfer_reported").WithTenant(org).WithEntity("billing_bank_transfers", &transfer.ID, &id).WithPayload(map[string]any{"invoice_uuid": invoice.Uuid, "reference": invoice.PaymentReference}))
	})
	return id, err
}
func (s *Service) Transfers(ctx context.Context, org int64, paging ...int32) ([]db.BillingTransfersRow, error) {
	limit, offset := paymentPaging(paging)

	return s.repo.Q.BillingTransfers(ctx, db.BillingTransfersParams{OrganizationID: org, RowLimit: limit, RowOffset: offset})
}

type TransferReview struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

func (s *Service) ReviewTransfer(ctx context.Context, org int64, id uuid.UUID, in TransferReview, actor int64) error {
	if actor <= 0 || len(in.Reason) > 1000 || (!in.Approve && strings.TrimSpace(in.Reason) == "") {
		return model.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		return s.reviewTransfer(ctx, r, org, id, in, actor)
	})
}

// reviewTransfer keeps statement matching and CF-28 settlement in one transaction.
func (s *Service) reviewTransfer(ctx context.Context, r *repository.Repository, org int64, id uuid.UUID, in TransferReview, actor int64) error {
	transfer, e := r.Q.BillingTransfer(ctx, db.BillingTransferParams{OrganizationID: org, Uuid: id})
	if e != nil {
		return e
	}
	desired := "rejected"
	if in.Approve {
		desired = "approved"
	}
	if transfer.Status != "reported" {
		if transfer.Status == desired {
			return nil
		}
		return model.ErrConflict
	}
	var paymentID pgtype.Int8
	if in.Approve {
		if !transfer.AttachmentUuid.Valid {
			return model.ErrConflict
		}
		if _, e = r.Q.BillingReceiptAttachment(ctx, db.BillingReceiptAttachmentParams{OrganizationID: org, Uuid: uuid.UUID(transfer.AttachmentUuid.Bytes)}); e != nil {
			return e
		}
		invoice, e := r.Q.BillingInvoiceID(ctx, db.BillingInvoiceIDParams{OrganizationID: org, ID: transfer.InvoiceID})
		if e != nil {
			return e
		}
		// Approvals may exceed the outstanding balance; settlement credits the excess.
		ref := "bank" + strings.ReplaceAll(id.String(), "-", "")
		p, e := r.Q.BillingNewPayment(ctx, db.BillingNewPaymentParams{OrganizationID: org, InvoiceID: invoice.ID, Provider: "bank_transfer", Kind: "bank_transfer", IdempotencyKey: "transfer:" + id.String(), AmountMinor: transfer.AmountMinor, OrderReference: text(ref)})
		if e != nil {
			return e
		}
		paymentID = pgtype.Int8{Int64: p.ID, Valid: true}
		if e = s.settle(ctx, r, p, payments.Notification{OrderRef: ref, Status: payments.StatusSucceeded, Amount: p.AmountMinor, PaidAmount: p.AmountMinor, Currency: "TRY", ProviderPaymentID: ref}, time.Now().UTC()); e != nil {
			return e
		}
	}
	if e = r.Q.BillingReviewTransfer(ctx, db.BillingReviewTransferParams{OrganizationID: org, ID: transfer.ID, Status: desired, ReviewedBy: pgtype.Int8{Int64: actor, Valid: true}, RejectionReason: text(in.Reason), PaymentID: paymentID}); e != nil {
		return e
	}
	return r.Emit(ctx, events.New("billing.transfer_"+desired).WithTenant(org).WithActor(actor).WithEntity("billing_bank_transfers", &transfer.ID, &id).WithPayload(map[string]any{"amount_minor": transfer.AmountMinor, "invoice_id": transfer.InvoiceID, "reason": in.Reason, "audit": fmt.Sprintf("reviewed by %d", actor)}))
}

func (s *Service) TransferReceipt(ctx context.Context, org int64, id uuid.UUID) (uuid.UUID, error) {
	row, err := s.repo.Q.BillingTransfer(ctx, db.BillingTransferParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return uuid.Nil, repository.Normalize(err)
	}
	if !row.AttachmentUuid.Valid {
		return uuid.Nil, model.ErrNotFound
	}
	return uuid.UUID(row.AttachmentUuid.Bytes), nil
}
