-- name: KnowledgeCollections :many
SELECT * FROM knowledge_collections WHERE organization_id=$1 ORDER BY name,id LIMIT $2 OFFSET $3;
-- name: KnowledgeCollection :one
SELECT * FROM knowledge_collections WHERE organization_id=$1 AND uuid=$2;
-- name: KnowledgeSaveCollection :one
INSERT INTO knowledge_collections(organization_id,uuid,parent_id,name,slug,kind) VALUES($1,$2,$3,$4,$5,$6)
 ON CONFLICT(uuid) DO UPDATE SET name=excluded.name,slug=excluded.slug
 WHERE knowledge_collections.organization_id=excluded.organization_id RETURNING *;
-- name: KnowledgeDeleteCollection :execrows
DELETE FROM knowledge_collections WHERE organization_id=$1 AND uuid=$2;
-- name: KnowledgeArticles :many
-- Admin list (CF-73): current drafts with filters and help center votes (CF-74).
SELECT a.uuid,a.slug,a.locale,a.title,a.tags,a.visibility,a.version,a.published_version,a.author_id,
 a.created_at,a.updated_at,c.uuid AS collection_uuid,
 COALESCE(f.helpful,0)::bigint AS helpful_count,COALESCE(f.not_helpful,0)::bigint AS not_helpful_count
 FROM knowledge_articles a
 LEFT JOIN knowledge_collections c ON c.organization_id=a.organization_id AND c.id=a.collection_id
 LEFT JOIN knowledge_collections p ON p.organization_id=c.organization_id AND p.id=c.parent_id
 LEFT JOIN LATERAL (SELECT count(*) FILTER (WHERE helpful) AS helpful,count(*) FILTER (WHERE NOT helpful) AS not_helpful
  FROM knowledge_article_feedback fb WHERE fb.organization_id=a.organization_id AND fb.article_id=a.id) f ON true
 WHERE a.organization_id=$1 AND a.deleted_at IS NULL
 AND (sqlc.arg(q)::text='' OR a.title ILIKE '%'||sqlc.arg(q)::text||'%' OR a.slug ILIKE '%'||sqlc.arg(q)::text||'%')
 AND (sqlc.arg(locale)::text='' OR a.locale=sqlc.arg(locale)::text)
 AND (sqlc.arg(visibility)::text='' OR a.visibility=sqlc.arg(visibility)::text)
 AND (sqlc.narg(collection_uuid)::uuid IS NULL OR c.uuid=sqlc.narg(collection_uuid)::uuid OR p.uuid=sqlc.narg(collection_uuid)::uuid)
 AND (sqlc.arg(state)::text='' OR (sqlc.arg(state)::text='draft' AND a.published_version IS NULL)
  OR (sqlc.arg(state)::text='published' AND a.published_version IS NOT NULL)
  OR (sqlc.arg(state)::text='changed' AND a.published_version IS NOT NULL AND a.published_version<>a.version))
 ORDER BY a.updated_at DESC,a.id DESC
 LIMIT $2 OFFSET $3;
-- name: KnowledgeArticleCount :one
SELECT count(*) FROM knowledge_articles a
 LEFT JOIN knowledge_collections c ON c.organization_id=a.organization_id AND c.id=a.collection_id
 LEFT JOIN knowledge_collections p ON p.organization_id=c.organization_id AND p.id=c.parent_id
 WHERE a.organization_id=$1 AND a.deleted_at IS NULL
 AND (sqlc.arg(q)::text='' OR a.title ILIKE '%'||sqlc.arg(q)::text||'%' OR a.slug ILIKE '%'||sqlc.arg(q)::text||'%')
 AND (sqlc.arg(locale)::text='' OR a.locale=sqlc.arg(locale)::text)
 AND (sqlc.arg(visibility)::text='' OR a.visibility=sqlc.arg(visibility)::text)
 AND (sqlc.narg(collection_uuid)::uuid IS NULL OR c.uuid=sqlc.narg(collection_uuid)::uuid OR p.uuid=sqlc.narg(collection_uuid)::uuid)
 AND (sqlc.arg(state)::text='' OR (sqlc.arg(state)::text='draft' AND a.published_version IS NULL)
  OR (sqlc.arg(state)::text='published' AND a.published_version IS NOT NULL)
  OR (sqlc.arg(state)::text='changed' AND a.published_version IS NOT NULL AND a.published_version<>a.version));
-- name: KnowledgeArticle :one
SELECT * FROM knowledge_articles WHERE organization_id=$1 AND uuid=$2 AND deleted_at IS NULL;
-- name: KnowledgeLockArticle :one
SELECT * FROM knowledge_articles WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: KnowledgeInsertArticle :one
INSERT INTO knowledge_articles(organization_id,uuid,collection_id,slug,locale,title,body,tags,visibility,author_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;
-- name: KnowledgeUpdateArticle :one
UPDATE knowledge_articles SET collection_id=$3,slug=$4,title=$5,body=$6,tags=$7,visibility=$8,author_id=$9,version=version+1
 WHERE organization_id=$1 AND uuid=$2 AND deleted_at IS NULL RETURNING *;
-- name: KnowledgeInsertVersion :exec
INSERT INTO knowledge_article_versions(organization_id,article_id,version,title,body,visibility,author_id) VALUES($1,$2,$3,$4,$5,$6,$7);
-- name: KnowledgeVersions :many
SELECT * FROM knowledge_article_versions WHERE organization_id=$1 AND article_id=$2 ORDER BY version DESC LIMIT $3 OFFSET $4;
-- name: KnowledgeSetPublished :one
UPDATE knowledge_articles SET published_version=$3,publication_revision=publication_revision+1 WHERE organization_id=$1 AND id=$2 RETURNING publication_revision;
-- name: KnowledgeDeleteArticle :exec
UPDATE knowledge_articles SET deleted_at=now(),published_version=NULL WHERE organization_id=$1 AND id=$2;
-- name: KnowledgePublished :one
SELECT a.id,a.uuid,a.slug,a.locale,a.published_version,v.title,v.body,v.visibility FROM knowledge_articles a
 JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
 WHERE a.organization_id=$1 AND a.uuid=$2 AND a.deleted_at IS NULL;
-- name: KnowledgeDeleteChunks :exec
DELETE FROM knowledge_chunks WHERE organization_id=$1 AND article_id=$2;
-- name: KnowledgeInsertChunk :one
-- brand_id comes from the article (trigger). While the organization is being
-- re-embedded the vector goes to pending_* (CF-214).
INSERT INTO knowledge_chunks(organization_id,article_id,version,ordinal,content,embedding,embedding_model,pending_embedding,pending_model)
 VALUES(sqlc.arg(organization_id),sqlc.arg(article_id),sqlc.arg(version),sqlc.arg(ordinal),sqlc.arg(content),
 sqlc.narg(embedding)::vector,sqlc.arg(embedding_model),sqlc.narg(pending_embedding)::vector,sqlc.narg(pending_model)::text)
 RETURNING uuid;
-- name: KnowledgeChunksByIDs :many
-- Hydrates vector hits (vectorstore.Search) with the published, visible article data.
SELECT c.id,c.uuid,c.article_id,c.version,c.content,a.uuid AS article_uuid,a.slug,a.locale,v.title
 FROM knowledge_chunks c JOIN knowledge_articles a ON a.id=c.article_id AND a.organization_id=c.organization_id
 JOIN knowledge_article_versions v ON v.organization_id=c.organization_id AND v.article_id=c.article_id AND v.version=c.version
 WHERE c.organization_id=sqlc.arg(organization_id) AND c.id=ANY(sqlc.arg(ids)::bigint[]) AND a.deleted_at IS NULL AND a.published_version=c.version
 AND a.locale=sqlc.arg(locale) AND (NOT sqlc.arg(public_only)::boolean OR v.visibility='public');
-- name: KnowledgeSearchLexical :many
-- Scope rules match vectorstore.Scope: the brand's rows and organization-wide rows.
SELECT c.uuid,c.article_id,c.version,c.content,a.uuid AS article_uuid,a.slug,a.locale,v.title,
 ts_rank_cd(to_tsvector('simple',c.content),plainto_tsquery('simple',sqlc.arg(q)::text))::double precision AS score,
 (c.brand_id IS NOT NULL OR c.department_id IS NOT NULL)::boolean AS specific
 FROM knowledge_chunks c JOIN knowledge_articles a ON a.id=c.article_id AND a.organization_id=c.organization_id
 JOIN knowledge_article_versions v ON v.organization_id=c.organization_id AND v.article_id=c.article_id AND v.version=c.version
 WHERE c.organization_id=sqlc.arg(organization_id) AND a.deleted_at IS NULL AND a.published_version=c.version
 AND a.locale=sqlc.arg(locale) AND (NOT sqlc.arg(public_only)::boolean OR v.visibility='public')
 AND (sqlc.arg(all_brands)::boolean OR c.brand_id IS NULL OR c.brand_id=sqlc.narg(brand_id)::uuid)
 AND (sqlc.arg(all_departments)::boolean OR c.department_id IS NULL OR c.department_id=sqlc.narg(department_id)::uuid)
 AND to_tsvector('simple',c.content) @@ plainto_tsquery('simple',sqlc.arg(q)::text)
 ORDER BY score DESC,c.id LIMIT 40;
-- name: KnowledgeValidChunks :many
SELECT c.uuid,c.article_id,c.version,c.content,a.uuid AS article_uuid,a.slug,a.locale,v.title,
 (c.brand_id IS NOT NULL OR c.department_id IS NOT NULL)::boolean AS specific
 FROM knowledge_chunks c JOIN knowledge_articles a ON a.id=c.article_id AND a.organization_id=c.organization_id
 JOIN knowledge_article_versions v ON v.organization_id=c.organization_id AND v.article_id=c.article_id AND v.version=c.version
 WHERE c.organization_id=sqlc.arg(organization_id) AND c.uuid=ANY(sqlc.arg(ids)::uuid[]) AND a.deleted_at IS NULL AND a.published_version=c.version
 AND a.locale=sqlc.arg(locale) AND (NOT sqlc.arg(public_only)::boolean OR v.visibility='public')
 AND (sqlc.arg(all_brands)::boolean OR c.brand_id IS NULL OR c.brand_id=sqlc.narg(brand_id)::uuid)
 AND (sqlc.arg(all_departments)::boolean OR c.department_id IS NULL OR c.department_id=sqlc.narg(department_id)::uuid);

-- name: KnowledgeCountCollections :one
SELECT count(*) FROM knowledge_collections WHERE organization_id=$1;
-- name: KnowledgeCountVersions :one
SELECT count(*) FROM knowledge_article_versions WHERE organization_id=$1 AND article_id=$2;
-- name: KnowledgeSetIndexState :exec
INSERT INTO knowledge_index_states(organization_id,article_id,status,reason,version) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(organization_id,article_id) DO UPDATE SET status=excluded.status,reason=excluded.reason,version=excluded.version;
-- name: KnowledgeClearIndexState :exec
DELETE FROM knowledge_index_states WHERE organization_id=$1 AND article_id=$2;
-- name: KnowledgeSkippedArticles :many
-- Published articles whose indexing was skipped (AI unavailable), oldest first.
SELECT a.uuid,a.published_version::int AS published_version FROM knowledge_index_states s
 JOIN knowledge_articles a ON a.organization_id=s.organization_id AND a.id=s.article_id
 WHERE s.organization_id=$1 AND s.status='skipped' AND a.deleted_at IS NULL AND a.published_version IS NOT NULL
 ORDER BY s.updated_at LIMIT 500;
