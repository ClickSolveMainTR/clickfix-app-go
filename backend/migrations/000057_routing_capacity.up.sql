-- CF-54: agent capacity. max_open caps the open conversations an agent holds
-- per channel type ('*' = any channel); user_id NULL rows are the
-- organization defaults. Effective limit: user + channel, user + '*',
-- default + channel, default + '*', else unlimited. 0 stops new
-- assignments.
CREATE TABLE agent_capacity (
    id              BIGSERIAL   PRIMARY KEY,
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         BIGINT      NULL REFERENCES users (id) ON DELETE CASCADE,
    channel_type    TEXT        NOT NULL DEFAULT '*',
    max_open        INTEGER     NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT agent_capacity_max_chk CHECK (max_open BETWEEN 0 AND 1000),
    CONSTRAINT agent_capacity_channel_chk CHECK (char_length(channel_type) BETWEEN 1 AND 32)
);

CREATE UNIQUE INDEX uq_agent_capacity ON agent_capacity (organization_id, COALESCE(user_id, 0), channel_type);

-- CF-53: automatic routing. One settings row per organization; rules send
-- matching conversations to a team queue and may require skills; cursors
-- remember the last agent of each round-robin pool; routing_events is the
-- audit trail of every automatic assignment.
CREATE TABLE routing_settings (
    organization_id     BIGINT      PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    enabled             BOOLEAN     NOT NULL DEFAULT FALSE,
    strategy            TEXT        NOT NULL DEFAULT 'round_robin',
    respect_capacity    BOOLEAN     NOT NULL DEFAULT TRUE,
    vip_account_owner   BOOLEAN     NOT NULL DEFAULT TRUE,
    reassign_on_offline BOOLEAN     NOT NULL DEFAULT TRUE,
    unanswered_minutes  INTEGER     NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT routing_settings_strategy_chk CHECK (strategy IN ('round_robin', 'load_balanced')),
    CONSTRAINT routing_settings_unanswered_chk CHECK (unanswered_minutes IS NULL OR unanswered_minutes BETWEEN 1 AND 1440)
);

CREATE TABLE routing_rules (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            TEXT         NOT NULL,
    position        INTEGER      NOT NULL DEFAULT 0,
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    -- {"channel_types": [], "priorities": [], "tag_uuids": []}; empty matches all.
    conditions      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    team_id         BIGINT       NULL,
    skills          TEXT[]       NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT routing_rules_uuid_uq UNIQUE (uuid),
    CONSTRAINT routing_rules_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT routing_rules_team_fk FOREIGN KEY (organization_id, team_id)
        REFERENCES teams (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT routing_rules_name_chk CHECK (char_length(name) BETWEEN 1 AND 120)
);

CREATE INDEX idx_routing_rules_org ON routing_rules (organization_id, position, id);

CREATE TRIGGER trg_routing_rules_set_updated_at
    BEFORE UPDATE ON routing_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE routing_cursors (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    scope_key       TEXT        NOT NULL,
    last_user_id    BIGINT      NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, scope_key)
);

CREATE TABLE routing_events (
    id              BIGSERIAL   PRIMARY KEY,
    organization_id BIGINT      NOT NULL,
    conversation_id BIGINT      NOT NULL,
    from_user_id    BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    to_user_id      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    team_id         BIGINT      NULL,
    rule_id         BIGINT      NULL,
    reason          TEXT        NOT NULL,
    strategy        TEXT        NOT NULL DEFAULT '',
    note            TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT routing_events_conversation_fk FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT routing_events_reason_chk CHECK (reason IN ('new', 'queue', 'offline', 'unanswered', 'vip'))
);

CREATE INDEX idx_routing_events_conversation ON routing_events (organization_id, conversation_id, created_at DESC);
CREATE INDEX idx_routing_events_org ON routing_events (organization_id, created_at DESC);
-- Queue drain: unassigned open conversations, oldest first.
CREATE INDEX idx_conversations_unassigned_open
    ON conversations (organization_id, created_at)
    WHERE assignee_user_id IS NULL AND status = 'open' AND deleted_at IS NULL;
