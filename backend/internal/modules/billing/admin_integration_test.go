package billing_test

import (
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/google/uuid"
)

func TestAdminSubscriptionListStatsAndFilters(t *testing.T) {
	f := fixture(t)
	slug := "admin-list-" + uuid.NewString()[:8]
	if _, err := f.pool.Exec(f.ctx, "UPDATE organizations SET name=$2, slug=$3 WHERE id=$1", f.org, "Admin Liste "+slug, slug); err != nil {
		t.Fatal(err)
	}

	rows, total, err := f.svc.AdminSubscriptions(f.ctx, usecase.AdminFilter{Search: slug, LiveOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("total=%d rows=%d", total, len(rows))
	}
	row := rows[0]
	if row.Uuid != f.sub.UUID || row.OrganizationUuid != f.orgUUID || row.OrganizationSlug != slug || row.PlanUuid != f.plan.UUID || row.VersionNumber != f.version.Number || row.PriceInterval != "monthly" || row.PriceAmountMinor != 10000 || row.Status != "active" {
		t.Fatalf("unexpected row %+v", row)
	}
	if row.CardLast4.Valid || row.PendingTransfers != 0 {
		t.Fatalf("no card/transfer expected: %+v", row)
	}

	if _, total, err = f.svc.AdminSubscriptions(f.ctx, usecase.AdminFilter{Search: slug, Status: "trialing"}); err != nil || total != 0 {
		t.Fatalf("status filter total=%d err=%v", total, err)
	}
	if _, _, err = f.svc.AdminSubscriptions(f.ctx, usecase.AdminFilter{Status: "bogus"}); err == nil {
		t.Fatal("unknown status must be rejected")
	}

	before, err := f.svc.AdminSubscriptionStats(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.ByStatus["active"] < 1 || before.MRRMinor["TRY"] < 10000 || before.Live < 1 {
		t.Fatalf("stats %+v", before)
	}

	// Canceling removes it from the live list and the MRR.
	if _, err = f.svc.Transition(f.ctx, f.org, f.sub.UUID, model.Transition{To: "canceled", Reason: "admin list test"}, f.actor); err != nil {
		t.Fatal(err)
	}
	if _, total, err = f.svc.AdminSubscriptions(f.ctx, usecase.AdminFilter{Search: slug, LiveOnly: true}); err != nil || total != 0 {
		t.Fatalf("live after cancel total=%d err=%v", total, err)
	}
	if _, total, err = f.svc.AdminSubscriptions(f.ctx, usecase.AdminFilter{Search: slug}); err != nil || total != 1 {
		t.Fatalf("all after cancel total=%d err=%v", total, err)
	}
	// Platform-wide MRR is shared with every other test package running against
	// the same database, so its exact delta is not asserted here; the live-list
	// checks above cover that a canceled subscription stops counting.
	if _, err := f.svc.AdminSubscriptionStats(f.ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAdminPaymentAndTransferQueues(t *testing.T) {
	f := fixture(t)
	invoice := invoiceFixture(t, f, "open", time.Now().UTC().Add(-time.Hour))
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO billing_payments(organization_id,invoice_id,provider,kind,idempotency_key,amount_minor,status,failure_code,completed_at)
		VALUES($1,$2,'paytr','recurring',$3,10000,'failed','card_declined',now())`, f.org, invoice.ID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	rows, total, err := f.svc.AdminPayments(f.ctx, usecase.AdminFilter{Status: "failed", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.InvoiceUuid == invoice.Uuid {
			found = r.OrganizationUuid == f.orgUUID && r.FailureCode.String == "card_declined" && r.Status == "failed"
		}
		if r.Status != "failed" {
			t.Fatalf("status filter leaked %s", r.Status)
		}
	}
	if !found || total < 1 {
		t.Fatalf("failed payment missing total=%d", total)
	}
	if _, _, err = f.svc.AdminPayments(f.ctx, usecase.AdminFilter{Status: "nope"}); err == nil {
		t.Fatal("unknown payment status must be rejected")
	}

	var bank int64
	if err = f.pool.QueryRow(f.ctx, `INSERT INTO billing_bank_accounts(bank_name,account_name,iban,active) VALUES('Test Bankası','ClickFix','TR330006100519786457841326',true) RETURNING id`).Scan(&bank); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO billing_bank_transfers(organization_id,invoice_id,bank_account_id,amount_minor,receipt_object_key,note) VALUES($1,$2,$3,10000,'',$4)`, f.org, invoice.ID, bank, "admin queue test"); err != nil {
		t.Fatal(err)
	}
	queue, total, err := f.svc.AdminTransfers(f.ctx, usecase.AdminFilter{Status: "reported", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, r := range queue {
		if r.InvoiceUuid == invoice.Uuid {
			found = r.OrganizationUuid == f.orgUUID && r.InvoiceTotalMinor == 10000 && r.BankName == "Test Bankası" && !r.HasReceipt
		}
	}
	if !found || total < 1 {
		t.Fatalf("transfer missing from queue total=%d", total)
	}
	if _, _, err = f.svc.AdminTransfers(f.ctx, usecase.AdminFilter{Status: "x"}); err == nil {
		t.Fatal("unknown transfer status must be rejected")
	}
}
