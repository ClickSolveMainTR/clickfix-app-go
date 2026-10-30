-- CF-52: dynamic request forms.

-- name: ListRequestForms :many
SELECT * FROM request_forms WHERE organization_id = $1 ORDER BY position, id;

-- name: GetRequestFormByUUID :one
SELECT * FROM request_forms WHERE organization_id = $1 AND uuid = $2;

-- name: FirstActiveRequestFormForChannel :one
-- The form a channel offers: the first active form (by position) listing it.
SELECT * FROM request_forms
WHERE organization_id = sqlc.arg(organization_id) AND is_active AND sqlc.arg(channel)::text = ANY(channels)
ORDER BY position, id
LIMIT 1;

-- name: NextRequestFormPosition :one
SELECT COALESCE(MAX(position), 0)::int + 1 FROM request_forms WHERE organization_id = $1;

-- name: InsertRequestForm :one
INSERT INTO request_forms (organization_id, name, description, is_active, channels, fields, position)
VALUES (sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(is_active),
        sqlc.arg(channels), sqlc.arg(fields), sqlc.arg(position))
RETURNING *;

-- name: UpdateRequestForm :one
UPDATE request_forms
SET name = sqlc.arg(name), description = sqlc.arg(description), is_active = sqlc.arg(is_active),
    channels = sqlc.arg(channels), fields = sqlc.arg(fields), position = sqlc.arg(position)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteRequestForm :execrows
DELETE FROM request_forms WHERE organization_id = $1 AND uuid = $2;

-- name: ListFormCustomFieldDefinitions :many
-- Conversation and contact custom fields a form may use.
SELECT * FROM custom_field_definitions
WHERE organization_id = $1 AND entity_type IN ('conversation', 'contact') AND object_type_id IS NULL
ORDER BY entity_type, position, id;

-- name: SetConversationFormData :exec
-- Writes a form submission onto the conversation it started.
UPDATE conversations
SET custom_fields = sqlc.arg(custom_fields),
    subject = sqlc.arg(subject),
    priority = sqlc.arg(priority),
    meta = jsonb_set(meta, '{request_form}', to_jsonb(sqlc.arg(form_uuid)::text))
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SetConversationCustomFields :exec
UPDATE conversations SET custom_fields = sqlc.arg(custom_fields)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SetContactFormData :exec
UPDATE contacts
SET name = COALESCE(NULLIF(sqlc.arg(name)::text, ''), name),
    email = COALESCE(email, NULLIF(sqlc.arg(email)::text, '')),
    phone = COALESCE(phone, NULLIF(sqlc.arg(phone)::text, '')),
    custom_fields = custom_fields || sqlc.arg(custom_fields)::jsonb
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);
