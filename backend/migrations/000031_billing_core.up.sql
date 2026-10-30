-- CF-18. Adapted from comcode plans/subscriptions/dunning, Takwim billing,
-- and otopoly billing core/invoices. See docs/billing/ARCHITECTURE.md.
-- Monetary *_minor fields are integer minor units, as explicitly required by CF-18.

CREATE TABLE billing_plans (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    code TEXT NOT NULL UNIQUE CHECK (code ~ '^[a-z][a-z0-9_-]*$'),
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    description TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public','hidden','custom')),
    sort_order INTEGER NOT NULL DEFAULT 0,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_billing_plans_updated BEFORE UPDATE ON billing_plans
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_plan_versions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    plan_id BIGINT NOT NULL REFERENCES billing_plans(id),
    version INTEGER NOT NULL CHECK (version > 0),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published')),
    trial_days INTEGER NOT NULL DEFAULT 0 CHECK (trial_days BETWEEN 0 AND 365),
    published_at TIMESTAMPTZ,
    UNIQUE (plan_id, version),
    CHECK ((status = 'published') = (published_at IS NOT NULL)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_billing_plan_versions_updated BEFORE UPDATE ON billing_plan_versions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_features (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    key TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('boolean','limit')),
    label_tr TEXT NOT NULL,
    label_en TEXT NOT NULL,
    unit TEXT NOT NULL DEFAULT '',
    aggregation TEXT NOT NULL DEFAULT 'none' CHECK (aggregation IN ('none','sum','gauge')),
    period TEXT NOT NULL DEFAULT 'none' CHECK (period IN ('none','month')),
    UNIQUE (id, kind),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_billing_features_updated BEFORE UPDATE ON billing_features
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_plan_entitlements (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    plan_version_id BIGINT NOT NULL REFERENCES billing_plan_versions(id),
    feature_id BIGINT NOT NULL,
    kind TEXT NOT NULL,
    value_bool BOOLEAN,
    value_limit BIGINT CHECK (value_limit >= 0),
    unlimited BOOLEAN NOT NULL DEFAULT false,
    enforcement TEXT NOT NULL DEFAULT 'hard' CHECK (enforcement IN ('soft','hard')),
    warn_percent INTEGER NOT NULL DEFAULT 80 CHECK (warn_percent BETWEEN 0 AND 100),
    FOREIGN KEY (feature_id, kind) REFERENCES billing_features(id, kind),
    UNIQUE (plan_version_id, feature_id),
    CHECK ((kind = 'boolean' AND value_bool IS NOT NULL AND value_limit IS NULL AND NOT unlimited)
        OR (kind = 'limit' AND value_bool IS NULL AND ((value_limit IS NOT NULL) <> unlimited))),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_billing_plan_entitlements_updated BEFORE UPDATE ON billing_plan_entitlements
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_prices (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    plan_version_id BIGINT NOT NULL REFERENCES billing_plan_versions(id),
    interval TEXT NOT NULL CHECK (interval IN ('monthly','yearly')),
    currency TEXT NOT NULL CHECK (currency IN ('TRY','USD','EUR')),
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    seat_amount_minor BIGINT NOT NULL DEFAULT 0 CHECK (seat_amount_minor >= 0),
    tax_inclusive BOOLEAN NOT NULL DEFAULT false,
    vat_basis_points INTEGER NOT NULL CHECK (vat_basis_points BETWEEN 0 AND 10000),
    UNIQUE (plan_version_id, interval, currency),
    UNIQUE (plan_version_id, id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_billing_prices_updated BEFORE UPDATE ON billing_prices
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_price_tiers (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    price_id BIGINT NOT NULL REFERENCES billing_prices(id),
    feature_id BIGINT NOT NULL REFERENCES billing_features(id),
    lower_bound BIGINT NOT NULL CHECK (lower_bound >= 0),
    upper_bound BIGINT CHECK (upper_bound > lower_bound),
    unit_amount_minor BIGINT NOT NULL CHECK (unit_amount_minor >= 0),
    UNIQUE (price_id, feature_id, lower_bound),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_billing_price_tiers_updated BEFORE UPDATE ON billing_price_tiers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_addons (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    feature_id BIGINT NOT NULL REFERENCES billing_features(id),
    units BIGINT NOT NULL CHECK (units > 0),
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    currency TEXT NOT NULL CHECK (currency IN ('TRY','USD','EUR')),
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_billing_addons_updated BEFORE UPDATE ON billing_addons
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_profiles (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    legal_name TEXT NOT NULL,
    tax_id TEXT NOT NULL DEFAULT '',
    tax_office TEXT NOT NULL DEFAULT '',
    address JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(address) = 'object'),
    email TEXT NOT NULL DEFAULT '',
    UNIQUE (organization_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_profiles_organization ON billing_profiles (organization_id);

CREATE TRIGGER trg_billing_profiles_updated BEFORE UPDATE ON billing_profiles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_payment_methods (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    provider TEXT NOT NULL CHECK (provider IN ('paytr','iyzico','fake')),
    merchant_account_uuid UUID NOT NULL,
    customer_token_enc TEXT NOT NULL,
    card_token_enc TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    last4 TEXT NOT NULL CHECK (last4 ~ '^[0-9]{4}$'),
    brand TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','removed')),
    removed_at TIMESTAMPTZ,
    CHECK ((status = 'removed') = (removed_at IS NOT NULL)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_payment_methods_organization ON billing_payment_methods (organization_id);

CREATE TRIGGER trg_billing_payment_methods_updated BEFORE UPDATE ON billing_payment_methods
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE UNIQUE INDEX uq_billing_payment_methods_live_token
    ON billing_payment_methods(organization_id, merchant_account_uuid, token_hash) WHERE status = 'active';

CREATE TABLE billing_subscriptions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    plan_version_id BIGINT NOT NULL REFERENCES billing_plan_versions(id),
    price_id BIGINT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('trialing','active','past_due','grace','paused','canceled','expired')),
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL CHECK (period_end > period_start),
    trial_ends_at TIMESTAMPTZ,
    grace_ends_at TIMESTAMPTZ,
    canceled_at TIMESTAMPTZ,
    cancel_at_period_end BOOLEAN NOT NULL DEFAULT false,
    pending_plan_version_id BIGINT REFERENCES billing_plan_versions(id),
    pending_price_id BIGINT,
    payment_method_id BIGINT,
    auto_pay BOOLEAN NOT NULL DEFAULT false,
    auto_pay_consented_at TIMESTAMPTZ,
    FOREIGN KEY (plan_version_id, price_id) REFERENCES billing_prices(plan_version_id, id),
    FOREIGN KEY (pending_plan_version_id, pending_price_id) REFERENCES billing_prices(plan_version_id, id) MATCH FULL,
    FOREIGN KEY (organization_id, payment_method_id) REFERENCES billing_payment_methods(organization_id, id),
    CHECK (NOT auto_pay OR (payment_method_id IS NOT NULL AND auto_pay_consented_at IS NOT NULL)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_subscriptions_organization ON billing_subscriptions (organization_id);

CREATE TRIGGER trg_billing_subscriptions_updated BEFORE UPDATE ON billing_subscriptions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE UNIQUE INDEX uq_billing_subscriptions_live ON billing_subscriptions(organization_id)
    WHERE status IN ('trialing','active','past_due','grace','paused');
CREATE INDEX idx_billing_subscriptions_due ON billing_subscriptions(period_end)
    WHERE status IN ('trialing','active','past_due','grace');

CREATE TABLE billing_subscription_items (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    subscription_id BIGINT NOT NULL,
    addon_id BIGINT NOT NULL REFERENCES billing_addons(id),
    quantity BIGINT NOT NULL CHECK (quantity > 0),
    unit_amount_minor BIGINT NOT NULL CHECK (unit_amount_minor >= 0),
    currency TEXT NOT NULL CHECK (currency IN ('TRY','USD','EUR')),
    FOREIGN KEY (organization_id, subscription_id) REFERENCES billing_subscriptions(organization_id, id),
    UNIQUE (subscription_id, addon_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_subscription_items_organization ON billing_subscription_items (organization_id);

CREATE TRIGGER trg_billing_subscription_items_updated BEFORE UPDATE ON billing_subscription_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_entitlement_overrides (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    feature_id BIGINT NOT NULL,
    kind TEXT NOT NULL,
    value_bool BOOLEAN,
    value_limit BIGINT CHECK (value_limit >= 0),
    unlimited BOOLEAN NOT NULL DEFAULT false,
    reason TEXT NOT NULL CHECK (length(trim(reason)) > 0),
    expires_at TIMESTAMPTZ,
    created_by BIGINT REFERENCES users(id),
    FOREIGN KEY (feature_id, kind) REFERENCES billing_features(id, kind),
    UNIQUE (organization_id, feature_id),
    CHECK ((kind = 'boolean' AND value_bool IS NOT NULL AND value_limit IS NULL AND NOT unlimited)
        OR (kind = 'limit' AND value_bool IS NULL AND ((value_limit IS NOT NULL) <> unlimited))),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_entitlement_overrides_organization ON billing_entitlement_overrides (organization_id);

CREATE TRIGGER trg_billing_entitlement_overrides_updated BEFORE UPDATE ON billing_entitlement_overrides
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_usage_periods (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL CHECK (ends_at > starts_at),
    closed_at TIMESTAMPTZ,
    UNIQUE (organization_id, starts_at),
    UNIQUE (organization_id, id, starts_at, ends_at),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_usage_periods_organization ON billing_usage_periods (organization_id);

CREATE TRIGGER trg_billing_usage_periods_updated BEFORE UPDATE ON billing_usage_periods
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_usage_records (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    usage_period_id BIGINT NOT NULL,
    feature_id BIGINT NOT NULL REFERENCES billing_features(id),
    idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 200),
    quantity BIGINT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (organization_id, usage_period_id) REFERENCES billing_usage_periods(organization_id, id),
    UNIQUE (organization_id, idempotency_key),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_usage_records_organization ON billing_usage_records (organization_id);

CREATE TABLE billing_usage_totals (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    usage_period_id BIGINT NOT NULL,
    feature_id BIGINT NOT NULL REFERENCES billing_features(id),
    quantity BIGINT NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    last_occurred_at TIMESTAMPTZ,
    FOREIGN KEY (organization_id, usage_period_id) REFERENCES billing_usage_periods(organization_id, id),
    UNIQUE (organization_id, usage_period_id, feature_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_usage_totals_organization ON billing_usage_totals (organization_id);

CREATE TRIGGER trg_billing_usage_totals_updated BEFORE UPDATE ON billing_usage_totals
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_coupons (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    code TEXT NOT NULL UNIQUE CHECK (code = upper(code)),
    percent_basis_points INTEGER CHECK (percent_basis_points BETWEEN 1 AND 10000),
    amount_minor BIGINT CHECK (amount_minor > 0),
    currency TEXT CHECK (currency IN ('TRY','USD','EUR')),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ CHECK (ends_at > starts_at),
    max_redemptions BIGINT CHECK (max_redemptions > 0),
    CHECK ((percent_basis_points IS NOT NULL AND amount_minor IS NULL AND currency IS NULL)
        OR (percent_basis_points IS NULL AND amount_minor IS NOT NULL AND currency IS NOT NULL)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_billing_coupons_updated BEFORE UPDATE ON billing_coupons
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_invoices (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    subscription_id BIGINT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL CHECK (period_end > period_start),
    kind TEXT NOT NULL DEFAULT 'renewal' CHECK (kind IN ('renewal','upgrade','manual')),
    idempotency_key TEXT NOT NULL,
    number TEXT UNIQUE,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','open','paid','void','uncollectible')),
    currency TEXT NOT NULL DEFAULT 'TRY' CHECK (currency = 'TRY'),
    source_currency TEXT NOT NULL CHECK (source_currency IN ('TRY','USD','EUR')),
    fx_rate NUMERIC(20,8) NOT NULL CHECK (fx_rate > 0),
    fx_source TEXT NOT NULL,
    fx_observed_at TIMESTAMPTZ NOT NULL,
    buyer_snapshot JSONB NOT NULL CHECK (jsonb_typeof(buyer_snapshot) = 'object'),
    seller_snapshot JSONB NOT NULL CHECK (jsonb_typeof(seller_snapshot) = 'object'),
    subtotal_minor BIGINT NOT NULL CHECK (subtotal_minor >= 0),
    discount_minor BIGINT NOT NULL DEFAULT 0 CHECK (discount_minor >= 0 AND discount_minor <= subtotal_minor),
    tax_minor BIGINT NOT NULL CHECK (tax_minor >= 0),
    total_minor BIGINT NOT NULL CHECK (total_minor >= 0),
    due_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    einvoice_status TEXT NOT NULL DEFAULT 'pending' CHECK (einvoice_status IN ('pending','issued','failed','void')),
    einvoice_reference TEXT,
    FOREIGN KEY (organization_id, subscription_id) REFERENCES billing_subscriptions(organization_id, id),
    UNIQUE (organization_id, idempotency_key),
    CHECK (total_minor = subtotal_minor - discount_minor + tax_minor),
    CHECK (source_currency <> 'TRY' OR fx_rate = 1),
    CHECK ((status = 'paid') = (paid_at IS NOT NULL)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_invoices_organization ON billing_invoices (organization_id);

CREATE TRIGGER trg_billing_invoices_updated BEFORE UPDATE ON billing_invoices
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE UNIQUE INDEX uq_billing_invoices_renewal ON billing_invoices(subscription_id, period_start)
    WHERE kind = 'renewal';

CREATE TABLE billing_invoice_lines (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    invoice_id BIGINT NOT NULL,
    description TEXT NOT NULL,
    quantity BIGINT NOT NULL CHECK (quantity > 0),
    unit_amount_minor BIGINT NOT NULL CHECK (unit_amount_minor >= 0),
    subtotal_minor BIGINT NOT NULL CHECK (subtotal_minor >= 0),
    discount_minor BIGINT NOT NULL DEFAULT 0 CHECK (discount_minor >= 0 AND discount_minor <= subtotal_minor),
    vat_basis_points INTEGER NOT NULL CHECK (vat_basis_points BETWEEN 0 AND 10000),
    tax_minor BIGINT NOT NULL CHECK (tax_minor >= 0),
    total_minor BIGINT NOT NULL CHECK (total_minor >= 0),
    FOREIGN KEY (organization_id, invoice_id) REFERENCES billing_invoices(organization_id, id),
    CHECK (total_minor = subtotal_minor - discount_minor + tax_minor),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_invoice_lines_organization ON billing_invoice_lines (organization_id);

CREATE TRIGGER trg_billing_invoice_lines_updated BEFORE UPDATE ON billing_invoice_lines
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_redemptions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    coupon_id BIGINT NOT NULL REFERENCES billing_coupons(id),
    invoice_id BIGINT NOT NULL,
    discount_minor BIGINT NOT NULL CHECK (discount_minor > 0),
    FOREIGN KEY (organization_id, invoice_id) REFERENCES billing_invoices(organization_id, id),
    UNIQUE (coupon_id, invoice_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_redemptions_organization ON billing_redemptions (organization_id);

CREATE TABLE billing_payments (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    invoice_id BIGINT NOT NULL,
    payment_method_id BIGINT,
    provider TEXT NOT NULL CHECK (provider IN ('paytr','iyzico','bank_transfer','fake')),
    merchant_account_uuid UUID,
    kind TEXT NOT NULL CHECK (kind IN ('checkout','recurring','bank_transfer','refund')),
    original_payment_id BIGINT,
    idempotency_key TEXT NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL DEFAULT 'TRY' CHECK (currency = 'TRY'),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','unknown','succeeded','failed')),
    provider_reference TEXT,
    failure_code TEXT,
    completed_at TIMESTAMPTZ,
    FOREIGN KEY (organization_id, invoice_id) REFERENCES billing_invoices(organization_id, id),
    FOREIGN KEY (organization_id, payment_method_id) REFERENCES billing_payment_methods(organization_id, id),
    FOREIGN KEY (organization_id, original_payment_id) REFERENCES billing_payments(organization_id, id),
    UNIQUE (organization_id, idempotency_key),
    CHECK ((kind = 'refund') = (original_payment_id IS NOT NULL)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_payments_organization ON billing_payments (organization_id);

CREATE TRIGGER trg_billing_payments_updated BEFORE UPDATE ON billing_payments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE UNIQUE INDEX uq_billing_payments_unresolved ON billing_payments(invoice_id)
    WHERE status IN ('pending','unknown') AND kind <> 'refund';
CREATE UNIQUE INDEX uq_billing_payments_provider_ref ON billing_payments(provider, merchant_account_uuid, provider_reference)
    WHERE provider_reference IS NOT NULL;

CREATE TABLE billing_gateway_transactions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    payment_id BIGINT NOT NULL,
    event_key TEXT NOT NULL,
    operation TEXT NOT NULL CHECK (operation IN ('checkout','callback','charge','refund','reconcile','tokenize')),
    verified BOOLEAN NOT NULL DEFAULT false,
    payload_digest TEXT NOT NULL,
    result_code TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (organization_id, payment_id) REFERENCES billing_payments(organization_id, id),
    UNIQUE (payment_id, event_key),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_gateway_transactions_organization ON billing_gateway_transactions (organization_id);

CREATE TABLE billing_bank_accounts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    bank_name TEXT NOT NULL,
    account_name TEXT NOT NULL,
    iban TEXT NOT NULL CHECK (iban ~ '^TR[0-9]{24}$'),
    currency TEXT NOT NULL DEFAULT 'TRY' CHECK (currency = 'TRY'),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_billing_bank_accounts_updated BEFORE UPDATE ON billing_bank_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_bank_transfers (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    invoice_id BIGINT NOT NULL,
    bank_account_id BIGINT NOT NULL REFERENCES billing_bank_accounts(id),
    payment_id BIGINT,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    status TEXT NOT NULL DEFAULT 'reported' CHECK (status IN ('reported','approved','rejected')),
    receipt_object_key TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    reviewed_by BIGINT REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    rejection_reason TEXT,
    FOREIGN KEY (organization_id, invoice_id) REFERENCES billing_invoices(organization_id, id),
    FOREIGN KEY (organization_id, payment_id) REFERENCES billing_payments(organization_id, id),
    CHECK ((status = 'reported' AND reviewed_at IS NULL AND reviewed_by IS NULL)
        OR (status <> 'reported' AND reviewed_at IS NOT NULL AND reviewed_by IS NOT NULL)),
    CHECK (status <> 'approved' OR payment_id IS NOT NULL),
    CHECK (status <> 'rejected' OR (rejection_reason IS NOT NULL AND length(trim(rejection_reason)) > 0)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_bank_transfers_organization ON billing_bank_transfers (organization_id);

CREATE TRIGGER trg_billing_bank_transfers_updated BEFORE UPDATE ON billing_bank_transfers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE billing_dunning (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    subscription_id BIGINT NOT NULL,
    invoice_id BIGINT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    first_failure_at TIMESTAMPTZ NOT NULL,
    next_attempt_at TIMESTAMPTZ,
    grace_ends_at TIMESTAMPTZ NOT NULL,
    lease_until TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    last_error_code TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (organization_id, subscription_id) REFERENCES billing_subscriptions(organization_id, id),
    FOREIGN KEY (organization_id, invoice_id) REFERENCES billing_invoices(organization_id, id),
    UNIQUE (invoice_id),
    CHECK (grace_ends_at > first_failure_at),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_dunning_organization ON billing_dunning (organization_id);

CREATE TRIGGER trg_billing_dunning_updated BEFORE UPDATE ON billing_dunning
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX idx_billing_dunning_due ON billing_dunning(next_attempt_at) WHERE resolved_at IS NULL;

CREATE TABLE billing_credit_ledger (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    subscription_id BIGINT NOT NULL,
    invoice_id BIGINT,
    unit TEXT NOT NULL CHECK (unit IN ('TRY','ai_credits')),
    delta BIGINT NOT NULL CHECK (delta <> 0),
    reason TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    reverses_id BIGINT,
    FOREIGN KEY (organization_id, subscription_id) REFERENCES billing_subscriptions(organization_id, id),
    FOREIGN KEY (organization_id, invoice_id) REFERENCES billing_invoices(organization_id, id),
    FOREIGN KEY (organization_id, reverses_id) REFERENCES billing_credit_ledger(organization_id, id),
    UNIQUE (organization_id, unit, idempotency_key),
    UNIQUE (reverses_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_billing_credit_ledger_organization ON billing_credit_ledger (organization_id);

CREATE FUNCTION billing_guard_version() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE version_id BIGINT; version_status TEXT;
BEGIN
    IF TG_TABLE_NAME = 'billing_plan_versions' THEN
        IF TG_OP <> 'INSERT' AND OLD.status = 'published' THEN
            RAISE EXCEPTION 'BILLING_VERSION_IMMUTABLE' USING ERRCODE = '23514';
        END IF;
    ELSE
        IF TG_OP = 'UPDATE' THEN
            IF TG_TABLE_NAME = 'billing_price_tiers' THEN
                IF NEW.price_id <> OLD.price_id THEN
                    RAISE EXCEPTION 'BILLING_VERSION_REPARENT_FORBIDDEN' USING ERRCODE = '23514';
                END IF;
            ELSIF NEW.plan_version_id <> OLD.plan_version_id THEN
                RAISE EXCEPTION 'BILLING_VERSION_REPARENT_FORBIDDEN' USING ERRCODE = '23514';
            END IF;
        END IF;
        IF TG_TABLE_NAME = 'billing_price_tiers' THEN
            IF TG_OP = 'DELETE' THEN
                SELECT plan_version_id INTO version_id FROM billing_prices WHERE id = OLD.price_id;
            ELSE
                SELECT plan_version_id INTO version_id FROM billing_prices WHERE id = NEW.price_id;
            END IF;
        ELSIF TG_OP = 'DELETE' THEN version_id := OLD.plan_version_id;
        ELSE version_id := NEW.plan_version_id;
        END IF;
        SELECT status INTO version_status FROM billing_plan_versions WHERE id = version_id FOR UPDATE;
        IF version_status = 'published' THEN
            RAISE EXCEPTION 'BILLING_VERSION_IMMUTABLE' USING ERRCODE = '23514';
        END IF;
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_billing_version_guard BEFORE UPDATE OR DELETE ON billing_plan_versions
    FOR EACH ROW EXECUTE FUNCTION billing_guard_version();
CREATE TRIGGER trg_billing_price_guard BEFORE INSERT OR UPDATE OR DELETE ON billing_prices
    FOR EACH ROW EXECUTE FUNCTION billing_guard_version();
CREATE TRIGGER trg_billing_tier_guard BEFORE INSERT OR UPDATE OR DELETE ON billing_price_tiers
    FOR EACH ROW EXECUTE FUNCTION billing_guard_version();
CREATE TRIGGER trg_billing_entitlement_guard BEFORE INSERT OR UPDATE OR DELETE ON billing_plan_entitlements
    FOR EACH ROW EXECUTE FUNCTION billing_guard_version();

-- Audit facts and credit entries are corrected with a compensating insert.
CREATE FUNCTION billing_guard_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'BILLING_RECORD_APPEND_ONLY' USING ERRCODE = '23514';
END;
$$;
CREATE TRIGGER trg_billing_credit_immutable BEFORE UPDATE OR DELETE ON billing_credit_ledger
    FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();
CREATE TRIGGER trg_billing_usage_immutable BEFORE UPDATE OR DELETE ON billing_usage_records
    FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();
CREATE TRIGGER trg_billing_gateway_immutable BEFORE UPDATE OR DELETE ON billing_gateway_transactions
    FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();
CREATE TRIGGER trg_billing_redemption_immutable BEFORE UPDATE OR DELETE ON billing_redemptions
    FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();

INSERT INTO billing_features(key, kind, label_tr, label_en, unit, aggregation, period) VALUES
    ('seats', 'limit', 'Koltuk', 'Seats', 'seat', 'gauge', 'none'),
    ('conversations.monthly', 'limit', 'Aylık görüşme', 'Monthly conversations', 'conversation', 'sum', 'month'),
    ('ai.tokens', 'limit', 'AI token', 'AI tokens', 'token', 'sum', 'month'),
    ('ai.credits', 'limit', 'AI kredi', 'AI credits', 'credit', 'sum', 'month'),
    ('channels', 'limit', 'Kanal', 'Channels', 'channel', 'gauge', 'none'),
    ('storage.bytes', 'limit', 'Depolama', 'Storage', 'byte', 'gauge', 'none'),
    ('ai.enabled', 'boolean', 'AI erişimi', 'AI access', '', 'none', 'none')
ON CONFLICT (key) DO NOTHING;

