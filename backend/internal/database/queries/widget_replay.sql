-- name: InsertWidgetSession :one
INSERT INTO widget_sessions(organization_id,conversation_id,visitor_id,kind,status,control_enabled,requested_by,accepted_at,timeout_at,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;
-- name: GetWidgetSession :one
SELECT * FROM widget_sessions WHERE organization_id=$1 AND uuid=$2;
-- name: LockWidgetSession :one
SELECT * FROM widget_sessions WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: ListWidgetSessions :many
SELECT * FROM widget_sessions WHERE organization_id=$1 AND conversation_id=$2 AND expires_at>now() ORDER BY id DESC LIMIT 100;
-- name: UpdateWidgetSession :one
UPDATE widget_sessions SET status=$3,accepted_at=COALESCE(accepted_at,$4),ended_at=$5,
 expires_at=CASE WHEN $3::text IN ('revoked','declined') THEN now() ELSE expires_at END
WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: WidgetReplayUsage :one
SELECT COALESCE(sum(c.size_bytes),0)::bigint FROM widget_replay_chunks c JOIN widget_sessions s ON s.organization_id=c.organization_id AND s.id=c.session_id
WHERE c.organization_id=$1;
-- name: WidgetDailySessionCount :one
SELECT count(*) FROM widget_sessions WHERE organization_id=$1 AND kind=$2 AND created_at>=date_trunc('day',now());
-- name: InsertWidgetReplayChunk :one
INSERT INTO widget_replay_chunks(organization_id,session_id,sequence,object_key,size_bytes) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: ListWidgetReplayChunks :many
SELECT * FROM widget_replay_chunks WHERE organization_id=$1 AND session_id=$2 ORDER BY sequence;
-- name: GetWidgetReplayChunk :one
SELECT c.* FROM widget_replay_chunks c JOIN widget_sessions s ON s.organization_id=c.organization_id AND s.id=c.session_id
 JOIN conversations v ON v.organization_id=s.organization_id AND v.id=s.conversation_id
WHERE c.organization_id=$1 AND c.uuid=$2 AND s.expires_at>now() AND s.status NOT IN ('revoked','declined') AND v.anonymized_at IS NULL AND v.deleted_at IS NULL;
-- name: ExpiredWidgetReplayChunks :many
SELECT c.* FROM widget_replay_chunks c JOIN widget_sessions s ON s.organization_id=c.organization_id AND s.id=c.session_id
WHERE s.expires_at<=now() ORDER BY c.id LIMIT 100;
-- name: DeleteWidgetReplayChunk :exec
DELETE FROM widget_replay_chunks WHERE organization_id=$1 AND uuid=$2;
-- name: PurgeWidgetSessions :exec
DELETE FROM widget_sessions WHERE expires_at<=now() AND NOT EXISTS (SELECT 1 FROM widget_replay_chunks c WHERE c.organization_id=widget_sessions.organization_id AND c.session_id=widget_sessions.id);

-- name: ConversationReplayChunks :many
SELECT c.* FROM widget_replay_chunks c JOIN widget_sessions s ON s.organization_id=c.organization_id AND s.id=c.session_id
WHERE c.organization_id=$1 AND s.conversation_id=$2;
-- name: EraseConversationReplaySessions :exec
UPDATE widget_sessions SET status='revoked',visitor_id='erased',ended_at=now(),expires_at=now() WHERE organization_id=$1 AND conversation_id=$2;
