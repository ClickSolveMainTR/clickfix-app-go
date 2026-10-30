-- Platform health (CF-14): platform-wide aggregates for staff. These read
-- across organizations on purpose (platform.organizations.read).

-- name: OutboxHealth :one
SELECT
    COUNT(*) FILTER (WHERE status = 'pending')::bigint AS pending,
    COUNT(*) FILTER (WHERE status = 'failed')::bigint AS failed,
    COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at) FILTER (WHERE status = 'pending'))), 0)::bigint AS oldest_pending_seconds
FROM outbox_events;

-- name: ChannelHealth :many
-- Per channel type: live instances, instances with an error in the window,
-- and outbound messages (total / failed) in the window.
SELECT ci.channel_type,
       COUNT(DISTINCT ci.id)::bigint AS instances,
       COUNT(DISTINCT ci.id) FILTER (
           WHERE ci.status <> 'active' OR (ci.last_error_at IS NOT NULL AND ci.last_error_at > sqlc.arg(since)::timestamptz)
       )::bigint AS instances_with_errors,
       COALESCE(SUM(m.total), 0)::bigint AS outbound_total,
       COALESCE(SUM(m.failed), 0)::bigint AS outbound_failed
FROM channel_instances ci
LEFT JOIN LATERAL (
    SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE msg.status = 'failed') AS failed
    FROM messages msg
    WHERE msg.organization_id = ci.organization_id
      AND msg.channel_instance_id = ci.id
      AND msg.direction = 'out'
      AND msg.created_at > sqlc.arg(since)::timestamptz
) m ON TRUE
WHERE ci.deleted_at IS NULL
GROUP BY ci.channel_type
ORDER BY ci.channel_type;

-- name: OrganizationCounts :one
SELECT
    COUNT(*) FILTER (WHERE status = 'active')::bigint AS active,
    COUNT(*) FILTER (WHERE status = 'suspended')::bigint AS suspended,
    COUNT(*) FILTER (WHERE created_at > sqlc.arg(since)::timestamptz)::bigint AS created_recently
FROM organizations
WHERE deleted_at IS NULL;

-- name: EmailDeliveryHealth :one
-- E-mail notification delivery since $1: sent, retrying (failed with attempts
-- left), dead-lettered (failed, no attempts left) and still waiting.
SELECT
    COUNT(*) FILTER (WHERE status IN ('sent', 'delivered', 'read') AND created_at >= sqlc.arg(since))::bigint AS sent,
    COUNT(*) FILTER (WHERE status = 'failed' AND attempt_count < max_attempts)::bigint AS retrying,
    COUNT(*) FILTER (WHERE status = 'failed' AND attempt_count >= max_attempts AND created_at >= sqlc.arg(since))::bigint AS dead,
    COUNT(*) FILTER (WHERE status IN ('queued', 'processing'))::bigint AS pending,
    MAX(sent_at)::timestamptz AS last_sent_at,
    MAX(failed_at)::timestamptz AS last_failed_at
FROM notifications
WHERE channel = 'email'
  AND (created_at >= sqlc.arg(since) OR status IN ('queued', 'processing', 'failed'));

-- name: LastEmailDeliveryError :one
SELECT COALESCE(last_error, '')::text AS last_error
FROM notifications
WHERE channel = 'email' AND status = 'failed' AND failed_at IS NOT NULL
ORDER BY failed_at DESC
LIMIT 1;
