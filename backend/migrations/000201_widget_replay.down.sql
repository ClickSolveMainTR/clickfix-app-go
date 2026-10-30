-- Data-loss note: removes replay metadata, consent records and co-browse sessions.
DROP TRIGGER IF EXISTS trg_conversation_replay_erasure ON conversations;
DROP FUNCTION IF EXISTS erase_conversation_replays();
DROP TABLE IF EXISTS widget_replay_chunks;
DROP TABLE IF EXISTS widget_sessions;
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.replays.read','tenant.cobrowse.use','tenant.cobrowse.control'));
DELETE FROM permissions WHERE slug IN ('tenant.replays.read','tenant.cobrowse.use','tenant.cobrowse.control');
-- Keep immutable billing feature catalog rows: existing grants, usage and financial
-- records may reference them. Without session routes these grants are inert.
