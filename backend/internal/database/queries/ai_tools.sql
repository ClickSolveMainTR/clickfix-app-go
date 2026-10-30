-- name: AITools :many
SELECT uuid,name,description,schema,url,requires_approval,enabled,daily_limit,(auth_enc<>'')::boolean AS has_auth FROM ai_tools WHERE organization_id=$1 ORDER BY id LIMIT $2 OFFSET $3;
-- name: AIToolCount :one
SELECT count(*) FROM ai_tools WHERE organization_id=$1;
-- name: AIToolByName :one
SELECT * FROM ai_tools WHERE organization_id=$1 AND name=$2;
-- name: AIToolSave :one
INSERT INTO ai_tools(organization_id,name,description,schema,url,auth_enc,requires_approval,enabled,daily_limit)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT(organization_id,name) DO UPDATE SET description=excluded.description,schema=excluded.schema,url=excluded.url,
auth_enc=CASE WHEN sqlc.arg(keep_auth)::boolean THEN ai_tools.auth_enc ELSE excluded.auth_enc END,
requires_approval=excluded.requires_approval,enabled=excluded.enabled,daily_limit=excluded.daily_limit RETURNING uuid;
-- name: AIToolDailyCalls :one
SELECT count(*) FROM ai_tool_calls WHERE organization_id=$1 AND tool_name=$2 AND created_at>=date_trunc('day',now());
-- name: AIToolStart :one
INSERT INTO ai_tool_calls(organization_id,conversation_id,tool_name,invocation_key,actor_id,dry_run)
VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING RETURNING *;
-- name: AIToolFinish :exec
UPDATE ai_tool_calls SET status=$3,error_code=$4 WHERE organization_id=$1 AND uuid=$2;
-- name: AIToolCalls :many
SELECT * FROM ai_tool_calls WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: AIToolCallsCount :one
SELECT count(*) FROM ai_tool_calls WHERE organization_id=$1;
