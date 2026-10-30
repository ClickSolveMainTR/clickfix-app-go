-- Tenant access: member roles (many per member) and
-- organization-specific roles. Every query is scoped by organization_id.

-- name: ListMemberPermissionSlugs :many
-- Union of the permissions of every role the member holds.
SELECT DISTINCT p.slug
FROM organization_member_roles omr
INNER JOIN roles r ON r.id = omr.role_id AND r.scope = 'tenant'
INNER JOIN role_permissions rp ON rp.role_id = omr.role_id
INNER JOIN permissions p ON p.id = rp.permission_id
WHERE omr.organization_id = sqlc.arg(organization_id)
  AND omr.member_id = sqlc.arg(member_id)
ORDER BY p.slug;

-- name: ListMemberRoles :many
SELECT omr.member_id, r.id, r.uuid, r.slug, r.name,
       (r.organization_id IS NULL)::boolean AS is_template
FROM organization_member_roles omr
INNER JOIN roles r ON r.id = omr.role_id AND r.scope = 'tenant'
WHERE omr.organization_id = sqlc.arg(organization_id)
  AND omr.member_id = ANY(sqlc.arg(member_ids)::bigint[])
ORDER BY omr.member_id, r.organization_id NULLS FIRST, r.id;

-- name: DeleteMemberRoles :exec
DELETE FROM organization_member_roles
WHERE organization_id = sqlc.arg(organization_id)
  AND member_id = sqlc.arg(member_id);

-- name: AddMemberRole :exec
INSERT INTO organization_member_roles (organization_id, member_id, role_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(member_id), sqlc.arg(role_id))
ON CONFLICT DO NOTHING;

-- name: SetMemberPrimaryRole :exec
UPDATE organization_members
SET role_id = sqlc.arg(role_id)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(member_id);

-- name: ListTenantMembers :many
SELECT om.id, om.user_id, om.created_at, om.updated_at, om.skills,
       u.uuid AS user_uuid, u.email, u.name, u.surname, u.status, u.last_login_at
FROM organization_members om
INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  )
  AND (
    sqlc.narg(role_ids)::bigint[] IS NULL
    OR EXISTS (
        SELECT 1 FROM organization_member_roles omr
        WHERE omr.organization_id = om.organization_id
          AND omr.member_id = om.id
          AND omr.role_id = ANY(sqlc.narg(role_ids)::bigint[])
    )
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'email' AND NOT sqlc.arg(sort_desc)::boolean THEN u.email END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'email' AND sqlc.arg(sort_desc)::boolean THEN u.email END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN u.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN u.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_login_at' AND NOT sqlc.arg(sort_desc)::boolean THEN u.last_login_at END ASC NULLS FIRST,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_login_at' AND sqlc.arg(sort_desc)::boolean THEN u.last_login_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN om.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN om.created_at END DESC,
  om.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountTenantMembers :one
SELECT COUNT(*)::bigint
FROM organization_members om
INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  )
  AND (
    sqlc.narg(role_ids)::bigint[] IS NULL
    OR EXISTS (
        SELECT 1 FROM organization_member_roles omr
        WHERE omr.organization_id = om.organization_id
          AND omr.member_id = om.id
          AND omr.role_id = ANY(sqlc.narg(role_ids)::bigint[])
    )
  );

-- name: GetTenantMemberByUserUUID :one
SELECT om.id, om.user_id, om.created_at, om.updated_at, om.skills,
       u.uuid AS user_uuid, u.email, u.name, u.surname, u.status, u.last_login_at
FROM organization_members om
INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id)
  AND u.uuid = sqlc.arg(user_uuid);

-- name: ListTenantRolesForOrganization :many
-- Template roles plus the roles created by this organization.
SELECT r.id, r.uuid, r.name, r.slug, r.description, r.is_system, r.is_billable, r.created_at, r.updated_at,
       (r.organization_id IS NULL)::boolean AS is_template,
       (
           SELECT COUNT(*) FROM organization_member_roles omr
           WHERE omr.organization_id = sqlc.arg(organization_id) AND omr.role_id = r.id
       )::bigint AS member_count
FROM roles r
WHERE r.scope = 'tenant'
  AND (r.organization_id IS NULL OR r.organization_id = sqlc.arg(organization_id))
ORDER BY r.organization_id NULLS FIRST, r.id;

-- name: GetTenantRoleByUUIDForOrganization :one
SELECT r.id, r.uuid, r.name, r.slug, r.description, r.is_system, r.is_billable, r.created_at, r.updated_at,
       (r.organization_id IS NULL)::boolean AS is_template,
       (
           SELECT COUNT(*) FROM organization_member_roles omr
           WHERE omr.organization_id = sqlc.arg(organization_id) AND omr.role_id = r.id
       )::bigint AS member_count
FROM roles r
WHERE r.scope = 'tenant'
  AND r.uuid = sqlc.arg(uuid)
  AND (r.organization_id IS NULL OR r.organization_id = sqlc.arg(organization_id));

-- name: TenantRoleSlugTaken :one
SELECT EXISTS (
    SELECT 1 FROM roles
    WHERE scope = 'tenant'
      AND slug = sqlc.arg(slug)
      AND (organization_id IS NULL OR organization_id = sqlc.arg(organization_id))
)::boolean;

-- name: CreateTenantRole :one
INSERT INTO roles (name, slug, description, is_system, scope, organization_id, is_billable)
VALUES (sqlc.arg(name), sqlc.arg(slug), sqlc.narg(description), false, 'tenant', sqlc.arg(organization_id), sqlc.arg(is_billable))
RETURNING id, uuid;

-- name: UpdateTenantRole :one
UPDATE roles
SET name = sqlc.arg(name),
    description = sqlc.narg(description),
    is_billable = sqlc.arg(is_billable)
WHERE uuid = sqlc.arg(uuid)
  AND scope = 'tenant'
  AND organization_id = sqlc.arg(organization_id)
RETURNING id;

-- name: DeleteTenantRole :execrows
DELETE FROM roles
WHERE uuid = sqlc.arg(uuid)
  AND scope = 'tenant'
  AND organization_id = sqlc.arg(organization_id);

-- name: ListRolePermissionSlugsByRoleIDs :many
SELECT rp.role_id, p.slug
FROM role_permissions rp
INNER JOIN permissions p ON p.id = rp.permission_id
WHERE rp.role_id = ANY(sqlc.arg(role_ids)::bigint[])
ORDER BY rp.role_id, p.slug;

-- name: ListTenantPermissions :many
SELECT id, name, slug
FROM permissions
WHERE slug LIKE 'tenant.%'
ORDER BY slug;

-- name: CountOrganizationSeats :one
-- Paid seats (CF-4): members holding at least one billable role; total =
-- every member of the organization.
SELECT
    COUNT(*) FILTER (WHERE EXISTS (
        SELECT 1 FROM organization_member_roles omr
        INNER JOIN roles r ON r.id = omr.role_id AND r.scope = 'tenant' AND r.is_billable
        WHERE omr.organization_id = om.organization_id AND omr.member_id = om.id
    ))::bigint AS billable,
    COUNT(*)::bigint AS total
FROM organization_members om
INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id);
