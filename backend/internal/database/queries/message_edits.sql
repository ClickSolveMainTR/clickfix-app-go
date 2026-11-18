-- CF-248: editing a sent message.

-- name: EditOutboundMessage :one
-- Rewrites an agent's own public reply. Messages being handed to the
-- channel (pending) are never rewritten, so a send never carries a text the
-- edit history does not know.
UPDATE messages SET content = sqlc.arg(content), edited_at = sqlc.arg(edited_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
  AND direction = 'out' AND sender_type = 'agent' AND sender_user_id = sqlc.arg(editor_user_id)
  AND NOT private AND deleted_at IS NULL
  AND status IN ('queued', 'failed', 'sent', 'delivered', 'read')
  AND content = sqlc.arg(previous_content)
RETURNING *;

-- name: InsertMessageEdit :one
INSERT INTO message_edits (organization_id, message_id, editor_user_id, previous_content, created_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(message_id), sqlc.narg(editor_user_id), sqlc.arg(previous_content), sqlc.arg(created_at))
RETURNING *;

-- name: ListMessageEdits :many
-- Oldest first: previous_content of the first row is the original text.
SELECT e.uuid, e.previous_content, e.created_at, u.uuid AS editor_uuid, u.name AS editor_name, u.surname AS editor_surname
FROM message_edits e
LEFT JOIN users u ON u.id = e.editor_user_id
WHERE e.organization_id = sqlc.arg(organization_id) AND e.message_id = sqlc.arg(message_id)
ORDER BY e.id;

-- name: SetConversationPreviewIfLatest :exec
-- The inbox list shows the edited text when the edited message is still
-- the newest one of the conversation.
UPDATE conversations c SET last_message_preview = sqlc.arg(preview)
WHERE c.organization_id = sqlc.arg(organization_id) AND c.id = sqlc.arg(conversation_id)
  AND NOT EXISTS (
    SELECT 1 FROM messages m
    WHERE m.organization_id = c.organization_id AND m.conversation_id = c.id
      AND m.id > sqlc.arg(message_id) AND m.deleted_at IS NULL
  );
