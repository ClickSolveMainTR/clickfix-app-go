-- Data-loss note: deletes diagnostic context and capture associations.
DROP TRIGGER IF EXISTS trg_conversation_diagnostics_erasure ON conversations;
DROP FUNCTION IF EXISTS erase_conversation_diagnostics();
DROP TABLE widget_diagnostics;
ALTER TABLE attachments DROP COLUMN expires_at;
