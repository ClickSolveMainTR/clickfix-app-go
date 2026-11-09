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
    required, options, config, default_value, condition, visibility, sensitive, widget_access, position
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(entity_type), sqlc.narg(object_type_id), sqlc.arg(key),
    sqlc.arg(label), sqlc.arg(description), sqlc.arg(field_type), sqlc.arg(required),
    sqlc.arg(options), sqlc.arg(config), sqlc.narg(default_value), sqlc.narg(condition),
    sqlc.arg(visibility), sqlc.arg(sensitive), sqlc.arg(widget_access),
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
    condition = sqlc.narg(condition),
    visibility = sqlc.arg(visibility),
    sensitive = sqlc.arg(sensitive),
    widget_access = sqlc.arg(widget_access)
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

-- CF-196: keys the widget sent that no field defines. One row per key; hits
-- and the last contact move on every sighting; a dismissed or resolved key
-- opens again when it is seen after that.
-- name: RecordUnknownCustomFieldKey :exec
INSERT INTO custom_field_unknown_keys (
    organization_id, entity_type, key, source, channel_instance_id, value_kind, last_contact_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(entity_type), sqlc.arg(key), sqlc.arg(source),
    sqlc.narg(channel_instance_id), sqlc.arg(value_kind), sqlc.narg(last_contact_id)
)
ON CONFLICT (organization_id, entity_type, key) DO UPDATE
SET hits = custom_field_unknown_keys.hits + 1,
    source = EXCLUDED.source,
    channel_instance_id = COALESCE(EXCLUDED.channel_instance_id, custom_field_unknown_keys.channel_instance_id),
    value_kind = EXCLUDED.value_kind,
    last_contact_id = COALESCE(EXCLUDED.last_contact_id, custom_field_unknown_keys.last_contact_id),
    last_seen_at = NOW(),
    resolved_at = NULL;

-- name: ListUnknownCustomFieldKeys :many
SELECT k.uuid, k.entity_type, k.key, k.source, k.value_kind, k.hits, k.first_seen_at, k.last_seen_at,
       ci.uuid AS channel_uuid, ci.name AS channel_name, ct.uuid AS last_contact_uuid
FROM custom_field_unknown_keys k
LEFT JOIN channel_instances ci ON ci.id = k.channel_instance_id AND ci.organization_id = k.organization_id
LEFT JOIN contacts ct ON ct.id = k.last_contact_id AND ct.organization_id = k.organization_id
WHERE k.organization_id = sqlc.arg(organization_id)
  AND k.dismissed_at IS NULL AND k.resolved_at IS NULL
  AND (sqlc.narg(channel_uuid)::uuid IS NULL OR ci.uuid = sqlc.narg(channel_uuid)::uuid)
ORDER BY k.last_seen_at DESC
LIMIT 100;

-- name: DismissUnknownCustomFieldKey :execrows
UPDATE custom_field_unknown_keys
SET dismissed_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND dismissed_at IS NULL;

-- name: ResolveUnknownCustomFieldKey :exec
UPDATE custom_field_unknown_keys
SET resolved_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND entity_type = sqlc.arg(entity_type)
  AND key = sqlc.arg(key) AND resolved_at IS NULL;

-- name: StoreContactUnknownField :exec
INSERT INTO custom_field_unknown_values (organization_id, contact_id, key, value, verified)
SELECT sqlc.arg(organization_id), sqlc.arg(contact_id), sqlc.arg(key)::text, sqlc.arg(value)::jsonb, sqlc.arg(verified)
WHERE EXISTS (SELECT 1 FROM custom_field_unknown_values
              WHERE organization_id = sqlc.arg(organization_id) AND contact_id = sqlc.arg(contact_id) AND key = sqlc.arg(key)::text)
   OR (SELECT COUNT(*) FROM custom_field_unknown_values
       WHERE organization_id = sqlc.arg(organization_id) AND contact_id = sqlc.arg(contact_id)) < 50
ON CONFLICT (contact_id, key) DO UPDATE
SET value = EXCLUDED.value, verified = EXCLUDED.verified, received_at = NOW();

-- name: ContactsWithUnknownField :many
SELECT u.contact_id, ct.custom_fields, u.value, u.verified
FROM custom_field_unknown_values u
JOIN contacts ct ON ct.id = u.contact_id AND ct.organization_id = u.organization_id
WHERE u.organization_id = sqlc.arg(organization_id) AND u.key = sqlc.arg(key)::text
ORDER BY u.contact_id
LIMIT 5000;

-- name: DeleteContactUnknownField :exec
DELETE FROM custom_field_unknown_values
WHERE organization_id = sqlc.arg(organization_id) AND contact_id = sqlc.arg(contact_id) AND key = sqlc.arg(key)::text;

-- name: SetContactCustomField :exec
UPDATE contacts
SET custom_fields = custom_fields || jsonb_build_object(sqlc.arg(key)::text, sqlc.arg(value)::jsonb)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ContactUnknownValues :many
SELECT key, value, verified, received_at
FROM custom_field_unknown_values
WHERE organization_id = sqlc.arg(organization_id) AND contact_id = sqlc.arg(contact_id)
ORDER BY key;

-- name: CustomFieldsOfContact :one
SELECT id, custom_fields
FROM contacts
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: CustomFieldsOfCompany :one
SELECT id, custom_fields
FROM companies
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: ContactAndCompanyCustomFields :one
SELECT ct.custom_fields AS contact_fields,
       COALESCE(co.custom_fields, '{}'::jsonb)::jsonb AS company_fields
FROM contacts ct
LEFT JOIN companies co ON co.id = ct.company_id AND co.organization_id = ct.organization_id
WHERE ct.organization_id = sqlc.arg(organization_id) AND ct.id = sqlc.arg(id);

-- name: ContactCustomFieldsByID :one
SELECT custom_fields FROM contacts WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: CompanyCustomFieldsByID :one
SELECT custom_fields FROM companies WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- CF-196: article audiences (catalog conditions by option id).
-- name: KnowledgeArticleAudience :one
SELECT a.id, COALESCE(au.conditions, '[]'::jsonb)::jsonb AS conditions
FROM knowledge_articles a
LEFT JOIN knowledge_article_audiences au ON au.article_id = a.id AND au.organization_id = a.organization_id
WHERE a.organization_id = sqlc.arg(organization_id) AND a.uuid = sqlc.arg(uuid) AND a.deleted_at IS NULL;

-- name: SaveKnowledgeArticleAudience :exec
INSERT INTO knowledge_article_audiences (organization_id, article_id, conditions)
VALUES (sqlc.arg(organization_id), sqlc.arg(article_id), sqlc.arg(conditions))
ON CONFLICT (article_id) DO UPDATE SET conditions = EXCLUDED.conditions, updated_at = NOW();

-- name: KnowledgeAudiencesByUUID :many
SELECT a.uuid, au.conditions
FROM knowledge_article_audiences au
JOIN knowledge_articles a ON a.id = au.article_id AND a.organization_id = au.organization_id
WHERE au.organization_id = sqlc.arg(organization_id) AND a.uuid = ANY(sqlc.arg(uuids)::uuid[])
  AND au.conditions <> '[]'::jsonb;
