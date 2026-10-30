-- name: ScheduleTemplates :many
SELECT * FROM shift_templates WHERE organization_id=$1 ORDER BY id LIMIT 100;
-- name: ScheduleCreateTemplate :one
INSERT INTO shift_templates(organization_id,name,timezone,start_minute,duration_minutes) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: ScheduleTemplate :one
SELECT * FROM shift_templates WHERE organization_id=$1 AND uuid=$2;
-- name: ScheduleMember :one
SELECT m.user_id FROM organization_members m JOIN users u ON u.id=m.user_id WHERE m.organization_id=$1 AND u.uuid=$2 AND u.status='active';
-- name: ScheduleShifts :many
SELECT s.uuid,u.uuid AS user_uuid,s.starts_at,s.ends_at,s.timezone FROM agent_shifts s JOIN users u ON u.id=s.user_id WHERE s.organization_id=$1 AND s.starts_at<$3 AND s.ends_at>$2 ORDER BY s.starts_at,s.id LIMIT 1000;
-- name: ScheduleTimeOff :many
SELECT t.uuid,u.uuid AS user_uuid,t.starts_at,t.ends_at,t.status FROM agent_time_off t JOIN users u ON u.id=t.user_id WHERE t.organization_id=$1 AND t.starts_at<$3 AND t.ends_at>$2 ORDER BY t.starts_at,t.id LIMIT 1000;
-- name: ScheduleCreateShift :one
INSERT INTO agent_shifts(organization_id,user_id,template_id,starts_at,ends_at,timezone) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;
-- name: ScheduleDeleteShift :execrows
DELETE FROM agent_shifts WHERE organization_id=$1 AND uuid=$2;
-- name: ScheduleRequestLeave :one
INSERT INTO agent_time_off(organization_id,user_id,starts_at,ends_at,reason) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: ScheduleReviewLeave :one
UPDATE agent_time_off SET status=$3,reviewed_by=$4 WHERE organization_id=$1 AND uuid=$2 AND status='pending' AND user_id<>$4 RETURNING *;
-- name: ScheduleCancelLeave :execrows
UPDATE agent_time_off SET status='canceled' WHERE organization_id=$1 AND uuid=$2 AND user_id=$3 AND status IN ('pending','approved');
-- name: ScheduleSaveSettings :exec
INSERT INTO schedule_settings(organization_id,enforce_shifts) VALUES($1,$2) ON CONFLICT(organization_id) DO UPDATE SET enforce_shifts=excluded.enforce_shifts;
-- name: ScheduleSettings :one
SELECT COALESCE((SELECT enforce_shifts FROM schedule_settings WHERE organization_id=$1),false)::boolean AS enforce_shifts;
-- name: ScheduleAvailable :one
SELECT (NOT EXISTS(SELECT 1 FROM agent_time_off t WHERE t.organization_id=$1 AND t.user_id=$2 AND t.status='approved' AND t.starts_at<=$3 AND t.ends_at>$3)
 AND (NOT COALESCE((SELECT enforce_shifts FROM schedule_settings WHERE organization_id=$1),false)
 OR EXISTS(SELECT 1 FROM agent_shifts s WHERE s.organization_id=$1 AND s.user_id=$2 AND s.starts_at<=$3 AND s.ends_at>$3)))::boolean AS available;
-- name: ScheduleLockMember :one
SELECT user_id FROM organization_members WHERE organization_id=$1 AND user_id=$2 FOR UPDATE;
-- name: ScheduleOverlap :one
SELECT EXISTS(SELECT 1 FROM agent_shifts WHERE organization_id=$1 AND user_id=$2 AND starts_at<$4 AND ends_at>$3)::boolean AS overlaps;
