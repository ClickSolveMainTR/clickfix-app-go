-- name: ListCustomFieldDefinitions :many
SELECT *
FROM custom_field_definitions
WHERE organization_id = sqlc.arg(organization_id)
  AND entity_type = sqlc.arg(entity_type)
  AND object_type_id IS NOT DISTINCT FROM sqlc.narg(object_type_id)::bigint
  AND (sqlc.arg(include_archived)::boolean OR archived_at IS NULL)
ORDER BY position ASC, id ASC;

-- name: GetCustomFieldDefinition :one
SELECT *
FROM custom_field_definitions
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: CountCustomFieldDefinitions :one
SELECT COUNT(*)::bigint
FROM custom_field_definitions
WHERE organization_id = sqlc.arg(organization_id)
  AND entity_type = sqlc.arg(entity_type)
  AND object_type_id IS NOT DISTINCT FROM sqlc.narg(object_type_id)::bigint;

-- name: CreateCustomFieldDefinition :one
INSERT INTO custom_field_definitions (
    organization_id, entity_type, object_type_id, key, label, description, field_type,
    required, options, config, default_value, condition, position
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(entity_type), sqlc.narg(object_type_id), sqlc.arg(key),
    sqlc.arg(label), sqlc.arg(description), sqlc.arg(field_type), sqlc.arg(required),
    sqlc.arg(options), sqlc.arg(config), sqlc.narg(default_value), sqlc.narg(condition),
    COALESCE((
        SELECT MAX(position) + 1 FROM custom_field_definitions
        WHERE organization_id = sqlc.arg(organization_id)
          AND entity_type = sqlc.arg(entity_type)
          AND object_type_id IS NOT DISTINCT FROM sqlc.narg(object_type_id)::bigint
    ), 0)
)
RETURNING *;

-- name: UpdateCustomFieldDefinition :one
UPDATE custom_field_definitions
SET label = sqlc.arg(label),
    description = sqlc.arg(description),
    required = sqlc.arg(required),
    options = sqlc.arg(options),
    config = sqlc.arg(config),
    default_value = sqlc.narg(default_value),
    condition = sqlc.narg(condition)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: SetCustomFieldDefinitionArchived :one
UPDATE custom_field_definitions
SET archived_at = CASE WHEN sqlc.arg(archived)::boolean THEN COALESCE(archived_at, NOW()) ELSE NULL END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: SetCustomFieldDefinitionPosition :exec
UPDATE custom_field_definitions
SET position = sqlc.arg(position)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: ListCustomObjectTypes :many
SELECT *
FROM custom_object_types
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.arg(include_archived)::boolean OR archived_at IS NULL)
ORDER BY singular_label ASC, id ASC;

-- name: GetCustomObjectType :one
SELECT *
FROM custom_object_types
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: CountCustomObjectTypes :one
SELECT COUNT(*)::bigint FROM custom_object_types WHERE organization_id = sqlc.arg(organization_id);

-- name: CreateCustomObjectType :one
INSERT INTO custom_object_types (organization_id, key, singular_label, plural_label, description, icon)
VALUES (sqlc.arg(organization_id), sqlc.arg(key), sqlc.arg(singular_label), sqlc.arg(plural_label), sqlc.arg(description), sqlc.arg(icon))
RETURNING *;

-- name: UpdateCustomObjectType :one
UPDATE custom_object_types
SET singular_label = sqlc.arg(singular_label),
    plural_label = sqlc.arg(plural_label),
    description = sqlc.arg(description),
    icon = sqlc.arg(icon),
    archived_at = CASE WHEN sqlc.arg(archived)::boolean THEN COALESCE(archived_at, NOW()) ELSE NULL END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: GetCustomObjectRecord :one
SELECT *
FROM custom_object_records
WHERE organization_id = sqlc.arg(organization_id)
  AND object_type_id = sqlc.arg(object_type_id)
  AND uuid = sqlc.arg(uuid);

-- name: CreateCustomObjectRecord :one
INSERT INTO custom_object_records (organization_id, object_type_id, display_name, custom_fields, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(object_type_id), sqlc.arg(display_name), sqlc.arg(custom_fields), sqlc.narg(created_by))
RETURNING *;

-- name: UpdateCustomObjectRecord :one
UPDATE custom_object_records
SET display_name = sqlc.arg(display_name),
    custom_fields = sqlc.arg(custom_fields)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteCustomObjectRecord :execrows
DELETE FROM custom_object_records
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: OrganizationMemberUserUUIDs :many
SELECT u.uuid
FROM organization_members m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND u.uuid = ANY(sqlc.arg(user_uuids)::uuid[]);
