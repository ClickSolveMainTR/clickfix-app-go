-- name: WorkforceHistory :many
SELECT (date_trunc('hour',c.created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')::timestamptz AS bucket,count(*)::bigint AS volume
FROM conversations c LEFT JOIN teams t ON t.organization_id=c.organization_id AND t.id=c.assignee_team_id
WHERE c.organization_id=$1 AND c.deleted_at IS NULL AND c.created_at >= sqlc.arg(since)::timestamptz AND c.created_at < sqlc.arg(until)::timestamptz
 AND (sqlc.arg(channel)::text='' OR c.channel_type=sqlc.arg(channel))
 AND (sqlc.narg(team)::uuid IS NULL OR t.uuid=sqlc.narg(team)) GROUP BY 1 ORDER BY 1;

-- name: WorkforceCoverage :many
SELECT h.bucket::timestamptz AS bucket,count(DISTINCT s.user_id)::bigint AS agents
FROM generate_series(sqlc.arg(since)::timestamptz,sqlc.arg(until)::timestamptz-interval '1 hour',interval '1 hour') h(bucket)
LEFT JOIN agent_shifts s ON s.organization_id=sqlc.arg(organization_id) AND s.starts_at<=h.bucket AND s.ends_at>=h.bucket+interval '1 hour'
 AND EXISTS(SELECT 1 FROM organization_members m WHERE m.organization_id=s.organization_id AND m.user_id=s.user_id)
 AND NOT EXISTS(SELECT 1 FROM agent_time_off o WHERE o.organization_id=s.organization_id AND o.user_id=s.user_id AND o.status='approved' AND o.starts_at<h.bucket+interval '1 hour' AND o.ends_at>h.bucket)
 AND (sqlc.narg(team)::uuid IS NULL OR EXISTS(SELECT 1 FROM team_members tm JOIN teams t ON t.organization_id=tm.organization_id AND t.id=tm.team_id WHERE tm.organization_id=s.organization_id AND tm.user_id=s.user_id AND t.uuid=sqlc.narg(team)))
GROUP BY h.bucket ORDER BY h.bucket;

-- name: WorkforceInsertEvent :one
INSERT INTO workforce_events(organization_id,user_id,client_uuid,state,conversation_id)
VALUES($1,$2,$3,$4,$5) ON CONFLICT(organization_id,user_id,client_uuid) DO UPDATE SET client_uuid=excluded.client_uuid RETURNING uuid,state,occurred_at;

-- name: WorkforceTimesheets :many
WITH intervals AS (
 SELECT e.user_id,e.state,e.conversation_id,e.occurred_at,
 LEAST(COALESCE(lead(e.occurred_at) OVER(PARTITION BY e.user_id ORDER BY e.occurred_at,e.id),sqlc.arg(until)::timestamptz),e.occurred_at+interval '5 minutes',sqlc.arg(until)::timestamptz,now()) AS ends_at
 FROM workforce_events e WHERE e.organization_id=sqlc.arg(organization_id)
 AND e.occurred_at>=sqlc.arg(since)::timestamptz-interval '5 minutes' AND e.occurred_at<sqlc.arg(until)::timestamptz
), durations AS (
 SELECT user_id,state,conversation_id,GREATEST(0,extract(epoch FROM ends_at-GREATEST(occurred_at,sqlc.arg(since)::timestamptz)))::double precision AS seconds FROM intervals
)
SELECT u.uuid AS agent_uuid,c.uuid AS conversation_uuid,d.state,sum(d.seconds)::double precision AS seconds
FROM durations d JOIN users u ON u.id=d.user_id
LEFT JOIN conversations c ON c.organization_id=sqlc.arg(organization_id) AND c.id=d.conversation_id AND c.deleted_at IS NULL
WHERE (sqlc.narg(agent)::uuid IS NULL OR u.uuid=sqlc.narg(agent)) AND d.state<>'offline' AND d.seconds>0
GROUP BY u.uuid,c.uuid,d.state ORDER BY u.uuid,c.uuid,d.state LIMIT 10001;
