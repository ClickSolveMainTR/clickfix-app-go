-- CF-61: channel health. A periodic check (and the reconnect action) stores
-- what the provider says about an instance; webhook traffic is tracked
-- separately from stored messages so a silent webhook is visible.
ALTER TABLE channel_instances
    ADD COLUMN last_webhook_at   TIMESTAMPTZ NULL,
    ADD COLUMN health_ok         BOOLEAN     NULL,
    ADD COLUMN health_detail     TEXT        NOT NULL DEFAULT '',
    ADD COLUMN health_checked_at TIMESTAMPTZ NULL,
    ADD COLUMN token_expires_at  TIMESTAMPTZ NULL;

-- Error rate of an inbox: outbound messages of the last day by status.
CREATE INDEX IF NOT EXISTS idx_messages_instance_out_recent
    ON messages (channel_instance_id, created_at DESC)
    WHERE direction = 'out' AND deleted_at IS NULL;

-- CF-56: e-mail sender domains live in ClickFix's single Resend account;
-- each domain belongs to the organization that registered it first, so no
-- other organization can read its records or send from it.
CREATE TABLE channel_email_domains (
    id               BIGSERIAL   PRIMARY KEY,
    organization_id  BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    domain           TEXT        NOT NULL,
    resend_domain_id TEXT        NOT NULL DEFAULT '',
    created_by       BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT channel_email_domains_domain_uq UNIQUE (domain),
    CONSTRAINT channel_email_domains_domain_chk CHECK (domain = lower(domain))
);
CREATE INDEX idx_channel_email_domains_org ON channel_email_domains (organization_id);
