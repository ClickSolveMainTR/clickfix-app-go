-- name: CalibrationCreate :one
INSERT INTO qa_calibrations(organization_id,conversation_id,scorecard_id,rubric,name,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;
-- name: CalibrationGet :one
SELECT s.*,c.uuid AS conversation_uuid FROM qa_calibrations s JOIN conversations c ON c.organization_id=s.organization_id AND c.id=s.conversation_id AND c.deleted_at IS NULL WHERE s.organization_id=$1 AND s.uuid=$2;
-- name: CalibrationLock :one
SELECT * FROM qa_calibrations WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: CalibrationList :many
SELECT s.uuid,s.name,s.closed_at,s.created_at,c.uuid AS conversation_uuid FROM qa_calibrations s JOIN conversations c ON c.organization_id=s.organization_id AND c.id=s.conversation_id AND c.deleted_at IS NULL WHERE s.organization_id=$1 ORDER BY s.created_at DESC,s.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: CalibrationCount :one
SELECT count(*) FROM qa_calibrations s JOIN conversations c ON c.organization_id=s.organization_id AND c.id=s.conversation_id AND c.deleted_at IS NULL WHERE s.organization_id=$1;
-- name: CalibrationScore :one
INSERT INTO qa_calibration_scores(organization_id,calibration_id,reviewer_id,scores,score,critical_failure) VALUES($1,$2,$3,$4,$5,$6) RETURNING uuid,score,critical_failure;
-- name: CalibrationScores :many
SELECT u.uuid AS reviewer_uuid,s.scores,s.score,s.critical_failure FROM qa_calibration_scores s JOIN users u ON u.id=s.reviewer_id WHERE s.organization_id=$1 AND s.calibration_id=$2 ORDER BY s.id;
-- name: CalibrationClose :exec
UPDATE qa_calibrations SET closed_at=now() WHERE organization_id=$1 AND id=$2;
-- name: CoachingCreate :one
INSERT INTO coaching_items(organization_id,agent_id,coach_id,kind,body,due_at,parent_id) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING uuid,kind,body,due_at,completed_at,created_at;
-- name: CoachingParent :one
SELECT id FROM coaching_items WHERE organization_id=$1 AND uuid=$2 AND agent_id=$3 AND kind='goal';
-- name: CoachingList :many
SELECT i.uuid,u.uuid AS agent_uuid,v.uuid AS coach_uuid,i.kind,i.body,i.due_at,i.completed_at,i.created_at,p.uuid AS parent_uuid
FROM coaching_items i JOIN users u ON u.id=i.agent_id JOIN users v ON v.id=i.coach_id
LEFT JOIN coaching_items p ON p.organization_id=i.organization_id AND p.id=i.parent_id
WHERE i.organization_id=$1 AND u.uuid=$2 ORDER BY i.created_at DESC,i.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: CoachingCount :one
SELECT count(*) FROM coaching_items i JOIN users u ON u.id=i.agent_id WHERE i.organization_id=$1 AND u.uuid=$2;
-- name: CoachingComplete :execrows
UPDATE coaching_items SET completed_at=CASE WHEN sqlc.arg(completed)::boolean THEN now() ELSE NULL END
WHERE organization_id=$1 AND uuid=$2 AND kind IN ('goal','task');
-- name: GameRuleSave :one
INSERT INTO gamification_rules(organization_id,name,metric,threshold,points,min_samples,enabled) VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(organization_id,name) DO UPDATE SET metric=excluded.metric,threshold=excluded.threshold,points=excluded.points,min_samples=excluded.min_samples,enabled=excluded.enabled RETURNING uuid,name,metric,threshold,points,min_samples,enabled;
-- name: GameRules :many
SELECT uuid,name,metric,threshold,points,min_samples,enabled FROM gamification_rules WHERE organization_id=$1 ORDER BY id LIMIT 101;
-- name: GameOptOut :exec
INSERT INTO gamification_preferences(organization_id,user_id,opted_out) VALUES($1,$2,$3) ON CONFLICT(organization_id,user_id) DO UPDATE SET opted_out=excluded.opted_out;
-- name: GamePreference :one
SELECT COALESCE((SELECT opted_out FROM gamification_preferences WHERE organization_id=$1 AND user_id=$2),false)::boolean AS opted_out;
-- name: GameMetrics :many
WITH members AS (
 SELECT m.user_id,u.uuid FROM organization_members m JOIN users u ON u.id=m.user_id AND u.status='active'
 WHERE m.organization_id=sqlc.arg(organization_id)
 AND NOT EXISTS(SELECT 1 FROM gamification_preferences p WHERE p.organization_id=m.organization_id AND p.user_id=m.user_id AND p.opted_out)
 AND (sqlc.narg(team)::uuid IS NULL OR EXISTS(SELECT 1 FROM team_members tm JOIN teams t ON t.organization_id=tm.organization_id AND t.id=tm.team_id WHERE tm.organization_id=m.organization_id AND tm.user_id=m.user_id AND t.uuid=sqlc.narg(team)))
), kpi AS (
 SELECT c.assignee_user_id AS user_id,count(*) FILTER(WHERE c.resolved_at>=sqlc.arg(since)::timestamptz AND c.resolved_at<sqlc.arg(until)::timestamptz)::bigint AS resolutions,
 avg(extract(epoch FROM c.first_response_at-c.created_at)) FILTER(WHERE c.first_response_at>=c.created_at AND c.first_response_at>=sqlc.arg(since)::timestamptz AND c.first_response_at<sqlc.arg(until)::timestamptz)::double precision AS frt,
 count(c.first_response_at) FILTER(WHERE c.first_response_at>=c.created_at AND c.first_response_at>=sqlc.arg(since)::timestamptz AND c.first_response_at<sqlc.arg(until)::timestamptz)::bigint AS frt_samples
 FROM conversations c WHERE c.organization_id=sqlc.arg(organization_id) AND c.deleted_at IS NULL GROUP BY c.assignee_user_id
), ratings AS (
 SELECT c.assignee_user_id AS user_id,avg(r.score)::double precision AS csat,count(*)::bigint AS csat_samples
 FROM conversation_ratings r JOIN conversations c ON c.organization_id=r.organization_id AND c.id=r.conversation_id AND c.deleted_at IS NULL
 WHERE r.organization_id=sqlc.arg(organization_id) AND r.created_at>=sqlc.arg(since)::timestamptz AND r.created_at<sqlc.arg(until)::timestamptz GROUP BY c.assignee_user_id
), qa AS (
 SELECT r.agent_id AS user_id,avg(r.score)::double precision AS qa,count(*)::bigint AS qa_samples FROM qa_reviews r
 WHERE r.organization_id=sqlc.arg(organization_id) AND r.created_at>=sqlc.arg(since)::timestamptz AND r.created_at<sqlc.arg(until)::timestamptz AND r.source='manual' GROUP BY r.agent_id
)
SELECT m.uuid AS agent_uuid,COALESCE(k.resolutions,0)::bigint AS resolutions,COALESCE(k.frt,0)::double precision AS frt,COALESCE(k.frt_samples,0)::bigint AS frt_samples,
 COALESCE(r.csat,0)::double precision AS csat,COALESCE(r.csat_samples,0)::bigint AS csat_samples,COALESCE(q.qa,0)::double precision AS qa,COALESCE(q.qa_samples,0)::bigint AS qa_samples
FROM members m LEFT JOIN kpi k ON k.user_id=m.user_id LEFT JOIN ratings r ON r.user_id=m.user_id LEFT JOIN qa q ON q.user_id=m.user_id ORDER BY m.uuid LIMIT 10001;
-- name: GameLock :exec
SELECT pg_advisory_xact_lock(sqlc.arg(organization_id)::bigint);
