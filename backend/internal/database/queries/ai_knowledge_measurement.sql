-- CF-220: knowledge layer measurement (evaluation sets, watches, insight panel).

-- name: AIEvaluationRunCases :many
-- Active cases of a run; a department run takes that department's cases only.
SELECT * FROM ai_evaluation_cases WHERE organization_id=$1 AND active
AND (sqlc.narg(department_id)::bigint IS NULL OR department_id=sqlc.narg(department_id)::bigint)
ORDER BY id LIMIT 101;

-- name: AIEvaluationCaseUpdate :one
UPDATE ai_evaluation_cases SET name=$3,transcript=$4,locale=$5,expected=$6,channel=$7,category=$8,department_id=sqlc.narg(department_id),
expected_answer=sqlc.arg(expected_answer),expected_article_uuids=sqlc.arg(expected_article_uuids)::uuid[],active=sqlc.arg(active),revision=revision+1
WHERE organization_id=$1 AND uuid=$2 RETURNING *;

-- name: AIEvaluationLatestCompleted :one
-- Previous completed run of the same scope: the "before" of a before/after pair.
SELECT * FROM ai_evaluations WHERE organization_id=$1 AND status='completed'
AND department_id IS NOT DISTINCT FROM sqlc.narg(department_id)::bigint
ORDER BY id DESC LIMIT 1;

-- name: AIEvaluationFingerprintParts :one
SELECT coalesce((SELECT c.revision FROM ai_agent_configs c WHERE c.organization_id=$1),0)::int AS config_revision,
coalesce((SELECT max(a.updated_at) FROM knowledge_articles a WHERE a.organization_id=$1),'epoch'::timestamptz)::timestamptz AS knowledge_updated_at,
(SELECT count(*) FROM knowledge_articles a WHERE a.organization_id=$1 AND a.deleted_at IS NULL AND a.published_version IS NOT NULL)::bigint AS knowledge_published,
coalesce((SELECT r.model FROM ai_runs r WHERE r.organization_id=$1 AND r.task='reply' AND r.status='completed' ORDER BY r.id DESC LIMIT 1),'')::text AS reply_model;

-- name: AIEvaluationWatchGet :one
SELECT * FROM ai_evaluation_watches WHERE organization_id=$1;

-- name: AIEvaluationWatchSave :one
INSERT INTO ai_evaluation_watches(organization_id,enabled,actor_id) VALUES($1,$2,$3)
ON CONFLICT(organization_id) DO UPDATE SET enabled=excluded.enabled,actor_id=excluded.actor_id RETURNING *;

-- name: AIEvaluationWatchClaim :many
-- Enabled watches not checked in the last 10 minutes; the lease keeps two
-- workers from running the same organization's set at once.
UPDATE ai_evaluation_watches w SET lease_until=now()+interval '10 minutes'
WHERE w.organization_id IN (
 SELECT x.organization_id FROM ai_evaluation_watches x
 WHERE x.enabled AND (x.lease_until IS NULL OR x.lease_until<now()) AND (x.last_checked_at IS NULL OR x.last_checked_at<now()-interval '10 minutes')
 ORDER BY x.last_checked_at NULLS FIRST LIMIT 3 FOR UPDATE SKIP LOCKED
) RETURNING w.*;

-- name: AIEvaluationWatchFinish :exec
UPDATE ai_evaluation_watches SET lease_until=NULL,last_checked_at=now(),last_fingerprint=$2 WHERE organization_id=$1;

-- name: AIInsightUsage :many
-- With a department: only runs of that department's conversations (CF-225).
SELECT task,count(*)::bigint AS calls,count(*) FILTER (WHERE status IN ('failed','blocked'))::bigint AS failed,
coalesce(sum(cost_micros),0)::bigint AS cost_micros,coalesce(avg(latency_ms) FILTER (WHERE status='completed'),0)::double precision AS avg_latency_ms
FROM ai_runs r WHERE r.organization_id=$1 AND r.created_at>=sqlc.arg(since)::timestamptz AND r.created_at<sqlc.arg(until)::timestamptz
AND (sqlc.narg(department_id)::bigint IS NULL OR EXISTS (SELECT 1 FROM conversations c WHERE c.organization_id=r.organization_id AND c.id=r.conversation_id AND c.department_id=sqlc.narg(department_id)::bigint))
GROUP BY task ORDER BY task;

-- name: AIInsightReplyOutcomes :one
-- Terminal outcome per conversation = its latest reply job in the window.
WITH latest AS (
 SELECT DISTINCT ON (j.conversation_id) j.status FROM ai_agent_jobs j
 WHERE j.organization_id=$1 AND j.task='reply' AND j.created_at>=sqlc.arg(since)::timestamptz AND j.created_at<sqlc.arg(until)::timestamptz
 AND (sqlc.narg(department_id)::bigint IS NULL OR EXISTS (SELECT 1 FROM conversations c WHERE c.organization_id=j.organization_id AND c.id=j.conversation_id AND c.department_id=sqlc.narg(department_id)::bigint))
 ORDER BY j.conversation_id,j.id DESC
) SELECT count(*)::bigint AS conversations,count(*) FILTER (WHERE status IN ('handoff','unknown'))::bigint AS handoffs FROM latest;

-- name: AIInsightGaps :many
WITH latest AS (
 SELECT DISTINCT ON (j.conversation_id) j.conversation_id,j.status,coalesce(nullif(t.result->>'category',''),'uncategorized')::text AS topic
 FROM ai_agent_jobs j LEFT JOIN LATERAL (SELECT result FROM ai_agent_jobs t WHERE t.organization_id=j.organization_id AND t.conversation_id=j.conversation_id AND t.task='triage' AND t.status='completed' ORDER BY t.id DESC LIMIT 1) t ON true
 WHERE j.organization_id=$1 AND j.task='reply' AND j.created_at>=sqlc.arg(since)::timestamptz AND j.created_at<sqlc.arg(until)::timestamptz
 AND (sqlc.narg(department_id)::bigint IS NULL OR EXISTS (SELECT 1 FROM conversations c WHERE c.organization_id=j.organization_id AND c.id=j.conversation_id AND c.department_id=sqlc.narg(department_id)::bigint))
 ORDER BY j.conversation_id,j.id DESC
) SELECT topic,count(*)::bigint AS conversations FROM latest WHERE status IN ('handoff','unknown') GROUP BY topic ORDER BY conversations DESC,topic LIMIT 10;

-- name: AIInsightSuggestions :many
SELECT status,count(*)::bigint AS total FROM knowledge_suggestions
WHERE organization_id=$1 AND created_at>=sqlc.arg(since)::timestamptz AND created_at<sqlc.arg(until)::timestamptz
AND (sqlc.narg(department_id)::bigint IS NULL OR department_id=sqlc.narg(department_id)::bigint) GROUP BY status;

-- name: AIInsightClassifications :many
-- Latest completed triage per conversation in the window; confidence -1 = none.
SELECT DISTINCT ON (j.conversation_id) coalesce(nullif(j.result->>'category',''),'uncategorized')::text AS category,
(CASE WHEN jsonb_typeof(j.result->'confidence')='number' THEN (j.result->>'confidence')::double precision ELSE -1 END)::double precision AS confidence
FROM ai_agent_jobs j WHERE j.organization_id=$1 AND j.task='triage' AND j.status='completed'
AND j.created_at>=sqlc.arg(since)::timestamptz AND j.created_at<sqlc.arg(until)::timestamptz
AND (sqlc.narg(department_id)::bigint IS NULL OR EXISTS (SELECT 1 FROM conversations c WHERE c.organization_id=j.organization_id AND c.id=j.conversation_id AND c.department_id=sqlc.narg(department_id)::bigint))
ORDER BY j.conversation_id,j.id DESC LIMIT 50001;
-- name: AIInsightCategoryAssignments :many
-- Current category of the conversations classified in the window by the
-- categorization layer (CF-218); with a department, that department's
-- conversations only.
SELECT coalesce(k.name,'uncategorized')::text AS category,a.confidence::double precision AS confidence
FROM category_assignments a LEFT JOIN categories k ON k.organization_id=a.organization_id AND k.id=a.category_id
WHERE a.organization_id=$1 AND a.subject_type='conversation'
AND a.updated_at>=sqlc.arg(since)::timestamptz AND a.updated_at<sqlc.arg(until)::timestamptz
AND (sqlc.narg(department_id)::bigint IS NULL OR EXISTS (SELECT 1 FROM conversations c WHERE c.organization_id=a.organization_id AND c.id=a.subject_id AND c.department_id=sqlc.narg(department_id)::bigint))
ORDER BY a.id DESC LIMIT 50001;
-- name: AIInsightCacheStats :one
-- Answer cache lookups and hits (CF-216); the counters are daily, so the
-- window is widened to whole days. With a department,
-- only the lookups scoped to it.
SELECT coalesce(sum(exact_hits+semantic_hits+misses),0)::bigint AS lookups,coalesce(sum(exact_hits+semantic_hits),0)::bigint AS hits
FROM ai_answer_cache_stats WHERE organization_id=$1 AND day>=sqlc.arg(since)::date AND day<=sqlc.arg(until)::date
AND (sqlc.narg(department_id)::bigint IS NULL OR department_id=sqlc.narg(department_id)::bigint);

-- name: AIInsightEvaluations :many
-- Last two completed runs of the scope: the latest and the one before it.
SELECT uuid,trigger_kind,model,created_at,answer_accuracy,source_hit_rate,cache_hit_rate,avg_latency_ms,cost_per_case_micros
FROM ai_evaluations WHERE organization_id=$1 AND status='completed' AND department_id IS NOT DISTINCT FROM sqlc.narg(department_id)::bigint
ORDER BY id DESC LIMIT 2;
