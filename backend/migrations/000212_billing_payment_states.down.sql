-- Data-loss note: 'expired' checkouts are folded into 'failed' (failure_code kept).
DROP INDEX IF EXISTS idx_billing_payments_unresolved_checkout;
DROP TRIGGER IF EXISTS trg_billing_payment_transition ON billing_payments;
DROP FUNCTION IF EXISTS billing_payment_transition_guard();
UPDATE billing_payments SET status='failed', failure_code=COALESCE(failure_code,'checkout_expired') WHERE status='expired';
ALTER TABLE billing_payments DROP CONSTRAINT IF EXISTS billing_payments_expired_kind_check;
ALTER TABLE billing_payments DROP CONSTRAINT billing_payments_status_check;
ALTER TABLE billing_payments ADD CONSTRAINT billing_payments_status_check
    CHECK (status IN ('pending','unknown','succeeded','failed'));
