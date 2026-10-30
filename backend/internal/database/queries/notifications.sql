-- name: CreateNotification :one
INSERT INTO notifications (
    user_id, channel, status, priority,
    title, body, payload, action_url, recipient, template_code, source_event,
    scheduled_at, max_attempts, organization_id
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8, $9, $10, $11,
    $12, $13, sqlc.narg(organization_id)
)
ON CONFLICT(user_id,channel,(payload->>'source_event_id')) WHERE payload ? 'source_event_id' DO UPDATE SET title=notifications.title
RETURNING *;

-- name: GetNotificationByUUID :one
SELECT * FROM notifications WHERE uuid = $1;

-- name: GetNotificationByID :one
SELECT * FROM notifications WHERE id = $1;

-- name: ListNotificationsForUser :many
SELECT * FROM notifications
WHERE user_id = sqlc.arg(user_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (
    sqlc.narg(unread)::bool IS NULL
    OR (sqlc.narg(unread)::bool = TRUE AND read_at IS NULL AND channel = 'inapp')
    OR (sqlc.narg(unread)::bool = FALSE AND (read_at IS NOT NULL OR channel <> 'inapp'))
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR title ILIKE '%' || sqlc.narg(q) || '%'
    OR body ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(template_code, '') ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(recipient, '') ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountNotificationsForUser :one
SELECT COUNT(*)::bigint FROM notifications
WHERE user_id = sqlc.arg(user_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (
    sqlc.narg(unread)::bool IS NULL
    OR (sqlc.narg(unread)::bool = TRUE AND read_at IS NULL AND channel = 'inapp')
    OR (sqlc.narg(unread)::bool = FALSE AND (read_at IS NOT NULL OR channel <> 'inapp'))
  )
  AND (
    sqlc.narg(q)::text IS NULL
    OR title ILIKE '%' || sqlc.narg(q) || '%'
    OR body ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(template_code, '') ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(recipient, '') ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: CountUnreadInappForUser :one
SELECT COUNT(*)::bigint FROM notifications
WHERE user_id = $1
  AND channel = 'inapp'
  AND read_at IS NULL
  AND status NOT IN ('cancelled', 'failed');

-- name: ListPlatformNotifications :many
SELECT * FROM notifications
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (sqlc.narg(user_id)::bigint IS NULL OR user_id = sqlc.narg(user_id))
  AND (
    sqlc.narg(q)::text IS NULL
    OR title ILIKE '%' || sqlc.narg(q) || '%'
    OR body ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountPlatformNotifications :one
SELECT COUNT(*)::bigint FROM notifications
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (sqlc.narg(user_id)::bigint IS NULL OR user_id = sqlc.narg(user_id))
  AND (
    sqlc.narg(q)::text IS NULL
    OR title ILIKE '%' || sqlc.narg(q) || '%'
    OR body ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListPlatformNotificationsForExport :many
SELECT * FROM notifications
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (sqlc.narg(user_id)::bigint IS NULL OR user_id = sqlc.narg(user_id))
  AND (
    sqlc.narg(q)::text IS NULL
    OR title ILIKE '%' || sqlc.narg(q) || '%'
    OR body ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY created_at DESC;

-- name: MarkNotificationProcessing :one
-- A failed row is retried only while attempts remain; once attempt_count
-- reaches max_attempts it is dead-lettered (terminal).
UPDATE notifications
SET status = 'processing',
    attempt_count = attempt_count + 1
WHERE id = $1
  AND status IN ('queued', 'failed')
  AND attempt_count < max_attempts
RETURNING *;

-- name: ResumeNotificationProcessing :one
-- A row left in processing (worker crash, lost status write) is resumed as a
-- new attempt, so a delivery that keeps hanging cannot loop forever.
UPDATE notifications
SET attempt_count = attempt_count + 1
WHERE id = $1
  AND status = 'processing'
  AND attempt_count < max_attempts
RETURNING *;

-- name: MarkNotificationDead :one
-- Dead letter: failed with no attempts left (max_attempts is lowered to the
-- attempts used when a permanent error stops retries early).
UPDATE notifications
SET status = 'failed',
    failed_at = NOW(),
    last_error = $2,
    max_attempts = GREATEST(LEAST(max_attempts, attempt_count), 1)
WHERE id = $1
RETURNING *;

-- name: ListStuckProcessingNotificationIDs :many
SELECT id FROM notifications
WHERE status = 'processing'
  AND updated_at < NOW() - make_interval(mins => sqlc.arg(stale_minutes)::int)
ORDER BY id
LIMIT 500;

-- name: MarkNotificationSent :one
-- Cast status on every use. Reusing an untyped param as both VARCHAR assignment
-- and IN ('delivered', 'read') makes Postgres raise 42P08
-- (inconsistent types deduced for parameter $2).
UPDATE notifications
SET status = sqlc.arg(status)::varchar,
    sent_at = COALESCE(sent_at, NOW()),
    delivered_at = CASE
        WHEN sqlc.arg(status)::varchar IN ('delivered', 'read') THEN COALESCE(delivered_at, NOW())
        ELSE delivered_at
    END,
    provider = sqlc.arg(provider),
    provider_reference = sqlc.arg(provider_reference),
    last_error = NULL
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MarkNotificationFailed :one
UPDATE notifications
SET status = 'failed',
    failed_at = NOW(),
    last_error = $2
WHERE id = $1
RETURNING *;

-- name: MarkNotificationRead :one
UPDATE notifications
SET status = 'read',
    read_at = COALESCE(read_at, NOW())
WHERE uuid = $1
  AND user_id = $2
  AND channel = 'inapp'
RETURNING *;

-- name: MarkAllNotificationsReadForUser :execrows
UPDATE notifications
SET status = 'read',
    read_at = COALESCE(read_at, NOW())
WHERE user_id = $1
  AND channel = 'inapp'
  AND read_at IS NULL;

-- name: InsertNotificationHistory :one
INSERT INTO notification_history (notification_id, event, metadata)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTemplateByCodeChannelLang :one
SELECT * FROM notification_templates
WHERE code = $1
  AND channel = $2
  AND language = $3
  AND active = TRUE;

-- name: GetNotificationPreferences :one
SELECT * FROM notification_preferences
WHERE user_id = $1;

-- name: UpsertNotificationPreferences :one
INSERT INTO notification_preferences (
    user_id, email_enabled, inapp_enabled, realtime_enabled, push_enabled,
    event_channels, quiet_hours_enabled, quiet_hours_start, quiet_hours_end,
    timezone, email_digest, digest_hour
)
VALUES (
    sqlc.arg(user_id), sqlc.arg(email_enabled), sqlc.arg(inapp_enabled), sqlc.arg(realtime_enabled), sqlc.arg(push_enabled),
    sqlc.arg(event_channels), sqlc.arg(quiet_hours_enabled), sqlc.arg(quiet_hours_start), sqlc.arg(quiet_hours_end),
    sqlc.arg(timezone), sqlc.arg(email_digest), sqlc.arg(digest_hour)
)
ON CONFLICT (user_id) DO UPDATE
SET email_enabled = EXCLUDED.email_enabled,
    inapp_enabled = EXCLUDED.inapp_enabled,
    realtime_enabled = EXCLUDED.realtime_enabled,
    push_enabled = EXCLUDED.push_enabled,
    event_channels = EXCLUDED.event_channels,
    quiet_hours_enabled = EXCLUDED.quiet_hours_enabled,
    quiet_hours_start = EXCLUDED.quiet_hours_start,
    quiet_hours_end = EXCLUDED.quiet_hours_end,
    timezone = EXCLUDED.timezone,
    email_digest = EXCLUDED.email_digest,
    digest_hour = EXCLUDED.digest_hour
RETURNING *;

-- name: InsertNotificationDigestItem :exec
INSERT INTO notification_digest_items (user_id, organization_id, event_type, title, body, action_url)
VALUES (sqlc.arg(user_id), sqlc.narg(organization_id), sqlc.arg(event_type), sqlc.arg(title), sqlc.arg(body), sqlc.narg(action_url));

-- name: ListDigestCandidates :many
-- Users with held digest items; the caller decides who is due.
SELECT p.*
FROM notification_preferences p
WHERE p.email_digest <> 'off'
  AND EXISTS (
      SELECT 1 FROM notification_digest_items i
      WHERE i.user_id = p.user_id AND i.digested_at IS NULL
  );

-- name: ListPendingDigestItems :many
SELECT *
FROM notification_digest_items
WHERE user_id = $1 AND digested_at IS NULL
ORDER BY created_at ASC, id ASC
LIMIT 200;

-- name: MarkDigestItemsSent :exec
UPDATE notification_digest_items
SET digested_at = NOW()
WHERE user_id = sqlc.arg(user_id) AND id = ANY(sqlc.arg(ids)::bigint[]);

-- name: SetNotificationDigestSent :exec
UPDATE notification_preferences
SET last_digest_at = NOW()
WHERE user_id = $1;

-- name: UpdateUserProfile :one
UPDATE users
SET name = COALESCE(sqlc.narg(name), name),
    surname = COALESCE(sqlc.narg(surname), surname)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SetUserEmailVerified :one
UPDATE users
SET email_verified_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: UpsertPushSubscription :one
INSERT INTO push_subscriptions (user_id, endpoint, key_p256dh, key_auth)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, endpoint) DO UPDATE SET
  key_p256dh = EXCLUDED.key_p256dh,
  key_auth = EXCLUDED.key_auth,
  updated_at = now()
RETURNING *;

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions
WHERE user_id = $1 AND endpoint = $2;

-- name: ListPushSubscriptionsByUser :many
SELECT * FROM push_subscriptions WHERE user_id = $1;

-- name: NotificationConversationContext :one
-- What a conversation notification says about it (CF-157): customer,
-- channel, last message, SLA deadlines and the organization's time zone.
SELECT c.uuid, c.number, c.subject, c.channel_type, c.assignee_user_id,
       c.last_message_preview, c.first_response_at,
       c.first_response_due_at, c.next_response_due_at, c.resolution_due_at,
       COALESCE(ct.name, '')::text AS contact_name, o.timezone AS organization_timezone
FROM conversations c
JOIN organizations o ON o.id = c.organization_id
LEFT JOIN contacts ct ON ct.id = c.contact_id AND ct.organization_id = c.organization_id
WHERE c.organization_id = sqlc.arg(organization_id) AND c.id = sqlc.arg(id);
