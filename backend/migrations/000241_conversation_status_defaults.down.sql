-- Data-loss note: removes the seeded "İşleniyor" status of every
-- organization; conversations that used it keep their base status
-- (status_id is set NULL by the foreign key).
DROP TRIGGER IF EXISTS trg_organizations_default_conversation_statuses ON organizations;
DROP FUNCTION IF EXISTS organizations_default_conversation_statuses();
DELETE FROM conversation_statuses WHERE slug = 'isleniyor';
