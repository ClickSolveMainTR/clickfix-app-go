-- CF-141: module guides (coach-mark tours + "what is this page for?") and
-- the ClickFix Asistan, the platform-owned AI support/setup chat.
--
-- Guides are platform content: one row per (key, locale). Steps live in a
-- JSONB array ({id, target, title, body, placement, action}); every save
-- bumps version. Rows shipped with the code have source = 'seed' and are
-- refreshed on boot while a platform admin has not edited them
-- (source becomes 'custom' on the first edit).
--
-- The assistant runs on the PLATFORM's AI key: its usage is never charged
-- to the organization's AI credits. assistant_usage records every provider
-- call for platform reporting and the per-organization daily limit.

CREATE TABLE guides (
    id           BIGSERIAL PRIMARY KEY,
    uuid         UUID        NOT NULL DEFAULT gen_random_uuid(),
    key          TEXT        NOT NULL,
    locale       TEXT        NOT NULL,
    surface      TEXT        NOT NULL,
    route        TEXT        NOT NULL,
    kind         TEXT        NOT NULL DEFAULT 'page',
    sort_order   INTEGER     NOT NULL DEFAULT 0,
    -- 0 = not part of the setup tour; otherwise the 1-based setup order.
    setup_order  INTEGER     NOT NULL DEFAULT 0,
    title        TEXT        NOT NULL,
    summary      TEXT        NOT NULL DEFAULT '',
    steps        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    version      INTEGER     NOT NULL DEFAULT 1,
    source       TEXT        NOT NULL DEFAULT 'custom',
    seed_version INTEGER     NOT NULL DEFAULT 0,
    is_published BOOLEAN     NOT NULL DEFAULT TRUE,
    updated_by   BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT guides_uuid_uq UNIQUE (uuid),
    CONSTRAINT guides_key_locale_uq UNIQUE (key, locale),
    CONSTRAINT guides_key_chk CHECK (key ~ '^[a-z0-9][a-z0-9_.-]{1,99}$'),
    CONSTRAINT guides_locale_chk CHECK (locale IN ('tr', 'en')),
    CONSTRAINT guides_surface_chk CHECK (surface IN ('web', 'mobile')),
    CONSTRAINT guides_kind_chk CHECK (kind IN ('tour', 'page')),
    CONSTRAINT guides_source_chk CHECK (source IN ('seed', 'custom', 'ai')),
    CONSTRAINT guides_route_chk CHECK (char_length(route) BETWEEN 1 AND 200),
    CONSTRAINT guides_title_chk CHECK (char_length(title) BETWEEN 1 AND 200),
    CONSTRAINT guides_steps_chk CHECK (jsonb_typeof(steps) = 'array')
);

CREATE INDEX idx_guides_route ON guides (surface, locale, route);
CREATE INDEX idx_guides_setup ON guides (surface, locale, setup_order) WHERE setup_order > 0;

CREATE TRIGGER trg_guides_updated_at
    BEFORE UPDATE ON guides
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Per user and organization: a member's progress in one workspace does
-- not leak into another.
CREATE TABLE guide_progress (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    guide_key       TEXT        NOT NULL,
    status          TEXT        NOT NULL,
    step_index      INTEGER     NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, user_id, guide_key),
    CONSTRAINT guide_progress_status_chk CHECK (status IN ('new', 'in_progress', 'completed', 'skipped')),
    CONSTRAINT guide_progress_step_chk CHECK (step_index BETWEEN 0 AND 100)
);

CREATE TRIGGER trg_guide_progress_updated_at
    BEFORE UPDATE ON guide_progress
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Singleton (id = 1) platform switch board of the assistant.
CREATE TABLE assistant_settings (
    id                  SMALLINT    PRIMARY KEY DEFAULT 1,
    enabled             BOOLEAN     NOT NULL DEFAULT TRUE,
    -- An aicore task route (AI_<TASK>_* environment): triage|reply|complex.
    model_route         TEXT        NOT NULL DEFAULT 'reply',
    daily_limit_per_org INTEGER     NOT NULL DEFAULT 100,
    system_prompt_extra TEXT        NOT NULL DEFAULT '',
    -- ClickFix's own support organization (escalations open a conversation there).
    support_org_uuid    UUID        NULL,
    updated_by          BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT assistant_settings_singleton_chk CHECK (id = 1),
    CONSTRAINT assistant_settings_route_chk CHECK (model_route IN ('triage', 'reply', 'complex')),
    CONSTRAINT assistant_settings_limit_chk CHECK (daily_limit_per_org BETWEEN 0 AND 100000),
    CONSTRAINT assistant_settings_prompt_chk CHECK (char_length(system_prompt_extra) <= 4000)
);

INSERT INTO assistant_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

CREATE TRIGGER trg_assistant_settings_updated_at
    BEFORE UPDATE ON assistant_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE assistant_threads (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title           TEXT        NOT NULL DEFAULT '',
    context         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    escalated_at    TIMESTAMPTZ NULL,
    ticket_ref      TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT assistant_threads_uuid_uq UNIQUE (uuid),
    CONSTRAINT assistant_threads_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT assistant_threads_title_chk CHECK (char_length(title) <= 200)
);

CREATE INDEX idx_assistant_threads_owner ON assistant_threads (organization_id, user_id, updated_at DESC);

CREATE TRIGGER trg_assistant_threads_updated_at
    BEFORE UPDATE ON assistant_threads
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Content is stored PII-masked (the same rules as other AI tasks).
-- actions holds the ProposedAction list of an assistant reply; confirm and
-- reject update the status inside it.
CREATE TABLE assistant_messages (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NOT NULL,
    thread_id       BIGINT      NOT NULL,
    role            TEXT        NOT NULL,
    content         TEXT        NOT NULL DEFAULT '',
    suggestions     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    actions         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT assistant_messages_uuid_uq UNIQUE (uuid),
    CONSTRAINT assistant_messages_thread_fk FOREIGN KEY (organization_id, thread_id)
        REFERENCES assistant_threads (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT assistant_messages_role_chk CHECK (role IN ('user', 'assistant', 'tool')),
    CONSTRAINT assistant_messages_content_chk CHECK (char_length(content) <= 20000),
    CONSTRAINT assistant_messages_actions_chk CHECK (jsonb_typeof(actions) = 'array'),
    CONSTRAINT assistant_messages_suggestions_chk CHECK (jsonb_typeof(suggestions) = 'array')
);

CREATE INDEX idx_assistant_messages_thread ON assistant_messages (thread_id, id);

-- One row per provider call (platform cost reporting + daily limit).
CREATE TABLE assistant_usage (
    id              BIGSERIAL PRIMARY KEY,
    -- NULL for platform-level calls (guide drafting).
    organization_id BIGINT      NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    thread_id       BIGINT      NULL REFERENCES assistant_threads (id) ON DELETE SET NULL,
    kind            TEXT        NOT NULL DEFAULT 'chat',
    provider        TEXT        NOT NULL DEFAULT '',
    model           TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL,
    input_tokens    BIGINT      NOT NULL DEFAULT 0,
    output_tokens   BIGINT      NOT NULL DEFAULT 0,
    cost_micros     BIGINT      NOT NULL DEFAULT 0,
    latency_ms      BIGINT      NOT NULL DEFAULT 0,
    error_code      TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT assistant_usage_kind_chk CHECK (kind IN ('chat', 'guide_generate')),
    CONSTRAINT assistant_usage_status_chk CHECK (status IN ('completed', 'failed'))
);

CREATE INDEX idx_assistant_usage_org_day ON assistant_usage (organization_id, created_at);

-- Feedback about ClickFix itself (web "Geri bildirim", mobile shake).
CREATE TABLE product_feedback (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id          BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    kind             TEXT        NOT NULL,
    message          TEXT        NOT NULL,
    route            TEXT        NOT NULL DEFAULT '',
    surface          TEXT        NOT NULL,
    app_version      TEXT        NOT NULL DEFAULT '',
    device           TEXT        NOT NULL DEFAULT '',
    attachment_uuids UUID[]      NOT NULL DEFAULT '{}',
    status           TEXT        NOT NULL DEFAULT 'new',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT product_feedback_uuid_uq UNIQUE (uuid),
    CONSTRAINT product_feedback_kind_chk CHECK (kind IN ('bug', 'idea', 'other')),
    CONSTRAINT product_feedback_surface_chk CHECK (surface IN ('web', 'mobile')),
    CONSTRAINT product_feedback_status_chk CHECK (status IN ('new', 'reviewed', 'closed')),
    CONSTRAINT product_feedback_message_chk CHECK (char_length(message) BETWEEN 1 AND 5000),
    CONSTRAINT product_feedback_attachments_chk CHECK (cardinality(attachment_uuids) <= 5)
);

CREATE INDEX idx_product_feedback_created ON product_feedback (created_at DESC);

CREATE TRIGGER trg_product_feedback_updated_at
    BEFORE UPDATE ON product_feedback
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Every member gets the assistant chat; platform staff manage it.
INSERT INTO permissions (name, slug) VALUES
    ('Use the ClickFix assistant', 'tenant.assistant.use'),
    ('Manage the ClickFix assistant settings', 'platform.assistant.manage'),
    ('Manage module guides', 'platform.guides.manage'),
    ('Read product feedback', 'platform.product_feedback.read')
ON CONFLICT (slug) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE p.slug IN ('tenant.assistant.use', 'platform.assistant.manage', 'platform.guides.manage', 'platform.product_feedback.read')
  AND ((r.scope = 'platform' AND r.slug = 'super_admin' AND p.slug LIKE 'platform.%')
    OR (r.scope = 'tenant' AND r.organization_id IS NULL AND p.slug = 'tenant.assistant.use'))
ON CONFLICT DO NOTHING;
