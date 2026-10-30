-- CF-48: canned responses and macros.
-- scope: personal (owner_user_id), team (team_id, FK when CF-5 lands) or
-- organization. content may hold variables ({{contact.first_name}},
-- {{agent.name}}, {{conversation.number}}, {{contact.custom.<key>}}) that
-- are rendered for a conversation. A macro is a list of actions applied in
-- one click (reply / note / status / priority / assign / tag).
CREATE TABLE canned_responses (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    scope           TEXT         NOT NULL DEFAULT 'organization',
    owner_user_id   BIGINT       NULL REFERENCES users (id) ON DELETE CASCADE,
    team_id         BIGINT       NULL,
    title           TEXT         NOT NULL,
    shortcut        TEXT         NOT NULL,
    category        TEXT         NOT NULL DEFAULT '',
    content         TEXT         NOT NULL,
    usage_count     INTEGER      NOT NULL DEFAULT 0,
    last_used_at    TIMESTAMPTZ  NULL,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT canned_responses_uuid_uq UNIQUE (uuid),
    CONSTRAINT canned_responses_scope_chk CHECK (scope IN ('personal', 'team', 'organization')),
    CONSTRAINT canned_responses_owner_chk CHECK ((scope = 'personal') = (owner_user_id IS NOT NULL)),
    CONSTRAINT canned_responses_team_chk CHECK ((scope = 'team') = (team_id IS NOT NULL)),
    CONSTRAINT canned_responses_title_chk CHECK (char_length(title) BETWEEN 1 AND 120),
    CONSTRAINT canned_responses_shortcut_chk CHECK (shortcut ~ '^[a-z0-9][a-z0-9_-]{0,39}$'),
    CONSTRAINT canned_responses_content_chk CHECK (char_length(content) BETWEEN 1 AND 10000)
);

CREATE UNIQUE INDEX uq_canned_responses_shortcut
    ON canned_responses (organization_id, COALESCE(owner_user_id, 0), shortcut);
CREATE INDEX idx_canned_responses_org ON canned_responses (organization_id, usage_count DESC);

CREATE TRIGGER trg_canned_responses_set_updated_at
    BEFORE UPDATE ON canned_responses FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE macros (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    scope           TEXT         NOT NULL DEFAULT 'organization',
    owner_user_id   BIGINT       NULL REFERENCES users (id) ON DELETE CASCADE,
    team_id         BIGINT       NULL,
    name            TEXT         NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    actions         JSONB        NOT NULL DEFAULT '[]'::jsonb,
    usage_count     INTEGER      NOT NULL DEFAULT 0,
    last_used_at    TIMESTAMPTZ  NULL,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT macros_uuid_uq UNIQUE (uuid),
    CONSTRAINT macros_scope_chk CHECK (scope IN ('personal', 'team', 'organization')),
    CONSTRAINT macros_owner_chk CHECK ((scope = 'personal') = (owner_user_id IS NOT NULL)),
    CONSTRAINT macros_team_chk CHECK ((scope = 'team') = (team_id IS NOT NULL)),
    CONSTRAINT macros_name_chk CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT macros_actions_chk CHECK (jsonb_typeof(actions) = 'array')
);

CREATE INDEX idx_macros_org ON macros (organization_id, usage_count DESC);

CREATE TRIGGER trg_macros_set_updated_at
    BEFORE UPDATE ON macros FOR EACH ROW EXECUTE FUNCTION set_updated_at();
