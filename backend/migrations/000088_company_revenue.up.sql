-- CF-86: revenue data of the organization's own customers (companies).
-- One current row per company plus an append-only history. Amounts are in
-- minor units (kuruş / cents) like the billing tables. source tells where
-- the last write came from: an agent (manual), the integration API (api) or
-- a payment connector (stripe / iyzico).
CREATE TABLE company_revenue (
    id                   BIGSERIAL    PRIMARY KEY,
    organization_id      BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    company_id           BIGINT       NOT NULL,
    mrr_minor            BIGINT       NOT NULL DEFAULT 0,
    currency             TEXT         NOT NULL DEFAULT 'TRY',
    plan                 TEXT         NOT NULL DEFAULT '',
    billing_status       TEXT         NOT NULL DEFAULT 'none',
    renewal_at           TIMESTAMPTZ  NULL,
    lifetime_value_minor BIGINT       NOT NULL DEFAULT 0,
    source               TEXT         NOT NULL DEFAULT 'manual',
    external_ref         TEXT         NOT NULL DEFAULT '',
    updated_by_user_id   BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT company_revenue_company_uq UNIQUE (organization_id, company_id),
    CONSTRAINT company_revenue_company_fk FOREIGN KEY (organization_id, company_id)
        REFERENCES companies (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT company_revenue_mrr_chk CHECK (mrr_minor >= 0),
    CONSTRAINT company_revenue_ltv_chk CHECK (lifetime_value_minor >= 0),
    CONSTRAINT company_revenue_currency_chk CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT company_revenue_plan_chk CHECK (char_length(plan) <= 120),
    CONSTRAINT company_revenue_status_chk CHECK (billing_status IN ('none', 'trialing', 'active', 'past_due', 'canceled')),
    CONSTRAINT company_revenue_source_chk CHECK (source IN ('manual', 'api', 'stripe', 'iyzico')),
    CONSTRAINT company_revenue_ref_chk CHECK (char_length(external_ref) <= 200)
);

CREATE INDEX idx_company_revenue_org_mrr ON company_revenue (organization_id, mrr_minor DESC);

CREATE TRIGGER trg_company_revenue_set_updated_at
    BEFORE UPDATE ON company_revenue FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE company_revenue_history (
    id                   BIGSERIAL    PRIMARY KEY,
    organization_id      BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    company_id           BIGINT       NOT NULL,
    mrr_minor            BIGINT       NOT NULL,
    currency             TEXT         NOT NULL,
    plan                 TEXT         NOT NULL,
    billing_status       TEXT         NOT NULL,
    renewal_at           TIMESTAMPTZ  NULL,
    lifetime_value_minor BIGINT       NOT NULL,
    source               TEXT         NOT NULL,
    changed_by_user_id   BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT company_revenue_history_company_fk FOREIGN KEY (organization_id, company_id)
        REFERENCES companies (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_company_revenue_history_company
    ON company_revenue_history (organization_id, company_id, created_at DESC);
