-- CF-25: conversations, messages, tags, participants, mentions and reads.

-- name: NextConversationNumber :one
INSERT INTO conversation_counters (organization_id, last_number) VALUES ($1, 1)
ON CONFLICT (organization_id) DO UPDATE SET last_number = conversation_counters.last_number + 1
RETURNING last_number;

-- name: InsertConversation :one
INSERT INTO conversations (
    organization_id, number, channel_type, channel_instance_id, contact_id, company_id, subject,
    status, priority, source, assignee_user_id, assignee_team_id, custom_fields, external_thread_id, last_message_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(number), sqlc.arg(channel_type), sqlc.narg(channel_instance_id),
    sqlc.arg(contact_id), sqlc.narg(company_id), sqlc.arg(subject), sqlc.arg(status), sqlc.arg(priority),
    sqlc.arg(source), sqlc.narg(assignee_user_id), sqlc.narg(assignee_team_id), sqlc.arg(custom_fields),
    sqlc.narg(external_thread_id), sqlc.arg(last_message_at)
)
RETURNING *;

-- name: GetConversationByID :one
SELECT * FROM conversations WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: GetConversationByUUID :one
SELECT * FROM conversations WHERE organization_id = $1 AND uuid = $2 AND deleted_at IS NULL;

-- name: LockConversation :one
SELECT * FROM conversations WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL FOR UPDATE;

-- name: FindLatestConversationForContact :one
-- The conversation an inbound message of this contact on this inbox joins
-- (the usecase decides whether a resolved one is reopened or a new one starts).
SELECT * FROM conversations
WHERE organization_id = sqlc.arg(organization_id)
  AND contact_id = sqlc.arg(contact_id)
  AND COALESCE(channel_instance_id, 0) = COALESCE(sqlc.narg(channel_instance_id)::bigint, 0)
  AND deleted_at IS NULL
ORDER BY last_message_at DESC, id DESC
LIMIT 1;

-- name: FindConversationByThread :one
SELECT * FROM conversations
WHERE organization_id = $1 AND channel_instance_id = $2 AND external_thread_id = $3 AND deleted_at IS NULL
ORDER BY id DESC
LIMIT 1;

-- name: GetConversationView :one
SELECT sqlc.embed(c),
       ct.uuid AS contact_uuid, ct.name AS contact_name, ct.email AS contact_email, ct.phone AS contact_phone,
       ct.external_id AS contact_external_id, ct.verified_at AS contact_verified_at,
       co.uuid AS company_uuid, co.name AS company_name, co.external_id AS company_external_id,
       inst.uuid AS channel_instance_uuid, inst.name AS channel_instance_name,
       u.uuid AS assignee_uuid, u.name AS assignee_name, u.surname AS assignee_surname,
       tm.uuid AS team_uuid, tm.name AS team_name,
       st.uuid AS status_uuid, st.name AS status_name, st.color AS status_color,
       COALESCE((SELECT array_agg(t.uuid ORDER BY lower(t.name)) FROM conversation_tags x
                 JOIN tags t ON t.organization_id = x.organization_id AND t.id = x.tag_id
                 WHERE x.organization_id = c.organization_id AND x.conversation_id = c.id), '{}')::uuid[] AS tag_uuids,
       COALESCE((SELECT COUNT(*) FROM messages m
                 WHERE m.organization_id = c.organization_id AND m.conversation_id = c.id
                   AND m.id > COALESCE(r.last_read_message_id, 0)
                   AND m.deleted_at IS NULL
                   AND (m.sender_user_id IS NULL OR m.sender_user_id <> sqlc.arg(viewer_id)::bigint)
                   AND m.direction <> 'internal'), 0)::int AS unread_count
FROM conversations c
JOIN contacts ct ON ct.organization_id = c.organization_id AND ct.id = c.contact_id
LEFT JOIN companies co ON co.organization_id = c.organization_id AND co.id = c.company_id
LEFT JOIN channel_instances inst ON inst.organization_id = c.organization_id AND inst.id = c.channel_instance_id
LEFT JOIN users u ON u.id = c.assignee_user_id
LEFT JOIN teams tm ON tm.organization_id = c.organization_id AND tm.id = c.assignee_team_id
LEFT JOIN conversation_statuses st ON st.organization_id = c.organization_id AND st.id = c.status_id
LEFT JOIN conversation_reads r ON r.organization_id = c.organization_id AND r.conversation_id = c.id AND r.user_id = sqlc.arg(viewer_id)::bigint
WHERE c.organization_id = sqlc.arg(organization_id) AND c.uuid = sqlc.arg(uuid) AND c.deleted_at IS NULL;

-- name: ListConversations :many
-- The inbox list. assignee: '' any, 'me' viewer, 'unassigned', or a user id
-- in assignee_user_id. unread_only keeps rows with messages after the
-- viewer's read marker. match_uuids: conversations the search index
-- matched for q (CF-45, message text); they join the ILIKE matches.
SELECT sqlc.embed(c),
       ct.uuid AS contact_uuid, ct.name AS contact_name, ct.email AS contact_email, ct.phone AS contact_phone,
       ct.external_id AS contact_external_id, ct.verified_at AS contact_verified_at,
       co.uuid AS company_uuid, co.name AS company_name, co.external_id AS company_external_id,
       inst.uuid AS channel_instance_uuid, inst.name AS channel_instance_name,
       u.uuid AS assignee_uuid, u.name AS assignee_name, u.surname AS assignee_surname,
       tm.uuid AS team_uuid, tm.name AS team_name,
       st.uuid AS status_uuid, st.name AS status_name, st.color AS status_color,
       COALESCE((SELECT array_agg(t.uuid ORDER BY lower(t.name)) FROM conversation_tags x
                 JOIN tags t ON t.organization_id = x.organization_id AND t.id = x.tag_id
                 WHERE x.organization_id = c.organization_id AND x.conversation_id = c.id), '{}')::uuid[] AS tag_uuids,
       COALESCE((SELECT COUNT(*) FROM messages m
                 WHERE m.organization_id = c.organization_id AND m.conversation_id = c.id
                   AND m.id > COALESCE(r.last_read_message_id, 0)
                   AND m.deleted_at IS NULL
                   AND (m.sender_user_id IS NULL OR m.sender_user_id <> sqlc.arg(viewer_id)::bigint)
                   AND m.direction <> 'internal'), 0)::int AS unread_count
FROM conversations c
JOIN contacts ct ON ct.organization_id = c.organization_id AND ct.id = c.contact_id
LEFT JOIN companies co ON co.organization_id = c.organization_id AND co.id = c.company_id AND co.deleted_at IS NULL
LEFT JOIN channel_instances inst ON inst.organization_id = c.organization_id AND inst.id = c.channel_instance_id
LEFT JOIN users u ON u.id = c.assignee_user_id
LEFT JOIN teams tm ON tm.organization_id = c.organization_id AND tm.id = c.assignee_team_id
LEFT JOIN conversation_statuses st ON st.organization_id = c.organization_id AND st.id = c.status_id
LEFT JOIN conversation_reads r ON r.organization_id = c.organization_id AND r.conversation_id = c.id AND r.user_id = sqlc.arg(viewer_id)::bigint
WHERE c.organization_id = sqlc.arg(organization_id)
 AND (sqlc.narg(brand_id)::uuid IS NULL OR c.brand_id=sqlc.narg(brand_id)::uuid)
  AND c.deleted_at IS NULL
  -- Team visibility (CF-44, docs/design/tenant-core.md §9): without
  -- tenant.conversations.read_all a member sees its own, its teams', the
  -- unrouted queue (no agent, no team) and the ones it participates in.
  AND (sqlc.arg(see_all)::boolean
       OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
       OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
       OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
       OR EXISTS (SELECT 1 FROM conversation_participants vp
                  WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                    AND vp.user_id = sqlc.arg(viewer_id)::bigint))
  AND (cardinality(sqlc.arg(statuses)::text[]) = 0 OR c.status = ANY(sqlc.arg(statuses)::text[]))
  AND (cardinality(sqlc.arg(priorities)::text[]) = 0 OR c.priority = ANY(sqlc.arg(priorities)::text[]))
  AND (cardinality(sqlc.arg(channel_types)::text[]) = 0 OR c.channel_type = ANY(sqlc.arg(channel_types)::text[]))
  AND (cardinality(sqlc.arg(channel_instance_ids)::bigint[]) = 0 OR c.channel_instance_id = ANY(sqlc.arg(channel_instance_ids)::bigint[]))
  AND (sqlc.arg(assignee)::text = ''
       OR (sqlc.arg(assignee)::text = 'me' AND c.assignee_user_id = sqlc.arg(viewer_id)::bigint)
       OR (sqlc.arg(assignee)::text = 'unassigned' AND c.assignee_user_id IS NULL)
       OR (sqlc.arg(assignee)::text = 'user' AND c.assignee_user_id = sqlc.narg(assignee_user_id)::bigint))
  AND (sqlc.narg(team_id)::bigint IS NULL OR c.assignee_team_id = sqlc.narg(team_id))
  AND (NOT sqlc.arg(my_teams)::boolean OR c.assignee_team_id IN (
        SELECT mt.team_id FROM team_members mt WHERE mt.organization_id = c.organization_id AND mt.user_id = sqlc.arg(viewer_id)::bigint))
  AND (sqlc.narg(contact_id)::bigint IS NULL OR c.contact_id = sqlc.narg(contact_id))
  AND (sqlc.narg(company_id)::bigint IS NULL OR c.company_id = sqlc.narg(company_id))
  AND (cardinality(sqlc.arg(tag_ids)::bigint[]) = 0 OR EXISTS (
        SELECT 1 FROM conversation_tags x WHERE x.organization_id = c.organization_id AND x.conversation_id = c.id AND x.tag_id = ANY(sqlc.arg(tag_ids)::bigint[])))
  AND (NOT sqlc.arg(mentioned)::boolean OR EXISTS (
        SELECT 1 FROM message_mentions mm JOIN messages m2 ON m2.organization_id = mm.organization_id AND m2.id = mm.message_id
        WHERE m2.organization_id = c.organization_id AND m2.conversation_id = c.id AND mm.user_id = sqlc.arg(viewer_id)::bigint))
  AND (NOT sqlc.arg(unread_only)::boolean OR EXISTS (
        SELECT 1 FROM messages m3
        WHERE m3.organization_id = c.organization_id AND m3.conversation_id = c.id
          AND m3.id > COALESCE(r.last_read_message_id, 0) AND m3.direction = 'in'))
  AND (sqlc.narg(sla_breached)::boolean IS NULL OR (c.sla_breached_at IS NOT NULL) = sqlc.narg(sla_breached))
  -- CF-195: who the contact is (verified with user_hash, anonymous, or
  -- named by itself without proof).
  AND (sqlc.narg(identity_state)::text IS NULL
       OR (sqlc.narg(identity_state)::text = 'verified' AND ct.verified_at IS NOT NULL)
       OR (sqlc.narg(identity_state)::text = 'anonymous' AND ct.verified_at IS NULL AND ct.name = '' AND ct.email IS NULL AND ct.phone IS NULL)
       OR (sqlc.narg(identity_state)::text = 'self_named' AND ct.verified_at IS NULL AND (ct.name <> '' OR ct.email IS NOT NULL OR ct.phone IS NOT NULL)))
  -- CF-245: the REST API channel's own thread id.
  AND (sqlc.narg(external_thread_id)::text IS NULL OR c.external_thread_id = sqlc.narg(external_thread_id)::text)
  AND (sqlc.narg(sla_due_before)::timestamptz IS NULL OR (
        c.sla_breached_at IS NULL AND c.status IN ('open', 'pending')
        AND LEAST(c.first_response_due_at, c.next_response_due_at, c.resolution_due_at) <= sqlc.narg(sla_due_before)::timestamptz))
  AND (sqlc.narg(q)::text IS NULL
       OR c.subject ILIKE '%' || sqlc.narg(q) || '%'
       OR c.last_message_preview ILIKE '%' || sqlc.narg(q) || '%'
       OR ct.name ILIKE '%' || sqlc.narg(q) || '%'
       OR ct.email ILIKE '%' || sqlc.narg(q) || '%'
       OR ct.phone ILIKE '%' || sqlc.narg(q) || '%'
       OR ct.external_id = sqlc.narg(q)
       OR c.number::text = sqlc.narg(q)
       OR c.uuid = ANY(sqlc.arg(match_uuids)::uuid[]))
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'last_message_at' AND sqlc.arg(sort_desc)::boolean THEN c.last_message_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_message_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.last_message_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN c.created_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'waiting_since' AND sqlc.arg(sort_desc)::boolean THEN c.waiting_since END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'waiting_since' AND NOT sqlc.arg(sort_desc)::boolean THEN c.waiting_since END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'priority' AND sqlc.arg(sort_desc)::boolean
       THEN CASE c.priority WHEN 'urgent' THEN 4 WHEN 'high' THEN 3 WHEN 'normal' THEN 2 ELSE 1 END END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'priority' AND NOT sqlc.arg(sort_desc)::boolean
       THEN CASE c.priority WHEN 'urgent' THEN 4 WHEN 'high' THEN 3 WHEN 'normal' THEN 2 ELSE 1 END END ASC,
  c.last_message_at DESC,
  c.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountConversations :one
SELECT COUNT(*)::bigint
FROM conversations c
JOIN contacts ct ON ct.organization_id = c.organization_id AND ct.id = c.contact_id
LEFT JOIN conversation_reads r ON r.organization_id = c.organization_id AND r.conversation_id = c.id AND r.user_id = sqlc.arg(viewer_id)::bigint
WHERE c.organization_id = sqlc.arg(organization_id)
 AND (sqlc.narg(brand_id)::uuid IS NULL OR c.brand_id=sqlc.narg(brand_id)::uuid)
  AND c.deleted_at IS NULL
  -- Team visibility (CF-44, docs/design/tenant-core.md §9): without
  -- tenant.conversations.read_all a member sees its own, its teams', the
  -- unrouted queue (no agent, no team) and the ones it participates in.
  AND (sqlc.arg(see_all)::boolean
       OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
       OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
       OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
       OR EXISTS (SELECT 1 FROM conversation_participants vp
                  WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                    AND vp.user_id = sqlc.arg(viewer_id)::bigint))
  AND (cardinality(sqlc.arg(statuses)::text[]) = 0 OR c.status = ANY(sqlc.arg(statuses)::text[]))
  AND (cardinality(sqlc.arg(priorities)::text[]) = 0 OR c.priority = ANY(sqlc.arg(priorities)::text[]))
  AND (cardinality(sqlc.arg(channel_types)::text[]) = 0 OR c.channel_type = ANY(sqlc.arg(channel_types)::text[]))
  AND (cardinality(sqlc.arg(channel_instance_ids)::bigint[]) = 0 OR c.channel_instance_id = ANY(sqlc.arg(channel_instance_ids)::bigint[]))
  AND (sqlc.arg(assignee)::text = ''
       OR (sqlc.arg(assignee)::text = 'me' AND c.assignee_user_id = sqlc.arg(viewer_id)::bigint)
       OR (sqlc.arg(assignee)::text = 'unassigned' AND c.assignee_user_id IS NULL)
       OR (sqlc.arg(assignee)::text = 'user' AND c.assignee_user_id = sqlc.narg(assignee_user_id)::bigint))
  AND (sqlc.narg(team_id)::bigint IS NULL OR c.assignee_team_id = sqlc.narg(team_id))
  AND (NOT sqlc.arg(my_teams)::boolean OR c.assignee_team_id IN (
        SELECT mt.team_id FROM team_members mt WHERE mt.organization_id = c.organization_id AND mt.user_id = sqlc.arg(viewer_id)::bigint))
  AND (sqlc.narg(contact_id)::bigint IS NULL OR c.contact_id = sqlc.narg(contact_id))
  AND (sqlc.narg(company_id)::bigint IS NULL OR c.company_id = sqlc.narg(company_id))
  AND (cardinality(sqlc.arg(tag_ids)::bigint[]) = 0 OR EXISTS (
        SELECT 1 FROM conversation_tags x WHERE x.organization_id = c.organization_id AND x.conversation_id = c.id AND x.tag_id = ANY(sqlc.arg(tag_ids)::bigint[])))
  AND (NOT sqlc.arg(mentioned)::boolean OR EXISTS (
        SELECT 1 FROM message_mentions mm JOIN messages m2 ON m2.organization_id = mm.organization_id AND m2.id = mm.message_id
        WHERE m2.organization_id = c.organization_id AND m2.conversation_id = c.id AND mm.user_id = sqlc.arg(viewer_id)::bigint))
  AND (NOT sqlc.arg(unread_only)::boolean OR EXISTS (
        SELECT 1 FROM messages m3
        WHERE m3.organization_id = c.organization_id AND m3.conversation_id = c.id
          AND m3.id > COALESCE(r.last_read_message_id, 0) AND m3.direction = 'in'))
  AND (sqlc.narg(sla_breached)::boolean IS NULL OR (c.sla_breached_at IS NOT NULL) = sqlc.narg(sla_breached))
  -- CF-195: who the contact is (verified with user_hash, anonymous, or
  -- named by itself without proof).
  AND (sqlc.narg(identity_state)::text IS NULL
       OR (sqlc.narg(identity_state)::text = 'verified' AND ct.verified_at IS NOT NULL)
       OR (sqlc.narg(identity_state)::text = 'anonymous' AND ct.verified_at IS NULL AND ct.name = '' AND ct.email IS NULL AND ct.phone IS NULL)
       OR (sqlc.narg(identity_state)::text = 'self_named' AND ct.verified_at IS NULL AND (ct.name <> '' OR ct.email IS NOT NULL OR ct.phone IS NOT NULL)))
  -- CF-245: the REST API channel's own thread id.
  AND (sqlc.narg(external_thread_id)::text IS NULL OR c.external_thread_id = sqlc.narg(external_thread_id)::text)
  AND (sqlc.narg(sla_due_before)::timestamptz IS NULL OR (
        c.sla_breached_at IS NULL AND c.status IN ('open', 'pending')
        AND LEAST(c.first_response_due_at, c.next_response_due_at, c.resolution_due_at) <= sqlc.narg(sla_due_before)::timestamptz))
  AND (sqlc.narg(q)::text IS NULL
       OR c.subject ILIKE '%' || sqlc.narg(q) || '%'
       OR c.last_message_preview ILIKE '%' || sqlc.narg(q) || '%'
       OR ct.name ILIKE '%' || sqlc.narg(q) || '%'
       OR ct.email ILIKE '%' || sqlc.narg(q) || '%'
       OR ct.phone ILIKE '%' || sqlc.narg(q) || '%'
       OR ct.external_id = sqlc.narg(q)
       OR c.number::text = sqlc.narg(q)
       OR c.uuid = ANY(sqlc.arg(match_uuids)::uuid[]));

-- name: ConversationInboxCounts :one
-- Sidebar badges of the inbox for one agent (open work only).
SELECT
    COUNT(*) FILTER (WHERE c.status IN ('open', 'pending'))::bigint AS open_total,
    COUNT(*) FILTER (WHERE c.status IN ('open', 'pending') AND c.assignee_user_id = sqlc.arg(viewer_id)::bigint)::bigint AS mine,
    COUNT(*) FILTER (WHERE c.status IN ('open', 'pending') AND c.assignee_user_id IS NULL)::bigint AS unassigned,
    COUNT(*) FILTER (WHERE c.status = 'snoozed')::bigint AS snoozed,
    COUNT(*) FILTER (WHERE c.status IN ('open', 'pending') AND c.sla_breached_at IS NOT NULL)::bigint AS breached
FROM conversations c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL
  -- Team visibility (CF-44, docs/design/tenant-core.md §9): without
  -- tenant.conversations.read_all a member sees its own, its teams', the
  -- unrouted queue (no agent, no team) and the ones it participates in.
  AND (sqlc.arg(see_all)::boolean
       OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
       OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
       OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
       OR EXISTS (SELECT 1 FROM conversation_participants vp
                  WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                    AND vp.user_id = sqlc.arg(viewer_id)::bigint));

-- name: ConversationStatusFacets :many
SELECT c.status, COUNT(*)::bigint AS count FROM conversations c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL
  -- Team visibility (CF-44, docs/design/tenant-core.md §9): without
  -- tenant.conversations.read_all a member sees its own, its teams', the
  -- unrouted queue (no agent, no team) and the ones it participates in.
  AND (sqlc.arg(see_all)::boolean
       OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
       OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
       OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
       OR EXISTS (SELECT 1 FROM conversation_participants vp
                  WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                    AND vp.user_id = sqlc.arg(viewer_id)::bigint))
GROUP BY c.status;

-- name: UpdateConversationState :one
-- One write for status, priority, assignment, subject and SLA due dates; the
-- usecase loads the row, applies the change and writes every column back.
UPDATE conversations SET
    subject = sqlc.arg(subject),
    status = sqlc.arg(status),
    status_id = sqlc.narg(status_id),
    priority = sqlc.arg(priority),
    assignee_user_id = sqlc.narg(assignee_user_id),
    assignee_team_id = sqlc.narg(assignee_team_id),
    company_id = sqlc.narg(company_id),
    custom_fields = sqlc.arg(custom_fields),
    snoozed_until = sqlc.narg(snoozed_until),
    resolved_at = sqlc.narg(resolved_at),
    closed_at = sqlc.narg(closed_at),
    -- Nobody waits on a resolved or closed conversation; a new inbound
    -- message reopens it and starts the wait again.
    waiting_since = CASE WHEN sqlc.arg(status)::text IN ('resolved', 'closed') THEN NULL ELSE waiting_since END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SetConversationSLA :exec
UPDATE conversations SET
    sla_policy_id = sqlc.narg(sla_policy_id),
    first_response_due_at = sqlc.narg(first_response_due_at),
    next_response_due_at = sqlc.narg(next_response_due_at),
    resolution_due_at = sqlc.narg(resolution_due_at),
    sla_breached_at = sqlc.narg(sla_breached_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ApplyInboundToConversation :one
-- A contact wrote: activity moves up, the conversation waits for an answer
-- and a snoozed or resolved conversation wakes up.
UPDATE conversations SET
    last_message_at = GREATEST(last_message_at, sqlc.arg(at)::timestamptz),
    last_inbound_at = GREATEST(COALESCE(last_inbound_at, sqlc.arg(at)::timestamptz), sqlc.arg(at)::timestamptz),
    last_message_preview = sqlc.arg(preview)::text,
    last_message_direction = 'in',
    waiting_since = COALESCE(waiting_since, sqlc.arg(at)::timestamptz),
    message_count = message_count + 1,
    status = CASE WHEN status IN ('snoozed', 'resolved') THEN 'open' ELSE status END,
    status_id = CASE WHEN status IN ('snoozed', 'resolved') THEN NULL ELSE status_id END,
    snoozed_until = CASE WHEN status IN ('snoozed', 'resolved') THEN NULL ELSE snoozed_until END,
    resolved_at = CASE WHEN status IN ('snoozed', 'resolved') THEN NULL ELSE resolved_at END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: ApplyOutboundToConversation :one
-- An agent or bot answered: the first agent answer stamps first_response_at
-- and the contact is no longer waiting.
UPDATE conversations SET
    last_message_at = GREATEST(last_message_at, sqlc.arg(at)::timestamptz),
    last_outbound_at = GREATEST(COALESCE(last_outbound_at, sqlc.arg(at)::timestamptz), sqlc.arg(at)::timestamptz),
    last_message_preview = sqlc.arg(preview)::text,
    last_message_direction = 'out',
    waiting_since = NULL,
    first_response_at = CASE WHEN sqlc.arg(by_agent)::boolean THEN COALESCE(first_response_at, sqlc.arg(at)::timestamptz)
                             ELSE first_response_at END,
    message_count = message_count + 1
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: ApplyInternalToConversation :exec
UPDATE conversations SET message_count = message_count + 1
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ConversationVisibleTo :one
-- The single-conversation form of the team visibility rule above (detail,
-- messages, replies, realtime conversation channel).
SELECT EXISTS (
    SELECT 1 FROM conversations c
    WHERE c.organization_id = sqlc.arg(organization_id) AND c.id = sqlc.arg(id)
      AND (sqlc.arg(see_all)::boolean
           OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
           OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
           OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
           OR EXISTS (SELECT 1 FROM conversation_participants vp
                      WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                        AND vp.user_id = sqlc.arg(viewer_id)::bigint))
)::boolean AS visible;

-- name: FilterVisibleConversationUUIDs :many
-- The conversations among uuids the viewer may see (search hits).
SELECT c.uuid FROM conversations c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = ANY(sqlc.arg(uuids)::uuid[])
  AND c.deleted_at IS NULL
  AND (sqlc.arg(see_all)::boolean
       OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
       OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
       OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
       OR EXISTS (SELECT 1 FROM conversation_participants vp
                  WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                    AND vp.user_id = sqlc.arg(viewer_id)::bigint));

-- name: ListConversationAudience :many
-- Members who may see a conversation without read_all: the assignee, the
-- assigned team's members and the participants; every member for an
-- unrouted conversation (realtime fan-out to inbox:{org}:{user}).
SELECT u.uuid FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = sqlc.arg(organization_id)
  AND u.status = 'active'
  AND (
    (sqlc.narg(assignee_user_id)::bigint IS NULL AND sqlc.narg(assignee_team_id)::bigint IS NULL)
    OR om.user_id = sqlc.narg(assignee_user_id)::bigint
    OR EXISTS (SELECT 1 FROM team_members tm
               WHERE tm.organization_id = om.organization_id AND tm.user_id = om.user_id
                 AND tm.team_id = sqlc.narg(assignee_team_id)::bigint)
    OR EXISTS (SELECT 1 FROM conversation_participants cp
               WHERE cp.organization_id = om.organization_id AND cp.user_id = om.user_id
                 AND cp.conversation_id = sqlc.arg(conversation_id)::bigint)
  );

-- name: WakeSnoozedConversations :many
-- Snoozes that ran out go back to open (the worker publishes the events).
UPDATE conversations SET status = 'open', status_id = NULL, snoozed_until = NULL
WHERE status = 'snoozed' AND snoozed_until <= sqlc.arg(now)::timestamptz AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteConversation :one
UPDATE conversations SET deleted_at = NOW()
WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: InsertConversationStatus :one
INSERT INTO conversation_statuses (organization_id, name, slug, category, color, sort_order, is_default)
VALUES (sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(slug), sqlc.arg(category), sqlc.arg(color),
        sqlc.arg(sort_order), sqlc.arg(is_default))
RETURNING *;

-- name: ListConversationStatuses :many
SELECT * FROM conversation_statuses WHERE organization_id = $1 ORDER BY category, sort_order, id;

-- name: GetConversationStatusByUUID :one
SELECT * FROM conversation_statuses WHERE organization_id = $1 AND uuid = $2;

-- name: GetConversationStatusByID :one
SELECT * FROM conversation_statuses WHERE organization_id = $1 AND id = $2;

-- name: ListConversationStatusesWithUsage :many
-- The settings screen (CF-249): every custom status with the number of
-- conversations that use it.
SELECT st.*,
       (SELECT COUNT(*) FROM conversations c
        WHERE c.organization_id = st.organization_id AND c.status_id = st.id AND c.deleted_at IS NULL)::bigint AS usage_count
FROM conversation_statuses st
WHERE st.organization_id = $1
ORDER BY st.sort_order, st.id;

-- name: NextConversationStatusOrder :one
SELECT COALESCE(MAX(sort_order), -1)::int + 1 FROM conversation_statuses WHERE organization_id = $1;

-- name: ConversationStatusSlugTaken :one
SELECT EXISTS (SELECT 1 FROM conversation_statuses WHERE organization_id = $1 AND slug = $2)::boolean;

-- name: UpdateConversationStatusDef :one
UPDATE conversation_statuses SET name = sqlc.arg(name), color = sqlc.arg(color), category = sqlc.arg(category),
    sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: ClearConversationStatusMismatch :execrows
-- A custom status moved to another base category: conversations whose base
-- status no longer matches lose it (they keep their base status).
UPDATE conversations SET status_id = NULL
WHERE organization_id = sqlc.arg(organization_id) AND status_id = sqlc.arg(status_id) AND status <> sqlc.arg(category)::text;

-- name: DeleteConversationStatusDef :execrows
-- Conversations keep their base status; status_id is set NULL by the FK.
DELETE FROM conversation_statuses WHERE organization_id = $1 AND id = $2;

-- Tags --------------------------------------------------------------------------

-- name: UpsertTag :one
INSERT INTO tags (organization_id, name, color) VALUES (sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(color))
ON CONFLICT (organization_id, lower(name)) DO UPDATE SET name = tags.name
RETURNING *;

-- name: GetTagByUUID :one
SELECT * FROM tags WHERE organization_id = $1 AND uuid = $2;

-- name: ListTags :many
SELECT t.*, (SELECT COUNT(*) FROM conversation_tags x WHERE x.organization_id = t.organization_id AND x.tag_id = t.id)::bigint AS usage_count
FROM tags t WHERE t.organization_id = $1 ORDER BY lower(t.name);

-- name: UpdateTag :one
UPDATE tags SET name = sqlc.arg(name), color = sqlc.arg(color)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteTag :execrows
DELETE FROM tags WHERE organization_id = $1 AND id = $2;

-- name: AddConversationTag :execrows
INSERT INTO conversation_tags (organization_id, conversation_id, tag_id, created_by)
VALUES ($1, $2, $3, sqlc.narg(created_by))
ON CONFLICT DO NOTHING;

-- name: RemoveConversationTag :execrows
DELETE FROM conversation_tags WHERE organization_id = $1 AND conversation_id = $2 AND tag_id = $3;

-- name: ListConversationTags :many
SELECT t.* FROM conversation_tags x
JOIN tags t ON t.organization_id = x.organization_id AND t.id = x.tag_id
WHERE x.organization_id = $1 AND x.conversation_id = $2
ORDER BY lower(t.name);

-- Participants, reads, mentions -------------------------------------------------

-- name: AddConversationParticipant :execrows
INSERT INTO conversation_participants (organization_id, conversation_id, user_id, role, added_by)
VALUES ($1, $2, $3, $4, sqlc.narg(added_by))
ON CONFLICT (conversation_id, user_id) DO NOTHING;

-- name: RemoveConversationParticipant :execrows
DELETE FROM conversation_participants WHERE organization_id = $1 AND conversation_id = $2 AND user_id = $3;

-- name: ListConversationParticipants :many
SELECT p.user_id, p.role, p.created_at, u.uuid AS user_uuid, u.name AS user_name, u.surname AS user_surname, u.email AS user_email
FROM conversation_participants p
JOIN users u ON u.id = p.user_id
WHERE p.organization_id = $1 AND p.conversation_id = $2
ORDER BY p.created_at;

-- name: MarkConversationRead :exec
-- The marker only moves forward.
INSERT INTO conversation_reads (organization_id, conversation_id, user_id, last_read_message_id, last_read_at)
VALUES ($1, $2, $3, sqlc.arg(message_id), NOW())
ON CONFLICT (conversation_id, user_id) DO UPDATE SET
    last_read_message_id = GREATEST(conversation_reads.last_read_message_id, EXCLUDED.last_read_message_id),
    last_read_at = NOW();

-- name: GetConversationReadMarker :one
-- The viewer's read marker (0 when it never opened the conversation).
SELECT COALESCE((SELECT r.last_read_message_id FROM conversation_reads r
                 WHERE r.organization_id = $1 AND r.conversation_id = $2 AND r.user_id = $3), 0)::bigint;

-- name: UnreadTargetMessageID :one
-- CF-247: the read marker that leaves the latest customer message unread
-- (without one, the latest message the viewer did not write); 0 when
-- the conversation has no such message.
SELECT GREATEST(COALESCE(
    (SELECT MAX(m.id) FROM messages m
     WHERE m.organization_id = $1 AND m.conversation_id = $2 AND m.direction = 'in' AND m.deleted_at IS NULL),
    (SELECT MAX(m.id) FROM messages m
     WHERE m.organization_id = $1 AND m.conversation_id = $2 AND m.direction <> 'internal' AND m.deleted_at IS NULL
       AND (m.sender_user_id IS NULL OR m.sender_user_id <> sqlc.arg(viewer_id)::bigint)),
    1) - 1, 0)::bigint;

-- name: SetConversationReadMarker :exec
-- CF-247: puts the marker exactly at message_id (mark unread, undo).
INSERT INTO conversation_reads (organization_id, conversation_id, user_id, last_read_message_id, last_read_at)
VALUES ($1, $2, $3, sqlc.arg(message_id), NOW())
ON CONFLICT (conversation_id, user_id) DO UPDATE SET
    last_read_message_id = EXCLUDED.last_read_message_id,
    last_read_at = NOW();

-- name: InsertMessageMention :execrows
INSERT INTO message_mentions (organization_id, message_id, user_id) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: ListMessageMentions :many
SELECT mm.message_id, u.uuid AS user_uuid, u.name AS user_name, u.surname AS user_surname
FROM message_mentions mm JOIN users u ON u.id = mm.user_id
WHERE mm.organization_id = $1 AND mm.message_id = ANY(sqlc.arg(message_ids)::bigint[]);

-- Messages ----------------------------------------------------------------------

-- name: InsertMessage :one
-- ON CONFLICT DO NOTHING covers both idempotency keys (external_id per
-- instance, client_id per conversation): a duplicate returns no row and the
-- caller loads the stored one.
INSERT INTO messages (
    organization_id, conversation_id, channel_instance_id, direction, sender_type, sender_user_id,
    sender_contact_id, kind, content, content_attributes, private, status, external_id, client_id,
    reply_to_id, sent_at, created_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(conversation_id), sqlc.narg(channel_instance_id), sqlc.arg(direction),
    sqlc.arg(sender_type), sqlc.narg(sender_user_id), sqlc.narg(sender_contact_id), sqlc.arg(kind),
    sqlc.arg(content), sqlc.arg(content_attributes), sqlc.arg(private), sqlc.arg(status), sqlc.narg(external_id),
    sqlc.narg(client_id), sqlc.narg(reply_to_id), sqlc.narg(sent_at), sqlc.arg(created_at)
)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetMessageByExternal :one
SELECT * FROM messages
WHERE organization_id = $1 AND channel_instance_id = $2 AND external_id = $3;

-- name: GetMessageByClientID :one
SELECT * FROM messages WHERE organization_id = $1 AND conversation_id = $2 AND client_id = $3;

-- name: GetMessageByUUID :one
SELECT * FROM messages WHERE organization_id = $1 AND uuid = $2 AND deleted_at IS NULL;

-- name: GetMessageByID :one
SELECT * FROM messages WHERE organization_id = $1 AND id = $2;

-- name: ListConversationMessages :many
-- Newest first, keyset paginated by id (before_id = 0 starts at the newest).
SELECT m.*, u.uuid AS sender_user_uuid, u.name AS sender_user_name, u.surname AS sender_user_surname,
       ct.uuid AS sender_contact_uuid, ct.name AS sender_contact_name,
       rt.uuid AS reply_to_uuid
FROM messages m
LEFT JOIN users u ON u.id = m.sender_user_id
LEFT JOIN contacts ct ON ct.organization_id = m.organization_id AND ct.id = m.sender_contact_id
LEFT JOIN messages rt ON rt.organization_id = m.organization_id AND rt.id = m.reply_to_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.conversation_id = sqlc.arg(conversation_id)
  AND m.deleted_at IS NULL
  AND (sqlc.arg(before_id)::bigint = 0 OR m.id < sqlc.arg(before_id)::bigint)
  AND (sqlc.arg(after_id)::bigint = 0 OR m.id > sqlc.arg(after_id)::bigint)
ORDER BY m.id DESC
LIMIT sqlc.arg(row_limit);

-- name: LatestMessageID :one
SELECT COALESCE(MAX(id), 0)::bigint FROM messages WHERE organization_id = $1 AND conversation_id = $2;

-- name: MarkMessageSent :one
UPDATE messages SET status = 'sent', external_id = sqlc.narg(external_id), sent_at = sqlc.arg(at)::timestamptz, error = ''
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status IN ('pending', 'queued', 'failed')
RETURNING *;

-- name: MarkMessageFailed :one
UPDATE messages SET status = 'failed', error = sqlc.arg(error)::text
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status IN ('pending', 'queued', 'sent')
RETURNING *;

-- name: MarkMessageQueued :one
UPDATE messages SET status = 'queued', error = ''
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status IN ('pending', 'failed')
RETURNING *;

-- name: UpdateMessageDeliveryByExternal :one
-- Provider receipts only move a message forward (sent → delivered → read);
-- a late "delivered" after "read" is ignored. failed wins unless the
-- message was already delivered or read.
UPDATE messages SET
    status = sqlc.arg(status)::text,
    error = CASE WHEN sqlc.arg(status)::text = 'failed' THEN sqlc.arg(error)::text ELSE error END,
    delivered_at = CASE WHEN sqlc.arg(status)::text IN ('delivered', 'read') THEN COALESCE(delivered_at, sqlc.arg(at)::timestamptz) ELSE delivered_at END,
    read_at = CASE WHEN sqlc.arg(status)::text = 'read' THEN COALESCE(read_at, sqlc.arg(at)::timestamptz) ELSE read_at END
WHERE organization_id = sqlc.arg(organization_id)
  AND channel_instance_id = sqlc.arg(channel_instance_id)
  AND external_id = sqlc.arg(external_id)::text
  AND direction = 'out'
  AND (
    (sqlc.arg(status)::text = 'failed' AND status NOT IN ('delivered', 'read', 'failed'))
    OR (sqlc.arg(status)::text = 'delivered' AND status IN ('pending', 'queued', 'sent'))
    OR (sqlc.arg(status)::text = 'read' AND status IN ('pending', 'queued', 'sent', 'delivered'))
  )
RETURNING *;

-- name: SoftDeleteMessage :one
UPDATE messages SET deleted_at = NOW()
WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: ClaimMessageForSend :one
-- One delivery attempt owns a message: queued → pending. A message left
-- pending (the provider accepted it but recording failed) is never sent
-- again automatically.
UPDATE messages SET status = 'pending'
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'queued' AND deleted_at IS NULL
RETURNING *;

-- name: ReleaseMessageClaim :exec
UPDATE messages SET status = 'queued', error = sqlc.arg(error)::text
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'pending';
-- Inbox API (CF-43 / CF-44) -----------------------------------------------------

-- name: ListOrganizationAgents :many
-- Members who can be assigned or @mentioned.
SELECT u.id, u.uuid, u.name, u.surname, u.email
FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = $1 AND u.status = 'active'
ORDER BY lower(u.name), lower(u.surname), u.id;

-- name: GetOrganizationAgentByUUID :one
SELECT u.id, u.uuid, u.name, u.surname, u.email
FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = $1 AND u.uuid = $2;

-- name: ListAgentsByUUIDs :many
SELECT u.id, u.uuid, u.name, u.surname
FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = $1 AND u.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: ListMessageAttachments :many
SELECT a.uuid, a.owner_id, a.filename, a.content_type, a.size_bytes, (a.thumb_key IS NOT NULL)::boolean AS has_thumbnail
FROM attachments a
WHERE a.organization_id = $1 AND a.owner_type = 'message' AND a.owner_id = ANY(sqlc.arg(message_uuids)::uuid[])
  AND a.deleted_at IS NULL
ORDER BY a.created_at, a.id;

-- name: AttachToMessage :execrows
-- Composer uploads are stored ownerless first and claimed by the message.
UPDATE attachments SET owner_type = 'message', owner_id = sqlc.arg(message_uuid)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = ANY(sqlc.arg(uuids)::uuid[])
  AND owner_type IS NULL AND deleted_at IS NULL AND uploaded_by = sqlc.arg(uploaded_by);

-- name: GetTagsByUUIDs :many
SELECT * FROM tags WHERE organization_id = $1 AND uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: GetChannelInstancesByUUIDs :many
SELECT id, uuid FROM channel_instances WHERE organization_id = $1 AND uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- Saved views (CF-47) --------------------------------------------------------

-- name: ListConversationViews :many
SELECT * FROM conversation_views
WHERE organization_id = sqlc.arg(organization_id)
  AND (scope <> 'personal' OR owner_user_id = sqlc.arg(viewer_id)::bigint)
ORDER BY position, lower(name), id;

-- name: GetConversationView2 :one
SELECT * FROM conversation_views
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid)
  AND (scope <> 'personal' OR owner_user_id = sqlc.arg(viewer_id)::bigint);

-- name: InsertConversationView :one
INSERT INTO conversation_views (organization_id, scope, owner_user_id, name, icon, filters, position, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(scope), sqlc.narg(owner_user_id), sqlc.arg(name), sqlc.arg(icon),
        sqlc.arg(filters), sqlc.arg(position), sqlc.narg(created_by))
RETURNING *;

-- name: UpdateConversationViewRow :one
UPDATE conversation_views SET name = sqlc.arg(name), icon = sqlc.arg(icon), filters = sqlc.arg(filters), position = sqlc.arg(position)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteConversationView :execrows
DELETE FROM conversation_views WHERE organization_id = $1 AND id = $2;

-- name: GetTeamRefByUUID :one
SELECT id, uuid, name FROM teams WHERE organization_id = $1 AND uuid = $2;

-- name: GetTeamRefByID :one
SELECT id, uuid, name FROM teams WHERE organization_id = $1 AND id = $2;

-- name: GetConversationExternalRefs :one
-- CF-195: the contact's and company's external ids for event payloads
-- (webhooks, integrations).
SELECT ct.uuid AS contact_uuid, ct.external_id AS contact_external_id, ct.verified_at AS contact_verified_at,
       co.uuid AS company_uuid, co.external_id AS company_external_id
FROM contacts ct
LEFT JOIN companies co ON co.organization_id = ct.organization_id AND co.id = sqlc.narg(company_id)::bigint
WHERE ct.organization_id = sqlc.arg(organization_id) AND ct.id = sqlc.arg(contact_id);
