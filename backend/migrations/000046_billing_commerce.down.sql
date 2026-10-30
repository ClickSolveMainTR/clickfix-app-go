DROP TABLE billing_invoice_usage_periods;
DROP TABLE billing_refund_credit_reservations;
ALTER TABLE billing_payments DROP CONSTRAINT billing_payments_provider_check,DROP CONSTRAINT billing_payments_kind_check;
ALTER TABLE billing_payments ADD CHECK(provider IN ('paytr','iyzico','bank_transfer','fake')),ADD CHECK(kind IN ('checkout','recurring','bank_transfer','refund'));
DROP TRIGGER trg_billing_numbered_lines ON billing_invoice_lines;
DROP FUNCTION billing_guard_invoice_line();
ALTER TABLE billing_invoices DROP COLUMN request_digest;
ALTER TABLE billing_payments DROP COLUMN refund_destination,DROP COLUMN refund_cancel;
DELETE FROM notification_templates WHERE code IN ('billing.welcome','billing.trial_ending','billing.payment_received','billing.invoice_issued','billing.payment_failed','billing.card_expiring','billing.renewal_reminder','billing.transfer_pending','billing.transfer_approved','billing.transfer_rejected','billing.quota_warning','billing.quota_exhausted','billing.suspended','billing.canceled','billing.payment_required','billing.reconciliation_mismatch','billing.refund_succeeded');
-- Data-loss note: removes commerce documents, FX history, changes and purchase configuration.
DROP TABLE billing_referrals,billing_referral_codes;
DROP TABLE billing_credit_allocations;
DROP TABLE billing_item_changes;
DROP TABLE billing_refund_notes,billing_provider_statement,billing_wallet_settings,billing_credit_purchases,billing_credit_packages,billing_plan_changes;
ALTER TABLE billing_credit_ledger DROP COLUMN expires_at;
ALTER TABLE billing_subscription_items DROP COLUMN pending_quantity;
DROP TABLE billing_subscription_coupons;
ALTER TABLE billing_coupons DROP COLUMN per_org_limit,DROP COLUMN duration_months,DROP COLUMN recurring,DROP COLUMN plan_id,DROP COLUMN interval,DROP COLUMN active;
DROP TRIGGER trg_billing_numbered_invoice ON billing_invoices;
DROP FUNCTION billing_guard_numbered_invoice();
DROP TABLE billing_invoice_documents;
DROP FUNCTION billing_guard_invoice_document();
ALTER TABLE billing_invoices DROP COLUMN original_invoice_id,DROP COLUMN issued_at;
-- Rollback requires new commerce invoice kinds to be archived before this migration.
ALTER TABLE billing_invoices DROP CONSTRAINT billing_invoices_kind_check;
ALTER TABLE billing_invoices ADD CHECK(kind IN ('renewal','upgrade','manual'));
DROP TABLE billing_invoice_sequences,billing_exchange_rates;
ALTER TABLE billing_profiles DROP COLUMN kind,DROP COLUMN country_code,DROP COLUMN emails,DROP COLUMN export_exemption;

ALTER TABLE billing_subscriptions DROP COLUMN contracted_seats;
