-- CF-6: domain event backbone.
-- outbox_events gains the owning organization and a schema version; the row
-- uuid is the event id, so re-enqueueing the same event is a no-op.
ALTER TABLE outbox_events
    ADD COLUMN organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE SET NULL,
    ADD COLUMN event_version   INT    NOT NULL DEFAULT 1,
    ADD CONSTRAINT chk_outbox_events_version CHECK (event_version >= 1);

CREATE INDEX idx_outbox_events_organization
    ON outbox_events (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;

-- Append-only event log: every published event once, source for analytics,
-- audit replays and the webhook delivery history.
CREATE TABLE domain_event_log (
    id              BIGSERIAL    PRIMARY KEY,
    event_id        UUID         NOT NULL,
    event_name      TEXT         NOT NULL,
    event_version   INT          NOT NULL DEFAULT 1,
    organization_id BIGINT       NULL REFERENCES organizations (id) ON DELETE CASCADE,
    actor_user_id   BIGINT       NULL,
    entity_type     TEXT         NULL,
    entity_id       BIGINT       NULL,
    entity_uuid     UUID         NULL,
    payload         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    occurred_at     TIMESTAMPTZ  NOT NULL,
    recorded_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_domain_event_log_event_id UNIQUE (event_id)
);

CREATE INDEX idx_domain_event_log_org_occurred
    ON domain_event_log (organization_id, occurred_at DESC);
CREATE INDEX idx_domain_event_log_name_occurred
    ON domain_event_log (event_name, occurred_at DESC);
CREATE INDEX idx_domain_event_log_entity
    ON domain_event_log (entity_type, entity_id)
    WHERE entity_id IS NOT NULL;

-- Idempotent consumers: one row per (consumer, event) once the handler
-- committed. Written in the handler's transaction.
CREATE TABLE event_consumptions (
    consumer     TEXT        NOT NULL,
    event_id     UUID        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (consumer, event_id)
);

CREATE INDEX idx_event_consumptions_processed
    ON event_consumptions (processed_at);

-- Dead letters: a consumer that exhausted its retries. Replay re-enqueues it.
CREATE TABLE event_dead_letters (
    id              BIGSERIAL   PRIMARY KEY,
    consumer        TEXT        NOT NULL,
    event_id        UUID        NOT NULL,
    event_name      TEXT        NOT NULL,
    organization_id BIGINT      NULL REFERENCES organizations (id) ON DELETE CASCADE,
    envelope        JSONB       NOT NULL,
    attempts        INT         NOT NULL DEFAULT 0,
    last_error      TEXT        NOT NULL DEFAULT '',
    replayed_at     TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_event_dead_letters_consumer_event UNIQUE (consumer, event_id)
);

CREATE INDEX idx_event_dead_letters_open
    ON event_dead_letters (created_at DESC)
    WHERE replayed_at IS NULL;
CREATE INDEX idx_event_dead_letters_organization
    ON event_dead_letters (organization_id)
    WHERE organization_id IS NOT NULL;

CREATE TRIGGER trg_event_dead_letters_set_updated_at
    BEFORE UPDATE ON event_dead_letters
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
