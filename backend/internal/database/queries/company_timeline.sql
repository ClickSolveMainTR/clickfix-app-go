-- CF-85: account screen and unified company timeline.

-- name: CreateCompanyNote :one
INSERT INTO company_notes (organization_id, company_id, author_user_id, body)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetCompanyNoteByUUID :one
SELECT * FROM company_notes WHERE organization_id = $1 AND uuid = $2;

-- name: DeleteCompanyNote :execrows
DELETE FROM company_notes WHERE organization_id = $1 AND id = $2;

-- name: CreateCompanyEvent :one
-- A retried webhook with the same idempotency key returns no row; the
-- caller then loads the stored event.
INSERT INTO company_events (organization_id, company_id, contact_id, name, properties, source, idempotency_key, occurred_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(company_id), sqlc.narg(contact_id), sqlc.arg(name), sqlc.arg(properties),
        sqlc.arg(source), sqlc.narg(idempotency_key), sqlc.arg(occurred_at))
ON CONFLICT (organization_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetCompanyEventByIdempotencyKey :one
SELECT * FROM company_events WHERE organization_id = $1 AND idempotency_key = $2;

-- name: GetCompanyOverviewStats :one
-- Conversations count when they name the company or come from one of its
-- contacts (contacts linked to the company after the conversation opened).
WITH conv AS (
    SELECT c.id, c.status, c.last_message_at
    FROM conversations c
    WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL
      AND (c.company_id = sqlc.arg(company_id)
           OR c.contact_id IN (SELECT ct.id FROM contacts ct
                               WHERE ct.organization_id = sqlc.arg(organization_id)
                                 AND ct.company_id = sqlc.arg(company_id) AND ct.deleted_at IS NULL))
)
SELECT
    (SELECT COUNT(*) FROM conv WHERE status NOT IN ('resolved', 'closed'))::bigint AS open_conversations,
    (SELECT COUNT(*) FROM conv WHERE status IN ('resolved', 'closed'))::bigint AS closed_conversations,
    (SELECT MAX(last_message_at) FROM conv)::timestamptz AS last_conversation_at,
    (SELECT COALESCE(AVG(r.score), 0) FROM conversation_ratings r
      WHERE r.organization_id = sqlc.arg(organization_id) AND r.conversation_id IN (SELECT id FROM conv)
        AND r.created_at > NOW() - INTERVAL '90 days')::float8 AS csat_average,
    (SELECT COUNT(*) FROM conversation_ratings r
      WHERE r.organization_id = sqlc.arg(organization_id) AND r.conversation_id IN (SELECT id FROM conv)
        AND r.created_at > NOW() - INTERVAL '90 days')::bigint AS csat_count;

-- name: ListCompanyConversations :many
SELECT c.uuid, c.number, c.subject, c.status, c.channel_type, c.priority, c.last_message_at,
       c.last_message_preview, c.created_at, ct.uuid AS contact_uuid, ct.name AS contact_name
FROM conversations c
JOIN contacts ct ON ct.organization_id = c.organization_id AND ct.id = c.contact_id
WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL
  AND (c.company_id = sqlc.arg(company_id) OR ct.company_id = sqlc.arg(company_id))
  AND (sqlc.narg(open_only)::boolean IS NULL
       OR (sqlc.narg(open_only)::boolean AND c.status NOT IN ('resolved', 'closed'))
       OR (NOT sqlc.narg(open_only)::boolean AND c.status IN ('resolved', 'closed')))
ORDER BY c.last_message_at DESC
LIMIT sqlc.arg(row_limit);

-- name: ListCompanyTimeline :many
-- One ordered stream over every source an account manager wants to see.
-- kinds filters sources (NULL = all); before pages backwards in time.
WITH conv AS (
    SELECT c.id, c.uuid, c.number, c.subject, c.status, c.channel_type, c.created_at, c.resolved_at,
           c.closed_at, ct.name AS contact_name
    FROM conversations c
    JOIN contacts ct ON ct.organization_id = c.organization_id AND ct.id = c.contact_id
    WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL
      AND (c.company_id = sqlc.arg(company_id) OR ct.company_id = sqlc.arg(company_id))
), health AS (
    SELECT h.id, h.score, h.created_at,
           LAG(h.score) OVER (ORDER BY h.created_at) AS prev_score
    FROM company_health_history h
    WHERE h.organization_id = sqlc.arg(organization_id) AND h.company_id = sqlc.arg(company_id)
), items AS (
    SELECT 'conversation_opened'::text AS kind, conv.uuid AS ref_uuid, conv.uuid::text AS sort_key, conv.created_at AS occurred_at,
           conv.subject AS title, ''::text AS body, conv.contact_name AS actor_name,
           jsonb_build_object('number', conv.number, 'status', conv.status, 'channel_type', conv.channel_type) AS data
    FROM conv
    UNION ALL
    SELECT 'conversation_resolved', conv.uuid, conv.uuid::text, COALESCE(conv.resolved_at, conv.closed_at), conv.subject, '',
           conv.contact_name,
           jsonb_build_object('number', conv.number, 'status', conv.status, 'channel_type', conv.channel_type)
    FROM conv WHERE conv.resolved_at IS NOT NULL OR conv.closed_at IS NOT NULL
    UNION ALL
    SELECT 'csat', r.uuid, r.uuid::text, r.created_at, conv.subject, r.comment, COALESCE(ct.name, ''),
           jsonb_build_object('score', r.score, 'conversation_uuid', conv.uuid, 'number', conv.number)
    FROM conversation_ratings r
    JOIN conv ON conv.id = r.conversation_id
    LEFT JOIN contacts ct ON ct.organization_id = r.organization_id AND ct.id = r.contact_id
    WHERE r.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'note', n.uuid, n.uuid::text, n.created_at, '', n.body, COALESCE(TRIM(u.name || ' ' || u.surname), ''),
           jsonb_build_object('author_uuid', u.uuid)
    FROM company_notes n
    LEFT JOIN users u ON u.id = n.author_user_id
    WHERE n.organization_id = sqlc.arg(organization_id) AND n.company_id = sqlc.arg(company_id)
    UNION ALL
    SELECT 'event', e.uuid, e.uuid::text, e.occurred_at, e.name, '', COALESCE(ct.name, ''),
           jsonb_build_object('properties', e.properties, 'source', e.source)
    FROM company_events e
    LEFT JOIN contacts ct ON ct.organization_id = e.organization_id AND ct.id = e.contact_id
    WHERE e.organization_id = sqlc.arg(organization_id) AND e.company_id = sqlc.arg(company_id)
    UNION ALL
    SELECT 'revenue', NULL::uuid, 'revenue-' || rh.id::text, rh.created_at, rh.plan, '', COALESCE(TRIM(u.name || ' ' || u.surname), ''),
           jsonb_build_object('mrr_minor', rh.mrr_minor, 'currency', rh.currency, 'billing_status', rh.billing_status,
                              'source', rh.source, 'renewal_at', rh.renewal_at)
    FROM company_revenue_history rh
    LEFT JOIN users u ON u.id = rh.changed_by_user_id
    WHERE rh.organization_id = sqlc.arg(organization_id) AND rh.company_id = sqlc.arg(company_id)
    UNION ALL
    -- Daily health snapshots only when the score moved noticeably.
    SELECT 'health', NULL::uuid, 'health-' || health.id::text, health.created_at, '', '', '',
           jsonb_build_object('score', health.score, 'previous', health.prev_score)
    FROM health
    WHERE health.prev_score IS NOT NULL AND ABS(health.score - health.prev_score) >= 10
)
-- Keyset paging on (occurred_at, kind, sort_key) so rows sharing a timestamp
-- are neither skipped nor repeated across pages.
SELECT kind, COALESCE(ref_uuid::text, '')::text AS ref, sort_key::text AS sort_key, occurred_at, title, body, actor_name, data
FROM items
WHERE occurred_at IS NOT NULL
  AND (sqlc.narg(kinds)::text[] IS NULL OR kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR (occurred_at, kind, sort_key) < (sqlc.narg(before)::timestamptz, sqlc.arg(before_kind)::text, sqlc.arg(before_key)::text))
ORDER BY occurred_at DESC, kind DESC, sort_key DESC
LIMIT sqlc.arg(row_limit);
