-- CF-114: sales-assisted quote; a high-entropy token authorizes only the customer decision.
CREATE TABLE enterprise_quotes (
 id BIGSERIAL PRIMARY KEY,uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 base_version_id BIGINT NOT NULL REFERENCES billing_plan_versions(id),
 title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200),
 status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','sent','accepted','rejected','converted','canceled')),
 currency TEXT NOT NULL CHECK(currency IN ('TRY','USD','EUR')),
 interval TEXT NOT NULL CHECK(interval IN ('monthly','yearly')),
 amount_minor BIGINT NOT NULL CHECK(amount_minor>=0),
 vat_basis_points INTEGER NOT NULL DEFAULT 2000 CHECK(vat_basis_points BETWEEN 0 AND 10000),
 lines JSONB NOT NULL CHECK(jsonb_typeof(lines)='array'),
 grants JSONB NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(grants)='array'),
 contract_months INTEGER NOT NULL CHECK(contract_months BETWEEN 1 AND 120),
 payment_terms_days INTEGER NOT NULL CHECK(payment_terms_days IN (0,15,30,60,90)),
 po_number TEXT NOT NULL DEFAULT '' CHECK(length(po_number)<=200),
 terms TEXT NOT NULL DEFAULT '' CHECK(length(terms)<=16000),
 valid_until TIMESTAMPTZ NOT NULL,share_token UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 sent_at TIMESTAMPTZ,decided_at TIMESTAMPTZ,decision_ip TEXT NOT NULL DEFAULT '',
 created_by BIGINT NOT NULL REFERENCES users(id),subscription_id BIGINT,
 requires_initial_invoice BOOLEAN NOT NULL DEFAULT false,
 initial_period_start TIMESTAMPTZ, initial_period_end TIMESTAMPTZ, initial_invoice_id BIGINT,
 FOREIGN KEY(organization_id,initial_invoice_id) REFERENCES billing_invoices(organization_id,id),
 CHECK(NOT requires_initial_invoice OR (initial_period_start IS NOT NULL AND initial_period_end>initial_period_start)),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,subscription_id) REFERENCES billing_subscriptions(organization_id,id)
);
CREATE INDEX idx_enterprise_quotes_org ON enterprise_quotes(organization_id,id DESC);
CREATE TRIGGER trg_enterprise_quotes_updated BEFORE UPDATE ON enterprise_quotes FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- Commercial terms remain attached to the quote after conversion; billing uses a published custom price.
ALTER TABLE billing_profiles ADD COLUMN payment_terms_days INTEGER NOT NULL DEFAULT 0 CHECK(payment_terms_days IN (0,15,30,60,90));
