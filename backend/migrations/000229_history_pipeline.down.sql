-- Data-loss note: drops the history archive, its runs, clusters and the
-- decision memory, and every suggestion that came from history.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.history.read', 'tenant.history.manage', 'tenant.history.review'));
DELETE FROM permissions WHERE slug IN ('tenant.history.read', 'tenant.history.manage', 'tenant.history.review');
DROP TABLE IF EXISTS history_decisions;
DELETE FROM knowledge_suggestions WHERE source = 'history';
DROP INDEX IF EXISTS uq_knowledge_suggestions_cluster_pending;
ALTER TABLE knowledge_suggestions
    DROP CONSTRAINT IF EXISTS knowledge_suggestions_source_chk,
    DROP CONSTRAINT IF EXISTS knowledge_suggestions_conflict_fk,
    DROP CONSTRAINT IF EXISTS knowledge_suggestions_import_fk,
    DROP CONSTRAINT IF EXISTS knowledge_suggestions_cluster_fk,
    DROP COLUMN IF EXISTS conflict_score,
    DROP COLUMN IF EXISTS conflict_article_id,
    DROP COLUMN IF EXISTS evidence_count,
    DROP COLUMN IF EXISTS import_id,
    DROP COLUMN IF EXISTS cluster_id,
    DROP COLUMN IF EXISTS source;
ALTER TABLE knowledge_suggestions ALTER COLUMN conversation_id SET NOT NULL;
DROP TABLE IF EXISTS history_cases;
DROP FUNCTION IF EXISTS history_case_read_only();
DROP TABLE IF EXISTS history_clusters;
DROP TABLE IF EXISTS history_imports;
DROP TABLE IF EXISTS history_settings;
