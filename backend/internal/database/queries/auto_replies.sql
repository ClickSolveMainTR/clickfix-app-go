-- Automatic customer replies (CF-198): one settings row per organization.

-- name: GetAutoReplySettings :one
SELECT settings FROM auto_replies WHERE organization_id = $1;

-- name: UpsertAutoReplySettings :exec
INSERT INTO auto_replies (organization_id, settings) VALUES ($1, $2)
ON CONFLICT (organization_id) DO UPDATE SET settings = EXCLUDED.settings;

-- name: CountContactMessages :one
-- Inbound messages the contact sent in a conversation (the first one gets
-- the acknowledgement).
SELECT COUNT(*)::bigint FROM messages
WHERE organization_id = sqlc.arg(organization_id) AND conversation_id = sqlc.arg(conversation_id)
  AND direction = 'in' AND sender_type = 'contact' AND NOT private AND deleted_at IS NULL;

-- name: LastAutoReplyAt :one
-- When the last automatic reply of one kind was posted in a conversation
-- (NULL when none).
SELECT MAX(created_at)::timestamptz FROM messages
WHERE organization_id = sqlc.arg(organization_id) AND conversation_id = sqlc.arg(conversation_id)
  AND sender_type = 'bot' AND content_attributes->>'auto_reply' = sqlc.arg(kind)::text AND deleted_at IS NULL;

-- name: ConversationHasAIReply :one
-- Whether the AI agent answered the customer in this conversation.
SELECT EXISTS(SELECT 1 FROM messages
    WHERE organization_id = sqlc.arg(organization_id) AND conversation_id = sqlc.arg(conversation_id)
      AND direction = 'out' AND sender_type = 'bot' AND content_attributes ? 'ai_trigger_message_uuid' AND deleted_at IS NULL)::boolean;
