-- name: WorkflowOrgLock :exec
SELECT id FROM organizations WHERE id=sqlc.arg(organization_id) FOR UPDATE;
-- name: WorkflowCreate :one
INSERT INTO workflows(organization_id,name,draft,created_by) VALUES($1,$2,$3,$4) RETURNING *;
-- name: WorkflowUpdate :one
UPDATE workflows SET name=$3,draft=$4,revision=revision+1 WHERE organization_id=$1 AND uuid=$2 AND revision=$5 RETURNING *;
-- name: WorkflowGet :one
SELECT * FROM workflows WHERE organization_id=$1 AND uuid=$2;
-- name: WorkflowList :many
SELECT * FROM workflows WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: WorkflowCount :one
SELECT count(*) FROM workflows WHERE organization_id=$1;
-- name: WorkflowVersionCreate :one
INSERT INTO workflow_versions(organization_id,workflow_id,version,graph,trigger_event,schedule_minutes,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: WorkflowPublish :one
UPDATE workflows SET published_version=revision,enabled=$3,next_schedule_at=now(),schedule_cursor=0 WHERE organization_id=$1 AND uuid=$2 AND revision=$4 RETURNING *;
-- name: WorkflowEnable :one
UPDATE workflows SET enabled=$3 WHERE organization_id=$1 AND uuid=$2 AND (NOT $3 OR published_version IS NOT NULL) RETURNING *;
-- name: WorkflowVersions :many
SELECT * FROM workflow_versions WHERE organization_id=$1 AND workflow_id=$2 ORDER BY version DESC;
-- name: WorkflowVersionGet :one
SELECT * FROM workflow_versions WHERE organization_id=$1 AND id=$2;
-- name: WorkflowMatching :many
SELECT v.*,w.uuid AS workflow_uuid FROM workflow_versions v JOIN workflows w ON w.organization_id=v.organization_id AND w.id=v.workflow_id AND w.published_version=v.version WHERE v.organization_id=$1 AND v.trigger_event=$2 AND w.enabled ORDER BY w.id;
-- name: WorkflowDueSchedules :many
SELECT v.*,w.uuid AS workflow_uuid,w.next_schedule_at,w.schedule_cursor FROM workflows w JOIN workflow_versions v ON v.organization_id=w.organization_id AND v.workflow_id=w.id AND v.version=w.published_version WHERE w.enabled AND v.schedule_minutes>0 AND w.next_schedule_at<=now() ORDER BY w.next_schedule_at LIMIT 20;
-- name: WorkflowScheduleAdvance :exec
UPDATE workflows SET next_schedule_at=$3,schedule_cursor=0 WHERE organization_id=$1 AND id=$2;
-- name: WorkflowRunCreate :one
INSERT INTO workflow_runs(organization_id,workflow_id,version_id,event_id,conversation_uuid,node_id,trace) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(organization_id,workflow_id,event_id,conversation_uuid) DO NOTHING RETURNING *;
-- name: WorkflowRunExisting :one
SELECT * FROM workflow_runs WHERE organization_id=$1 AND workflow_id=$2 AND event_id=$3 AND conversation_uuid=$4;
-- name: WorkflowDailyRuns :one
SELECT count(*) FROM workflow_runs WHERE organization_id=$1 AND created_at>=date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC';
-- name: WorkflowMinuteRuns :one
SELECT count(*) FROM workflow_runs WHERE organization_id=$1 AND created_at>=now()-interval '1 minute';
-- name: WorkflowRunDue :many
WITH due AS (SELECT id FROM workflow_runs WHERE status IN ('pending','waiting','running') AND due_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY due_at LIMIT 1 FOR UPDATE SKIP LOCKED)
UPDATE workflow_runs r SET status='running',lease_until=now()+interval '5 minutes' FROM due WHERE r.id=due.id RETURNING r.*;
-- name: WorkflowRunCheckpoint :exec
UPDATE workflow_runs SET node_id=$3,step_index=$4,status=$5,due_at=$6,error=$7,lease_until=CASE WHEN $5 IN ('pending','running') THEN now()+interval '5 minutes' ELSE NULL END WHERE organization_id=$1 AND id=$2;
-- name: WorkflowRunList :many
SELECT * FROM workflow_runs WHERE organization_id=$1 AND workflow_id=$2 ORDER BY id DESC LIMIT $3 OFFSET $4;
-- name: WorkflowRunCount :one
SELECT count(*) FROM workflow_runs WHERE organization_id=$1 AND workflow_id=$2;
-- name: WorkflowRunGet :one
SELECT * FROM workflow_runs WHERE organization_id=$1 AND uuid=$2;
-- name: WorkflowStepGet :one
SELECT * FROM workflow_steps WHERE organization_id=$1 AND run_id=$2 AND step_index=$3;
-- name: WorkflowStepStart :exec
INSERT INTO workflow_steps(organization_id,run_id,step_index,node_id,status) VALUES($1,$2,$3,$4,'started');
-- name: WorkflowStepFinish :exec
UPDATE workflow_steps SET status=$4,output=$5,error=$6 WHERE organization_id=$1 AND run_id=$2 AND step_index=$3;
-- name: WorkflowSteps :many
SELECT * FROM workflow_steps WHERE organization_id=$1 AND run_id=$2 ORDER BY step_index;
-- name: WorkflowPublishedProcedure :one
SELECT * FROM ai_procedures WHERE organization_id=$1 AND uuid=$2 AND published IS NOT NULL;

-- name: WorkflowScheduleCursor :exec
UPDATE workflows SET schedule_cursor=$3 WHERE organization_id=$1 AND id=$2;
