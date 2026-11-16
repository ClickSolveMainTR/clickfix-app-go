-- CF-233 remote support sessions (internal/modules/remotesupport).

-- name: RemoteSupportSettings :one
SELECT * FROM remote_support_settings WHERE organization_id = $1;

-- name: RemoteSupportSaveSettings :one
INSERT INTO remote_support_settings(organization_id, enabled, allow_control, recording, max_duration_minutes, recording_retention_days)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (organization_id) DO UPDATE SET enabled = EXCLUDED.enabled, allow_control = EXCLUDED.allow_control,
    recording = EXCLUDED.recording, max_duration_minutes = EXCLUDED.max_duration_minutes,
    recording_retention_days = EXCLUDED.recording_retention_days
RETURNING *;

-- name: RemoteSupportSetPlatformState :one
INSERT INTO remote_support_settings(organization_id, platform_state, platform_reason, platform_changed_at, platform_changed_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(platform_state), sqlc.arg(platform_reason), now(), sqlc.narg(platform_changed_by))
ON CONFLICT (organization_id) DO UPDATE SET platform_state = EXCLUDED.platform_state, platform_reason = EXCLUDED.platform_reason,
    platform_changed_at = now(), platform_changed_by = EXCLUDED.platform_changed_by
RETURNING *;

-- name: RemoteSupportCreateSession :one
INSERT INTO remote_support_sessions(organization_id, conversation_id, agent_user_id, contact_id, code_lookup, code_hash,
    code_expires_at, join_token_hash, mode_requested, recording_consent, room_name, max_duration_minutes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: RemoteSupportSession :one
SELECT * FROM remote_support_sessions WHERE organization_id = $1 AND uuid = $2;

-- name: RemoteSupportSessionsForConversation :many
SELECT * FROM remote_support_sessions WHERE organization_id = $1 AND conversation_id = $2
ORDER BY created_at DESC LIMIT 20;

-- name: RemoteSupportOpenForConversation :one
SELECT count(*) FROM remote_support_sessions
WHERE organization_id = $1 AND conversation_id = $2 AND status IN ('pending','joined','active');

-- name: RemoteSupportSessionByUUID :one
SELECT * FROM remote_support_sessions WHERE uuid = $1;

-- name: RemoteSupportPendingByCode :one
SELECT * FROM remote_support_sessions WHERE code_lookup = $1 AND status = 'pending';

-- name: RemoteSupportSessionByJoinToken :one
SELECT * FROM remote_support_sessions WHERE join_token_hash = $1;

-- name: RemoteSupportSessionByRoom :one
SELECT * FROM remote_support_sessions WHERE room_name = $1;

-- name: RemoteSupportSessionByEgress :one
SELECT * FROM remote_support_sessions WHERE egress_id = $1 AND egress_id <> '';

-- name: RemoteSupportCodeAttempt :one
-- A wrong code charged to the pending session that owns its lookup prefix.
UPDATE remote_support_sessions SET code_attempts = code_attempts + 1
WHERE organization_id = $1 AND id = $2 AND status = 'pending'
RETURNING code_attempts;

-- name: RemoteSupportResolve :one
-- Single use: the code and the join link stop working once resolved.
UPDATE remote_support_sessions SET status = 'joined', resolved_at = now(), client_secret_hash = $3,
    client_os = $4, client_version = $5
WHERE organization_id = $1 AND id = $2 AND status = 'pending' AND code_expires_at > now()
RETURNING *;

-- name: RemoteSupportSetConsent :one
UPDATE remote_support_sessions SET
    view_consent_at = COALESCE(view_consent_at, sqlc.narg(view_consent_at)),
    control_consent_at = CASE WHEN sqlc.arg(set_control)::boolean THEN sqlc.narg(control_consent_at) ELSE control_consent_at END,
    control_revoked_at = CASE WHEN sqlc.arg(set_control)::boolean THEN sqlc.narg(control_revoked_at) ELSE control_revoked_at END,
    recording_consent = CASE WHEN sqlc.arg(set_recording)::boolean THEN sqlc.narg(recording_consent) ELSE recording_consent END,
    recording_consent_at = CASE WHEN sqlc.arg(set_recording)::boolean THEN now() ELSE recording_consent_at END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status IN ('joined','active')
RETURNING *;

-- name: RemoteSupportActivate :one
UPDATE remote_support_sessions SET status = 'active', started_at = COALESCE(started_at, sqlc.arg(started_at))
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status IN ('joined','active')
RETURNING *;

-- name: RemoteSupportEnd :one
UPDATE remote_support_sessions SET status = sqlc.arg(status), ended_at = sqlc.arg(ended_at), ended_reason = sqlc.arg(ended_reason),
    ended_by = sqlc.arg(ended_by), ended_by_user_id = sqlc.narg(ended_by_user_id)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status IN ('pending','joined','active')
RETURNING *;

-- name: RemoteSupportSetBilled :exec
UPDATE remote_support_sessions SET billed_minutes = $3 WHERE organization_id = $1 AND id = $2;

-- name: RemoteSupportReport :one
UPDATE remote_support_sessions SET reported_at = COALESCE(reported_at, now()), report_reason = $3
WHERE organization_id = $1 AND id = $2
RETURNING *;

-- name: RemoteSupportRecentReports :one
SELECT count(*) FROM remote_support_sessions
WHERE organization_id = $1 AND reported_at > sqlc.arg(since);

-- name: RemoteSupportMinutesSince :one
SELECT COALESCE(sum(billed_minutes), 0)::bigint FROM remote_support_sessions
WHERE organization_id = $1 AND ended_at >= sqlc.arg(since);

-- name: RemoteSupportStartRecording :one
UPDATE remote_support_sessions SET recording_status = 'starting', recording_key = $3, recording_expires_at = $4
WHERE organization_id = $1 AND id = $2 AND recording_status = 'none'
RETURNING *;

-- name: RemoteSupportSetEgress :exec
UPDATE remote_support_sessions SET egress_id = $3, recording_status = 'active'
WHERE organization_id = $1 AND id = $2 AND recording_status = 'starting';

-- name: RemoteSupportSetRecordingStatus :exec
UPDATE remote_support_sessions SET recording_status = $3, recording_size_bytes = GREATEST(recording_size_bytes, $4)
WHERE organization_id = $1 AND id = $2 AND recording_status NOT IN ('complete','failed','deleted');

-- name: RemoteSupportOpenSessions :many
-- Sweep: every open session, with the conversation status so a closed
-- conversation ends its session.
SELECT sqlc.embed(s), c.status AS conversation_status
FROM remote_support_sessions s
JOIN conversations c ON c.organization_id = s.organization_id AND c.id = s.conversation_id
WHERE s.status IN ('pending','joined','active')
ORDER BY s.id
LIMIT 500;

-- name: RemoteSupportOpenSessionsForOrg :many
SELECT * FROM remote_support_sessions WHERE organization_id = $1 AND status IN ('pending','joined','active');

-- name: RemoteSupportExpiredRecordings :many
SELECT * FROM remote_support_sessions
WHERE recording_status = 'complete' AND recording_expires_at IS NOT NULL AND recording_expires_at < now()
ORDER BY recording_expires_at
LIMIT 100;

-- name: RemoteSupportMarkRecordingDeleted :exec
UPDATE remote_support_sessions SET recording_status = 'deleted' WHERE organization_id = $1 AND id = $2;

-- name: RemoteSupportPlatformOrganizations :many
-- Platform view: organizations that use remote support, were reported or
-- are not active, with their report counts of the last 30 days.
SELECT o.uuid, o.name, o.slug,
    COALESCE(st.enabled, false)::boolean AS enabled,
    COALESCE(st.platform_state, 'active')::text AS platform_state,
    COALESCE(st.platform_reason, '')::text AS platform_reason,
    st.platform_changed_at,
    (SELECT count(*) FROM remote_support_sessions s WHERE s.organization_id = o.id AND s.reported_at > now() - interval '30 days')::bigint AS reports_30d,
    (SELECT count(*) FROM remote_support_sessions s WHERE s.organization_id = o.id AND s.created_at > now() - interval '30 days')::bigint AS sessions_30d
FROM organizations o
LEFT JOIN remote_support_settings st ON st.organization_id = o.id
WHERE o.deleted_at IS NULL
  AND (st.organization_id IS NOT NULL OR EXISTS (SELECT 1 FROM remote_support_sessions s WHERE s.organization_id = o.id))
  AND (sqlc.narg(state)::text IS NULL OR COALESCE(st.platform_state, 'active') = sqlc.narg(state)::text)
ORDER BY reports_30d DESC, o.name
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: RemoteSupportPlatformReports :many
SELECT s.uuid, s.status, s.mode_requested, s.reported_at, s.report_reason, s.ended_reason, s.created_at,
    o.uuid AS organization_uuid, o.name AS organization_name,
    COALESCE(u.name || ' ' || u.surname, '')::text AS agent_name
FROM remote_support_sessions s
JOIN organizations o ON o.id = s.organization_id
LEFT JOIN users u ON u.id = s.agent_user_id
WHERE s.reported_at IS NOT NULL
ORDER BY s.reported_at DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
