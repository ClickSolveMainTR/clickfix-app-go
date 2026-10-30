-- name: AIAgentConfig :one
SELECT * FROM ai_agent_configs WHERE organization_id=$1;
-- name: AIAgentSaveConfig :one
INSERT INTO ai_agent_configs(organization_id,config,updated_by) VALUES($1,$2,$3)
ON CONFLICT(organization_id) DO UPDATE SET config=excluded.config,updated_by=excluded.updated_by,revision=ai_agent_configs.revision+1 RETURNING *;
-- name: AIAgentClaim :one
INSERT INTO ai_agent_jobs(organization_id,conversation_id,message_id,task) VALUES($1,$2,$3,$4)
ON CONFLICT(organization_id,message_id,task) DO NOTHING RETURNING *;
-- name: AIAgentFinish :exec
UPDATE ai_agent_jobs j SET status=sqlc.arg(status),
run_uuid=(SELECT a.uuid FROM ai_runs a WHERE a.organization_id=j.organization_id AND a.uuid=sqlc.narg(run_uuid)::uuid),
result=sqlc.arg(result),reason=sqlc.arg(reason)
WHERE j.organization_id=sqlc.arg(organization_id) AND j.id=sqlc.arg(id) AND j.status='running' ;
-- name: AIAgentJobs :many
SELECT j.* FROM ai_agent_jobs j JOIN conversations c ON c.organization_id=j.organization_id AND c.id=j.conversation_id
WHERE j.organization_id=$1 AND c.uuid=$2 ORDER BY j.id DESC LIMIT $3 OFFSET $4;
-- name: AIAgentJobCount :one
SELECT count(*) FROM ai_agent_jobs j JOIN conversations c ON c.organization_id=j.organization_id AND c.id=j.conversation_id
WHERE j.organization_id=$1 AND c.uuid=$2;
-- name: AIAgentMessages :many
SELECT * FROM (SELECT * FROM messages WHERE organization_id=$1 AND conversation_id=$2 AND deleted_at IS NULL
AND NOT private AND direction IN ('in','out') ORDER BY id DESC LIMIT 30) m ORDER BY id;
-- name: AIAgentHumanSince :one
SELECT EXISTS(SELECT 1 FROM messages WHERE organization_id=$1 AND conversation_id=$2
AND id>sqlc.arg(trigger_id)::bigint AND direction='out' AND sender_type='agent' AND deleted_at IS NULL)::boolean;
-- name: AIAgentStale :exec
UPDATE ai_agent_jobs SET status='unknown',reason='Interrupted execution; review AI run before retrying'
WHERE organization_id=$1 AND status='running' AND created_at<now()-interval '10 minutes';
-- name: AIAgentLockConfig :one
SELECT * FROM ai_agent_configs WHERE organization_id=$1 FOR SHARE;
-- name: AIAgentNewerInbound :one
SELECT EXISTS(SELECT 1 FROM messages WHERE organization_id=$1 AND conversation_id=$2 AND id>sqlc.arg(trigger_id)::bigint AND direction='in' AND deleted_at IS NULL)::boolean;

-- name: AIAgentResolution :one
INSERT INTO ai_resolutions(organization_id,conversation_id)
SELECT c.organization_id,c.id FROM conversations c
WHERE c.organization_id=$1 AND c.uuid=$2 AND c.status='resolved'
AND EXISTS(SELECT 1 FROM ai_agent_jobs j WHERE j.organization_id=c.organization_id AND j.conversation_id=c.id AND j.task='reply' AND j.status='completed')
AND EXISTS(SELECT 1 FROM messages m WHERE m.organization_id=c.organization_id AND m.conversation_id=c.id AND m.sender_type='bot' AND m.status IN ('sent','delivered','read') AND m.deleted_at IS NULL)
AND NOT EXISTS(SELECT 1 FROM messages m WHERE m.organization_id=c.organization_id AND m.conversation_id=c.id AND m.sender_type='agent' AND m.direction='out' AND m.deleted_at IS NULL)
ON CONFLICT DO NOTHING RETURNING *;
-- name: AIAgentContactHistory :many
SELECT uuid,subject,status,created_at,last_message_preview FROM conversations
WHERE organization_id=$1 AND contact_id=$2 AND deleted_at IS NULL AND id<>sqlc.arg(current_conversation_id)
ORDER BY created_at DESC LIMIT 10;
