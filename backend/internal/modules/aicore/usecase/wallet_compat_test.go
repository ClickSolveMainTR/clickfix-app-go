package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
)

// Exercise the exact optional schema contract of CF-37 without importing the
// unmerged commerce migration or permanently changing the worker database.
func TestCF37WalletExpiryAllocationAndRestoration(t *testing.T) {
	svc, _, org, _ := aiFixture(t)
	ctx := context.Background()
	tx, err := svc.Repo.Billing.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT to_regclass('public.billing_credit_allocations') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		if _, err = tx.Exec(ctx, `ALTER TABLE billing_credit_ledger ADD COLUMN expires_at timestamptz;
   CREATE TABLE billing_credit_allocations(organization_id BIGINT NOT NULL REFERENCES organizations(id),debit_id BIGINT NOT NULL,credit_id BIGINT NOT NULL,amount BIGINT NOT NULL CHECK(amount>0),PRIMARY KEY(debit_id,credit_id),FOREIGN KEY(organization_id,debit_id) REFERENCES billing_credit_ledger(organization_id,id),FOREIGN KEY(organization_id,credit_id) REFERENCES billing_credit_ledger(organization_id,id));
   CREATE TRIGGER trg_billing_allocations_immutable BEFORE UPDATE OR DELETE ON billing_credit_allocations FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();`); err != nil {
			t.Fatal(err)
		}
	}
	q := db.New(tx)
	// The fixture's original 1000-credit grant has no expiry. Consume it first
	// through the adapter so every subsequent balance is tied to the added lots.
	first := uuid.New()
	funded, err := q.AIWalletReserve(ctx, db.AIWalletReserveParams{OrganizationID: org, RunUuid: first, Credits: 1000})
	if err != nil || !funded {
		t.Fatalf("initial %v %v", funded, err)
	}
	expiry := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	var grant, subscription int64
	if err = tx.QueryRow(ctx, "SELECT id FROM billing_subscriptions WHERE organization_id=$1 ORDER BY id DESC LIMIT 1", org).Scan(&subscription); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `INSERT INTO billing_credit_ledger(organization_id,subscription_id,unit,delta,reason,idempotency_key,expires_at) VALUES($1,$2,'ai_credits',20,'Purchased test','purchase:fake',$3) RETURNING id`, org, subscription, expiry).Scan(&grant); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO billing_credit_ledger(organization_id,subscription_id,unit,delta,reason,idempotency_key,expires_at) VALUES($1,$2,'ai_credits',50,'Expired test','expired',now()-interval '1 hour')`, org, subscription); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	funded, err = q.AIWalletReserve(ctx, db.AIWalletReserveParams{OrganizationID: org, RunUuid: id, Credits: 21})
	if err != nil || funded {
		t.Fatalf("expired credits spendable %v %v", funded, err)
	}
	funded, err = q.AIWalletReserve(ctx, db.AIWalletReserveParams{OrganizationID: org, RunUuid: id, Credits: 15})
	if err != nil || !funded {
		t.Fatalf("reserve %v %v", funded, err)
	}
	balance, err := q.AICreditBalance(ctx, org)
	if err != nil || balance != 5 {
		t.Fatalf("balance=%d %v", balance, err)
	}
	var allocated int64
	if err = tx.QueryRow(ctx, "SELECT sum(amount)::bigint FROM billing_credit_allocations WHERE organization_id=$1 AND credit_id=$2", org, grant).Scan(&allocated); err != nil || allocated != 15 {
		t.Fatalf("allocation %d %v", allocated, err)
	}
	if err = q.AIWalletRelease(ctx, db.AIWalletReleaseParams{OrganizationID: org, RunUuid: id, Credits: 10}); err != nil {
		t.Fatal(err)
	}
	if err = q.AIWalletRelease(ctx, db.AIWalletReleaseParams{OrganizationID: org, RunUuid: id, Credits: 10}); err != nil {
		t.Fatal(err)
	}
	balance, err = q.AICreditBalance(ctx, org)
	if err != nil || balance != 15 {
		t.Fatalf("release/replay balance=%d %v", balance, err)
	}
	var restoredExpiry time.Time
	var amount int64
	if err = tx.QueryRow(ctx, "SELECT expires_at,delta FROM billing_credit_ledger WHERE organization_id=$1 AND idempotency_key LIKE 'refund-restore:ai:%'", org).Scan(&restoredExpiry, &amount); err != nil || amount != 10 || !restoredExpiry.Equal(expiry) {
		t.Fatalf("expiry/lineage %v %d %v", restoredExpiry, amount, err)
	}
	var available int64
	if err = tx.QueryRow(ctx, `SELECT sum(l.delta-COALESCE((SELECT sum(a.amount) FROM billing_credit_allocations a WHERE a.organization_id=l.organization_id AND a.credit_id=l.id),0))::bigint FROM billing_credit_ledger l WHERE l.organization_id=$1 AND l.unit='ai_credits' AND l.delta>0 AND (l.expires_at IS NULL OR l.expires_at>now()) AND NOT EXISTS(SELECT 1 FROM billing_credit_ledger r WHERE r.organization_id=l.organization_id AND r.reverses_id=l.id)`, org).Scan(&available); err != nil || available != balance {
		t.Fatalf("wallet lot drift available=%d balance=%d err=%v", available, balance, err)
	}
}
