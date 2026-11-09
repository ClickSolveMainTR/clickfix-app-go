-- CF-25 / CF-8: channel instances (the inboxes of an organization).

-- name: InsertChannelInstance :one
-- The uuid is chosen by the caller: the sealed credentials are bound to it.
INSERT INTO channel_instances (uuid, organization_id, channel_type, name, status, external_id, credentials_enc, settings, created_by, brand_id)
VALUES (sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(channel_type), sqlc.arg(name), sqlc.arg(status), sqlc.narg(external_id),
        sqlc.arg(credentials_enc), sqlc.arg(settings), sqlc.narg(created_by), sqlc.narg(brand_id))
RETURNING *;

-- name: GetChannelInstanceByUUID :one
SELECT * FROM channel_instances WHERE organization_id = $1 AND uuid = $2 AND deleted_at IS NULL;

-- name: GetChannelInstanceByID :one
SELECT * FROM channel_instances WHERE organization_id = $1 AND id = $2;

-- name: GetChannelInstanceByPublicUUID :one
-- Webhooks address an instance by its uuid only (no organization in the
-- request); the uuid is unguessable and the adapter still verifies the
-- signature with the instance's own secret.
SELECT * FROM channel_instances WHERE uuid = $1 AND deleted_at IS NULL;

-- name: GetChannelInstanceByExternal :one
SELECT * FROM channel_instances
WHERE channel_type = $1 AND external_id = $2 AND deleted_at IS NULL;

-- name: ListChannelInstances :many
SELECT * FROM channel_instances
WHERE organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
  AND (sqlc.narg(channel_type)::text IS NULL OR channel_type = sqlc.narg(channel_type))
ORDER BY channel_type, lower(name), id;

-- name: UpdateChannelInstance :one
UPDATE channel_instances SET
    brand_id=sqlc.narg(brand_id), name = sqlc.arg(name),
    status = sqlc.arg(status),
    external_id = sqlc.narg(external_id),
    credentials_enc = sqlc.arg(credentials_enc),
    settings = sqlc.arg(settings)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: TouchChannelInstanceInbound :exec
UPDATE channel_instances SET last_inbound_at = sqlc.arg(at)::timestamptz,
    status = CASE WHEN status = 'error' THEN 'active' ELSE status END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: TouchChannelInstanceOutbound :exec
UPDATE channel_instances SET last_outbound_at = sqlc.arg(at)::timestamptz
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SetChannelInstanceError :exec
UPDATE channel_instances SET last_error = sqlc.arg(error)::text, last_error_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SoftDeleteChannelInstance :one
UPDATE channel_instances SET deleted_at = NOW(), status = 'disconnected', credentials_enc = ''
WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
RETURNING *;

-- CF-61: health.

-- name: TouchChannelInstanceWebhook :exec
UPDATE channel_instances SET last_webhook_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SetChannelInstanceHealth :one
-- A failed check moves an active inbox to error (it shows "reconnect");
-- a passing one clears error back to active.
UPDATE channel_instances SET
    health_ok = sqlc.arg(ok)::boolean,
    health_detail = sqlc.arg(detail)::text,
    health_checked_at = NOW(),
    token_expires_at = sqlc.narg(token_expires_at)::timestamptz,
    status = CASE
        WHEN NOT sqlc.arg(ok)::boolean AND status = 'active' THEN 'error'
        WHEN sqlc.arg(ok)::boolean AND status = 'error' THEN 'active'
        ELSE status END,
    last_error = CASE WHEN NOT sqlc.arg(ok)::boolean THEN sqlc.arg(detail)::text ELSE last_error END,
    last_error_at = CASE WHEN NOT sqlc.arg(ok)::boolean THEN NOW() ELSE last_error_at END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: ListLiveChannelInstances :many
-- Every connected instance of the given types (health and reconcile jobs).
SELECT * FROM channel_instances
WHERE deleted_at IS NULL AND status IN ('active', 'error')
  AND channel_type = ANY(sqlc.arg(types)::text[])
ORDER BY id;

-- name: CountChannelInstances :one
SELECT COUNT(*)::bigint FROM channel_instances WHERE organization_id = $1 AND deleted_at IS NULL;

-- name: ChannelInstanceOutboundStats :many
-- Outbound messages per instance and status since a time (error rate).
SELECT channel_instance_id::bigint AS instance_id, status, COUNT(*)::bigint AS count
FROM messages
WHERE organization_id = sqlc.arg(organization_id) AND direction = 'out' AND deleted_at IS NULL
  AND channel_instance_id IS NOT NULL AND created_at >= sqlc.arg(since)::timestamptz
GROUP BY channel_instance_id, status;

-- name: ClaimEmailDomain :one
-- Registers a sender domain for an organization; a domain already owned by
-- another organization returns no row.
INSERT INTO channel_email_domains (organization_id, domain, resend_domain_id, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(domain), sqlc.arg(resend_domain_id), sqlc.narg(created_by))
ON CONFLICT (domain) DO UPDATE SET resend_domain_id = EXCLUDED.resend_domain_id
    WHERE channel_email_domains.organization_id = EXCLUDED.organization_id
RETURNING *;

-- name: GetEmailDomain :one
SELECT * FROM channel_email_domains WHERE organization_id = $1 AND domain = $2;

-- name: ListWidgetIdentityRejections :many
-- CF-195: rejected identify calls of the last week per widget (CF-176
-- records them); channel health turns them into a warning.
SELECT channel_instance_id, code, rejected_at FROM widget_identity_rejections
WHERE organization_id = sqlc.arg(organization_id) AND rejected_at > now() - interval '7 days';
