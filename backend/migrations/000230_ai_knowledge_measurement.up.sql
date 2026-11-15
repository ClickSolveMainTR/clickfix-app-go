-- CF-220: knowledge layer measurement. Department question sets on top of
-- ai_evaluation_cases, before/after metrics on every evaluation run, an
-- opt-in watch that reruns the set when the agent config, the reply model
-- or the published knowledge changes, and the AI insight permissions.
--
-- department_id stays NULL (= organization wide) and has no foreign key
-- until departments land (CF-215); that migration adds the reference.

ALTER TABLE ai_evaluation_cases
    ADD COLUMN department_id BIGINT,
    ADD COLUMN expected_answer TEXT NOT NULL DEFAULT '' CHECK (char_length(expected_answer) <= 6000),
    ADD COLUMN expected_article_uuids UUID[] NOT NULL DEFAULT '{}'::uuid[] CHECK (cardinality(expected_article_uuids) <= 20),
    ADD COLUMN active BOOLEAN NOT NULL DEFAULT TRUE;
CREATE INDEX idx_ai_evaluation_cases_department ON ai_evaluation_cases (organization_id, department_id) WHERE active;

-- Metric columns are NULL when a run cannot measure them (no expected
-- sources, no answer cache connected, no completed case).
ALTER TABLE ai_evaluations
    ADD COLUMN trigger_kind TEXT NOT NULL DEFAULT 'manual'
        CHECK (trigger_kind IN ('manual', 'initial', 'config_change', 'model_change', 'knowledge_change')),
    ADD COLUMN department_id BIGINT,
    ADD COLUMN fingerprint JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN model TEXT NOT NULL DEFAULT '' CHECK (char_length(model) <= 200),
    ADD COLUMN answer_accuracy DOUBLE PRECISION CHECK (answer_accuracy BETWEEN 0 AND 1),
    ADD COLUMN source_hit_rate DOUBLE PRECISION CHECK (source_hit_rate BETWEEN 0 AND 1),
    ADD COLUMN cache_hit_rate DOUBLE PRECISION CHECK (cache_hit_rate BETWEEN 0 AND 1),
    ADD COLUMN avg_latency_ms DOUBLE PRECISION CHECK (avg_latency_ms >= 0),
    ADD COLUMN cost_per_case_micros DOUBLE PRECISION CHECK (cost_per_case_micros >= 0);
CREATE INDEX idx_ai_evaluations_history ON ai_evaluations (organization_id, status, id DESC);

ALTER TABLE ai_evaluation_results
    ADD COLUMN department_id BIGINT,
    ADD COLUMN answer_correct BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN source_hit BOOLEAN,
    ADD COLUMN cache_hit BOOLEAN,
    ADD COLUMN latency_ms BIGINT NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    ADD COLUMN cost_micros BIGINT NOT NULL DEFAULT 0 CHECK (cost_micros >= 0);

-- One watch per organization. actor_id is the member who turned it on;
-- automatic runs are recorded under that member.
CREATE TABLE ai_evaluation_watches (
    organization_id  BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    enabled          BOOLEAN NOT NULL DEFAULT FALSE,
    actor_id         BIGINT NOT NULL REFERENCES users(id),
    last_fingerprint JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_checked_at  TIMESTAMPTZ,
    lease_until      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_evaluation_watches_due ON ai_evaluation_watches (last_checked_at) WHERE enabled;
CREATE TRIGGER trg_ai_evaluation_watches_updated BEFORE UPDATE ON ai_evaluation_watches FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (name, slug) VALUES
    ('View AI knowledge insights and evaluation results', 'tenant.ai_insights.read'),
    ('Manage AI evaluation sets and run evaluations', 'tenant.ai_insights.manage')
ON CONFLICT (slug) DO NOTHING;

-- Evaluation routes used tenant.settings.*; every role (system or custom)
-- that held those keeps the same access through the new slugs.
INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, n.id FROM role_permissions rp
JOIN permissions o ON o.id = rp.permission_id AND o.slug = 'tenant.settings.read'
CROSS JOIN permissions n WHERE n.slug = 'tenant.ai_insights.read'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, n.id FROM role_permissions rp
JOIN permissions o ON o.id = rp.permission_id AND o.slug = 'tenant.settings.write'
CROSS JOIN permissions n WHERE n.slug IN ('tenant.ai_insights.read', 'tenant.ai_insights.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin', 'supervisor')
  AND p.slug = 'tenant.ai_insights.read'
ON CONFLICT DO NOTHING;
