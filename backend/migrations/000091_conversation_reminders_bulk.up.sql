-- CF-51: reminders and bulk operations on conversations.
-- A reminder fires at due_at: it reopens the conversation and notifies the
-- user. condition 'no_reply' cancels it when the contact wrote after the
-- reminder was set ("3 gün yanıt gelmezse tekrar aç ve bana bildir").
CREATE TABLE conversation_reminders (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL,
    conversation_id BIGINT       NOT NULL,
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    due_at          TIMESTAMPTZ  NOT NULL,
    condition       TEXT         NOT NULL DEFAULT 'always',
    note            TEXT         NOT NULL DEFAULT '',
    status          TEXT         NOT NULL DEFAULT 'pending',
    fired_at        TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT conversation_reminders_uuid_uq UNIQUE (uuid),
    CONSTRAINT conversation_reminders_conversation_fk FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_reminders_condition_chk CHECK (condition IN ('always', 'no_reply')),
    CONSTRAINT conversation_reminders_status_chk CHECK (status IN ('pending', 'fired', 'cancelled')),
    CONSTRAINT conversation_reminders_note_chk CHECK (char_length(note) <= 500)
);

CREATE INDEX idx_conversation_reminders_due ON conversation_reminders (due_at) WHERE status = 'pending';
CREATE INDEX idx_conversation_reminders_conversation ON conversation_reminders (organization_id, conversation_id);

-- Bulk operations keep each conversation's previous state for 24 hours so
-- the whole operation can be undone.
CREATE TABLE conversation_bulk_ops (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    actor_id        BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    action          TEXT         NOT NULL,
    value           TEXT         NOT NULL DEFAULT '',
    items           JSONB        NOT NULL DEFAULT '[]'::jsonb,
    applied         INTEGER      NOT NULL DEFAULT 0,
    failed          INTEGER      NOT NULL DEFAULT 0,
    reverted_at     TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT conversation_bulk_ops_uuid_uq UNIQUE (uuid)
);

CREATE INDEX idx_conversation_bulk_ops_org ON conversation_bulk_ops (organization_id, created_at DESC);
