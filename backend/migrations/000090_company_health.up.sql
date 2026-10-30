-- CF-87: customer health score (0-100) and churn-risk alerts.
-- Settings: per organization signal weights (0-100 each, renormalized over
-- the signals that have data) and the alert threshold. company_health holds
-- the current score per company and the alert state (below_threshold, so one
-- alert per downward crossing); history keeps one row per computation.
CREATE TABLE company_health_settings (
    organization_id BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    enabled         BOOLEAN      NOT NULL DEFAULT TRUE,
    threshold       SMALLINT     NOT NULL DEFAULT 40,
    weights         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT company_health_settings_threshold_chk CHECK (threshold BETWEEN 0 AND 100)
);

CREATE TRIGGER trg_company_health_settings_set_updated_at
    BEFORE UPDATE ON company_health_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE company_health (
    id                BIGSERIAL    PRIMARY KEY,
    organization_id   BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    company_id        BIGINT       NOT NULL,
    score             SMALLINT     NULL,
    signals           JSONB        NOT NULL DEFAULT '{}'::jsonb,
    below_threshold   BOOLEAN      NOT NULL DEFAULT FALSE,
    alerted_at        TIMESTAMPTZ  NULL,
    usage_score       SMALLINT     NULL,
    usage_reported_at TIMESTAMPTZ  NULL,
    computed_at       TIMESTAMPTZ  NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT company_health_company_uq UNIQUE (organization_id, company_id),
    CONSTRAINT company_health_company_fk FOREIGN KEY (organization_id, company_id)
        REFERENCES companies (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT company_health_score_chk CHECK (score IS NULL OR score BETWEEN 0 AND 100),
    CONSTRAINT company_health_usage_chk CHECK (usage_score IS NULL OR usage_score BETWEEN 0 AND 100)
);

CREATE INDEX idx_company_health_org_score ON company_health (organization_id, score);

CREATE TRIGGER trg_company_health_set_updated_at
    BEFORE UPDATE ON company_health FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE company_health_history (
    id              BIGSERIAL    PRIMARY KEY,
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    company_id      BIGINT       NOT NULL,
    score           SMALLINT     NOT NULL,
    signals         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT company_health_history_company_fk FOREIGN KEY (organization_id, company_id)
        REFERENCES companies (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_company_health_history_company
    ON company_health_history (organization_id, company_id, created_at DESC);

-- Notification templates of the churn-risk alert (preference matrix row
-- company.health_dropped).
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('company.health_dropped', 'inapp', 'tr', 'Kayıp riski: {{company}}', '{{company}} sağlık skoru {{score}} oldu (eşik {{threshold}}).', TRUE),
    ('company.health_dropped', 'inapp', 'en', 'Churn risk: {{company}}', 'Health score of {{company}} dropped to {{score}} (threshold {{threshold}}).', TRUE),
    ('company.health_dropped', 'email', 'tr', 'Kayıp riski: {{company}}', E'Merhaba {{name}},\n\nSorumlu olduğunuz {{company}} hesabının sağlık skoru {{score}} oldu (eşik {{threshold}}, önceki {{previous}}).\n\n{{action_url}}', TRUE),
    ('company.health_dropped', 'email', 'en', 'Churn risk: {{company}}', E'Hello {{name}},\n\nThe health score of {{company}}, an account you own, dropped to {{score}} (threshold {{threshold}}, previous {{previous}}).\n\n{{action_url}}', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
