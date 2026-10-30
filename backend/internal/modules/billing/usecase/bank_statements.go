package usecase

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/iban"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// BankStatementSource is the extension point for authenticated Open Banking feeds.
// Imports are always explicitly scoped to the organization being reconciled.
type BankStatementSource interface {
	FetchBankStatement(context.Context, uuid.UUID, time.Time, time.Time) ([]BankStatementInput, error)
}

type BankStatementInput struct {
	DestinationIBAN string    `json:"-"`
	ExternalID      string    `json:"external_id"`
	Description     string    `json:"description"`
	Reference       string    `json:"reference"`
	SenderIBAN      string    `json:"sender_iban"`
	AmountMinor     int64     `json:"amount_minor"`
	Currency        string    `json:"currency"`
	BookedAt        time.Time `json:"booked_at"`
}

type BankStatementReview struct {
	TransferUUID uuid.UUID `json:"transfer_uuid"`
	Ignore       bool      `json:"ignore"`
	Reason       string    `json:"reason"`
}

func (in BankStatementInput) valid() bool {
	badText := len(in.ExternalID) == 0 || len(in.ExternalID) > 160 || len(in.Description) > 4000 || len(in.Reference) > 160
	badMoney := in.AmountMinor <= 0 || in.AmountMinor > money.MaxMinor || in.Currency != "TRY"
	badDate := in.BookedAt.IsZero() || in.BookedAt.After(time.Now().Add(24*time.Hour))
	return !badText && !badMoney && !badDate && (in.SenderIBAN == "" || iban.Validate(in.SenderIBAN) == nil)
}

// ImportBankStatement atomically persists rows, their decisions and CF-28 settlement.
// A reimport with the same external ID must carry exactly the same financial facts.
func (s *Service) ImportBankStatement(ctx context.Context, org, actor int64, bankID uuid.UUID, inputs []BankStatementInput) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(inputs))
	if org <= 0 || actor <= 0 || len(inputs) == 0 || len(inputs) > 1000 {
		return ids, model.ErrInvalid
	}
	err := s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		bank, err := r.Q.BillingBankAccount(ctx, bankID)
		if err != nil {
			return err
		}
		for _, in := range inputs {
			in.SenderIBAN = iban.Normalize(in.SenderIBAN)
			if in.DestinationIBAN != "" && in.DestinationIBAN != bank.Iban {
				return model.ErrInvalid
			}
			if !in.valid() {
				return model.ErrInvalid
			}
			row, err := r.Q.BillingInsertBankStatement(ctx, db.BillingInsertBankStatementParams{
				OrganizationID: org, BankAccountID: bank.ID, ExternalID: in.ExternalID, Description: in.Description,
				Reference: in.Reference, SenderIban: in.SenderIBAN, AmountMinor: in.AmountMinor, Currency: in.Currency, BookedAt: repository.TS(in.BookedAt),
			})
			if err != nil {
				return err
			}
			sameFacts := row.AmountMinor == in.AmountMinor && row.Currency == in.Currency && row.BookedAt.Time.Equal(in.BookedAt)
			sameText := row.Description == in.Description && row.Reference == in.Reference && row.SenderIban == in.SenderIBAN
			if !sameFacts || !sameText {
				return model.ErrConflict
			}
			ids = append(ids, row.Uuid)
			if row.Status != "review" {
				continue
			}
			candidates, err := r.Q.BillingBankStatementCandidates(ctx, db.BillingBankStatementCandidatesParams{
				OrganizationID: org, BankAccountID: bank.ID, BookedAt: row.BookedAt,
			})
			if err != nil {
				return err
			}
			best, score, exact := scoreBankCandidates(row, candidates)
			decision := db.BillingResolveBankStatementParams{OrganizationID: org, ID: row.ID, Status: "review", Confidence: score}
			if best.Uuid != uuid.Nil {
				decision.CandidateUuid = pgtype.UUID{Bytes: best.Uuid, Valid: true}
			}
			if exact {
				if err = s.reviewTransfer(ctx, r, org, best.Uuid, TransferReview{Approve: true, Reason: "Exact bank statement match"}, actor); err != nil {
					return err
				}
				decision.Status = "approved"
				decision.TransferID = pgtype.Int8{Int64: best.ID, Valid: true}
				decision.ReviewedBy = pgtype.Int8{Int64: actor, Valid: true}
				decision.Reason = "Exact reference, amount, bank account and date match"
			}
			if err = r.Q.BillingResolveBankStatement(ctx, decision); err != nil {
				return err
			}
			ev := events.New("billing.bank_statement_imported").WithTenant(org).WithActor(actor).
				WithEntity("billing_bank_statement_lines", &row.ID, &row.Uuid).
				WithPayload(map[string]any{"status": decision.Status, "confidence": score})
			ev.EventID = uuid.NewSHA1(row.Uuid, []byte(decision.Status))
			if err = r.Emit(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return []uuid.UUID{}, err
	}
	return ids, nil
}

// Scores are evidence weights, not statistical probabilities. Amount-only
// matches never authorize payment. Ambiguous exact matches always require review.
func scoreBankCandidates(row db.BillingBankStatementLine, candidates []db.BillingBankStatementCandidatesRow) (db.BillingBankStatementCandidatesRow, int32, bool) {
	var best db.BillingBankStatementCandidatesRow
	var score int32
	exactCount := 0
	for _, c := range candidates {
		var current int32 = 10 // account and three-day window constrained by SQL
		amount := c.AmountMinor == row.AmountMinor
		reference := bankReferenceMatches(c.PaymentReference, row.Reference+" "+row.Description)
		ibanMatch := c.SenderIban != "" && c.SenderIban == row.SenderIban
		ibanConflict := c.SenderIban != "" && row.SenderIban != "" && c.SenderIban != row.SenderIban
		if amount {
			current += 30
		}
		if reference {
			current += 50
		}
		if ibanMatch {
			current += 10
		}
		exact := amount && reference && !ibanConflict
		if exact {
			current = 100
			exactCount++
		}
		if ibanConflict {
			current = 0
		}
		if current > score {
			best, score = c, current
		}
	}
	return best, score, score == 100 && exactCount == 1
}

func bankReferenceMatches(reference, description string) bool {
	if reference == "" {
		return false
	}
	tokens := strings.FieldsFunc(strings.ToUpper(description), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' })
	for _, token := range tokens {
		if token == strings.ToUpper(reference) {
			return true
		}
	}
	return false
}

func (s *Service) BankStatementLines(ctx context.Context, org int64, status string, limit, offset int32) ([]db.BillingBankStatementsRow, int64, error) {
	if status != "" && status != "review" && status != "approved" && status != "ignored" {
		return nil, 0, model.ErrInvalid
	}
	rows, err := s.repo.Q.BillingBankStatements(ctx, db.BillingBankStatementsParams{OrganizationID: org, Status: status, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	count, err := s.repo.Q.BillingBankStatementCount(ctx, db.BillingBankStatementCountParams{OrganizationID: org, Status: status})
	return rows, count, err
}

func (s *Service) ReviewBankStatement(ctx context.Context, org, actor int64, id uuid.UUID, in BankStatementReview) error {
	if actor <= 0 || len(strings.TrimSpace(in.Reason)) < 3 || len(in.Reason) > 1000 || (in.Ignore && in.TransferUUID != uuid.Nil) {
		return model.ErrInvalid
	}
	return s.repo.Within(ctx, func(r *repository.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		line, err := r.Q.BillingBankStatement(ctx, db.BillingBankStatementParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if line.Status != "review" {
			return model.ErrConflict
		}
		decision := db.BillingResolveBankStatementParams{OrganizationID: org, ID: line.ID, Status: "ignored", Confidence: line.Confidence, Reason: in.Reason, ReviewedBy: pgtype.Int8{Int64: actor, Valid: true}}
		if !in.Ignore {
			transfer, err := r.Q.BillingTransfer(ctx, db.BillingTransferParams{OrganizationID: org, Uuid: in.TransferUUID})
			if err != nil {
				return err
			}
			if transfer.BankAccountID != line.BankAccountID || transfer.AmountMinor != line.AmountMinor || transfer.Status != "reported" {
				return model.ErrConflict
			}
			if err = s.reviewTransfer(ctx, r, org, in.TransferUUID, TransferReview{Approve: true, Reason: in.Reason}, actor); err != nil {
				return err
			}
			decision.Status = "approved"
			decision.TransferID = pgtype.Int8{Int64: transfer.ID, Valid: true}
			decision.CandidateUuid = pgtype.UUID{Bytes: transfer.Uuid, Valid: true}
		}
		if err = r.Q.BillingResolveBankStatement(ctx, decision); err != nil {
			return err
		}
		return r.Emit(ctx, events.New("billing.bank_statement_reviewed").WithTenant(org).WithActor(actor).
			WithEntity("billing_bank_statement_lines", &line.ID, &id).WithPayload(map[string]any{"status": decision.Status, "reason": in.Reason}))
	})
}
