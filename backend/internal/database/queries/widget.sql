-- CF-55: public web widget (visitor API). Every query is scoped to the
-- organization and, for visitor data, to the visitor's contact and widget.

-- name: ListWidgetConversations :many
SELECT c.uuid, c.number, c.status, c.subject, c.last_message_at, c.last_message_preview,
       c.last_message_direction, c.created_at, c.resolved_at, c.closed_at,
       COALESCE((SELECT COUNT(*) FROM messages m
                 WHERE m.organization_id = c.organization_id AND m.conversation_id = c.id
                   AND m.direction = 'out' AND m.private = FALSE AND m.deleted_at IS NULL
                   AND m.status IN ('sent', 'delivered')), 0)::bigint AS unread_count,
       r.score AS rating_score
FROM conversations c
LEFT JOIN conversation_ratings r ON r.organization_id = c.organization_id AND r.conversation_id = c.id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.contact_id = sqlc.arg(contact_id)
  AND c.channel_instance_id = sqlc.arg(channel_instance_id)
  AND c.deleted_at IS NULL
ORDER BY c.last_message_at DESC, c.id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetWidgetConversation :one
SELECT * FROM conversations
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND contact_id = sqlc.arg(contact_id)
  AND channel_instance_id = sqlc.arg(channel_instance_id)
  AND deleted_at IS NULL;

-- name: ListWidgetMessages :many
-- What the contact may see: their own messages and agent replies the
-- channel delivered; never private notes or system events.
SELECT m.id, m.uuid, m.direction, m.sender_type, m.kind, m.content, m.status, m.client_id,
       m.created_at, m.read_at, u.name AS sender_user_name
FROM messages m
LEFT JOIN users u ON u.id = m.sender_user_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.conversation_id = sqlc.arg(conversation_id)
  AND m.deleted_at IS NULL
  AND m.private = FALSE
  AND (m.direction = 'in' OR (m.direction = 'out' AND m.status IN ('sent', 'delivered', 'read')))
  AND (sqlc.arg(after_id)::bigint = 0 OR m.id > sqlc.arg(after_id)::bigint)
  AND (sqlc.arg(before_id)::bigint = 0 OR m.id < sqlc.arg(before_id)::bigint)
ORDER BY m.id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetWidgetMessageID :one
SELECT id FROM messages
WHERE organization_id = $1 AND conversation_id = $2 AND uuid = $3 AND deleted_at IS NULL;

-- name: ListMessageAttachmentsByOwners :many
SELECT * FROM attachments
WHERE organization_id = sqlc.arg(organization_id)
  AND owner_type = 'message'
  AND owner_id = ANY(sqlc.arg(owner_ids)::uuid[])
  AND deleted_at IS NULL
  AND scan_status <> 'infected'
ORDER BY id ASC;

-- name: ListUnreadOutboundExternalIDs :many
SELECT external_id::text FROM messages
WHERE organization_id = $1 AND conversation_id = $2
  AND direction = 'out' AND private = FALSE AND deleted_at IS NULL
  AND status IN ('sent', 'delivered') AND external_id IS NOT NULL
ORDER BY id ASC
LIMIT 200;

-- name: GetConversationRating :one
SELECT * FROM conversation_ratings WHERE organization_id = $1 AND conversation_id = $2;

-- name: InsertConversationRating :one
INSERT INTO conversation_ratings (organization_id, conversation_id, contact_id, channel_instance_id, score, comment)
VALUES (sqlc.arg(organization_id), sqlc.arg(conversation_id), sqlc.narg(contact_id), sqlc.narg(channel_instance_id),
        sqlc.arg(score), sqlc.arg(comment))
ON CONFLICT (organization_id, conversation_id) DO NOTHING
RETURNING *;

-- name: ListWidgetTeam :many
-- Active members shown on the widget header (first names only).
SELECT u.uuid, u.name, u.surname
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL AND u.status = 'active'
WHERE om.organization_id = sqlc.arg(organization_id)
  AND (cardinality(sqlc.arg(user_uuids)::uuid[]) = 0 OR u.uuid = ANY(sqlc.arg(user_uuids)::uuid[]))
ORDER BY om.created_at ASC
LIMIT sqlc.arg(row_limit);

-- name: UpdateWidgetContactProfile :exec
-- Profile edits of the widget visitor update the same contact (never a new
-- one, docs/widget/05). Name always; e-mail and phone only fill empty ones.
UPDATE contacts SET
    name = CASE WHEN sqlc.arg(name)::text <> '' THEN sqlc.arg(name)::text ELSE name END,
    email = COALESCE(email, sqlc.narg(email)::text),
    phone = COALESCE(phone, sqlc.narg(phone)::text),
    updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SetContactCompanyIfEmpty :exec
UPDATE contacts SET company_id = sqlc.arg(company_id), updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND company_id IS NULL;

-- name: GetCompanyByExternalID :one
SELECT * FROM companies
WHERE organization_id = $1 AND external_id = sqlc.arg(external_id)::text AND deleted_at IS NULL
ORDER BY id ASC
LIMIT 1;

-- CF-206: a signed company id (user_hash covers user_id and company.id).

-- name: GetWidgetCompanyByID :one
SELECT * FROM companies
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SetCompanyExternalIDIfEmpty :exec
-- The company matched by domain takes the proved external id when it has
-- none (the id was looked up first, so no other live company holds it).
UPDATE companies SET external_id = sqlc.arg(external_id)::text, updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND external_id IS NULL AND deleted_at IS NULL;

-- name: MarkCompanyVerified :exec
UPDATE companies SET verified_at = sqlc.arg(verified_at), updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SetContactCompany :exec
-- A proved company id moves the contact to that company even when it had
-- another one (the unsigned path only fills an empty company).
UPDATE contacts SET company_id = sqlc.arg(company_id), updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
  AND company_id IS DISTINCT FROM sqlc.arg(company_id);

-- CF-150: editor drafts, published versions and install detection.

-- name: GetWidgetDraft :one
SELECT * FROM widget_drafts
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id);

-- name: UpsertWidgetDraft :one
INSERT INTO widget_drafts (organization_id, channel_instance_id, settings, base_version, updated_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(channel_instance_id), sqlc.arg(settings), sqlc.arg(base_version), sqlc.narg(updated_by))
ON CONFLICT (channel_instance_id) DO UPDATE SET
    settings = excluded.settings, base_version = excluded.base_version, updated_by = excluded.updated_by
RETURNING *;

-- name: DeleteWidgetDraft :exec
DELETE FROM widget_drafts
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id);

-- name: LatestWidgetVersion :one
SELECT COALESCE(MAX(version), 0)::int AS version FROM widget_versions
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id);

-- name: InsertWidgetVersion :one
INSERT INTO widget_versions (organization_id, channel_instance_id, version, settings, note, rollback_of, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(channel_instance_id), sqlc.arg(version), sqlc.arg(settings), sqlc.arg(note),
        sqlc.narg(rollback_of), sqlc.narg(created_by))
RETURNING *;

-- name: ListWidgetVersions :many
SELECT v.uuid, v.version, v.note, v.rollback_of, v.created_at,
       COALESCE(NULLIF(TRIM(u.name || ' ' || u.surname), ''), '')::text AS created_by_name
FROM widget_versions v
LEFT JOIN users u ON u.id = v.created_by
WHERE v.organization_id = sqlc.arg(organization_id) AND v.channel_instance_id = sqlc.arg(channel_instance_id)
ORDER BY v.version DESC
LIMIT sqlc.arg(row_limit);

-- name: GetWidgetVersion :one
SELECT * FROM widget_versions
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id)
  AND version = sqlc.arg(version);

-- name: SetWidgetInstanceSettings :one
UPDATE channel_instances SET settings = sqlc.arg(settings), updated_at = now()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND channel_type = 'widget' AND deleted_at IS NULL
RETURNING *;

-- name: TouchWidgetInstall :exec
-- Bootstrap records where the widget runs; a row is rewritten at most every
-- five minutes so busy sites do not turn every page view into a write.
INSERT INTO widget_installs (organization_id, channel_instance_id, origin, sdk_version, last_page_url)
VALUES (sqlc.arg(organization_id), sqlc.arg(channel_instance_id), sqlc.arg(origin), sqlc.arg(sdk_version), sqlc.arg(last_page_url))
ON CONFLICT (channel_instance_id, origin) DO UPDATE SET
    sdk_version = excluded.sdk_version, last_page_url = excluded.last_page_url,
    hits = widget_installs.hits + 1, last_seen_at = now()
WHERE widget_installs.last_seen_at < now() - interval '5 minutes';

-- name: ListWidgetInstalls :many
SELECT origin, sdk_version, last_page_url, hits, first_seen_at, last_seen_at FROM widget_installs
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id)
ORDER BY last_seen_at DESC
LIMIT 50;

-- name: TouchWidgetIdentityRejection :exec
-- A rejected identify call (CF-176). The request is not authenticated as a
-- user, so only a fixed code and the time are stored, and the row is
-- rewritten at most once a minute unless the code changed (which only
-- happens when the widget's identity secret was added or removed).
INSERT INTO widget_identity_rejections (organization_id, channel_instance_id, code)
VALUES (sqlc.arg(organization_id), sqlc.arg(channel_instance_id), sqlc.arg(code))
ON CONFLICT (channel_instance_id) DO UPDATE SET code = excluded.code, rejected_at = now()
WHERE widget_identity_rejections.organization_id = excluded.organization_id
  AND (widget_identity_rejections.code <> excluded.code
       OR widget_identity_rejections.rejected_at < now() - interval '1 minute');

-- name: ClearWidgetIdentityRejection :exec
DELETE FROM widget_identity_rejections
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id);

-- name: GetWidgetIdentityRejection :one
SELECT code, rejected_at FROM widget_identity_rejections
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id);

-- name: WidgetMedianFirstResponse :one
-- Median first response (seconds) of the widget's conversations of the last
-- 14 days; -1 without answered conversations.
SELECT COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM first_response_at - created_at)), -1)::float8 AS seconds
FROM conversations
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id)
  AND first_response_at IS NOT NULL AND first_response_at >= created_at
  AND created_at > now() - interval '14 days' AND deleted_at IS NULL;

-- name: MergeWidgetContactCustomFields :exec
-- Mapped visitor attributes (already validated against the definitions).
UPDATE contacts SET custom_fields = custom_fields || sqlc.arg(patch)::jsonb, updated_at = now()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- CF-201: mobile app keys (hash only; the key is shown once on creation).

-- name: CreateWidgetAppKey :one
INSERT INTO widget_app_keys (organization_id, channel_instance_id, platform, package_name, label, key_prefix, key_hash, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(channel_instance_id), sqlc.arg(platform), sqlc.arg(package_name), sqlc.arg(label), sqlc.arg(key_prefix), sqlc.arg(key_hash), sqlc.narg(created_by))
RETURNING *;

-- name: ListWidgetAppKeys :many
SELECT * FROM widget_app_keys
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id)
ORDER BY revoked_at IS NOT NULL, created_at DESC
LIMIT 100;

-- name: RevokeWidgetAppKey :one
UPDATE widget_app_keys SET revoked_at = now()
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id)
  AND uuid = sqlc.arg(uuid) AND revoked_at IS NULL
RETURNING *;

-- name: GetWidgetAppKeyByHash :one
-- The visitor API looks the key up by its hash; the caller checks the
-- instance, the package name and revoked_at (a revoked key must fail the
-- same way as an unknown one).
SELECT * FROM widget_app_keys WHERE key_hash = sqlc.arg(key_hash);

-- name: TouchWidgetAppKey :exec
-- last_used_at is rewritten at most every five minutes (busy apps).
UPDATE widget_app_keys SET last_used_at = now()
WHERE id = sqlc.arg(id) AND (last_used_at IS NULL OR last_used_at < now() - interval '5 minutes');
