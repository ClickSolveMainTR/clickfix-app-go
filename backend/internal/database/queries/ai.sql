-- name: AIPolicy :one
SELECT * FROM ai_policies WHERE organization_id=$1;
-- name: AISavePolicy :one
-- batch_share_percent NULL keeps the stored share (50 on a new row).
INSERT INTO ai_policies(organization_id,enabled,mask_pii,allow_unmask,daily_calls,concurrency,daily_budget_micros,batch_share_percent)
 VALUES($1,$2,$3,$4,$5,$6,$7,COALESCE(sqlc.narg(batch_share_percent)::smallint,50)) ON CONFLICT(organization_id) DO UPDATE SET enabled=excluded.enabled,mask_pii=excluded.mask_pii,
 allow_unmask=excluded.allow_unmask,daily_calls=excluded.daily_calls,concurrency=excluded.concurrency,daily_budget_micros=excluded.daily_budget_micros,
 batch_share_percent=COALESCE(sqlc.narg(batch_share_percent)::smallint,ai_policies.batch_share_percent) RETURNING *;
-- name: AIUsageToday :one
SELECT count(*)::bigint AS calls,COALESCE(sum(GREATEST(cost_micros,reserved_cost_micros)),0)::bigint AS cost,
 count(*) FILTER (WHERE lane='batch')::bigint AS batch_calls,
 COALESCE(sum(GREATEST(cost_micros,reserved_cost_micros)) FILTER (WHERE lane='batch'),0)::bigint AS batch_cost
 FROM ai_runs WHERE organization_id=$1 AND status<>'blocked' AND error_code<>'AI_PROVIDER_REJECTED'
 AND created_at>=date_trunc('day',now() AT TIME ZONE 'Europe/Istanbul') AT TIME ZONE 'Europe/Istanbul';
-- name: AIActive :one
SELECT count(*) FROM ai_runs WHERE organization_id=$1 AND status='running';
-- name: AIInsertRun :one
WITH recording AS (
 SELECT id FROM recording_transcripts WHERE organization_id=$1 AND uuid=sqlc.narg(recording_uuid)::uuid AND status='processing' FOR KEY SHARE
)
INSERT INTO ai_runs(organization_id,uuid,actor_id,task,provider,model,status,input,retrieved_chunks,reserved_cost_micros,reserved_credits,reserved_tokens,conversation_id,lane)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,COALESCE(sqlc.narg(lane)::text,'live')
 WHERE sqlc.narg(recording_uuid)::uuid IS NULL OR EXISTS(SELECT 1 FROM recording) RETURNING *;
-- name: AIFinishRun :execrows
UPDATE ai_runs SET status=sqlc.arg(status),output=CASE WHEN pii_erased_at IS NULL THEN sqlc.arg(output)::text ELSE '' END,
tool_calls=CASE WHEN pii_erased_at IS NULL THEN sqlc.arg(tool_calls)::jsonb ELSE '[]'::jsonb END,
input_tokens=sqlc.arg(input_tokens),output_tokens=sqlc.arg(output_tokens),cost_micros=sqlc.arg(cost_micros),
reserved_cost_micros=sqlc.arg(reserved_cost_micros),charged_credits=sqlc.arg(charged_credits),latency_ms=sqlc.arg(latency_ms),error_code=sqlc.arg(error_code),model=sqlc.arg(model),provider=sqlc.arg(provider),finished_at=now()
WHERE organization_id=sqlc.arg(organization_id) AND uuid=sqlc.arg(uuid) AND status='running';
-- name: AIGetRun :one
SELECT * FROM ai_runs WHERE organization_id=$1 AND uuid=$2;
-- name: AIListRuns :many
SELECT * FROM ai_runs WHERE organization_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3;
-- name: AICountRuns :one
SELECT count(*) FROM ai_runs WHERE organization_id=$1;
-- name: AICreditBalance :one
SELECT COALESCE(sum(delta),0)::bigint AS balance FROM billing_credit_ledger WHERE organization_id=$1 AND unit='ai_credits';
-- name: AICreditEntry :exec
INSERT INTO billing_credit_ledger(organization_id,subscription_id,unit,delta,reason,idempotency_key)
 SELECT s.organization_id,s.id,'ai_credits',$2,$3,$4 FROM billing_subscriptions s WHERE s.organization_id=$1 ORDER BY s.id DESC LIMIT 1
 ON CONFLICT(organization_id,unit,idempotency_key) DO NOTHING;

-- name: AIQuotaMonth :one
SELECT COALESCE(sum(CASE WHEN status IN ('running','unknown') THEN reserved_tokens ELSE input_tokens+output_tokens END),0)::bigint AS tokens,
 COALESCE(sum(CASE WHEN status IN ('running','unknown') THEN reserved_credits ELSE charged_credits END),0)::bigint AS credits
 FROM ai_runs WHERE organization_id=$1 AND created_at>=date_trunc('month',now() AT TIME ZONE 'Europe/Istanbul') AT TIME ZONE 'Europe/Istanbul';
-- name: AIStaleRuns :many
SELECT * FROM ai_runs WHERE organization_id IS NOT NULL AND status='running' AND created_at<now()-interval '10 minutes' ORDER BY created_at LIMIT 100;

-- name: AIWalletReserve :one
SELECT ai_wallet_reserve(sqlc.arg(organization_id)::bigint,sqlc.arg(run_uuid)::uuid,sqlc.arg(credits)::bigint)::boolean AS allowed;
-- name: AIWalletRelease :exec
SELECT ai_wallet_release(sqlc.arg(organization_id)::bigint,sqlc.arg(run_uuid)::uuid,sqlc.arg(credits)::bigint);
