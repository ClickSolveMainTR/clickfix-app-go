-- CF-47: saved inbox views. filters is the inbox filter set (statuses,
-- priorities, channel types, inboxes, assignee, tags, unread, mentioned,
-- sla_breached, sla_due_within_minutes, q, sort) applied by the list API.
CREATE TABLE conversation_views (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    scope           TEXT         NOT NULL DEFAULT 'personal',
    owner_user_id   BIGINT       NULL REFERENCES users (id) ON DELETE CASCADE,
    name            TEXT         NOT NULL,
    icon            TEXT         NOT NULL DEFAULT '',
    filters         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    position        INTEGER      NOT NULL DEFAULT 0,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT conversation_views_uuid_uq UNIQUE (uuid),
    CONSTRAINT conversation_views_scope_chk CHECK (scope IN ('personal', 'team', 'organization')),
    CONSTRAINT conversation_views_owner_chk CHECK ((scope = 'personal') = (owner_user_id IS NOT NULL)),
    CONSTRAINT conversation_views_name_chk CHECK (char_length(name) BETWEEN 1 AND 60),
    CONSTRAINT conversation_views_filters_chk CHECK (jsonb_typeof(filters) = 'object')
);

CREATE INDEX idx_conversation_views_org ON conversation_views (organization_id, position);

CREATE TRIGGER trg_conversation_views_set_updated_at
    BEFORE UPDATE ON conversation_views FOR EACH ROW EXECUTE FUNCTION set_updated_at();
