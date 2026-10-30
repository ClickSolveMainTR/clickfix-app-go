-- CF-98: merge / split, linked conversations and side conversations.

-- A merged conversation is closed and points at the conversation that
-- absorbed its messages; inbound mail on its thread follows the pointer.
ALTER TABLE conversations
    ADD COLUMN merged_into_id BIGINT      NULL,
    ADD COLUMN merged_at      TIMESTAMPTZ NULL,
    ADD CONSTRAINT conversations_merged_into_fk FOREIGN KEY (organization_id, merged_into_id)
        REFERENCES conversations (organization_id, id) ON DELETE SET NULL (merged_into_id),
    ADD CONSTRAINT conversations_not_self_merged_chk CHECK (merged_into_id IS NULL OR merged_into_id <> id);

CREATE INDEX idx_conversations_merged_into
    ON conversations (organization_id, merged_into_id)
    WHERE merged_into_id IS NOT NULL;

-- Links between two conversations of one organization. kind 'parent':
-- from_id is the parent of to_id (a split creates one). kind 'related' is
-- symmetric and stored once with from_id < to_id.
CREATE TABLE conversation_links (
    id              BIGSERIAL   PRIMARY KEY,
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    from_id         BIGINT      NOT NULL,
    to_id           BIGINT      NOT NULL,
    kind            TEXT        NOT NULL,
    created_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT conversation_links_from_fk FOREIGN KEY (organization_id, from_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_links_to_fk FOREIGN KEY (organization_id, to_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_links_kind_chk CHECK (kind IN ('parent', 'related')),
    CONSTRAINT conversation_links_not_self_chk CHECK (from_id <> to_id),
    CONSTRAINT conversation_links_related_order_chk CHECK (kind <> 'related' OR from_id < to_id),
    CONSTRAINT conversation_links_pair_uq UNIQUE (organization_id, from_id, to_id)
);

CREATE INDEX idx_conversation_links_to ON conversation_links (organization_id, to_id);
-- A conversation has at most one parent.
CREATE UNIQUE INDEX conversation_links_one_parent_uq
    ON conversation_links (organization_id, to_id) WHERE kind = 'parent';

-- Side conversations: an e-mail thread an agent runs from inside a
-- conversation with someone who is not the customer (another team, a
-- supplier). Sent through one of the organization's e-mail inboxes; replies
-- come back on thread_root (our Message-ID carries it) and never open a
-- customer conversation.
CREATE TABLE side_conversations (
    id                  BIGSERIAL   PRIMARY KEY,
    uuid                UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    conversation_id     BIGINT      NOT NULL,
    channel_instance_id BIGINT      NULL,
    subject             TEXT        NOT NULL,
    participants        TEXT[]      NOT NULL DEFAULT '{}',
    status              TEXT        NOT NULL DEFAULT 'open',
    thread_root         TEXT        NOT NULL,
    created_by          BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    message_count       INTEGER     NOT NULL DEFAULT 0,
    last_message_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT side_conversations_uuid_uq UNIQUE (uuid),
    CONSTRAINT side_conversations_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT side_conversations_thread_uq UNIQUE (thread_root),
    CONSTRAINT side_conversations_conversation_fk FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT side_conversations_instance_fk FOREIGN KEY (organization_id, channel_instance_id)
        REFERENCES channel_instances (organization_id, id) ON DELETE SET NULL (channel_instance_id),
    CONSTRAINT side_conversations_status_chk CHECK (status IN ('open', 'closed')),
    CONSTRAINT side_conversations_subject_chk CHECK (char_length(subject) BETWEEN 1 AND 500)
);

CREATE INDEX idx_side_conversations_conversation
    ON side_conversations (organization_id, conversation_id, created_at DESC);

CREATE TABLE side_conversation_messages (
    id                   BIGSERIAL   PRIMARY KEY,
    uuid                 UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id      BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    side_conversation_id BIGINT      NOT NULL,
    direction            TEXT        NOT NULL,
    sender_user_id       BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    from_address         TEXT        NOT NULL DEFAULT '',
    from_name            TEXT        NOT NULL DEFAULT '',
    recipients           TEXT[]      NOT NULL DEFAULT '{}',
    content              TEXT        NOT NULL DEFAULT '',
    status               TEXT        NOT NULL,
    error                TEXT        NOT NULL DEFAULT '',
    external_id          TEXT        NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT side_conversation_messages_uuid_uq UNIQUE (uuid),
    CONSTRAINT side_conversation_messages_side_fk FOREIGN KEY (organization_id, side_conversation_id)
        REFERENCES side_conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT side_conversation_messages_direction_chk CHECK (direction IN ('in', 'out')),
    CONSTRAINT side_conversation_messages_status_chk CHECK (status IN ('pending', 'sent', 'failed', 'received')),
    CONSTRAINT side_conversation_messages_content_chk CHECK (char_length(content) <= 65536)
);

CREATE INDEX idx_side_conversation_messages_side
    ON side_conversation_messages (organization_id, side_conversation_id, id);
CREATE UNIQUE INDEX side_conversation_messages_external_uq
    ON side_conversation_messages (organization_id, side_conversation_id, external_id)
    WHERE external_id IS NOT NULL;
