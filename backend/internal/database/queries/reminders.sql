-- CF-51: reminders and bulk operations.

-- name: InsertConversationReminder :one
INSERT INTO conversation_reminders (organization_id, conversation_id, user_id, due_at, condition, note)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListConversationReminders :many
SELECT * FROM conversation_reminders
WHERE organization_id = $1 AND conversation_id = $2 AND status = 'pending'
ORDER BY due_at;

-- name: CancelConversationReminder :execrows
UPDATE conversation_reminders SET status = 'cancelled'
WHERE organization_id = $1 AND uuid = $2 AND status = 'pending';

-- name: ClaimDueReminders :many
-- One worker tick claims due reminders (SKIP LOCKED keeps ticks apart).
UPDATE conversation_reminders r SET status = 'fired', fired_at = NOW()
WHERE r.id IN (
    SELECT id FROM conversation_reminders
    WHERE status = 'pending' AND due_at <= sqlc.arg(now)::timestamptz
    ORDER BY due_at
    LIMIT 200
    FOR UPDATE SKIP LOCKED
)
RETURNING r.*;

-- name: InsertConversationBulkOp :one
INSERT INTO conversation_bulk_ops (organization_id, actor_id, action, value, items, applied, failed)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetConversationBulkOp :one
SELECT * FROM conversation_bulk_ops WHERE organization_id = $1 AND uuid = $2;

-- name: MarkConversationBulkOpReverted :execrows
UPDATE conversation_bulk_ops SET reverted_at = NOW()
WHERE organization_id = $1 AND id = $2 AND reverted_at IS NULL;

-- name: GetConversationsByUUIDs :many
SELECT * FROM conversations
WHERE organization_id = $1 AND uuid = ANY(sqlc.arg(uuids)::uuid[]) AND deleted_at IS NULL;
