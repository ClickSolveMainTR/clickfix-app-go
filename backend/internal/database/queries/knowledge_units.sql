-- CF-216: knowledge units, answer cache, case memory, unanswered questions.

-- name: KnowledgeUnitArticleSource :one
SELECT a.id,a.uuid,a.locale,a.brand_id,a.published_version,v.title,v.body,v.visibility,
 coalesce(c.name,'')::text AS category
FROM knowledge_articles a
JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
LEFT JOIN knowledge_collections c ON c.organization_id=a.organization_id AND c.id=a.collection_id
WHERE a.organization_id=$1 AND a.uuid=$2 AND a.deleted_at IS NULL;
-- name: KnowledgeUnitArticleID :one
SELECT id FROM knowledge_articles WHERE organization_id=$1 AND uuid=$2;
-- name: KnowledgeUnitSuggestionSource :one
SELECT s.id,s.uuid,s.topic,s.title,s.body,s.status,s.updated_at,c.brand_id
FROM knowledge_suggestions s JOIN conversations c ON c.organization_id=s.organization_id AND c.id=s.conversation_id
WHERE s.organization_id=$1 AND s.uuid=$2;
-- name: KnowledgeUnitSourceHash :one
SELECT coalesce(min(source_hash),'')::text FROM knowledge_units
WHERE organization_id=$1 AND (article_id=sqlc.narg(article_id)::bigint OR suggestion_id=sqlc.narg(suggestion_id)::bigint);
-- name: KnowledgeUnitDeleteArticle :execrows
DELETE FROM knowledge_units WHERE organization_id=$1 AND article_id=$2;
-- name: KnowledgeUnitDeleteSuggestion :execrows
DELETE FROM knowledge_units WHERE organization_id=$1 AND suggestion_id=$2;
-- name: KnowledgeUnitInsert :one
INSERT INTO knowledge_units(organization_id,brand_id,department_id,article_id,suggestion_id,source_version,source_hash,ordinal,locale,visibility,category,question,variants,answer,embedding_model)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING *;
-- name: KnowledgeUnitInsertQuestion :exec
INSERT INTO knowledge_unit_questions(organization_id,unit_id,ordinal,content,embedding) VALUES($1,$2,$3,$4,$5);
-- name: KnowledgeUnitMatch :many
-- Scope: org-wide rows always qualify, brand/department rows only for the same
-- brand/department. The caller prefers the most specific match.
SELECT u.id,u.uuid,u.question,u.answer,u.category,u.visibility,u.brand_id,u.department_id,
 a.uuid AS article_uuid,
 max((1-(q.embedding <=> sqlc.arg(embedding)::vector)))::double precision AS score
FROM knowledge_unit_questions q
JOIN knowledge_units u ON u.organization_id=q.organization_id AND u.id=q.unit_id
LEFT JOIN knowledge_articles a ON a.organization_id=u.organization_id AND a.id=u.article_id
WHERE q.organization_id=$1 AND u.locale=$2 AND u.embedding_model=sqlc.arg(embedding_model)
 AND (NOT sqlc.arg(public_only)::boolean OR u.visibility='public')
 AND (u.brand_id IS NULL OR u.brand_id=sqlc.narg(brand_id)::uuid)
 AND (u.department_id IS NULL OR u.department_id=sqlc.narg(department_id)::bigint)
GROUP BY u.id,a.uuid
ORDER BY score DESC,u.id LIMIT 10;
-- name: KnowledgeUnitList :many
SELECT u.uuid,u.question,u.variants,u.answer,u.category,u.locale,u.visibility,u.brand_id,u.department_id,u.source_version,u.created_at,
 a.uuid AS article_uuid,s.uuid AS suggestion_uuid
FROM knowledge_units u
LEFT JOIN knowledge_articles a ON a.organization_id=u.organization_id AND a.id=u.article_id
LEFT JOIN knowledge_suggestions s ON s.organization_id=u.organization_id AND s.id=u.suggestion_id
WHERE u.organization_id=$1 ORDER BY u.id DESC LIMIT $2 OFFSET $3;
-- name: KnowledgeUnitCount :one
SELECT count(*) FROM knowledge_units WHERE organization_id=$1;

-- name: AnswerCacheGet :one
-- A cached row is valid only while its unit exists (source changes delete it)
-- and still passes the audience's visibility rule.
SELECT c.id,c.score,u.uuid AS unit_uuid,u.question,u.answer,u.category,u.visibility,a.uuid AS article_uuid
FROM ai_answer_cache c
JOIN knowledge_units u ON u.organization_id=c.organization_id AND u.id=c.unit_id
LEFT JOIN knowledge_articles a ON a.organization_id=u.organization_id AND a.id=u.article_id
WHERE c.organization_id=$1 AND c.audience=$2 AND c.locale=$3 AND c.scope_key=$4 AND c.question_hash=$5
 AND (c.audience='agent' OR u.visibility='public');
-- name: AnswerCacheTouch :exec
UPDATE ai_answer_cache SET hits=hits+1,last_hit_at=now() WHERE organization_id=$1 AND id=$2;
-- name: AnswerCachePut :exec
INSERT INTO ai_answer_cache(organization_id,unit_id,audience,locale,scope_key,question_hash,score) VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(organization_id,audience,locale,scope_key,question_hash) DO UPDATE SET unit_id=excluded.unit_id,score=excluded.score,last_hit_at=now();
-- name: AnswerCacheCount :one
SELECT count(*) FROM ai_answer_cache WHERE organization_id=$1;
-- name: AnswerCacheStat :exec
INSERT INTO ai_answer_cache_stats(organization_id,day,audience,exact_hits,semantic_hits,misses)
VALUES($1,current_date,$2,sqlc.arg(exact)::bigint,sqlc.arg(semantic)::bigint,sqlc.arg(miss)::bigint)
ON CONFLICT(organization_id,day,audience) DO UPDATE SET exact_hits=ai_answer_cache_stats.exact_hits+excluded.exact_hits,
 semantic_hits=ai_answer_cache_stats.semantic_hits+excluded.semantic_hits,misses=ai_answer_cache_stats.misses+excluded.misses;
-- name: AnswerCacheStats :many
SELECT audience,sum(exact_hits)::bigint AS exact_hits,sum(semantic_hits)::bigint AS semantic_hits,sum(misses)::bigint AS misses
FROM ai_answer_cache_stats WHERE organization_id=$1 AND day>=sqlc.arg(since)::date GROUP BY audience ORDER BY audience;

-- name: CaseMemoryConversation :one
SELECT c.id,c.uuid,c.status,c.subject,c.brand_id,c.contact_id,
 coalesce(ct.name,'')::text AS contact_name,coalesce(ct.email,'')::text AS contact_email,coalesce(ct.phone,'')::text AS contact_phone,
 coalesce(co.name,'')::text AS company_name,coalesce(co.domain,'')::text AS company_domain
FROM conversations c
LEFT JOIN contacts ct ON ct.organization_id=c.organization_id AND ct.id=c.contact_id
LEFT JOIN companies co ON co.organization_id=c.organization_id AND co.id=ct.company_id
WHERE c.organization_id=$1 AND c.uuid=$2 AND c.deleted_at IS NULL;
-- name: CaseMemoryMessages :many
SELECT sender_type,content FROM (SELECT id,sender_type,content FROM messages WHERE organization_id=$1 AND conversation_id=$2 AND deleted_at IS NULL
AND NOT private AND direction IN ('in','out') ORDER BY id DESC LIMIT 40) m ORDER BY id;
-- name: CaseMemoryUpsert :one
-- A re-summarized conversation goes back to pending review.
INSERT INTO ai_case_memories(organization_id,conversation_id,brand_id,department_id,locale,category,problem,resolution,anonymized,embedding,embedding_model)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(organization_id,conversation_id) DO UPDATE SET brand_id=excluded.brand_id,department_id=excluded.department_id,locale=excluded.locale,
 category=excluded.category,problem=excluded.problem,resolution=excluded.resolution,anonymized=excluded.anonymized,embedding=excluded.embedding,
 embedding_model=excluded.embedding_model,status='pending',reviewed_by=NULL,reviewed_at=NULL RETURNING *;
-- name: CaseMemoryReview :one
UPDATE ai_case_memories SET status=$3,reviewed_by=$4,reviewed_at=now()
WHERE organization_id=$1 AND uuid=$2 AND ($3::text<>'approved' OR anonymized) RETURNING *;
-- name: CaseMemoryList :many
SELECT m.uuid,m.locale,m.category,m.problem,m.resolution,m.status,m.anonymized,m.brand_id,m.department_id,m.created_at,m.reviewed_at,c.uuid AS conversation_uuid
FROM ai_case_memories m JOIN conversations c ON c.organization_id=m.organization_id AND c.id=m.conversation_id
WHERE m.organization_id=$1 AND (sqlc.narg(status)::text IS NULL OR m.status=sqlc.narg(status)::text)
ORDER BY m.id DESC LIMIT $2 OFFSET $3;
-- name: CaseMemoryCount :one
SELECT count(*) FROM ai_case_memories WHERE organization_id=$1 AND (sqlc.narg(status)::text IS NULL OR status=sqlc.narg(status)::text);
-- name: CaseMemoryMatch :many
-- approved_only is true for the customer AI. The source contact and company
-- names come back only so the caller can scrub them once more; they are never
-- returned to a customer.
SELECT m.uuid,m.conversation_id,m.problem,m.resolution,m.category,m.status,c.uuid AS conversation_uuid,
 coalesce(ct.name,'')::text AS contact_name,coalesce(ct.email,'')::text AS contact_email,coalesce(ct.phone,'')::text AS contact_phone,
 coalesce(co.name,'')::text AS company_name,coalesce(co.domain,'')::text AS company_domain,
 (1-(m.embedding <=> sqlc.arg(embedding)::vector))::double precision AS score
FROM ai_case_memories m
JOIN conversations c ON c.organization_id=m.organization_id AND c.id=m.conversation_id
LEFT JOIN contacts ct ON ct.organization_id=c.organization_id AND ct.id=c.contact_id
LEFT JOIN companies co ON co.organization_id=ct.organization_id AND co.id=ct.company_id
WHERE m.organization_id=$1 AND m.locale=$2 AND m.embedding_model=sqlc.arg(embedding_model) AND m.conversation_id<>sqlc.arg(exclude_conversation_id)::bigint
 AND m.status<>'rejected' AND (NOT sqlc.arg(approved_only)::boolean OR (m.status='approved' AND m.anonymized))
 AND (m.brand_id IS NULL OR m.brand_id=sqlc.narg(brand_id)::uuid)
 AND (m.department_id IS NULL OR m.department_id=sqlc.narg(department_id)::bigint)
ORDER BY score DESC,m.id LIMIT 5;

-- name: KnowledgeGapQuestionRecord :exec
INSERT INTO knowledge_gap_questions(organization_id,audience,locale,scope_key,brand_id,department_id,question_hash,question,category,conversation_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT(organization_id,audience,locale,scope_key,question_hash) DO UPDATE SET occurrences=knowledge_gap_questions.occurrences+1,
 last_seen_at=now(),conversation_id=coalesce(excluded.conversation_id,knowledge_gap_questions.conversation_id),
 category=CASE WHEN excluded.category<>'' THEN excluded.category ELSE knowledge_gap_questions.category END;
-- name: KnowledgeGapQuestions :many
SELECT g.audience,g.locale,g.question,g.category,g.occurrences,g.first_seen_at,g.last_seen_at,g.brand_id,g.department_id,c.uuid AS conversation_uuid
FROM knowledge_gap_questions g LEFT JOIN conversations c ON c.organization_id=g.organization_id AND c.id=g.conversation_id
WHERE g.organization_id=$1 AND g.last_seen_at>=sqlc.arg(since)::timestamptz
ORDER BY g.occurrences DESC,g.last_seen_at DESC,g.id LIMIT $2 OFFSET $3;
-- name: KnowledgeGapQuestionCount :one
SELECT count(*) FROM knowledge_gap_questions WHERE organization_id=$1 AND last_seen_at>=sqlc.arg(since)::timestamptz;
-- name: CaseMemoryExists :one
SELECT EXISTS(SELECT 1 FROM ai_case_memories WHERE organization_id=$1 AND uuid=$2);
