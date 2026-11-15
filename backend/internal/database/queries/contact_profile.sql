-- CF-221: contact detail page. Every query is bound to one contact of one
-- organization. Conversation-derived rows apply the inbox team visibility
-- rule (see ConversationVisibleTo): see_all, viewer_id and viewer_team_ids;
-- can_conversations is false when the member may not read conversations.

-- name: CreateContactNote :one
INSERT INTO contact_notes (organization_id, contact_id, author_user_id, body)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetContactNoteByUUID :one
SELECT * FROM contact_notes WHERE organization_id = $1 AND uuid = $2;

-- name: DeleteContactNote :execrows
DELETE FROM contact_notes WHERE organization_id = $1 AND id = $2;

-- name: MoveContactNotes :execrows
-- A merge keeps the notes of the merged contact on the survivor.
UPDATE contact_notes SET contact_id = sqlc.arg(into_id)
WHERE organization_id = sqlc.arg(organization_id) AND contact_id = sqlc.arg(from_id);

-- name: DeleteContactNotes :execrows
-- KVKK erase: notes are free text about the person.
DELETE FROM contact_notes WHERE organization_id = $1 AND contact_id = $2;

-- name: GetContactAISummary :one
SELECT * FROM contact_ai_summaries WHERE organization_id = $1 AND contact_id = $2 AND audience = $3;

-- name: UpsertContactAISummary :one
INSERT INTO contact_ai_summaries (organization_id, contact_id, audience, summary, open_topics, sentiment, locale, run_uuid, source_at, generated_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(contact_id), sqlc.arg(audience), sqlc.arg(summary), sqlc.arg(open_topics), sqlc.arg(sentiment),
        sqlc.arg(locale), sqlc.narg(run_uuid), sqlc.narg(source_at), sqlc.narg(generated_by))
ON CONFLICT (organization_id, contact_id, audience) DO UPDATE
SET summary = EXCLUDED.summary, open_topics = EXCLUDED.open_topics, sentiment = EXCLUDED.sentiment,
    locale = EXCLUDED.locale, run_uuid = EXCLUDED.run_uuid, source_at = EXCLUDED.source_at,
    generated_by = EXCLUDED.generated_by, created_at = NOW()
RETURNING *;

-- name: DeleteContactAISummaries :execrows
-- Erase and merge drop the cached summaries (they name the person).
DELETE FROM contact_ai_summaries WHERE organization_id = $1 AND contact_id = $2;

-- name: GetContactProfileStats :one
WITH conv AS (
    SELECT c.id, c.status, c.created_at, c.last_message_at, c.last_inbound_at, c.sla_breached_at,
           c.first_response_due_at, c.next_response_due_at, c.resolution_due_at, c.first_response_at, c.message_count
    FROM conversations c
    WHERE c.organization_id = sqlc.arg(organization_id) AND c.contact_id = sqlc.arg(contact_id)
      AND c.deleted_at IS NULL AND sqlc.arg(can_conversations)::boolean
      AND (sqlc.arg(see_all)::boolean
           OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
           OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
           OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
           OR EXISTS (SELECT 1 FROM conversation_participants vp
                      WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                        AND vp.user_id = sqlc.arg(viewer_id)::bigint))
), open_conv AS (
    SELECT * FROM conv WHERE status NOT IN ('resolved', 'closed')
)
SELECT
    (SELECT COUNT(*) FROM open_conv)::bigint AS open_conversations,
    (SELECT COUNT(*) FROM conv WHERE status IN ('resolved', 'closed'))::bigint AS closed_conversations,
    (SELECT COUNT(*) FROM open_conv WHERE sla_breached_at IS NOT NULL)::bigint AS sla_breached,
    -- Open, not yet breached, and a response or resolution falls due within an hour.
    (SELECT COUNT(*) FROM open_conv WHERE sla_breached_at IS NULL
        AND LEAST(CASE WHEN first_response_at IS NULL THEN first_response_due_at END,
                  next_response_due_at, resolution_due_at) < NOW() + INTERVAL '1 hour')::bigint AS sla_at_risk,
    (SELECT COALESCE(SUM(message_count), 0) FROM conv)::bigint AS message_count,
    (SELECT MIN(created_at) FROM conv)::timestamptz AS first_conversation_at,
    (SELECT MAX(last_message_at) FROM conv)::timestamptz AS last_conversation_at,
    (SELECT MAX(last_inbound_at) FROM conv)::timestamptz AS last_inbound_at,
    (SELECT COALESCE(AVG(r.score), 0) FROM conversation_ratings r
      WHERE r.organization_id = sqlc.arg(organization_id) AND r.conversation_id IN (SELECT id FROM conv)
        AND r.created_at > NOW() - INTERVAL '90 days')::float8 AS csat_average,
    (SELECT COUNT(*) FROM conversation_ratings r
      WHERE r.organization_id = sqlc.arg(organization_id) AND r.conversation_id IN (SELECT id FROM conv)
        AND r.created_at > NOW() - INTERVAL '90 days')::bigint AS csat_count,
    (SELECT COUNT(*) FROM conversation_ratings r
      WHERE r.organization_id = sqlc.arg(organization_id) AND r.conversation_id IN (SELECT id FROM conv))::bigint AS csat_total,
    -- Latest NPS / CES answers: -1 when none (survey results need read_all).
    COALESCE((SELECT sr.score FROM survey_responses sr
      JOIN survey_invitations si ON si.organization_id = sr.organization_id AND si.id = sr.invitation_id
      WHERE sr.organization_id = sqlc.arg(organization_id) AND sr.kind = 'nps' AND sqlc.arg(can_surveys)::boolean
        AND si.conversation_id IN (SELECT id FROM conv)
      ORDER BY sr.created_at DESC LIMIT 1), -1)::int AS nps_latest,
    COALESCE((SELECT sr.score FROM survey_responses sr
      JOIN survey_invitations si ON si.organization_id = sr.organization_id AND si.id = sr.invitation_id
      WHERE sr.organization_id = sqlc.arg(organization_id) AND sr.kind = 'ces' AND sqlc.arg(can_surveys)::boolean
        AND si.conversation_id IN (SELECT id FROM conv)
      ORDER BY sr.created_at DESC LIMIT 1), -1)::int AS ces_latest,
    (SELECT COUNT(*) FROM attachments a
      WHERE a.organization_id = sqlc.arg(organization_id) AND a.deleted_at IS NULL AND a.owner_type = 'message'
        AND a.owner_id IN (SELECT m.uuid FROM messages m
                           WHERE m.organization_id = sqlc.arg(organization_id) AND m.conversation_id IN (SELECT id FROM conv)))::bigint AS attachment_count,
    (SELECT COUNT(*) FROM contact_notes n
      WHERE n.organization_id = sqlc.arg(organization_id) AND n.contact_id = sqlc.arg(contact_id))::bigint AS note_count;

-- name: ListContactProfileConversations :many
-- Open conversations first, then the most recent closed ones.
SELECT c.uuid, c.number, c.subject, c.status, c.channel_type, c.priority, c.last_message_at,
       c.last_message_preview, c.created_at, c.message_count, c.sla_breached_at,
       LEAST(CASE WHEN c.first_response_at IS NULL THEN c.first_response_due_at END,
             c.next_response_due_at, c.resolution_due_at)::timestamptz AS sla_due_at,
       COALESCE(TRIM(u.name || ' ' || u.surname), '')::text AS assignee_name,
       COALESCE(tm.name, '')::text AS team_name,
       COALESCE((SELECT array_agg(t.name ORDER BY t.name) FROM conversation_tags ct
                 JOIN tags t ON t.organization_id = ct.organization_id AND t.id = ct.tag_id
                 WHERE ct.organization_id = c.organization_id AND ct.conversation_id = c.id), '{}')::text[] AS tags
FROM conversations c
LEFT JOIN users u ON u.id = c.assignee_user_id
LEFT JOIN teams tm ON tm.organization_id = c.organization_id AND tm.id = c.assignee_team_id
WHERE c.organization_id = sqlc.arg(organization_id) AND c.contact_id = sqlc.arg(contact_id) AND c.deleted_at IS NULL
  AND (sqlc.arg(see_all)::boolean
       OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
       OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
       OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
       OR EXISTS (SELECT 1 FROM conversation_participants vp
                  WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                    AND vp.user_id = sqlc.arg(viewer_id)::bigint))
ORDER BY (c.status IN ('resolved', 'closed')), c.last_message_at DESC
LIMIT sqlc.arg(row_limit);

-- name: ListContactProfileTags :many
SELECT t.uuid, t.name, t.color, COUNT(*)::bigint AS conversations
FROM conversation_tags ct
JOIN tags t ON t.organization_id = ct.organization_id AND t.id = ct.tag_id
JOIN conversations c ON c.organization_id = ct.organization_id AND c.id = ct.conversation_id
WHERE ct.organization_id = sqlc.arg(organization_id) AND c.contact_id = sqlc.arg(contact_id) AND c.deleted_at IS NULL
  AND (sqlc.arg(see_all)::boolean
       OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
       OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
       OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
       OR EXISTS (SELECT 1 FROM conversation_participants vp
                  WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                    AND vp.user_id = sqlc.arg(viewer_id)::bigint))
GROUP BY t.uuid, t.name, t.color
ORDER BY COUNT(*) DESC, t.name
LIMIT 30;

-- name: ListContactProfileAttachments :many
-- Files the contact sent or received; they open in their conversation.
SELECT a.uuid, a.filename, a.content_type, a.size_bytes, a.created_at, c.uuid AS conversation_uuid, c.number AS conversation_number
FROM attachments a
JOIN messages m ON m.organization_id = a.organization_id AND m.uuid = a.owner_id
JOIN conversations c ON c.organization_id = m.organization_id AND c.id = m.conversation_id
WHERE a.organization_id = sqlc.arg(organization_id) AND a.owner_type = 'message' AND a.deleted_at IS NULL
  AND c.contact_id = sqlc.arg(contact_id) AND c.deleted_at IS NULL
  AND (sqlc.arg(see_all)::boolean
       OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
       OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
       OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
       OR EXISTS (SELECT 1 FROM conversation_participants vp
                  WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                    AND vp.user_id = sqlc.arg(viewer_id)::bigint))
ORDER BY a.created_at DESC
LIMIT 20;

-- name: ListContactProfileAppointments :many
-- Upcoming appointments first (soonest), then past ones (latest).
SELECT a.uuid, a.title, a.status, a.starts_at, a.ends_at, a.timezone, a.location_kind,
       COALESCE(ty.name, '')::text AS type_name, COALESCE(TRIM(u.name || ' ' || u.surname), '')::text AS host_name
FROM appointments a
LEFT JOIN appointment_types ty ON ty.organization_id = a.organization_id AND ty.id = a.type_id
LEFT JOIN users u ON u.id = a.host_user_id
WHERE a.organization_id = sqlc.arg(organization_id) AND a.contact_id = sqlc.arg(contact_id)
ORDER BY (a.starts_at < NOW()), CASE WHEN a.starts_at >= NOW() THEN a.starts_at END ASC, a.starts_at DESC
LIMIT 20;

-- name: ListContactProfileConsents :many
-- The latest decision for each purpose.
SELECT DISTINCT ON (cc.purpose) cc.uuid, cc.purpose, cc.granted, cc.notice_version, cc.source, cc.created_at
FROM contact_consents cc
WHERE cc.organization_id = $1 AND cc.contact_id = $2
ORDER BY cc.purpose, cc.created_at DESC, cc.id DESC;

-- name: ListContactProfileRequests :many
SELECT r.uuid, r.kind, r.status, r.created_at, r.completed_at, r.expires_at,
       COALESCE(TRIM(u.name || ' ' || u.surname), '')::text AS requested_by_name
FROM data_subject_requests r
LEFT JOIN users u ON u.id = r.requested_by
WHERE r.organization_id = $1 AND r.contact_id = $2
ORDER BY r.created_at DESC
LIMIT 20;

-- name: ListContactProfileFeedback :many
-- Roadmap votes and comments of the contact.
SELECT 'vote'::text AS kind, fr.uuid AS request_uuid, fr.title, fr.status, ''::text AS body, v.created_at
FROM feature_votes v
JOIN feature_requests fr ON fr.organization_id = v.organization_id AND fr.id = v.request_id
WHERE v.organization_id = sqlc.arg(organization_id) AND v.contact_id = sqlc.arg(contact_id)
UNION ALL
SELECT 'comment', fr.uuid, fr.title, fr.status, fc.body, fc.created_at
FROM feedback_comments fc
JOIN feature_requests fr ON fr.organization_id = fc.organization_id AND fr.id = fc.request_id
WHERE fc.organization_id = sqlc.arg(organization_id) AND fc.contact_id = sqlc.arg(contact_id)
ORDER BY created_at DESC
LIMIT 30;

-- name: ListContactProfileForumPosts :many
-- Topics and replies; only published ones unless the member moderates.
SELECT p.uuid, p.slug, p.status, p.created_at, (p.topic_id IS NULL)::boolean AS is_topic,
       COALESCE(NULLIF(p.title, ''), tp.title, '')::text AS title, LEFT(p.body, 280)::text AS excerpt
FROM forum_posts p
LEFT JOIN forum_posts tp ON tp.organization_id = p.organization_id AND tp.id = p.topic_id
WHERE p.organization_id = sqlc.arg(organization_id) AND p.contact_id = sqlc.arg(contact_id) AND p.removed_at IS NULL
  AND (sqlc.arg(all_statuses)::boolean OR p.status = 'published')
ORDER BY p.created_at DESC
LIMIT 20;

-- name: ListContactProfileStatusNotices :many
-- Status incidents the contact was notified about.
SELECT i.uuid, i.title, i.severity, i.status, d.created_at
FROM status_incident_deliveries d
JOIN status_incidents i ON i.organization_id = d.organization_id AND i.id = d.incident_id
WHERE d.organization_id = $1 AND d.contact_id = $2
ORDER BY d.created_at DESC
LIMIT 20;

-- name: ContactLatestActivityAt :one
-- Newest moment anything happened for the contact; the AI summary is
-- stale when this is later than its source_at.
SELECT GREATEST(
    (SELECT MAX(c.last_message_at) FROM conversations c
      WHERE c.organization_id = sqlc.arg(organization_id) AND c.contact_id = sqlc.arg(contact_id) AND c.deleted_at IS NULL),
    (SELECT MAX(n.created_at) FROM contact_notes n
      WHERE n.organization_id = sqlc.arg(organization_id) AND n.contact_id = sqlc.arg(contact_id))
)::timestamptz AS latest;

-- name: ContactSummarySource :many
-- What the AI summary reads: the latest conversations with their opening
-- and latest customer / agent messages (public messages only), newest first.
SELECT c.number, c.subject, c.status, c.channel_type, c.created_at, c.last_message_at,
       COALESCE((SELECT string_agg(LEFT(x.content, 600), E'\n---\n' ORDER BY x.created_at)
                 FROM (SELECT m.content, m.created_at FROM messages m
                       WHERE m.organization_id = c.organization_id AND m.conversation_id = c.id AND m.deleted_at IS NULL
                         AND m.private = false AND m.content <> ''
                       ORDER BY m.created_at DESC LIMIT 6) x), '')::text AS excerpt
FROM conversations c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.contact_id = sqlc.arg(contact_id) AND c.deleted_at IS NULL
  AND (sqlc.arg(see_all)::boolean
       OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
       OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
       OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
       OR EXISTS (SELECT 1 FROM conversation_participants vp
                  WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                    AND vp.user_id = sqlc.arg(viewer_id)::bigint))
ORDER BY c.last_message_at DESC
LIMIT 8;

-- name: ListContactTimeline :many
-- One ordered stream over every source of a contact. kinds is the set the
-- caller may and wants to see (never NULL); q searches title and body;
-- from / to bound occurred_at; before pages backwards in time.
WITH conv AS (
    SELECT c.id, c.uuid, c.number, c.subject, c.status, c.channel_type, c.created_at, c.resolved_at, c.closed_at
    FROM conversations c
    WHERE c.organization_id = sqlc.arg(organization_id) AND c.contact_id = sqlc.arg(contact_id) AND c.deleted_at IS NULL
      AND (sqlc.arg(see_all)::boolean
           OR c.assignee_user_id = sqlc.arg(viewer_id)::bigint
           OR c.assignee_team_id = ANY(sqlc.arg(viewer_team_ids)::bigint[])
           OR (c.assignee_user_id IS NULL AND c.assignee_team_id IS NULL)
           OR EXISTS (SELECT 1 FROM conversation_participants vp
                      WHERE vp.organization_id = c.organization_id AND vp.conversation_id = c.id
                        AND vp.user_id = sqlc.arg(viewer_id)::bigint))
), items AS (
    SELECT 'conversation_opened'::text AS kind, conv.uuid AS ref_uuid, conv.uuid::text AS sort_key, conv.created_at AS occurred_at,
           conv.subject AS title, ''::text AS body, ''::text AS actor_name,
           jsonb_build_object('number', conv.number, 'status', conv.status, 'channel_type', conv.channel_type) AS data
    FROM conv
    UNION ALL
    SELECT 'conversation_resolved', conv.uuid, conv.uuid::text, COALESCE(conv.resolved_at, conv.closed_at), conv.subject, '', '',
           jsonb_build_object('number', conv.number, 'status', conv.status, 'channel_type', conv.channel_type)
    FROM conv WHERE conv.resolved_at IS NOT NULL OR conv.closed_at IS NOT NULL
    UNION ALL
    SELECT 'csat', r.uuid, r.uuid::text, r.created_at, conv.subject, r.comment, '',
           jsonb_build_object('score', r.score, 'conversation_uuid', conv.uuid, 'number', conv.number)
    FROM conversation_ratings r
    JOIN conv ON conv.id = r.conversation_id
    WHERE r.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'survey', sr.uuid, sr.uuid::text, sr.created_at, si.question, sr.comment, '',
           jsonb_build_object('survey_kind', sr.kind, 'score', sr.score, 'conversation_uuid', conv.uuid, 'number', conv.number)
    FROM survey_responses sr
    JOIN survey_invitations si ON si.organization_id = sr.organization_id AND si.id = sr.invitation_id
    JOIN conv ON conv.id = si.conversation_id
    WHERE sr.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'note', n.uuid, n.uuid::text, n.created_at, '', n.body, COALESCE(TRIM(u.name || ' ' || u.surname), ''),
           jsonb_build_object('author_uuid', u.uuid)
    FROM contact_notes n
    LEFT JOIN users u ON u.id = n.author_user_id
    WHERE n.organization_id = sqlc.arg(organization_id) AND n.contact_id = sqlc.arg(contact_id)
    UNION ALL
    SELECT 'appointment', a.uuid, a.uuid::text, a.created_at, a.title, a.notes, COALESCE(TRIM(u.name || ' ' || u.surname), ''),
           jsonb_build_object('status', a.status, 'starts_at', a.starts_at, 'ends_at', a.ends_at, 'source', a.source)
    FROM appointments a
    LEFT JOIN users u ON u.id = a.host_user_id
    WHERE a.organization_id = sqlc.arg(organization_id) AND a.contact_id = sqlc.arg(contact_id)
    UNION ALL
    SELECT 'consent', cc.uuid, cc.uuid::text, cc.created_at, cc.purpose, '', COALESCE(TRIM(u.name || ' ' || u.surname), ''),
           jsonb_build_object('granted', cc.granted, 'notice_version', cc.notice_version, 'source', cc.source)
    FROM contact_consents cc
    LEFT JOIN users u ON u.id = cc.recorded_by
    WHERE cc.organization_id = sqlc.arg(organization_id) AND cc.contact_id = sqlc.arg(contact_id)
    UNION ALL
    SELECT 'privacy_request', dr.uuid, dr.uuid::text, dr.created_at, dr.kind, '', COALESCE(TRIM(u.name || ' ' || u.surname), ''),
           jsonb_build_object('request_kind', dr.kind, 'status', dr.status, 'completed_at', dr.completed_at)
    FROM data_subject_requests dr
    LEFT JOIN users u ON u.id = dr.requested_by
    WHERE dr.organization_id = sqlc.arg(organization_id) AND dr.contact_id = sqlc.arg(contact_id)
    UNION ALL
    SELECT 'event', e.uuid, e.uuid::text, e.occurred_at, e.name, '', '',
           jsonb_build_object('properties', e.properties, 'source', e.source)
    FROM company_events e
    WHERE e.organization_id = sqlc.arg(organization_id) AND e.contact_id = sqlc.arg(contact_id)
    UNION ALL
    SELECT 'feedback', fr.uuid, 'vote-' || v.id::text, v.created_at, fr.title, '', '',
           jsonb_build_object('feedback_kind', 'vote', 'status', fr.status)
    FROM feature_votes v
    JOIN feature_requests fr ON fr.organization_id = v.organization_id AND fr.id = v.request_id
    WHERE v.organization_id = sqlc.arg(organization_id) AND v.contact_id = sqlc.arg(contact_id)
    UNION ALL
    SELECT 'feedback', fr.uuid, fc.uuid::text, fc.created_at, fr.title, fc.body, '',
           jsonb_build_object('feedback_kind', 'comment', 'status', fr.status, 'approved', fc.approved)
    FROM feedback_comments fc
    JOIN feature_requests fr ON fr.organization_id = fc.organization_id AND fr.id = fc.request_id
    WHERE fc.organization_id = sqlc.arg(organization_id) AND fc.contact_id = sqlc.arg(contact_id)
    UNION ALL
    SELECT 'forum', p.uuid, p.uuid::text, p.created_at, COALESCE(NULLIF(p.title, ''), tp.title, ''), LEFT(p.body, 500), '',
           jsonb_build_object('status', p.status, 'slug', COALESCE(tp.slug, p.slug), 'is_topic', p.topic_id IS NULL)
    FROM forum_posts p
    LEFT JOIN forum_posts tp ON tp.organization_id = p.organization_id AND tp.id = p.topic_id
    WHERE p.organization_id = sqlc.arg(organization_id) AND p.contact_id = sqlc.arg(contact_id) AND p.removed_at IS NULL
      AND (sqlc.arg(forum_all)::boolean OR p.status = 'published')
    UNION ALL
    SELECT 'status_notice', i.uuid, d.uuid::text, d.created_at, i.title, '', '',
           jsonb_build_object('severity', i.severity, 'status', i.status)
    FROM status_incident_deliveries d
    JOIN status_incidents i ON i.organization_id = d.organization_id AND i.id = d.incident_id
    WHERE d.organization_id = sqlc.arg(organization_id) AND d.contact_id = sqlc.arg(contact_id)
    UNION ALL
    -- System events: the contact record itself, its channel identities and
    -- audited changes (edit, merge, notes) from the activity log.
    SELECT 'system', NULL::uuid, 'created-' || ct.id::text, ct.created_at, 'contact.created', '', '',
           '{}'::jsonb
    FROM contacts ct
    WHERE ct.organization_id = sqlc.arg(organization_id) AND ct.id = sqlc.arg(contact_id)
    UNION ALL
    SELECT 'system', NULL::uuid, 'identity-' || ci.id::text, ci.created_at, 'identity.added', '', '',
           jsonb_build_object('channel_type', ci.channel_type)
    FROM contact_identities ci
    WHERE ci.organization_id = sqlc.arg(organization_id) AND ci.contact_id = sqlc.arg(contact_id)
    UNION ALL
    SELECT 'system', NULL::uuid, ae.uuid::text, ae.created_at, ae.action, '', COALESCE(TRIM(u.name || ' ' || u.surname), ''),
           '{}'::jsonb
    FROM activity_events ae
    LEFT JOIN users u ON u.id = ae.actor_user_id
    WHERE ae.organization_id = sqlc.arg(organization_id) AND ae.resource = 'contacts'
      AND ae.resource_uuid = sqlc.arg(contact_uuid)::uuid
      AND ae.action IN ('contacts.updated', 'contacts.merged')
)
-- Keyset paging on (occurred_at, kind, sort_key) like the company timeline.
SELECT kind, COALESCE(ref_uuid::text, '')::text AS ref, sort_key::text AS sort_key, occurred_at, title, body, actor_name, data
FROM items
WHERE occurred_at IS NOT NULL
  AND kind = ANY(sqlc.arg(kinds)::text[])
  AND (sqlc.narg(q)::text IS NULL OR (title || ' ' || body || ' ' || actor_name) ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR occurred_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR occurred_at < sqlc.narg(to_at)::timestamptz)
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR (occurred_at, kind, sort_key) < (sqlc.narg(before)::timestamptz, sqlc.arg(before_kind)::text, sqlc.arg(before_key)::text))
ORDER BY occurred_at DESC, kind DESC, sort_key DESC
LIMIT sqlc.arg(row_limit);

-- name: ListContactNotesForExport :many
-- KVKK export: every note about the contact, oldest first.
SELECT n.uuid, n.body, n.created_at, COALESCE(TRIM(u.name || ' ' || u.surname), '')::text AS author_name
FROM contact_notes n
LEFT JOIN users u ON u.id = n.author_user_id
WHERE n.organization_id = $1 AND n.contact_id = $2
ORDER BY n.created_at, n.id;
