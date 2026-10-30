-- name: CreateIntegrationKey :one
INSERT INTO public_api_keys(organization_id,user_id,name,token_hash,key_prefix,scopes,rate_limit,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;
-- name: ListIntegrationKeys :many
SELECT uuid,name,key_prefix,scopes,rate_limit,expires_at,revoked_at,last_used_at,created_at FROM public_api_keys WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: CountIntegrationKeys :one
SELECT count(*) FROM public_api_keys WHERE organization_id=$1;
-- name: RevokeIntegrationKey :execrows
UPDATE public_api_keys SET revoked_at=COALESCE(revoked_at,NOW()) WHERE organization_id=$1 AND uuid=$2;
-- name: AuthenticateIntegrationKey :one
SELECT k.*,u.uuid AS user_uuid,o.uuid AS organization_uuid FROM public_api_keys k JOIN users u ON u.id=k.user_id JOIN organizations o ON o.id=k.organization_id
WHERE k.token_hash=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>NOW()) AND u.status='active' AND u.deleted_at IS NULL AND o.deleted_at IS NULL AND o.status='active';
-- name: TouchIntegrationKey :exec
UPDATE public_api_keys SET last_used_at=NOW() WHERE organization_id=$1 AND id=$2;
-- name: ClaimIntegrationRequest :execrows
INSERT INTO public_api_idempotency(organization_id,key_id,request_key,request_hash) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING;
-- name: GetIntegrationRequest :one
SELECT * FROM public_api_idempotency WHERE organization_id=$1 AND key_id=$2 AND request_key=$3;
-- name: FinishIntegrationRequest :exec
UPDATE public_api_idempotency SET status_code=$4,response_enc=$5 WHERE organization_id=$1 AND key_id=$2 AND request_key=$3;
-- name: PurgeIntegrationRequests :exec
DELETE FROM public_api_idempotency WHERE expires_at<NOW();
-- name: ListIntegrationContacts :many
SELECT uuid,name,email,phone,custom_fields,created_at FROM contacts WHERE organization_id=$1 AND deleted_at IS NULL AND (sqlc.arg(q)::text='' OR name ILIKE '%'||sqlc.arg(q)::text||'%') ORDER BY id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: CountIntegrationContacts :one
SELECT count(*) FROM contacts WHERE organization_id=$1 AND deleted_at IS NULL AND (sqlc.arg(q)::text='' OR name ILIKE '%'||sqlc.arg(q)::text||'%');
-- name: ListIntegrationCompanies :many
SELECT uuid,name,created_at FROM companies WHERE organization_id=$1 AND deleted_at IS NULL ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: CountIntegrationCompanies :one
SELECT count(*) FROM companies WHERE organization_id=$1 AND deleted_at IS NULL;
-- name: CreateAutomationRule :one
INSERT INTO automation_rules(organization_id,created_by,name,trigger_event,conditions,actions,position,enabled,stop_on_match,schedule_minutes)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;
-- name: UpdateAutomationRule :one
UPDATE automation_rules SET created_by=sqlc.arg(created_by),name=$3,trigger_event=$4,conditions=$5,actions=$6,position=$7,enabled=$8,stop_on_match=$9,schedule_minutes=$10,next_run_at=NOW() WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: GetAutomationRule :one
SELECT * FROM automation_rules WHERE organization_id=$1 AND uuid=$2;
-- name: ListAutomationRules :many
SELECT * FROM automation_rules WHERE organization_id=$1 ORDER BY position,id LIMIT $2 OFFSET $3;
-- name: CountAutomationRules :one
SELECT count(*) FROM automation_rules WHERE organization_id=$1;
-- name: DeleteAutomationRule :execrows
DELETE FROM automation_rules WHERE organization_id=$1 AND uuid=$2;
-- name: MatchAutomationRules :many
SELECT * FROM automation_rules WHERE organization_id=$1 AND trigger_event=$2 AND enabled AND schedule_minutes=0 ORDER BY position,id;
-- name: ClaimScheduledAutomationRules :many
WITH due AS(SELECT id FROM automation_rules WHERE enabled AND schedule_minutes>0 AND next_run_at<=NOW() AND scan_lease_until<=NOW() ORDER BY next_run_at LIMIT 100 FOR UPDATE SKIP LOCKED)
UPDATE automation_rules r SET scan_lease_until=NOW()+interval '15 minutes' FROM due WHERE r.id=due.id RETURNING r.*;
-- name: ListAutomationConversations :many
SELECT uuid FROM conversations WHERE organization_id=$1 AND deleted_at IS NULL AND status NOT IN ('closed','resolved') AND id>sqlc.arg(after_id) ORDER BY id LIMIT 100;
-- name: AutomationConversationPage :many
SELECT id,uuid FROM conversations WHERE organization_id=$1 AND deleted_at IS NULL AND status NOT IN ('closed','resolved') AND id>sqlc.arg(after_id) ORDER BY id LIMIT 100;
-- name: ClaimAutomationExecution :one
INSERT INTO automation_executions(organization_id,rule_id,event_id,conversation_uuid,matched,status)
VALUES($1,$2,$3,$4,$5,'running') ON CONFLICT(organization_id,rule_id,event_id,conversation_uuid) DO UPDATE SET status='running',lease_until=NOW()+interval '2 minutes' WHERE automation_executions.status='failed' OR (automation_executions.status='running' AND automation_executions.lease_until<NOW()) RETURNING *;
-- name: FinishAutomationExecution :exec
UPDATE automation_executions SET status=$3,matched=$4,error=$5,action_index=$6,finished_at=NOW() WHERE organization_id=$1 AND id=$2;
-- name: ListAutomationExecutions :many
SELECT * FROM automation_executions WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: CountAutomationExecutions :one
SELECT count(*) FROM automation_executions WHERE organization_id=$1;
-- name: AutomationExecutionExists :one
SELECT EXISTS(SELECT 1 FROM automation_executions WHERE organization_id=$1 AND event_id=$2 AND conversation_uuid=$3 AND rule_id=$4);
-- name: CreateOutboundWebhook :one
INSERT INTO outbound_webhooks(organization_id,name,url,secret_enc,events,enabled) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;
-- name: UpdateOutboundWebhook :one
UPDATE outbound_webhooks SET name=$3,url=$4,events=$5,enabled=$6 WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: ListOutboundWebhooks :many
SELECT uuid,name,url,events,enabled,created_at,updated_at FROM outbound_webhooks WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: CountOutboundWebhooks :one
SELECT count(*) FROM outbound_webhooks WHERE organization_id=$1;
-- name: DeleteOutboundWebhook :execrows
DELETE FROM outbound_webhooks WHERE organization_id=$1 AND uuid=$2;
-- name: GetOutboundWebhook :one
SELECT * FROM outbound_webhooks WHERE organization_id=$1 AND uuid=$2;
-- name: EnqueueOutboundWebhooks :exec
INSERT INTO outbound_webhook_deliveries(organization_id,webhook_id,event_id,event_name,payload)
SELECT w.organization_id,w.id,sqlc.arg(event_id),sqlc.arg(event_name),sqlc.arg(payload) FROM outbound_webhooks w WHERE w.organization_id=sqlc.arg(organization_id) AND enabled AND sqlc.arg(event_name)=ANY(events) ON CONFLICT DO NOTHING;
-- name: EnqueueSingleOutboundWebhook :exec
INSERT INTO outbound_webhook_deliveries(organization_id,webhook_id,event_id,event_name,payload)
SELECT w.organization_id,w.id,sqlc.arg(event_id),sqlc.arg(event_name),sqlc.arg(payload) FROM outbound_webhooks w WHERE w.organization_id=sqlc.arg(organization_id) AND w.uuid=sqlc.arg(webhook_uuid) AND enabled ON CONFLICT DO NOTHING;
-- name: ClaimOutboundDeliveries :many
WITH due AS (SELECT id FROM outbound_webhook_deliveries WHERE status='pending' AND next_attempt_at<=NOW() AND lease_until<=NOW() ORDER BY next_attempt_at,id LIMIT 50 FOR UPDATE SKIP LOCKED)
UPDATE outbound_webhook_deliveries d SET lease_until=NOW()+interval '2 minutes',attempts=attempts+1 FROM due WHERE d.id=due.id RETURNING d.*;
-- name: GetOutboundWebhookByID :one
SELECT * FROM outbound_webhooks WHERE organization_id=$1 AND id=$2;
-- name: FinishOutboundDelivery :exec
UPDATE outbound_webhook_deliveries SET status=$3,last_status=$4,last_error=$5,next_attempt_at=$6,delivered_at=$7 WHERE organization_id=$1 AND id=$2 AND attempts=$8;
-- name: ReplayOutboundDelivery :execrows
UPDATE outbound_webhook_deliveries SET status='pending',attempts=0,next_attempt_at=NOW(),delivered_at=NULL,last_error='' WHERE organization_id=$1 AND uuid=$2 AND status<>'pending' AND lease_until<=NOW();
-- name: ListOutboundDeliveries :many
SELECT uuid,event_id,event_name,status,attempts,next_attempt_at,last_status,last_error,delivered_at,created_at FROM outbound_webhook_deliveries WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: CountOutboundDeliveries :one
SELECT count(*) FROM outbound_webhook_deliveries WHERE organization_id=$1;

-- name: CheckpointAutomationExecution :exec
UPDATE automation_executions SET action_index=$3,lease_until=NOW()+interval '2 minutes' WHERE organization_id=$1 AND id=$2;
-- name: AutomationConversationUUID :one
SELECT uuid FROM conversations WHERE organization_id=$1 AND id=$2 AND deleted_at IS NULL;

-- name: GetAutomationExecution :one
SELECT * FROM automation_executions WHERE organization_id=$1 AND rule_id=$2 AND event_id=$3 AND conversation_uuid=$4;

-- name: CompleteScheduledAutomationRule :exec
UPDATE automation_rules SET next_run_at=CASE WHEN sqlc.arg(succeeded)::boolean THEN NOW()+make_interval(mins=>schedule_minutes) ELSE next_run_at END,scan_lease_until=NOW() WHERE organization_id=$1 AND id=$2;
-- name: HeartbeatScheduledAutomationRule :exec
UPDATE automation_rules SET scan_lease_until=NOW()+interval '15 minutes' WHERE organization_id=$1 AND id=$2;
-- name: PendingAutomationExecutions :many
SELECT conversation_uuid FROM automation_executions WHERE organization_id=$1 AND rule_id=$2 AND event_id=$3 AND status IN ('failed','running') ORDER BY id;
