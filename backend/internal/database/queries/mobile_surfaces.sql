-- CF-152: widget tokens, Live Activity push tokens and the Live Activity switch.

-- name: RevokeWidgetTokensForDevice :execrows
UPDATE widget_tokens
SET revoked_at = NOW()
WHERE user_id = sqlc.arg(user_id)
  AND device_id = sqlc.arg(device_id)
  AND revoked_at IS NULL;

-- name: CreateWidgetToken :one
INSERT INTO widget_tokens (user_id, organization_id, device_id, token_hash, session_uuid, expires_at)
VALUES (sqlc.arg(user_id), sqlc.arg(organization_id), sqlc.arg(device_id), sqlc.arg(token_hash),
        sqlc.narg(session_uuid), sqlc.arg(expires_at))
RETURNING *;

-- name: GetValidWidgetToken :one
-- A token is valid while it is neither revoked nor expired and, when it is
-- bound to a refresh session, while that session is active.
SELECT wt.id, wt.user_id, wt.organization_id, wt.device_id, wt.session_uuid,
       wt.expires_at, wt.last_used_at, u.uuid AS user_uuid
FROM widget_tokens wt
JOIN users u ON u.id = wt.user_id
WHERE wt.token_hash = sqlc.arg(token_hash)
  AND wt.revoked_at IS NULL
  AND wt.expires_at > NOW()
  AND (
    wt.session_uuid IS NULL
    OR EXISTS (
      SELECT 1 FROM refresh_tokens rt
      WHERE rt.uuid = wt.session_uuid
        AND rt.user_id = wt.user_id
        AND rt.revoked_at IS NULL
        AND rt.expires_at > NOW()
    )
  );

-- name: TouchWidgetToken :exec
UPDATE widget_tokens
SET last_used_at = NOW()
WHERE id = sqlc.arg(id)
  AND (last_used_at IS NULL OR last_used_at < NOW() - INTERVAL '1 minute');

-- name: RebindWidgetTokensSession :execrows
-- Refresh rotation replaces the refresh row; widget tokens follow it.
UPDATE widget_tokens
SET session_uuid = sqlc.arg(to_session)
WHERE user_id = sqlc.arg(user_id)
  AND session_uuid = sqlc.arg(from_session)
  AND revoked_at IS NULL;

-- name: UpsertLiveActivityToken :one
-- The token is unique: a token re-registered (same phone, other account or a
-- new activity) moves to the caller and is re-enabled.
INSERT INTO live_activity_tokens (user_id, device_id, kind, activity_type, activity_id, ref, token, environment)
VALUES (sqlc.arg(user_id), sqlc.arg(device_id), sqlc.arg(kind), sqlc.arg(activity_type),
        sqlc.arg(activity_id), sqlc.arg(ref), sqlc.arg(token), sqlc.arg(environment))
ON CONFLICT (token) DO UPDATE
SET user_id = EXCLUDED.user_id,
    device_id = EXCLUDED.device_id,
    kind = EXCLUDED.kind,
    activity_type = EXCLUDED.activity_type,
    activity_id = EXCLUDED.activity_id,
    ref = EXCLUDED.ref,
    environment = EXCLUDED.environment,
    disabled_at = NULL,
    disabled_reason = ''
RETURNING *;

-- name: DisableSupersededPushToStartTokens :exec
-- iOS hands out one push-to-start token per activity type and device; a new
-- one replaces the previous token.
UPDATE live_activity_tokens
SET disabled_at = NOW(), disabled_reason = 'superseded'
WHERE user_id = sqlc.arg(user_id)
  AND device_id = sqlc.arg(device_id)
  AND kind = 'push_to_start'
  AND activity_type = sqlc.arg(activity_type)
  AND token <> sqlc.arg(token)
  AND disabled_at IS NULL;

-- name: DeleteLiveActivityToken :execrows
DELETE FROM live_activity_tokens
WHERE user_id = sqlc.arg(user_id)
  AND token = sqlc.arg(token);

-- name: DeleteLiveActivityTokensForUser :execrows
DELETE FROM live_activity_tokens
WHERE user_id = sqlc.arg(user_id);

-- name: ListPushToStartLiveActivityTokens :many
SELECT * FROM live_activity_tokens
WHERE user_id = sqlc.arg(user_id)
  AND kind = 'push_to_start'
  AND activity_type = sqlc.arg(activity_type)
  AND disabled_at IS NULL
ORDER BY id;

-- name: ListUpdateLiveActivityTokensByRef :many
-- Every running activity about ref, whoever holds it (an ended conversation
-- ends the activity on each device that shows it).
SELECT * FROM live_activity_tokens
WHERE kind = 'update'
  AND activity_type = sqlc.arg(activity_type)
  AND ref = sqlc.arg(ref)
  AND disabled_at IS NULL
ORDER BY id;

-- name: DisableLiveActivityToken :exec
UPDATE live_activity_tokens
SET disabled_at = NOW(), disabled_reason = sqlc.arg(reason)
WHERE id = sqlc.arg(id)
  AND disabled_at IS NULL;

-- name: GetMobileSurfacePreferences :one
SELECT * FROM mobile_surface_preferences WHERE user_id = sqlc.arg(user_id);

-- name: UpsertMobileSurfacePreferences :one
INSERT INTO mobile_surface_preferences (user_id, live_activities_enabled)
VALUES (sqlc.arg(user_id), sqlc.arg(live_activities_enabled))
ON CONFLICT (user_id) DO UPDATE
SET live_activities_enabled = EXCLUDED.live_activities_enabled
RETURNING *;

-- name: GetLiveActivityRecipient :one
-- Push gates and locale of a user. A missing notification_preferences row
-- means push is off (mirrors the notifications defaults); a missing
-- mobile_surface_preferences row means Live Activities are on.
SELECT u.id, u.uuid, u.locale, u.status,
       COALESCE(np.push_enabled, FALSE)::boolean AS push_enabled,
       COALESCE(msp.live_activities_enabled, TRUE)::boolean AS live_activities_enabled
FROM users u
LEFT JOIN notification_preferences np ON np.user_id = u.id
LEFT JOIN mobile_surface_preferences msp ON msp.user_id = u.id
WHERE u.id = sqlc.arg(user_id);

-- name: GetLiveActivityConversation :one
SELECT c.id, c.uuid, c.organization_id, c.number, c.channel_type, c.subject, c.status,
       c.assignee_user_id, c.last_message_preview, c.sla_breached_at,
       COALESCE(ct.name, '')::text AS contact_name
FROM conversations c
LEFT JOIN contacts ct ON ct.id = c.contact_id AND ct.organization_id = c.organization_id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.id = sqlc.arg(id)
  AND c.deleted_at IS NULL;
