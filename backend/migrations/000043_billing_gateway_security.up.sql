-- CF-26/27/40. Provider correlation persists before network calls.
ALTER TABLE billing_payments ADD COLUMN order_reference TEXT;
CREATE UNIQUE INDEX uq_billing_payments_order ON billing_payments(provider,order_reference) WHERE order_reference IS NOT NULL;
CREATE TABLE billing_checkout_sessions (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 payment_id BIGINT NOT NULL,
 token TEXT NOT NULL DEFAULT '',
 store_card BOOLEAN NOT NULL DEFAULT false,
 auto_pay_consent BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,payment_id) REFERENCES billing_payments(organization_id,id),
 UNIQUE(payment_id)
);
CREATE INDEX idx_billing_checkout_org ON billing_checkout_sessions(organization_id);
CREATE TRIGGER trg_billing_checkout_updated BEFORE UPDATE ON billing_checkout_sessions FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE billing_webhook_receipts (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 payment_id BIGINT NOT NULL,
 provider TEXT NOT NULL,
 event_key TEXT NOT NULL,
 body_enc TEXT NOT NULL,
 payload_digest TEXT NOT NULL,
 signature TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,payment_id) REFERENCES billing_payments(organization_id,id),
 UNIQUE(payment_id,event_key)
);
CREATE INDEX idx_billing_webhook_org ON billing_webhook_receipts(organization_id,created_at);
CREATE TABLE billing_reconciliation_runs (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 run_date DATE NOT NULL UNIQUE,
 completed_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE billing_reconciliation_mismatches (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 payment_id BIGINT NOT NULL,
 run_id BIGINT NOT NULL REFERENCES billing_reconciliation_runs(id),
 reason TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,payment_id) REFERENCES billing_payments(organization_id,id),
 UNIQUE(run_id,payment_id)
);
CREATE INDEX idx_billing_reconciliation_org ON billing_reconciliation_mismatches(organization_id,created_at);

DROP INDEX uq_billing_payments_unresolved;
CREATE UNIQUE INDEX uq_billing_payments_unresolved ON billing_payments(invoice_id)
 WHERE status IN ('pending','unknown') AND kind NOT IN ('refund','bank_transfer');
