-- CF-140: AI-assisted sign-up and ready-made set-up templates.
--
-- A set-up run carries one SetupPlan (JSON, schema version 1) produced
-- either by the AI from the company website + a short questionnaire
-- (source 'ai') or picked from the curated template gallery (source
-- 'template', templates live in the repo as versioned JSON). The owner
-- previews the plan, unticks what they do not want and applies it in one
-- background job; progress goes out over Centrifugo and is also kept on
-- the run so a reload resumes the live screen.
--
-- onboarding_setup_items is the provenance ledger: every entity the job
-- created (or setting it changed) has a row with source = 'onboarding',
-- the entity type and id, and for settings the previous value. "Geri al"
-- (undo, within 24 hours) walks this ledger in reverse; entities that
-- already existed are recorded as 'skipped' and never touched.

CREATE TABLE onboarding_setup_runs (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    source           TEXT        NOT NULL,
    template_key     TEXT        NOT NULL DEFAULT '',
    template_version INTEGER     NOT NULL DEFAULT 0,
    -- Discovery input (website URL, questionnaire answers) and the
    -- extracted public website summary; kept for review, never secrets.
    discovery        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    plan             JSONB       NOT NULL,
    -- Item keys the owner kept ticked in the preview.
    selection        TEXT[]      NOT NULL DEFAULT '{}',
    status           TEXT        NOT NULL DEFAULT 'draft',
    total            INTEGER     NOT NULL DEFAULT 0,
    done             INTEGER     NOT NULL DEFAULT 0,
    failed           INTEGER     NOT NULL DEFAULT 0,
    error            TEXT        NOT NULL DEFAULT '',
    ai_run_uuid      UUID        NULL,
    created_by       BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    started_at       TIMESTAMPTZ NULL,
    finished_at      TIMESTAMPTZ NULL,
    undone_at        TIMESTAMPTZ NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT onboarding_setup_runs_uuid_uq UNIQUE (uuid),
    CONSTRAINT onboarding_setup_runs_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT onboarding_setup_runs_source_chk CHECK (source IN ('ai', 'template')),
    CONSTRAINT onboarding_setup_runs_status_chk
        CHECK (status IN ('draft', 'queued', 'running', 'succeeded', 'partial', 'failed', 'undone')),
    CONSTRAINT onboarding_setup_runs_plan_chk CHECK (jsonb_typeof(plan) = 'object')
);

CREATE INDEX idx_onboarding_setup_runs_org ON onboarding_setup_runs (organization_id, created_at DESC);
-- One apply at a time per organization.
CREATE UNIQUE INDEX uq_onboarding_setup_runs_active
    ON onboarding_setup_runs (organization_id) WHERE status IN ('queued', 'running');

CREATE TRIGGER trg_onboarding_setup_runs_updated_at
    BEFORE UPDATE ON onboarding_setup_runs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE onboarding_setup_items (
    id              BIGSERIAL PRIMARY KEY,
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    run_id          BIGINT      NOT NULL,
    item_key        TEXT        NOT NULL,
    kind            TEXT        NOT NULL,
    label           TEXT        NOT NULL DEFAULT '',
    source          TEXT        NOT NULL DEFAULT 'onboarding',
    status          TEXT        NOT NULL,
    entity_id       BIGINT      NULL,
    entity_uuid     UUID        NULL,
    -- Settings the job overwrote (help center, widget, AI persona):
    -- the value before, restored by undo.
    previous        JSONB       NULL,
    error           TEXT        NOT NULL DEFAULT '',
    undone_at       TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT onboarding_setup_items_run_fk FOREIGN KEY (organization_id, run_id)
        REFERENCES onboarding_setup_runs (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT onboarding_setup_items_key_uq UNIQUE (run_id, item_key),
    CONSTRAINT onboarding_setup_items_source_chk CHECK (source = 'onboarding'),
    CONSTRAINT onboarding_setup_items_status_chk
        CHECK (status IN ('created', 'updated', 'skipped', 'unavailable', 'failed'))
);

CREATE INDEX idx_onboarding_setup_items_org ON onboarding_setup_items (organization_id, run_id);

-- Applying a set-up plan creates and removes workspace-wide configuration
-- (teams, SLA, AI persona, ...): owners and admins only.
INSERT INTO permissions (name, slug) VALUES ('Run the guided workspace set-up', 'tenant.onboarding.manage')
ON CONFLICT (slug) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE p.slug = 'tenant.onboarding.manage'
  AND r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin')
ON CONFLICT DO NOTHING;
