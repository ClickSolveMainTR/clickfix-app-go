-- Data-loss note: drops departments, their shares and transfer history; the
-- department of teams, knowledge and conversations is lost (everything is
-- organization-wide again).
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('tenant.departments.read', 'tenant.departments.write', 'tenant.departments.read_all'));
DELETE FROM permissions WHERE slug IN ('tenant.departments.read', 'tenant.departments.write', 'tenant.departments.read_all');

DROP TABLE IF EXISTS conversation_department_transfers;
DROP FUNCTION IF EXISTS department_visible(BIGINT, TEXT, BIGINT, BIGINT, BIGINT[]);

DROP TRIGGER IF EXISTS trg_business_context_sections_drop_shares ON business_context_sections;
DROP TRIGGER IF EXISTS trg_knowledge_sources_drop_shares ON knowledge_sources;
DROP TRIGGER IF EXISTS trg_knowledge_articles_drop_shares ON knowledge_articles;
DROP FUNCTION IF EXISTS drop_department_knowledge_shares();
DROP TABLE IF EXISTS department_knowledge_shares;

DROP TRIGGER IF EXISTS trg_knowledge_suggestion_department ON knowledge_suggestions;
DROP FUNCTION IF EXISTS inherit_suggestion_department();

ALTER TABLE conversations
    DROP COLUMN IF EXISTS department_routed_at,
    DROP COLUMN IF EXISTS department_confidence,
    DROP COLUMN IF EXISTS department_source,
    DROP COLUMN IF EXISTS department_id;
ALTER TABLE knowledge_suggestions DROP COLUMN IF EXISTS department_id;
ALTER TABLE business_context_sections DROP COLUMN IF EXISTS department_id;
ALTER TABLE knowledge_sources DROP COLUMN IF EXISTS department_id;
ALTER TABLE knowledge_articles DROP COLUMN IF EXISTS department_id;
ALTER TABLE teams DROP COLUMN IF EXISTS department_id;

DROP TABLE IF EXISTS departments;
