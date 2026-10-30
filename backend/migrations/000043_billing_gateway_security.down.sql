-- Data-loss note: removes gateway correlation, retained webhooks and reconciliation history.
DROP TABLE billing_reconciliation_mismatches,billing_reconciliation_runs,billing_webhook_receipts,billing_checkout_sessions;
DROP INDEX uq_billing_payments_order;
ALTER TABLE billing_payments DROP COLUMN order_reference;

DROP INDEX uq_billing_payments_unresolved;
CREATE UNIQUE INDEX uq_billing_payments_unresolved ON billing_payments(invoice_id)
 WHERE status IN ('pending','unknown') AND kind <> 'refund';
