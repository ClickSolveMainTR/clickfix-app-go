-- Public help center (CF-74). Every query reads the PUBLISHED snapshot
-- (knowledge_article_versions at published_version) with public visibility;
-- current drafts, internal and deleted articles never match.

-- name: HelpOrganization :one
SELECT id,uuid,slug,name,logo_object_key FROM organizations
 WHERE slug=$1 AND deleted_at IS NULL AND status='active'
 AND (access_ends_at IS NULL OR access_ends_at>now());

-- name: HelpOrganizationByID :one
SELECT id,uuid,slug,name,logo_object_key FROM organizations
 WHERE id=$1 AND deleted_at IS NULL AND status='active';

-- name: HelpPublicLocales :many
SELECT a.locale,count(*)::bigint AS article_count FROM knowledge_articles a
 JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
 WHERE a.organization_id=$1 AND a.deleted_at IS NULL AND v.visibility='public'
 GROUP BY a.locale ORDER BY a.locale;

-- name: HelpCollections :many
SELECT c.id,c.uuid,c.name,c.slug,c.kind,COALESCE(p.slug,'')::text AS parent_slug,
 (SELECT count(*) FROM knowledge_articles a
  JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
  WHERE a.organization_id=c.organization_id AND a.collection_id=c.id AND a.deleted_at IS NULL
  AND a.locale=sqlc.arg(locale)::text AND v.visibility='public')::bigint AS article_count
 FROM knowledge_collections c
 LEFT JOIN knowledge_collections p ON p.organization_id=c.organization_id AND p.id=c.parent_id
 WHERE c.organization_id=$1 ORDER BY c.name,c.id;

-- name: HelpArticles :many
SELECT a.uuid,a.slug,a.locale,v.title,left(v.body,800)::text AS excerpt,v.created_at AS updated_at,
 COALESCE(c.slug,'')::text AS collection_slug
 FROM knowledge_articles a
 JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
 LEFT JOIN knowledge_collections c ON c.organization_id=a.organization_id AND c.id=a.collection_id
 WHERE a.organization_id=$1 AND a.deleted_at IS NULL AND v.visibility='public' AND a.locale=sqlc.arg(locale)::text
 AND (cardinality(sqlc.arg(collection_ids)::bigint[])=0 OR a.collection_id=ANY(sqlc.arg(collection_ids)::bigint[]))
 AND a.id<>sqlc.arg(exclude_id)::bigint
 ORDER BY v.created_at DESC,a.id DESC
 LIMIT $2 OFFSET $3;

-- name: HelpArticleCount :one
SELECT count(*) FROM knowledge_articles a
 JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
 WHERE a.organization_id=$1 AND a.deleted_at IS NULL AND v.visibility='public' AND a.locale=sqlc.arg(locale)::text
 AND (cardinality(sqlc.arg(collection_ids)::bigint[])=0 OR a.collection_id=ANY(sqlc.arg(collection_ids)::bigint[]));

-- name: HelpArticleBySlug :one
SELECT a.id,a.uuid,a.slug,a.locale,a.tags,a.collection_id,a.published_version,v.title,v.body,
 v.created_at AS updated_at,a.created_at
 FROM knowledge_articles a
 JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
 WHERE a.organization_id=$1 AND a.locale=$2 AND a.slug=$3 AND a.deleted_at IS NULL AND v.visibility='public';

-- name: HelpSearchArticles :many
SELECT a.uuid,a.slug,a.locale,v.title,left(v.body,800)::text AS excerpt,v.created_at AS updated_at,
 COALESCE(c.slug,'')::text AS collection_slug,
 (CASE WHEN v.title ILIKE sqlc.arg(pattern)::text THEN 1 ELSE 0 END
  + ts_rank_cd(to_tsvector('simple',v.title||' '||v.body),plainto_tsquery('simple',sqlc.arg(q)::text)))::double precision AS score
 FROM knowledge_articles a
 JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
 LEFT JOIN knowledge_collections c ON c.organization_id=a.organization_id AND c.id=a.collection_id
 WHERE a.organization_id=$1 AND a.deleted_at IS NULL AND v.visibility='public' AND a.locale=sqlc.arg(locale)::text
 AND (v.title ILIKE sqlc.arg(pattern)::text OR v.body ILIKE sqlc.arg(pattern)::text
  OR to_tsvector('simple',v.title||' '||v.body) @@ plainto_tsquery('simple',sqlc.arg(q)::text))
 ORDER BY score DESC,v.created_at DESC,a.id
 LIMIT $2;

-- name: HelpArticlesByUUIDs :many
SELECT a.uuid,a.slug,a.locale,v.title,left(v.body,800)::text AS excerpt,v.created_at AS updated_at,
 COALESCE(c.slug,'')::text AS collection_slug
 FROM knowledge_articles a
 JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
 LEFT JOIN knowledge_collections c ON c.organization_id=a.organization_id AND c.id=a.collection_id
 WHERE a.organization_id=$1 AND a.uuid=ANY(sqlc.arg(ids)::uuid[]) AND a.deleted_at IS NULL
 AND v.visibility='public' AND a.locale=sqlc.arg(locale)::text;

-- name: HelpSitemap :many
SELECT a.slug,a.locale,v.created_at AS updated_at FROM knowledge_articles a
 JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
 WHERE a.organization_id=$1 AND a.deleted_at IS NULL AND v.visibility='public'
 ORDER BY v.created_at DESC,a.id DESC LIMIT 5000;

-- name: HelpUpsertFeedback :exec
INSERT INTO knowledge_article_feedback(organization_id,article_id,version,helpful,voter_hash)
 VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(organization_id,article_id,voter_hash) DO UPDATE SET helpful=excluded.helpful,version=excluded.version;

-- name: HelpFeedbackCounts :one
SELECT count(*) FILTER (WHERE helpful)::bigint AS helpful,count(*) FILTER (WHERE NOT helpful)::bigint AS not_helpful
 FROM knowledge_article_feedback WHERE organization_id=$1 AND article_id=$2;

-- name: HelpAccentColor :one
-- The help center reuses the organization's first live web widget colour (CF-55 settings).
SELECT COALESCE(settings->>'color','')::text AS color FROM channel_instances
 WHERE organization_id=$1 AND channel_type='widget' AND status IN ('active','error')
 ORDER BY id LIMIT 1;
