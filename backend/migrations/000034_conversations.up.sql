-- CF-25: conversation data model (docs/design/conversations.md).
--
-- The conversation is the core object: a ticket is a conversation that
-- carries status, SLA and assignment. Chat, social DMs and e-mail all live in
-- the same tables. Every row is organization scoped; child tables reference
-- their parent through (organization_id, id) so a row can never point at a
-- record of another organization.

-- 1. Companies ------------------------------------------------------------------
CREATE TABLE companies (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            TEXT         NOT NULL,
    domain          TEXT         NULL,
    external_id     TEXT         NULL,
    notes           TEXT         NOT NULL DEFAULT '',
    custom_fields   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ  NULL,
    CONSTRAINT companies_uuid_uq UNIQUE (uuid),
    CONSTRAINT companies_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT companies_name_chk CHECK (char_length(name) BETWEEN 1 AND 255)
);

CREATE UNIQUE INDEX uq_companies_org_domain
    ON companies (organization_id, lower(domain))
    WHERE deleted_at IS NULL AND domain IS NOT NULL;
CREATE INDEX idx_companies_org_name
    ON companies (organization_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_companies_set_updated_at
    BEFORE UPDATE ON companies FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 2. Contacts -------------------------------------------------------------------
-- The person on the other side. merged_into_id points at the surviving
-- contact after a merge (CF-84); merged rows are soft deleted.
CREATE TABLE contacts (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    company_id      BIGINT       NULL,
    name            TEXT         NOT NULL DEFAULT '',
    email           TEXT         NULL,
    phone           TEXT         NULL,
    locale          TEXT         NULL,
    timezone        TEXT         NULL,
    notes           TEXT         NOT NULL DEFAULT '',
    custom_fields   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    blocked         BOOLEAN      NOT NULL DEFAULT FALSE,
    last_seen_at    TIMESTAMPTZ  NULL,
    merged_into_id  BIGINT       NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ  NULL,
    CONSTRAINT contacts_uuid_uq UNIQUE (uuid),
    CONSTRAINT contacts_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT contacts_company_fk FOREIGN KEY (organization_id, company_id)
        REFERENCES companies (organization_id, id) ON DELETE SET NULL (company_id),
    CONSTRAINT contacts_merged_into_fk FOREIGN KEY (organization_id, merged_into_id)
        REFERENCES contacts (organization_id, id) ON DELETE SET NULL (merged_into_id),
    CONSTRAINT contacts_name_chk CHECK (char_length(name) <= 255),
    CONSTRAINT contacts_not_self_merged_chk CHECK (merged_into_id IS NULL OR merged_into_id <> id)
);

CREATE INDEX idx_contacts_org_created ON contacts (organization_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_org_email ON contacts (organization_id, lower(email)) WHERE deleted_at IS NULL AND email IS NOT NULL;
CREATE INDEX idx_contacts_org_phone ON contacts (organization_id, phone) WHERE deleted_at IS NULL AND phone IS NOT NULL;
CREATE INDEX idx_contacts_org_company ON contacts (organization_id, company_id) WHERE deleted_at IS NULL AND company_id IS NOT NULL;
CREATE INDEX idx_contacts_custom_fields ON contacts USING GIN (custom_fields jsonb_path_ops);

CREATE TRIGGER trg_contacts_set_updated_at
    BEFORE UPDATE ON contacts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 3. Channel instances (inboxes) ------------------------------------------------
-- An account an organization connected on a channel type (a WhatsApp number,
-- an Instagram account, a mailbox, the web widget). This is the "inbox" of
-- the unified inbox. credentials_enc is sealed with platform/crypto and never
-- leaves the server. external_id is the provider account id webhooks are
-- routed by (phone_number_id, page id, bot id, inbound address).
CREATE TABLE channel_instances (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    channel_type    TEXT         NOT NULL,
    name            TEXT         NOT NULL,
    status          TEXT         NOT NULL DEFAULT 'active',
    external_id     TEXT         NULL,
    credentials_enc TEXT         NOT NULL DEFAULT '',
    settings        JSONB        NOT NULL DEFAULT '{}'::jsonb,
    last_inbound_at TIMESTAMPTZ  NULL,
    last_outbound_at TIMESTAMPTZ NULL,
    last_error      TEXT         NOT NULL DEFAULT '',
    last_error_at   TIMESTAMPTZ  NULL,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ  NULL,
    CONSTRAINT channel_instances_uuid_uq UNIQUE (uuid),
    CONSTRAINT channel_instances_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT channel_instances_type_chk CHECK (channel_type ~ '^[a-z][a-z0-9_]{1,31}$'),
    CONSTRAINT channel_instances_status_chk CHECK (status IN ('active', 'paused', 'error', 'disconnected')),
    CONSTRAINT channel_instances_name_chk CHECK (char_length(name) BETWEEN 1 AND 120)
);

-- One live instance per provider account across all organizations: a
-- provider webhook resolves to exactly one inbox.
CREATE UNIQUE INDEX uq_channel_instances_external
    ON channel_instances (channel_type, external_id)
    WHERE deleted_at IS NULL AND external_id IS NOT NULL;
CREATE INDEX idx_channel_instances_org ON channel_instances (organization_id, channel_type) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_channel_instances_set_updated_at
    BEFORE UPDATE ON channel_instances FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 4. Contact identities ---------------------------------------------------------
-- How a contact is known on a channel: phone on WhatsApp, IGSID on Instagram,
-- address on e-mail, visitor id on the widget. channel_instance_id is set for
-- ids that are only meaningful inside one account (page scoped ids, bot chat
-- ids) and NULL for global ids (e-mail address, phone number).
CREATE TABLE contact_identities (
    id                  BIGSERIAL    PRIMARY KEY,
    organization_id     BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    contact_id          BIGINT       NOT NULL,
    channel_type        TEXT         NOT NULL,
    channel_instance_id BIGINT       NULL,
    external_id         TEXT         NOT NULL,
    display_name        TEXT         NOT NULL DEFAULT '',
    username            TEXT         NOT NULL DEFAULT '',
    meta                JSONB        NOT NULL DEFAULT '{}'::jsonb,
    last_seen_at        TIMESTAMPTZ  NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT contact_identities_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT contact_identities_contact_fk FOREIGN KEY (organization_id, contact_id)
        REFERENCES contacts (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT contact_identities_instance_fk FOREIGN KEY (organization_id, channel_instance_id)
        REFERENCES channel_instances (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT contact_identities_external_chk CHECK (char_length(external_id) BETWEEN 1 AND 320)
);

CREATE UNIQUE INDEX uq_contact_identities_external
    ON contact_identities (organization_id, channel_type, COALESCE(channel_instance_id, 0), external_id);
CREATE INDEX idx_contact_identities_contact ON contact_identities (organization_id, contact_id);

-- 5. Custom conversation statuses -----------------------------------------------
-- Organizations may name their own statuses ("Waiting on supplier"); each maps
-- to one of the base categories the product logic understands, which is what
-- conversations.status holds.
CREATE TABLE conversation_statuses (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            TEXT         NOT NULL,
    slug            TEXT         NOT NULL,
    category        TEXT         NOT NULL,
    color           TEXT         NOT NULL DEFAULT '',
    sort_order      INTEGER      NOT NULL DEFAULT 0,
    is_default      BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT conversation_statuses_uuid_uq UNIQUE (uuid),
    CONSTRAINT conversation_statuses_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT conversation_statuses_slug_uq UNIQUE (organization_id, slug),
    CONSTRAINT conversation_statuses_category_chk CHECK (category IN ('open', 'pending', 'snoozed', 'resolved', 'closed'))
);

CREATE UNIQUE INDEX uq_conversation_statuses_default
    ON conversation_statuses (organization_id, category) WHERE is_default;

CREATE TRIGGER trg_conversation_statuses_set_updated_at
    BEFORE UPDATE ON conversation_statuses FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 6. Conversations --------------------------------------------------------------
-- number is the per-organization display number (#1042), handed out by
-- conversation_counters. assignee_team_id is a plain id until the teams
-- module (CF-5) adds its table and foreign key. sla_* columns are written by
-- the SLA engine (CF-46); waiting_since is when the contact's oldest
-- unanswered message arrived (NULL once an agent answered).
CREATE TABLE conversation_counters (
    organization_id BIGINT PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    last_number     BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE conversations (
    id                     BIGSERIAL    PRIMARY KEY,
    uuid                   UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id        BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    number                 BIGINT       NOT NULL,
    channel_type           TEXT         NOT NULL,
    channel_instance_id    BIGINT       NULL,
    contact_id             BIGINT       NOT NULL,
    company_id             BIGINT       NULL,
    subject                TEXT         NOT NULL DEFAULT '',
    status                 TEXT         NOT NULL DEFAULT 'open',
    status_id              BIGINT       NULL,
    priority               TEXT         NOT NULL DEFAULT 'normal',
    source                 TEXT         NOT NULL DEFAULT 'inbound',
    assignee_user_id       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    assignee_team_id       BIGINT       NULL,
    custom_fields          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    external_thread_id     TEXT         NULL,
    sla_policy_id          BIGINT       NULL,
    first_response_due_at  TIMESTAMPTZ  NULL,
    next_response_due_at   TIMESTAMPTZ  NULL,
    resolution_due_at      TIMESTAMPTZ  NULL,
    sla_breached_at        TIMESTAMPTZ  NULL,
    first_response_at      TIMESTAMPTZ  NULL,
    waiting_since          TIMESTAMPTZ  NULL,
    snoozed_until          TIMESTAMPTZ  NULL,
    resolved_at            TIMESTAMPTZ  NULL,
    closed_at              TIMESTAMPTZ  NULL,
    last_message_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_inbound_at        TIMESTAMPTZ  NULL,
    last_outbound_at       TIMESTAMPTZ  NULL,
    last_message_preview   TEXT         NOT NULL DEFAULT '',
    last_message_direction TEXT         NOT NULL DEFAULT 'in',
    message_count          INTEGER      NOT NULL DEFAULT 0,
    meta                   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at             TIMESTAMPTZ  NULL,
    CONSTRAINT conversations_uuid_uq UNIQUE (uuid),
    CONSTRAINT conversations_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT conversations_number_uq UNIQUE (organization_id, number),
    CONSTRAINT conversations_instance_fk FOREIGN KEY (organization_id, channel_instance_id)
        REFERENCES channel_instances (organization_id, id) ON DELETE SET NULL (channel_instance_id),
    CONSTRAINT conversations_contact_fk FOREIGN KEY (organization_id, contact_id)
        REFERENCES contacts (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT conversations_company_fk FOREIGN KEY (organization_id, company_id)
        REFERENCES companies (organization_id, id) ON DELETE SET NULL (company_id),
    CONSTRAINT conversations_status_fk FOREIGN KEY (organization_id, status_id)
        REFERENCES conversation_statuses (organization_id, id) ON DELETE SET NULL (status_id),
    CONSTRAINT conversations_status_chk CHECK (status IN ('open', 'pending', 'snoozed', 'resolved', 'closed')),
    CONSTRAINT conversations_priority_chk CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    CONSTRAINT conversations_source_chk CHECK (source IN ('inbound', 'outbound')),
    CONSTRAINT conversations_direction_chk CHECK (last_message_direction IN ('in', 'out', 'internal')),
    CONSTRAINT conversations_snooze_chk CHECK (status <> 'snoozed' OR snoozed_until IS NOT NULL),
    CONSTRAINT conversations_subject_chk CHECK (char_length(subject) <= 500)
);

-- Inbox queries: list by status (all / mine / team / inbox), newest activity first.
CREATE INDEX idx_conversations_org_status_last
    ON conversations (organization_id, status, last_message_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_conversations_org_assignee
    ON conversations (organization_id, assignee_user_id, status, last_message_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_conversations_org_team
    ON conversations (organization_id, assignee_team_id, status, last_message_at DESC)
    WHERE deleted_at IS NULL AND assignee_team_id IS NOT NULL;
CREATE INDEX idx_conversations_org_instance
    ON conversations (organization_id, channel_instance_id, status, last_message_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_conversations_org_contact
    ON conversations (organization_id, contact_id, last_message_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_conversations_org_company
    ON conversations (organization_id, company_id) WHERE deleted_at IS NULL AND company_id IS NOT NULL;
CREATE INDEX idx_conversations_snoozed
    ON conversations (snoozed_until) WHERE status = 'snoozed' AND deleted_at IS NULL;
CREATE INDEX idx_conversations_thread
    ON conversations (organization_id, channel_instance_id, external_thread_id)
    WHERE external_thread_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_conversations_custom_fields ON conversations USING GIN (custom_fields jsonb_path_ops);

CREATE TRIGGER trg_conversations_set_updated_at
    BEFORE UPDATE ON conversations FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 7. Tags -----------------------------------------------------------------------
CREATE TABLE tags (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            TEXT         NOT NULL,
    color           TEXT         NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT tags_uuid_uq UNIQUE (uuid),
    CONSTRAINT tags_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT tags_name_chk CHECK (char_length(name) BETWEEN 1 AND 64)
);

CREATE UNIQUE INDEX uq_tags_org_name ON tags (organization_id, lower(name));

CREATE TABLE conversation_tags (
    organization_id BIGINT      NOT NULL,
    conversation_id BIGINT      NOT NULL,
    tag_id          BIGINT      NOT NULL,
    created_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (conversation_id, tag_id),
    CONSTRAINT conversation_tags_conversation_fk FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_tags_tag_fk FOREIGN KEY (organization_id, tag_id)
        REFERENCES tags (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_conversation_tags_tag ON conversation_tags (organization_id, tag_id);

-- 8. Messages -------------------------------------------------------------------
-- direction: in (from the contact), out (to the contact), internal (private
-- note or system event, never sent). external_id is the provider message id,
-- unique per channel instance so a redelivered webhook is stored once.
-- client_id is the composer's id so a retried optimistic send is stored once.
-- Files are attachments rows with owner_type 'message' and owner_id = uuid.
CREATE TABLE messages (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    conversation_id     BIGINT       NOT NULL,
    channel_instance_id BIGINT       NULL,
    direction           TEXT         NOT NULL,
    sender_type         TEXT         NOT NULL,
    sender_user_id      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    sender_contact_id   BIGINT       NULL,
    kind                TEXT         NOT NULL DEFAULT 'text',
    content             TEXT         NOT NULL DEFAULT '',
    content_attributes  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    private             BOOLEAN      NOT NULL DEFAULT FALSE,
    status              TEXT         NOT NULL,
    error               TEXT         NOT NULL DEFAULT '',
    external_id         TEXT         NULL,
    client_id           UUID         NULL,
    reply_to_id         BIGINT       NULL,
    sent_at             TIMESTAMPTZ  NULL,
    delivered_at        TIMESTAMPTZ  NULL,
    read_at             TIMESTAMPTZ  NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    edited_at           TIMESTAMPTZ  NULL,
    deleted_at          TIMESTAMPTZ  NULL,
    CONSTRAINT messages_uuid_uq UNIQUE (uuid),
    CONSTRAINT messages_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT messages_conversation_fk FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT messages_instance_fk FOREIGN KEY (organization_id, channel_instance_id)
        REFERENCES channel_instances (organization_id, id) ON DELETE SET NULL (channel_instance_id),
    CONSTRAINT messages_contact_fk FOREIGN KEY (organization_id, sender_contact_id)
        REFERENCES contacts (organization_id, id) ON DELETE SET NULL (sender_contact_id),
    CONSTRAINT messages_reply_to_fk FOREIGN KEY (organization_id, reply_to_id)
        REFERENCES messages (organization_id, id) ON DELETE SET NULL (reply_to_id),
    CONSTRAINT messages_direction_chk CHECK (direction IN ('in', 'out', 'internal')),
    CONSTRAINT messages_sender_chk CHECK (sender_type IN ('contact', 'agent', 'bot', 'system')),
    CONSTRAINT messages_kind_chk CHECK (kind IN ('text', 'media', 'template', 'interactive', 'email', 'note', 'event', 'unsupported')),
    CONSTRAINT messages_status_chk CHECK (status IN ('received', 'pending', 'queued', 'sent', 'delivered', 'read', 'failed', 'recorded')),
    CONSTRAINT messages_private_chk CHECK (NOT private OR direction = 'internal'),
    CONSTRAINT messages_content_chk CHECK (char_length(content) <= 65536)
);

CREATE UNIQUE INDEX uq_messages_external
    ON messages (organization_id, channel_instance_id, external_id) WHERE external_id IS NOT NULL;
CREATE UNIQUE INDEX uq_messages_client
    ON messages (organization_id, conversation_id, client_id) WHERE client_id IS NOT NULL;
CREATE INDEX idx_messages_conversation ON messages (organization_id, conversation_id, id DESC);
CREATE INDEX idx_messages_outbound_pending
    ON messages (organization_id, status, created_at) WHERE direction = 'out' AND status IN ('pending', 'queued');

-- 9. Mentions -------------------------------------------------------------------
CREATE TABLE message_mentions (
    organization_id BIGINT      NOT NULL,
    message_id      BIGINT      NOT NULL,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id),
    CONSTRAINT message_mentions_message_fk FOREIGN KEY (organization_id, message_id)
        REFERENCES messages (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_message_mentions_user ON message_mentions (organization_id, user_id, created_at DESC);

-- 10. Participants --------------------------------------------------------------
-- Agents following a conversation beside the assignee (watchers,
-- collaborators, everyone @mentioned).
CREATE TABLE conversation_participants (
    organization_id BIGINT      NOT NULL,
    conversation_id BIGINT      NOT NULL,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role            TEXT        NOT NULL DEFAULT 'watcher',
    added_by        BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (conversation_id, user_id),
    CONSTRAINT conversation_participants_conversation_fk FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_participants_role_chk CHECK (role IN ('watcher', 'collaborator'))
);

CREATE INDEX idx_conversation_participants_user ON conversation_participants (organization_id, user_id);

-- 11. Read receipts -------------------------------------------------------------
-- Per agent: the last message they have seen. Unread = messages after it
-- that the agent did not write.
CREATE TABLE conversation_reads (
    organization_id      BIGINT      NOT NULL,
    conversation_id      BIGINT      NOT NULL,
    user_id              BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    last_read_message_id BIGINT      NOT NULL DEFAULT 0,
    last_read_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (conversation_id, user_id),
    CONSTRAINT conversation_reads_conversation_fk FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_conversation_reads_user ON conversation_reads (organization_id, user_id);
