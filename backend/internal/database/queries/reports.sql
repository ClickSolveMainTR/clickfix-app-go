-- name: ReportsCreate :one
INSERT INTO custom_reports(organization_id,created_by,name,definition) VALUES($1,$2,$3,$4) RETURNING *;
-- name: ReportsUpdate :one
UPDATE custom_reports SET name=$3,definition=$4 WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: ReportsGet :one
SELECT * FROM custom_reports WHERE organization_id=$1 AND uuid=$2;
-- name: ReportsList :many
SELECT * FROM custom_reports WHERE organization_id=$1 ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: ReportsCount :one
SELECT count(*) FROM custom_reports WHERE organization_id=$1;
-- name: ReportsDelete :execrows
DELETE FROM custom_reports WHERE organization_id=$1 AND uuid=$2;
-- name: ReportsSchedule :one
INSERT INTO report_schedules(organization_id,report_id,created_by,frequency,timezone,format,recipients,next_run_at,enabled)
SELECT r.organization_id,r.id,sqlc.arg(created_by),sqlc.arg(frequency),sqlc.arg(timezone),sqlc.arg(format),sqlc.arg(recipients)::uuid[],sqlc.arg(next_run_at),sqlc.arg(enabled)
FROM custom_reports r WHERE r.organization_id=sqlc.arg(organization_id) AND r.uuid=sqlc.arg(report_uuid) RETURNING *;
-- name: ReportsSchedules :many
SELECT s.uuid,r.uuid AS report_uuid,r.name,s.frequency,s.timezone,s.format,s.recipients,s.enabled,s.next_run_at
FROM report_schedules s JOIN custom_reports r ON r.organization_id=s.organization_id AND r.id=s.report_id
WHERE s.organization_id=$1 ORDER BY s.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: ReportsScheduleCount :one
SELECT count(*) FROM report_schedules WHERE organization_id=$1;
-- name: ReportsDeleteSchedule :execrows
DELETE FROM report_schedules WHERE organization_id=$1 AND uuid=$2;
-- name: ReportsDue :many
SELECT s.organization_id,s.id FROM report_schedules s WHERE s.enabled AND s.next_run_at<=now() ORDER BY s.next_run_at LIMIT 100;
-- name: ReportsLockSchedule :one
SELECT s.*,r.definition,r.name,o.uuid AS organization_uuid FROM report_schedules s
JOIN custom_reports r ON r.organization_id=s.organization_id AND r.id=s.report_id
JOIN organizations o ON o.id=s.organization_id AND o.deleted_at IS NULL
WHERE s.organization_id=$1 AND s.id=$2 AND s.enabled AND s.next_run_at<=now() FOR UPDATE OF s SKIP LOCKED;
-- name: ReportsAdvance :exec
UPDATE report_schedules SET next_run_at=$3 WHERE organization_id=$1 AND id=$2;
-- name: ReportsStoreDelivery :one
INSERT INTO report_deliveries(organization_id,schedule_id,scheduled_at,format,content) VALUES($1,$2,$3,$4,$5) RETURNING uuid;
-- name: ReportsDownload :one
SELECT format,content FROM report_deliveries WHERE organization_id=$1 AND uuid=$2 AND expires_at>now();
-- name: ReportsPurge :exec
DELETE FROM report_deliveries WHERE expires_at<now();
-- name: ReportsMember :one
SELECT u.id,u.email,o.uuid AS organization_uuid FROM organization_members m
JOIN users u ON u.id=m.user_id AND u.deleted_at IS NULL AND u.status='active' JOIN organizations o ON o.id=m.organization_id AND o.status='active' AND o.deleted_at IS NULL
WHERE m.organization_id=$1 AND u.uuid=$2;
-- name: ReportsEvaluate :one
WITH scoped AS (
 SELECT a.*,(SELECT avg(q.score) FROM qa_reviews q WHERE q.organization_id=a.organization_id AND q.conversation_id=a.conversation_id
 AND (q.source='manual' OR NOT q.needs_review OR q.reviewed_at IS NOT NULL)) AS qa_score
 FROM analytics_conversations a JOIN conversations c ON c.organization_id=a.organization_id AND c.id=a.conversation_id AND c.deleted_at IS NULL
 WHERE a.organization_id=sqlc.arg(organization_id) AND a.opened_at>=sqlc.arg(from_at)::timestamptz AND a.opened_at<sqlc.arg(to_at)::timestamptz
 AND (sqlc.arg(channel)::text='' OR a.channel_type=sqlc.arg(channel)::text)
 AND (sqlc.narg(team)::uuid IS NULL OR a.team_uuid=sqlc.narg(team)::uuid)
 AND (sqlc.narg(agent)::uuid IS NULL OR a.agent_uuid=sqlc.narg(agent)::uuid)
), grouped AS (
 SELECT CASE sqlc.arg(dimension)::text WHEN 'agent' THEN agent_uuid::text WHEN 'team' THEN team_uuid::text WHEN 'channel' THEN channel_type
 WHEN 'day' THEN to_char(opened_at AT TIME ZONE sqlc.arg(timezone)::text,'YYYY-MM-DD') ELSE 'all' END AS dimension,
 count(*) AS volume,avg(extract(epoch FROM first_response_at-opened_at)) FILTER(WHERE first_response_at>=opened_at) AS first_response_seconds,
 avg(extract(epoch FROM resolved_at-opened_at)) FILTER(WHERE resolved_at>=opened_at) AS resolution_seconds,
 avg(csat) AS csat_average,avg(qa_score) AS qa_score,COALESCE(sum(sla_breached),0) AS sla_breaches
 FROM scoped GROUP BY 1 ORDER BY 1 NULLS LAST LIMIT 10001
) SELECT COALESCE(jsonb_agg(to_jsonb(grouped)),'[]'::jsonb)::jsonb AS rows FROM grouped;
