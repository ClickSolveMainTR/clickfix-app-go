-- CF-197: connected system ("Sistemini bağla").

-- name: IntegrationConnection :one
SELECT * FROM integration_connections WHERE organization_id = $1;

-- name: IntegrationSaveConnection :one
INSERT INTO integration_connections (organization_id, name, base_url, auth_type, auth_header, auth_enc, timeout_ms, updated_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(base_url), sqlc.arg(auth_type), sqlc.arg(auth_header), sqlc.arg(auth_enc), sqlc.arg(timeout_ms), sqlc.narg(updated_by))
ON CONFLICT (organization_id) DO UPDATE SET
    name = excluded.name, base_url = excluded.base_url, auth_type = excluded.auth_type, auth_header = excluded.auth_header,
    auth_enc = CASE WHEN sqlc.arg(keep_auth)::boolean THEN integration_connections.auth_enc ELSE excluded.auth_enc END,
    timeout_ms = excluded.timeout_ms, updated_by = excluded.updated_by
RETURNING *;

-- name: IntegrationSources :many
SELECT s.*, t.uuid AS object_type_uuid, t.key AS object_type_key, t.singular_label AS object_type_label
FROM integration_sources s JOIN custom_object_types t ON t.id = s.object_type_id AND t.organization_id = s.organization_id
WHERE s.organization_id = $1 AND t.archived_at IS NULL
ORDER BY t.singular_label;

-- name: IntegrationSourceByKey :one
SELECT s.*, t.uuid AS object_type_uuid, t.key AS object_type_key, t.singular_label AS object_type_label
FROM integration_sources s JOIN custom_object_types t ON t.id = s.object_type_id AND t.organization_id = s.organization_id
WHERE s.organization_id = $1 AND t.key = sqlc.arg(key) AND t.archived_at IS NULL;

-- name: IntegrationSaveSource :one
INSERT INTO integration_sources (organization_id, object_type_id, path_template, identity_key, items_path, fields, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (organization_id, object_type_id) DO UPDATE SET
    path_template = excluded.path_template, identity_key = excluded.identity_key, items_path = excluded.items_path,
    fields = excluded.fields, enabled = excluded.enabled
RETURNING *;

-- name: IntegrationDeleteSource :execrows
DELETE FROM integration_sources WHERE organization_id = $1 AND object_type_id = $2;

-- name: IntegrationObjectTypeByUUID :one
SELECT * FROM custom_object_types WHERE organization_id = $1 AND uuid = $2 AND archived_at IS NULL;

-- name: IntegrationObjectTypeByKey :one
SELECT * FROM custom_object_types WHERE organization_id = $1 AND key = sqlc.arg(key) AND archived_at IS NULL;

-- name: IntegrationActions :many
SELECT a.*, t.key AS object_type_key
FROM integration_actions a LEFT JOIN custom_object_types t ON t.id = a.object_type_id AND t.organization_id = a.organization_id
WHERE a.organization_id = $1
ORDER BY a.title;

-- name: IntegrationActionByKey :one
SELECT a.*, t.key AS object_type_key
FROM integration_actions a LEFT JOIN custom_object_types t ON t.id = a.object_type_id AND t.organization_id = a.organization_id
WHERE a.organization_id = $1 AND a.key = sqlc.arg(key);

-- name: IntegrationSaveAction :one
INSERT INTO integration_actions (organization_id, key, title, description, object_type_id, method, path_template, params, requires_approval, allowed_for, enabled)
VALUES (sqlc.arg(organization_id), sqlc.arg(key), sqlc.arg(title), sqlc.arg(description), sqlc.narg(object_type_id), sqlc.arg(method),
        sqlc.arg(path_template), sqlc.arg(params), sqlc.arg(requires_approval), sqlc.arg(allowed_for)::text[], sqlc.arg(enabled))
ON CONFLICT (organization_id, key) DO UPDATE SET
    title = excluded.title, description = excluded.description, object_type_id = excluded.object_type_id, method = excluded.method,
    path_template = excluded.path_template, params = excluded.params, requires_approval = excluded.requires_approval,
    allowed_for = excluded.allowed_for, enabled = excluded.enabled
RETURNING *;

-- name: IntegrationDeleteAction :execrows
DELETE FROM integration_actions WHERE organization_id = $1 AND key = sqlc.arg(key);

-- name: IntegrationPlaybooks :many
SELECT p.*, COALESCE(pr.intent, '')::text AS procedure_intent, pr.uuid AS procedure_uuid
FROM integration_playbooks p LEFT JOIN ai_procedures pr ON pr.id = p.procedure_id AND pr.organization_id = p.organization_id
WHERE p.organization_id = $1
ORDER BY p.title;

-- name: IntegrationPlaybookByIntent :one
SELECT p.*, COALESCE(pr.intent, '')::text AS procedure_intent, pr.uuid AS procedure_uuid
FROM integration_playbooks p LEFT JOIN ai_procedures pr ON pr.id = p.procedure_id AND pr.organization_id = p.organization_id
WHERE p.organization_id = $1 AND p.intent = sqlc.arg(intent);

-- name: IntegrationSavePlaybook :one
INSERT INTO integration_playbooks (organization_id, intent, title, instructions, concept_keys, action_keys, procedure_id, enabled)
VALUES (sqlc.arg(organization_id), sqlc.arg(intent), sqlc.arg(title), sqlc.arg(instructions), sqlc.arg(concept_keys)::text[],
        sqlc.arg(action_keys)::text[], sqlc.narg(procedure_id), sqlc.arg(enabled))
ON CONFLICT (organization_id, intent) DO UPDATE SET
    title = excluded.title, instructions = excluded.instructions, concept_keys = excluded.concept_keys,
    action_keys = excluded.action_keys, procedure_id = excluded.procedure_id, enabled = excluded.enabled
RETURNING *;

-- name: IntegrationDeletePlaybook :execrows
DELETE FROM integration_playbooks WHERE organization_id = $1 AND intent = sqlc.arg(intent);

-- name: IntegrationProcedureByUUID :one
SELECT id, intent FROM ai_procedures WHERE organization_id = $1 AND uuid = $2;

-- name: IntegrationLogCall :exec
INSERT INTO integration_calls (organization_id, kind, target, channel, method, masked_path, status_code, ok, error_code, duration_ms, actor_user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(kind), sqlc.arg(target), sqlc.arg(channel), sqlc.arg(method), sqlc.arg(masked_path),
        sqlc.arg(status_code), sqlc.arg(ok), sqlc.arg(error_code), sqlc.arg(duration_ms), sqlc.narg(actor_user_id));

-- name: IntegrationRecentCalls :many
SELECT kind, target, channel, method, masked_path, status_code, ok, error_code, duration_ms, created_at
FROM integration_calls WHERE organization_id = $1 ORDER BY id DESC LIMIT 20;

-- Identity of a customer for the connected system. The widget keys a
-- signed-in user as "user:<id>" (CF-176); the company external id is the
-- raw companies.external_id. CF-195 replaces both with first-class
-- columns; see modules/integrations/systems/usecase/identity.go.

-- name: IntegrationContactIdentity :one
SELECT c.id, c.uuid, c.name, c.custom_fields,
    COALESCE((SELECT ci.external_id FROM contact_identities ci
        WHERE ci.organization_id = c.organization_id AND ci.contact_id = c.id AND ci.external_id LIKE 'user:%'
        ORDER BY ci.last_seen_at DESC NULLS LAST, ci.id DESC LIMIT 1), '')::text AS user_identity,
    co.uuid AS company_uuid, COALESCE(co.name, '')::text AS company_name, COALESCE(co.external_id, '')::text AS company_external_id
FROM contacts c LEFT JOIN companies co ON co.id = c.company_id AND co.organization_id = c.organization_id AND co.deleted_at IS NULL
WHERE c.organization_id = $1 AND c.uuid = $2 AND c.deleted_at IS NULL;

-- name: IntegrationContactByExternalID :one
SELECT c.uuid FROM contact_identities ci JOIN contacts c ON c.id = ci.contact_id AND c.organization_id = ci.organization_id AND c.deleted_at IS NULL
WHERE ci.organization_id = $1 AND ci.external_id = sqlc.arg(identity)::text
ORDER BY ci.last_seen_at DESC NULLS LAST, ci.id DESC LIMIT 1;

-- name: IntegrationConversationContact :one
SELECT c.uuid FROM conversations v JOIN contacts c ON c.id = v.contact_id AND c.organization_id = v.organization_id
WHERE v.organization_id = $1 AND v.id = $2;

-- name: IntegrationConversationContactByUUID :one
SELECT c.uuid FROM conversations v JOIN contacts c ON c.id = v.contact_id AND c.organization_id = v.organization_id
WHERE v.organization_id = $1 AND v.uuid = $2;

-- name: IntegrationWidgetRejections :many
SELECT ci.uuid AS channel_uuid, ci.name AS channel_name, r.code, r.rejected_at
FROM widget_identity_rejections r JOIN channel_instances ci ON ci.id = r.channel_instance_id AND ci.organization_id = r.organization_id
WHERE r.organization_id = $1 ORDER BY r.rejected_at DESC LIMIT 10;

-- Screen context (CF-197): the page and tracked events the widget sent
-- with the latest inbound message of a conversation.

-- name: ConversationScreenContext :one
SELECT COALESCE(content_attributes->>'page_url', '')::text AS page_url,
    COALESCE(content_attributes->'events', '[]'::jsonb)::jsonb AS events, created_at
FROM messages
WHERE organization_id = $1 AND conversation_id = $2 AND direction = 'in'
  AND (content_attributes ? 'page_url' OR content_attributes ? 'events')
ORDER BY id DESC LIMIT 1;
