-- name: BusinessContextList :many
SELECT * FROM business_context_sections WHERE organization_id=$1
 AND (NOT sqlc.arg(dept_restrict)::boolean OR department_visible(organization_id,'context',id,department_id,sqlc.arg(viewer_departments)::bigint[]))
 ORDER BY sort_order,id LIMIT $2 OFFSET $3;
-- name: BusinessContextCount :one
SELECT count(*) FROM business_context_sections WHERE organization_id=$1
 AND (NOT sqlc.arg(dept_restrict)::boolean OR department_visible(organization_id,'context',id,department_id,sqlc.arg(viewer_departments)::bigint[]));
-- name: BusinessContextCreate :one
INSERT INTO business_context_sections(organization_id,kind,title,body,active,sort_order,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: BusinessContextUpdate :one
UPDATE business_context_sections SET kind=$3,title=$4,body=$5,active=$6,sort_order=$7,updated_by=$8,revision=revision+1
WHERE organization_id=$1 AND uuid=$2 AND revision=sqlc.arg(expected_revision) RETURNING *;
-- name: BusinessContextSnapshot :exec
INSERT INTO business_context_versions(organization_id,section_id,revision,kind,title,body,active,sort_order,updated_by)
SELECT s.organization_id,s.id,s.revision,s.kind,s.title,s.body,s.active,s.sort_order,s.updated_by FROM business_context_sections s WHERE s.organization_id=$1 AND s.id=$2;
-- name: BusinessContextVersions :many
SELECT v.* FROM business_context_versions v JOIN business_context_sections s ON s.organization_id=v.organization_id AND s.id=v.section_id
WHERE v.organization_id=$1 AND s.uuid=$2 ORDER BY v.revision DESC LIMIT $3 OFFSET $4;
-- name: AISimilarConversations :many
-- CF-214: same brand first, then brandless (organization-wide) conversations;
-- another brand's conversations never match. A brandless conversation only
-- matches brandless ones.
SELECT c.id,c.uuid,c.subject,c.status,ts_rank_cd(to_tsvector('simple',c.subject||' '||c.last_message_preview),plainto_tsquery('simple',sqlc.arg(q)::text))::real AS score
FROM conversations c WHERE c.organization_id=$1 AND c.id<>sqlc.arg(current_id) AND c.deleted_at IS NULL
AND (c.brand_id IS NULL OR c.brand_id=sqlc.narg(brand_id)::uuid)
AND to_tsvector('simple',c.subject||' '||c.last_message_preview) @@ plainto_tsquery('simple',sqlc.arg(q)::text)
ORDER BY (c.brand_id IS NOT NULL) DESC,score DESC,c.id DESC LIMIT 10;
-- name: AISaveSimilarLink :exec
INSERT INTO ai_conversation_links(organization_id,conversation_id,similar_id,score) VALUES($1,$2,$3,$4)
ON CONFLICT(organization_id,conversation_id,similar_id) DO UPDATE SET score=excluded.score;
-- name: AIEvaluationCases :many
SELECT * FROM ai_evaluation_cases WHERE organization_id=$1 ORDER BY id LIMIT $2 OFFSET $3;
-- name: AIEvaluationCaseCount :one
SELECT count(*) FROM ai_evaluation_cases WHERE organization_id=$1;
-- name: AIEvaluationCaseCreate :one
INSERT INTO ai_evaluation_cases(organization_id,name,transcript,locale,expected,author_id,channel,category,scenario_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;
-- name: AIEvaluationStart :one
INSERT INTO ai_evaluations(organization_id,config,config_revision,baseline_uuid,author_id) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: AIEvaluationFinish :one
UPDATE ai_evaluations SET status=$3,report=$4 WHERE organization_id=$1 AND uuid=$2 AND status='running' RETURNING *;
-- name: AIEvaluationGet :one
SELECT * FROM ai_evaluations WHERE organization_id=$1 AND uuid=$2;
-- name: AIEvaluationResultCreate :exec
INSERT INTO ai_evaluation_results(organization_id,evaluation_uuid,case_uuid,case_revision,case_snapshot,output,score,run_uuid,error_code)
VALUES($1,$2,$3,$4,$5,$6,$7,(SELECT uuid FROM ai_runs WHERE organization_id=$1 AND uuid=sqlc.narg(run_uuid)::uuid),sqlc.arg(error_code));
-- name: AIEvaluationResults :many
SELECT * FROM ai_evaluation_results WHERE organization_id=$1 AND evaluation_uuid=$2 ORDER BY id LIMIT $3 OFFSET $4;
-- name: AIKnowledgeGapTopics :many
WITH latest AS (
 SELECT DISTINCT ON (j.conversation_id) j.conversation_id,j.status,coalesce(nullif(t.result->>'category',''), 'uncategorized') AS topic
 FROM ai_agent_jobs j LEFT JOIN LATERAL (SELECT result FROM ai_agent_jobs t WHERE t.organization_id=j.organization_id AND t.conversation_id=j.conversation_id AND t.task='triage' AND t.status='completed' ORDER BY t.id DESC LIMIT 1) t ON true
 WHERE j.organization_id=$1 AND j.task='reply' AND j.created_at>=sqlc.arg(since)::timestamptz ORDER BY j.conversation_id,j.id DESC
) SELECT topic,count(*)::bigint AS conversations,(array_agg(conversation_id ORDER BY conversation_id))[1:20]::bigint[] AS conversation_ids FROM latest WHERE status IN ('handoff','unknown') GROUP BY topic ORDER BY conversations DESC,topic LIMIT $2 OFFSET $3;
-- name: AIKnowledgeSuggestionCreate :one
INSERT INTO knowledge_suggestions(organization_id,topic,conversation_id,title,body) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(organization_id,conversation_id) DO UPDATE SET topic=knowledge_suggestions.topic RETURNING *;
-- name: AIKnowledgeSuggestions :many
SELECT * FROM knowledge_suggestions WHERE organization_id=$1
 AND (NOT sqlc.arg(dept_restrict)::boolean OR department_id IS NULL OR department_id=ANY(sqlc.arg(viewer_departments)::bigint[]))
 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: AIKnowledgeSuggestionCount :one
SELECT count(*) FROM knowledge_suggestions WHERE organization_id=$1
 AND (NOT sqlc.arg(dept_restrict)::boolean OR department_id IS NULL OR department_id=ANY(sqlc.arg(viewer_departments)::bigint[]));
-- name: AIKnowledgeSuggestionLock :one
SELECT * FROM knowledge_suggestions WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: AIKnowledgeSuggestionReview :one
UPDATE knowledge_suggestions SET status=$3,reviewed_by=$4,article_id=$5 WHERE organization_id=$1 AND uuid=$2 AND status='pending' RETURNING *;
-- name: KnowledgeTranslationSave :exec
INSERT INTO knowledge_translations(organization_id,source_article_id,target_article_id,source_version) VALUES($1,$2,$3,$4);
-- name: KnowledgeReviewCreate :one
INSERT INTO knowledge_reviews(organization_id,article_id,article_version,reason,detail,owner_id) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(organization_id,article_id,article_version,reason) DO UPDATE SET detail=excluded.detail RETURNING *;
-- name: KnowledgeStaleReviewSweep :exec
INSERT INTO knowledge_reviews(organization_id,article_id,article_version,reason,owner_id)
SELECT a.organization_id,a.id,a.version,'age',a.author_id FROM knowledge_articles a
WHERE a.organization_id=$1 AND a.deleted_at IS NULL AND a.updated_at<sqlc.arg(before)::timestamptz
ON CONFLICT(organization_id,article_id,article_version,reason) DO NOTHING;
-- name: KnowledgeTranslationReviewSweep :exec
INSERT INTO knowledge_reviews(organization_id,article_id,article_version,reason,owner_id)
SELECT t.organization_id,a.id,a.version,'translation_outdated',a.author_id FROM knowledge_translations t
JOIN knowledge_articles a ON a.organization_id=t.organization_id AND a.id=t.target_article_id
JOIN knowledge_articles s ON s.organization_id=t.organization_id AND s.id=t.source_article_id
WHERE t.organization_id=$1 AND a.deleted_at IS NULL AND s.version>t.source_version
ON CONFLICT(organization_id,article_id,article_version,reason) DO NOTHING;
-- name: KnowledgeReviews :many
SELECT r.*,a.uuid AS article_uuid,a.title,a.locale FROM knowledge_reviews r JOIN knowledge_articles a ON a.organization_id=r.organization_id AND a.id=r.article_id
WHERE r.organization_id=$1 ORDER BY r.id DESC LIMIT $2 OFFSET $3;
-- name: KnowledgeReviewCount :one
SELECT count(*) FROM knowledge_reviews WHERE organization_id=$1;
-- name: KnowledgeReviewDone :one
UPDATE knowledge_reviews SET status='done' WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: AIKnowledgeSources :many
SELECT id,uuid,organization_id,kind,title,locale,enabled,max_depth,interval_hours,next_sync_at,status,last_error,last_synced_at,created_at,updated_at,department_id FROM knowledge_sources WHERE organization_id=$1
 AND (NOT sqlc.arg(dept_restrict)::boolean OR department_visible(organization_id,'source',id,department_id,sqlc.arg(viewer_departments)::bigint[]))
 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: AIKnowledgeSourceCount :one
SELECT count(*) FROM knowledge_sources WHERE organization_id=$1
 AND (NOT sqlc.arg(dept_restrict)::boolean OR department_visible(organization_id,'source',id,department_id,sqlc.arg(viewer_departments)::bigint[]));
-- name: AIKnowledgeSourceCreate :one
INSERT INTO knowledge_sources(organization_id,kind,location,title,locale,author_id,max_depth,interval_hours) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;
-- name: AIKnowledgeSourceGet :one
SELECT * FROM knowledge_sources WHERE organization_id=$1 AND uuid=$2;
-- name: AIKnowledgeSourceClaim :one
UPDATE knowledge_sources SET lease_until=now()+interval '5 minutes',status='running'
WHERE organization_id=$1 AND uuid=$2 AND enabled AND (lease_until IS NULL OR lease_until<now()) RETURNING *;
-- name: AIKnowledgeSourceFinish :exec
UPDATE knowledge_sources SET lease_until=NULL,status=$3,last_error=$4,last_synced_at=CASE WHEN $3='completed' THEN now() ELSE last_synced_at END,
next_sync_at=now()+make_interval(hours=>interval_hours) WHERE organization_id=$1 AND uuid=$2;
-- name: AIKnowledgeSourceToggle :one
UPDATE knowledge_sources SET enabled=$3 WHERE organization_id=$1 AND uuid=$2 RETURNING uuid;
-- name: AIKnowledgeSourceDue :many
SELECT organization_id,uuid FROM knowledge_sources WHERE enabled AND (kind='url' OR status='pending') AND next_sync_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY next_sync_at LIMIT 2;
-- name: AIKnowledgeSourceDocument :one
SELECT * FROM knowledge_source_documents WHERE organization_id=$1 AND source_id=$2 AND location=$3;
-- name: AIKnowledgeSourceDocumentSave :exec
INSERT INTO knowledge_source_documents(organization_id,source_id,location,digest,article_id,article_version) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(organization_id,source_id,location) DO UPDATE SET digest=excluded.digest,article_id=excluded.article_id,article_version=excluded.article_version;
-- name: AIPerformanceJobs :many
SELECT j.conversation_id,j.status,j.reason,j.created_at,coalesce(nullif(t.result->>'category',''),'uncategorized')::text AS topic,
coalesce(r.cost_micros,0)::bigint AS cost_micros
FROM ai_agent_jobs j LEFT JOIN ai_runs r ON r.organization_id=j.organization_id AND r.uuid=j.run_uuid
LEFT JOIN LATERAL (SELECT result FROM ai_agent_jobs t WHERE t.organization_id=j.organization_id AND t.conversation_id=j.conversation_id AND t.task='triage' AND t.status='completed' ORDER BY t.id DESC LIMIT 1) t ON true
WHERE j.organization_id=$1 AND j.task='reply' AND j.created_at>=sqlc.arg(since)::timestamptz AND j.created_at<sqlc.arg(until)::timestamptz ORDER BY j.created_at,j.id LIMIT 50001;
-- name: AIPerformanceResolutions :many
SELECT conversation_id FROM ai_resolutions WHERE organization_id=$1 AND created_at>=sqlc.arg(since)::timestamptz AND created_at<sqlc.arg(until)::timestamptz;
-- name: AIKnowledgeSourceLock :one
SELECT * FROM knowledge_sources WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: KnowledgeSourceArticleLock :one
SELECT * FROM knowledge_articles WHERE organization_id=$1 AND id=$2 FOR UPDATE;
-- name: BusinessContextVersionCount :one
SELECT count(*) FROM business_context_versions v JOIN business_context_sections s ON s.organization_id=v.organization_id AND s.id=v.section_id WHERE v.organization_id=$1 AND s.uuid=$2;
-- name: AIEvaluationList :many
SELECT * FROM ai_evaluations WHERE organization_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: AIEvaluationCount :one
SELECT count(*) FROM ai_evaluations WHERE organization_id=$1;
-- name: AIEvaluationResultCount :one
SELECT count(*) FROM ai_evaluation_results WHERE organization_id=$1 AND evaluation_uuid=$2;
-- name: AIKnowledgeGapTopicCount :one
SELECT count(DISTINCT coalesce(nullif(t.result->>'category',''),'uncategorized'))
FROM ai_agent_jobs j LEFT JOIN LATERAL (SELECT result FROM ai_agent_jobs t WHERE t.organization_id=j.organization_id AND t.conversation_id=j.conversation_id AND t.task='triage' AND t.status='completed' ORDER BY t.id DESC LIMIT 1) t ON true
WHERE j.organization_id=$1 AND j.task='reply' AND j.status IN ('handoff','unknown') AND j.created_at>=sqlc.arg(since)::timestamptz
AND j.id=(SELECT max(n.id) FROM ai_agent_jobs n WHERE n.organization_id=j.organization_id AND n.conversation_id=j.conversation_id AND n.task='reply');
-- name: KnowledgeReviewDueOrganizations :many
SELECT DISTINCT a.organization_id FROM knowledge_articles a WHERE a.deleted_at IS NULL AND a.updated_at<now()-interval '6 months'
AND NOT EXISTS(SELECT 1 FROM knowledge_reviews r WHERE r.organization_id=a.organization_id AND r.article_id=a.id AND r.article_version=a.version AND r.reason='age')
UNION
SELECT t.organization_id FROM knowledge_translations t
JOIN knowledge_articles source ON source.organization_id=t.organization_id AND source.id=t.source_article_id
JOIN knowledge_articles target ON target.organization_id=t.organization_id AND target.id=t.target_article_id
WHERE target.deleted_at IS NULL AND source.version>t.source_version
AND NOT EXISTS(SELECT 1 FROM knowledge_reviews r WHERE r.organization_id=t.organization_id AND r.article_id=target.id AND r.article_version=target.version AND r.reason='translation_outdated')
ORDER BY organization_id LIMIT 20;
-- name: AIPerformanceCSAT :one
SELECT count(*)::bigint AS samples,coalesce(sum(r.score),0)::bigint AS score_sum FROM conversation_ratings r
JOIN ai_resolutions a ON a.organization_id=r.organization_id AND a.conversation_id=r.conversation_id
WHERE r.organization_id=$1 AND r.created_at>=sqlc.arg(since)::timestamptz AND r.created_at<sqlc.arg(until)::timestamptz;
-- name: AIAgentAssignmentCandidates :many
SELECT u.uuid,u.name,u.surname FROM organization_members m JOIN users u ON u.id=m.user_id
WHERE m.organization_id=$1 AND u.status='active' ORDER BY u.id LIMIT 20;
