-- Support access grants (CF-14). Organization-side queries filter by
-- organization_id; the support resolver looks a grant up by organization.

-- name: CreateSupportGrant :one
INSERT INTO support_access_grants (organization_id, mode, reason, granted_by, expires_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(mode), sqlc.arg(reason), sqlc.narg(granted_by), sqlc.arg(expires_at))
RETURNING *;

-- name: RevokeOpenSupportGrants :execrows
-- A new grant replaces the open one (one active grant per organization).
UPDATE support_access_grants
SET revoked_at = NOW(), revoked_by = sqlc.narg(revoked_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND revoked_at IS NULL AND expires_at > NOW();

-- name: RevokeSupportGrant :one
UPDATE support_access_grants
SET revoked_at = NOW(), revoked_by = sqlc.narg(revoked_by)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid)
  AND revoked_at IS NULL AND expires_at > NOW()
RETURNING *;

-- name: GetActiveSupportGrant :one
SELECT * FROM support_access_grants
WHERE organization_id = sqlc.arg(organization_id)
  AND revoked_at IS NULL AND starts_at <= NOW() AND expires_at > NOW()
ORDER BY created_at DESC
LIMIT 1;

-- name: ListSupportGrants :many
SELECT g.id, g.uuid, g.mode, g.reason, g.starts_at, g.expires_at, g.revoked_at, g.created_at,
       gu.uuid AS granted_by_uuid,
       COALESCE(gu.name, '')::text AS granted_by_name,
       COALESCE(gu.surname, '')::text AS granted_by_surname,
       ru.uuid AS revoked_by_uuid,
       COALESCE(ru.name, '')::text AS revoked_by_name,
       COALESCE(ru.surname, '')::text AS revoked_by_surname
FROM support_access_grants g
LEFT JOIN users gu ON gu.id = g.granted_by
LEFT JOIN users ru ON ru.id = g.revoked_by
WHERE g.organization_id = sqlc.arg(organization_id)
ORDER BY g.created_at DESC, g.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountSupportGrants :one
SELECT COUNT(*)::bigint FROM support_access_grants
WHERE organization_id = sqlc.arg(organization_id);

-- name: ListActiveSupportGrantsPlatform :many
-- Platform view: organizations staff may enter right now.
SELECT g.uuid, g.mode, g.reason, g.starts_at, g.expires_at,
       o.id AS organization_id, o.uuid AS organization_uuid, o.slug AS organization_slug,
       o.name AS organization_name, o.logo_object_key, o.status AS organization_status
FROM support_access_grants g
INNER JOIN organizations o ON o.id = g.organization_id AND o.deleted_at IS NULL
WHERE g.revoked_at IS NULL AND g.starts_at <= NOW() AND g.expires_at > NOW()
ORDER BY g.expires_at ASC;

-- name: UserHasPlatformPermission :one
SELECT EXISTS (
    SELECT 1
    FROM user_roles ur
    INNER JOIN roles r ON r.id = ur.role_id AND r.scope = 'platform'
    INNER JOIN role_permissions rp ON rp.role_id = r.id
    INNER JOIN permissions p ON p.id = rp.permission_id
    WHERE ur.user_id = sqlc.arg(user_id) AND p.slug = sqlc.arg(slug)::text
)::boolean;

-- name: ListTenantPermissionSlugs :many
SELECT slug FROM permissions WHERE slug LIKE 'tenant.%' ORDER BY slug;
