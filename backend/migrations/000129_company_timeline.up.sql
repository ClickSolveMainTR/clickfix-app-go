-- CF-85: account screen and unified timeline.
-- company_notes: account notes the team writes on a company.
-- company_events: in-product events the customer's systems send through the
-- API (signed up, invited a user, hit a limit, ...). idempotency_key makes
-- webhook retries safe.
CREATE TABLE company_notes (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    company_id      BIGINT       NOT NULL,
    author_user_id  BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    body            TEXT         NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT company_notes_uuid_uq UNIQUE (uuid),
    CONSTRAINT company_notes_body_ck CHECK (length(body) BETWEEN 1 AND 10000),
    CONSTRAINT company_notes_company_fk FOREIGN KEY (organization_id, company_id)
        REFERENCES companies (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_company_notes_company ON company_notes (organization_id, company_id, created_at DESC);

CREATE TRIGGER trg_company_notes_set_updated_at
    BEFORE UPDATE ON company_notes FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE company_events (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    company_id      BIGINT       NOT NULL,
    contact_id      BIGINT       NULL,
    name            TEXT         NOT NULL,
    properties      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    source          TEXT         NOT NULL DEFAULT 'api',
    idempotency_key TEXT         NULL,
    occurred_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT company_events_uuid_uq UNIQUE (uuid),
    CONSTRAINT company_events_name_ck CHECK (length(name) BETWEEN 1 AND 100),
    CONSTRAINT company_events_company_fk FOREIGN KEY (organization_id, company_id)
        REFERENCES companies (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT company_events_contact_fk FOREIGN KEY (organization_id, contact_id)
        REFERENCES contacts (organization_id, id) ON DELETE SET NULL (contact_id)
);

CREATE INDEX idx_company_events_company ON company_events (organization_id, company_id, occurred_at DESC);
CREATE UNIQUE INDEX company_events_idempotency_uq ON company_events (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
