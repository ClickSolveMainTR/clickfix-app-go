-- Data-loss note: drops evaluation watches, the per-run metrics, the
-- per-case department, expected answer and expected sources.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.ai_insights.read', 'tenant.ai_insights.manage'));
DELETE FROM permissions WHERE slug IN ('tenant.ai_insights.read', 'tenant.ai_insights.manage');
DROP TABLE IF EXISTS ai_evaluation_watches;
ALTER TABLE ai_evaluation_results
    DROP COLUMN IF EXISTS cost_micros,
    DROP COLUMN IF EXISTS latency_ms,
    DROP COLUMN IF EXISTS cache_hit,
    DROP COLUMN IF EXISTS source_hit,
    DROP COLUMN IF EXISTS answer_correct,
    DROP COLUMN IF EXISTS department_id;
DROP INDEX IF EXISTS idx_ai_evaluations_history;
ALTER TABLE ai_evaluations
    DROP COLUMN IF EXISTS cost_per_case_micros,
    DROP COLUMN IF EXISTS avg_latency_ms,
    DROP COLUMN IF EXISTS cache_hit_rate,
    DROP COLUMN IF EXISTS source_hit_rate,
    DROP COLUMN IF EXISTS answer_accuracy,
    DROP COLUMN IF EXISTS model,
    DROP COLUMN IF EXISTS fingerprint,
    DROP COLUMN IF EXISTS department_id,
    DROP COLUMN IF EXISTS trigger_kind;
DROP INDEX IF EXISTS idx_ai_evaluation_cases_department;
ALTER TABLE ai_evaluation_cases
    DROP COLUMN IF EXISTS active,
    DROP COLUMN IF EXISTS expected_article_uuids,
    DROP COLUMN IF EXISTS expected_answer,
    DROP COLUMN IF EXISTS department_id;
