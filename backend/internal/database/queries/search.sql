-- Search index sources (CF-45). Each query walks one table in id order
-- (keyset: after_id, row_limit) across all organizations for the bulk
-- reindex, or returns the one row named by uuid for an incremental upsert.
-- Every row carries organization_id: the index filters on it.

-- name: SearchConversationDocs :many
SELECT c.id, c.uuid, c.organization_id, c.number, c.subject, c.status, c.priority, c.channel_type,
       c.last_message_preview,
       ct.name AS contact_name, ct.email AS contact_email, ct.phone AS contact_phone,
       COALESCE(co.name, '')::text AS company_name,
       COALESCE(inst.name, '')::text AS inbox_name,
       COALESCE((SELECT string_agg(t.name, ' ' ORDER BY lower(t.name)) FROM conversation_tags x
                 JOIN tags t ON t.organization_id = x.organization_id AND t.id = x.tag_id
                 WHERE x.conversation_id = c.id), '')::text AS tag_names
FROM conversations c
JOIN contacts ct ON ct.organization_id = c.organization_id AND ct.id = c.contact_id
LEFT JOIN companies co ON co.organization_id = c.organization_id AND co.id = c.company_id
LEFT JOIN channel_instances inst ON inst.organization_id = c.organization_id AND inst.id = c.channel_instance_id
WHERE c.deleted_at IS NULL
  AND c.id > sqlc.arg(after_id)::bigint
  AND (sqlc.narg(uuid)::uuid IS NULL OR c.uuid = sqlc.narg(uuid)::uuid)
ORDER BY c.id
LIMIT sqlc.arg(row_limit);

-- name: SearchMessageDocs :many
-- Timeline events and empty messages are not indexed. content is cut to
-- max_content characters: hits link to the conversation, the snippet only
-- needs the start of a long message.
SELECT m.id, m.uuid, m.organization_id, m.direction, m.kind, m.private, m.created_at,
       left(m.content, sqlc.arg(max_content)::int)::text AS content,
       c.uuid AS conversation_uuid, c.number AS conversation_number, c.subject AS conversation_subject,
       ct.name AS contact_name,
       COALESCE(trim(u.name || ' ' || u.surname), '')::text AS sender_user_name
FROM messages m
JOIN conversations c ON c.organization_id = m.organization_id AND c.id = m.conversation_id
JOIN contacts ct ON ct.organization_id = c.organization_id AND ct.id = c.contact_id
LEFT JOIN users u ON u.id = m.sender_user_id
WHERE m.deleted_at IS NULL
  AND c.deleted_at IS NULL
  AND m.kind <> 'event'
  AND m.content <> ''
  AND m.id > sqlc.arg(after_id)::bigint
  AND (sqlc.narg(uuid)::uuid IS NULL OR m.uuid = sqlc.narg(uuid)::uuid)
ORDER BY m.id
LIMIT sqlc.arg(row_limit);

-- name: SearchContactDocs :many
-- Merged (CF-84) and deleted contacts are not indexed.
SELECT ct.id, ct.uuid, ct.organization_id, ct.name, ct.email, ct.phone,
       COALESCE(co.name, '')::text AS company_name
FROM contacts ct
LEFT JOIN companies co ON co.organization_id = ct.organization_id AND co.id = ct.company_id
WHERE ct.deleted_at IS NULL
  AND ct.merged_into_id IS NULL
  AND ct.id > sqlc.arg(after_id)::bigint
  AND (sqlc.narg(uuid)::uuid IS NULL OR ct.uuid = sqlc.narg(uuid)::uuid)
ORDER BY ct.id
LIMIT sqlc.arg(row_limit);

-- name: SearchCompanyDocs :many
SELECT co.id, co.uuid, co.organization_id, co.name, co.domain
FROM companies co
WHERE co.deleted_at IS NULL
  AND co.id > sqlc.arg(after_id)::bigint
  AND (sqlc.narg(uuid)::uuid IS NULL OR co.uuid = sqlc.narg(uuid)::uuid)
ORDER BY co.id
LIMIT sqlc.arg(row_limit);
