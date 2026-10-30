DROP TABLE IF EXISTS side_conversation_messages;
DROP TABLE IF EXISTS side_conversations;
DROP TABLE IF EXISTS conversation_links;
DROP INDEX IF EXISTS idx_conversations_merged_into;
ALTER TABLE conversations
    DROP CONSTRAINT IF EXISTS conversations_not_self_merged_chk,
    DROP CONSTRAINT IF EXISTS conversations_merged_into_fk,
    DROP COLUMN IF EXISTS merged_at,
    DROP COLUMN IF EXISTS merged_into_id;
