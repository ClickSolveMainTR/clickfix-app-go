-- CF-98: merge / split, linked conversations and side conversations.

-- name: MoveConversationMessages :execrows
-- Moves every message of one conversation into another (merge).
UPDATE messages SET conversation_id = sqlc.arg(into_id)
WHERE organization_id = sqlc.arg(organization_id) AND conversation_id = sqlc.arg(from_id);

-- name: MoveMessagesFrom :execrows
-- Moves the messages of a conversation from first_id on (or only first_id)
-- into another conversation (split).
UPDATE messages SET conversation_id = sqlc.arg(into_id)
WHERE organization_id = sqlc.arg(organization_id) AND conversation_id = sqlc.arg(from_id)
  AND (id = sqlc.arg(first_id) OR (sqlc.arg(following)::boolean AND id > sqlc.arg(first_id)));

-- name: RecountConversation :one
-- Rebuilds the counters a message move changes from the messages left.
UPDATE conversations c SET
    message_count = s.cnt,
    last_message_at = COALESCE(s.last_at, c.created_at),
    last_inbound_at = s.last_in,
    last_outbound_at = s.last_out,
    last_message_preview = COALESCE(s.preview, ''),
    last_message_direction = COALESCE(s.direction, 'in'),
    updated_at = NOW()
FROM (
    SELECT COUNT(*)::int AS cnt,
           MAX(m.created_at) AS last_at,
           MAX(m.created_at) FILTER (WHERE m.direction = 'in') AS last_in,
           MAX(m.created_at) FILTER (WHERE m.direction = 'out') AS last_out,
           (SELECT left(l.content, 140) FROM messages l
             WHERE l.organization_id = sqlc.arg(organization_id) AND l.conversation_id = sqlc.arg(id)
               AND l.deleted_at IS NULL AND l.direction <> 'internal'
             ORDER BY l.id DESC LIMIT 1) AS preview,
           (SELECT l.direction FROM messages l
             WHERE l.organization_id = sqlc.arg(organization_id) AND l.conversation_id = sqlc.arg(id)
               AND l.deleted_at IS NULL AND l.direction <> 'internal'
             ORDER BY l.id DESC LIMIT 1) AS direction
    FROM messages m
    WHERE m.organization_id = sqlc.arg(organization_id) AND m.conversation_id = sqlc.arg(id) AND m.deleted_at IS NULL
) s
WHERE c.organization_id = sqlc.arg(organization_id) AND c.id = sqlc.arg(id)
RETURNING c.*;

-- name: MarkConversationMerged :one
UPDATE conversations SET
    merged_into_id = sqlc.arg(into_id), merged_at = NOW(),
    status = 'closed', status_id = NULL, snoozed_until = NULL,
    resolved_at = COALESCE(resolved_at, NOW()), closed_at = COALESCE(closed_at, NOW()),
    waiting_since = NULL, updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: RepointMergedConversations :exec
-- Conversations merged into the source follow it to the target.
UPDATE conversations SET merged_into_id = sqlc.arg(into_id)
WHERE organization_id = sqlc.arg(organization_id) AND merged_into_id = sqlc.arg(from_id);

-- name: CopyConversationTags :exec
INSERT INTO conversation_tags (organization_id, conversation_id, tag_id)
SELECT t.organization_id, sqlc.arg(into_id)::bigint, t.tag_id FROM conversation_tags t
WHERE t.organization_id = sqlc.arg(organization_id) AND t.conversation_id = sqlc.arg(from_id)
ON CONFLICT DO NOTHING;

-- name: CopyConversationParticipants :exec
INSERT INTO conversation_participants (organization_id, conversation_id, user_id, role, added_by)
SELECT p.organization_id, sqlc.arg(into_id)::bigint, p.user_id, p.role, p.added_by FROM conversation_participants p
WHERE p.organization_id = sqlc.arg(organization_id) AND p.conversation_id = sqlc.arg(from_id)
ON CONFLICT DO NOTHING;

-- name: MoveSideConversations :exec
UPDATE side_conversations SET conversation_id = sqlc.arg(into_id), updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND conversation_id = sqlc.arg(from_id);

-- name: MoveConversationLinks :exec
-- Links of a merged conversation are dropped: the target keeps its own and
-- the source stays reachable through merged_into_id.
DELETE FROM conversation_links
WHERE organization_id = sqlc.arg(organization_id) AND (from_id = sqlc.arg(id) OR to_id = sqlc.arg(id));

-- name: InsertConversationLink :one
INSERT INTO conversation_links (organization_id, from_id, to_id, kind, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(from_id), sqlc.arg(to_id), sqlc.arg(kind), sqlc.narg(created_by))
ON CONFLICT (organization_id, from_id, to_id) DO UPDATE SET kind = EXCLUDED.kind
RETURNING *;

-- name: DeleteConversationLink :execrows
DELETE FROM conversation_links
WHERE organization_id = sqlc.arg(organization_id)
  AND ((from_id = sqlc.arg(a_id) AND to_id = sqlc.arg(b_id)) OR (from_id = sqlc.arg(b_id) AND to_id = sqlc.arg(a_id)));

-- name: ConversationHasParent :one
SELECT EXISTS (
    SELECT 1 FROM conversation_links
    WHERE organization_id = sqlc.arg(organization_id) AND to_id = sqlc.arg(id) AND kind = 'parent'
)::boolean;

-- name: GetConversationParentID :one
SELECT from_id FROM conversation_links
WHERE organization_id = sqlc.arg(organization_id) AND to_id = sqlc.arg(id) AND kind = 'parent';

-- name: ListConversationLinks :many
-- Links of a conversation seen from it: relation is parent (the other one
-- is its parent), child or related.
SELECT CASE WHEN l.kind = 'related' THEN 'related'
            WHEN l.to_id = sqlc.arg(id) THEN 'parent'
            ELSE 'child' END::text AS relation,
       o.uuid, o.number, o.subject, o.status, o.channel_type, o.last_message_at,
       l.created_at
FROM conversation_links l
JOIN conversations o ON o.organization_id = l.organization_id
 AND o.id = CASE WHEN l.from_id = sqlc.arg(id) THEN l.to_id ELSE l.from_id END
WHERE l.organization_id = sqlc.arg(organization_id)
  AND (l.from_id = sqlc.arg(id) OR l.to_id = sqlc.arg(id))
  AND o.deleted_at IS NULL
ORDER BY l.created_at;

-- name: ListMergedIntoConversation :many
SELECT uuid, number, subject, status, channel_type, last_message_at, merged_at
FROM conversations
WHERE organization_id = sqlc.arg(organization_id) AND merged_into_id = sqlc.arg(id) AND deleted_at IS NULL
ORDER BY merged_at;

-- name: GetConversationBrief :one
SELECT uuid, number, subject, status, channel_type, last_message_at
FROM conversations
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: InsertSideConversation :one
INSERT INTO side_conversations (organization_id, conversation_id, channel_instance_id, subject, participants, thread_root, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(conversation_id), sqlc.narg(channel_instance_id), sqlc.arg(subject),
        sqlc.arg(participants)::text[], sqlc.arg(thread_root), sqlc.narg(created_by))
RETURNING *;

-- name: GetSideConversationByUUID :one
SELECT * FROM side_conversations WHERE organization_id = $1 AND uuid = $2;

-- name: LockSideConversation :one
SELECT * FROM side_conversations WHERE organization_id = $1 AND id = $2 FOR UPDATE;

-- name: FindSideConversationByThread :one
SELECT * FROM side_conversations WHERE organization_id = $1 AND thread_root = $2;

-- name: ListSideConversations :many
SELECT s.*, u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname
FROM side_conversations s
LEFT JOIN users u ON u.id = s.created_by
WHERE s.organization_id = sqlc.arg(organization_id) AND s.conversation_id = sqlc.arg(conversation_id)
ORDER BY s.created_at DESC;

-- name: UpdateSideConversationStatus :one
UPDATE side_conversations SET status = sqlc.arg(status), updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: TouchSideConversation :one
UPDATE side_conversations SET
    message_count = message_count + 1,
    last_message_at = GREATEST(last_message_at, sqlc.arg(at)::timestamptz),
    participants = ARRAY(SELECT DISTINCT unnest(participants || sqlc.arg(participants)::text[])),
    status = CASE WHEN sqlc.arg(reopen)::boolean THEN 'open' ELSE status END,
    updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: InsertSideConversationMessage :one
INSERT INTO side_conversation_messages (organization_id, side_conversation_id, direction, sender_user_id,
    from_address, from_name, recipients, content, status, external_id, created_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(side_conversation_id), sqlc.arg(direction), sqlc.narg(sender_user_id),
        sqlc.arg(from_address), sqlc.arg(from_name), sqlc.arg(recipients)::text[], sqlc.arg(content),
        sqlc.arg(status), sqlc.narg(external_id), sqlc.arg(created_at))
ON CONFLICT (organization_id, side_conversation_id, external_id) WHERE external_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: MarkSideMessageSent :one
UPDATE side_conversation_messages SET status = 'sent', external_id = sqlc.narg(external_id), error = ''
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: MarkSideMessageFailed :one
UPDATE side_conversation_messages SET status = 'failed', error = sqlc.arg(error)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: ListSideConversationMessages :many
SELECT m.*, u.uuid AS sender_user_uuid, u.name AS sender_user_name, u.surname AS sender_user_surname
FROM side_conversation_messages m
LEFT JOIN users u ON u.id = m.sender_user_id
WHERE m.organization_id = sqlc.arg(organization_id) AND m.side_conversation_id = sqlc.arg(side_conversation_id)
ORDER BY m.id;

-- name: LastSideMessageExternalID :one
SELECT COALESCE((SELECT external_id FROM side_conversation_messages
                 WHERE organization_id = sqlc.arg(organization_id) AND side_conversation_id = sqlc.arg(side_conversation_id)
                   AND external_id IS NOT NULL AND external_id LIKE '<%'
                 ORDER BY id DESC LIMIT 1), '')::text;

-- name: ListEmailInstances :many
SELECT id, uuid, name FROM channel_instances
WHERE organization_id = sqlc.arg(organization_id) AND channel_type = 'email' AND deleted_at IS NULL
ORDER BY id;
