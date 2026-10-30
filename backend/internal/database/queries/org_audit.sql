-- Organization audit log (CF-12): activity_events of one organization.

-- name: ListOrganizationAuditEvents :many
SELECT e.id, e.uuid, e.action, e.resource, e.resource_uuid, e.payload, e.ip_address, e.created_at,
       u.uuid AS actor_uuid,
       COALESCE(u.email, '')::text AS actor_email,
       COALESCE(u.name, '')::text AS actor_name,
       COALESCE(u.surname, '')::text AS actor_surname
FROM activity_events e
LEFT JOIN users u ON u.id = e.actor_user_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(actor_uuid)::uuid IS NULL OR u.uuid = sqlc.narg(actor_uuid)::uuid)
  AND (sqlc.narg(resources)::text[] IS NULL OR e.resource = ANY(sqlc.narg(resources)::text[]))
  AND (sqlc.narg(actions)::text[] IS NULL OR e.action = ANY(sqlc.narg(actions)::text[]))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR e.created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR e.created_at < sqlc.narg(to_at)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.action ILIKE '%' || sqlc.narg(q) || '%'
    OR e.resource ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_desc)::boolean THEN e.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::boolean THEN e.created_at END ASC,
  e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOrganizationAuditEvents :one
SELECT COUNT(*)::bigint
FROM activity_events e
LEFT JOIN users u ON u.id = e.actor_user_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(actor_uuid)::uuid IS NULL OR u.uuid = sqlc.narg(actor_uuid)::uuid)
  AND (sqlc.narg(resources)::text[] IS NULL OR e.resource = ANY(sqlc.narg(resources)::text[]))
  AND (sqlc.narg(actions)::text[] IS NULL OR e.action = ANY(sqlc.narg(actions)::text[]))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR e.created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR e.created_at < sqlc.narg(to_at)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.action ILIKE '%' || sqlc.narg(q) || '%'
    OR e.resource ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListOrganizationAuditFacets :many
-- Distinct resource / action pairs of the organization (filter options).
SELECT DISTINCT e.resource, e.action
FROM activity_events e
WHERE e.organization_id = sqlc.arg(organization_id)
ORDER BY e.resource, e.action;
