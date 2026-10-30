package billing_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/google/uuid"
)

func statementFixture(t *testing.T, f *billingFixture, amount int64) (uuid.UUID, uuid.UUID, string) {
	t.Helper()
	invoice := invoiceFixture(t, f, "open", f.sub.PeriodEnd)
	bank, err := f.svc.SaveBankAccount(f.ctx, usecase.BankAccountInput{BankName: "Statement test", AccountName: "Seller", IBAN: "TR330006100519786457841326", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	var attachment uuid.UUID
	if err = f.pool.QueryRow(f.ctx, "INSERT INTO attachments(organization_id,filename,content_type,size_bytes,sha256,object_key) VALUES($1,'receipt.pdf','application/pdf',1,'test',$2) RETURNING uuid", f.org, "test/"+uuid.NewString()).Scan(&attachment); err != nil {
		t.Fatal(err)
	}
	transfer, err := f.svc.ReportTransfer(f.ctx, f.org, usecase.TransferInput{InvoiceUUID: invoice.Uuid, BankAccountUUID: bank.UUID, AttachmentUUID: attachment, AmountMinor: amount})
	if err != nil {
		t.Fatal(err)
	}
	return bank.UUID, transfer, invoice.PaymentReference
}

func TestBankStatementSettlementReplayAndIsolation(t *testing.T) {
	f := fixture(t)
	bank, _, reference := statementFixture(t, f, 10000)
	in := usecase.BankStatementInput{ExternalID: "bank-" + uuid.NewString(), Description: "EFT " + reference, AmountMinor: 10000, Currency: "TRY", BookedAt: time.Now().UTC().Truncate(time.Microsecond)}
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := f.svc.ImportBankStatement(f.ctx, f.org, f.actor, bank, []usecase.BankStatementInput{in})
			failures <- err
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, count, err := f.svc.BankStatementLines(f.ctx, f.org, "", 20, 0)
	if err != nil || count != 1 || rows[0].Status != "approved" {
		t.Fatalf("rows=%+v count=%d err=%v", rows, count, err)
	}
	var paid int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM billing_payments WHERE organization_id=$1 AND status='succeeded'", f.org).Scan(&paid); err != nil || paid != 1 {
		t.Fatalf("paid=%d err=%v", paid, err)
	}
	in.AmountMinor++
	if _, err = f.svc.ImportBankStatement(f.ctx, f.org, f.actor, bank, []usecase.BankStatementInput{in}); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("changed replay: %v", err)
	}
	other := fixture(t)
	if err = f.svc.ReviewBankStatement(f.ctx, other.org, other.actor, rows[0].Uuid, usecase.BankStatementReview{Ignore: true, Reason: "foreign row"}); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("cross tenant review: %v", err)
	}
}

func TestBankStatementManualQueueAndAtomicImport(t *testing.T) {
	f := fixture(t)
	bank, transfer, _ := statementFixture(t, f, 10000)
	in := usecase.BankStatementInput{ExternalID: uuid.NewString(), Description: "no reference", AmountMinor: 10000, Currency: "TRY", BookedAt: time.Now().UTC().Truncate(time.Microsecond)}
	invalid := in
	invalid.ExternalID = uuid.NewString()
	invalid.AmountMinor = -1
	if _, err := f.svc.ImportBankStatement(f.ctx, f.org, f.actor, bank, []usecase.BankStatementInput{in, invalid}); err == nil {
		t.Fatal("invalid batch accepted")
	}
	_, count, err := f.svc.BankStatementLines(f.ctx, f.org, "", 20, 0)
	if err != nil || count != 0 {
		t.Fatalf("batch did not roll back: %d %v", count, err)
	}
	ids, err := f.svc.ImportBankStatement(f.ctx, f.org, f.actor, bank, []usecase.BankStatementInput{in})
	if err != nil {
		t.Fatal(err)
	}
	rows, count, err := f.svc.BankStatementLines(f.ctx, f.org, "review", 20, 0)
	if err != nil || count != 1 || rows[0].Confidence >= 100 {
		t.Fatalf("manual queue: %+v %d %v", rows, count, err)
	}
	if err = f.svc.ReviewBankStatement(f.ctx, f.org, f.actor, ids[0], usecase.BankStatementReview{TransferUUID: transfer, Reason: "Verified against bank receipt"}); err != nil {
		t.Fatal(err)
	}
	// A second row cannot settle the same transfer again, even manually.
	in.ExternalID = uuid.NewString()
	ids, err = f.svc.ImportBankStatement(f.ctx, f.org, f.actor, bank, []usecase.BankStatementInput{in})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.ReviewBankStatement(f.ctx, f.org, f.actor, ids[0], usecase.BankStatementReview{TransferUUID: transfer, Reason: "duplicate"}); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("duplicate transfer: %v", err)
	}
	if err = f.svc.ReviewBankStatement(f.ctx, f.org, f.actor, ids[0], usecase.BankStatementReview{Ignore: true, Reason: "Duplicate bank entry"}); err != nil {
		t.Fatal(err)
	}
}
