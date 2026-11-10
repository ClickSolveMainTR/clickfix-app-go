-- name: ListBrands :many
SELECT * FROM brands WHERE organization_id=$1 ORDER BY name;
-- name: GetBrand :one
SELECT * FROM brands WHERE organization_id=$1 AND uuid=$2;
-- name: CountBrands :one
SELECT count(*) FROM brands WHERE organization_id=$1;
-- name: CreateBrand :one
INSERT INTO brands(organization_id,name,logo_attachment_id,primary_color,secondary_color,support_email,sender_name,reply_to,help_subdomain,help_path,widget_defaults,signature,sla_policy_id,ai_persona)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING *;
-- name: UpdateBrand :one
UPDATE brands SET name=$3,logo_attachment_id=$4,primary_color=$5,secondary_color=$6,support_email=$7,sender_name=$8,reply_to=$9,help_subdomain=$10,help_path=$11,widget_defaults=$12,signature=$13,sla_policy_id=$14,ai_persona=$15
WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: DeleteBrand :execrows
DELETE FROM brands WHERE organization_id=$1 AND uuid=$2;
-- name: ListBrandEmailTemplates :many
SELECT * FROM brand_email_templates WHERE organization_id=$1 AND brand_id=$2 ORDER BY code;
-- name: SaveBrandEmailTemplate :one
INSERT INTO brand_email_templates(organization_id,brand_id,code,subject,body) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(organization_id,brand_id,code) DO UPDATE SET subject=excluded.subject,body=excluded.body RETURNING *;
-- name: GetBrandEmailTemplate :one
SELECT * FROM brand_email_templates WHERE organization_id=$1 AND brand_id=$2 AND code=$3;
-- name: AssignArticleBrand :execrows
UPDATE knowledge_articles SET brand_id=sqlc.narg(brand_id) WHERE organization_id=sqlc.arg(organization_id) AND uuid=sqlc.arg(uuid) AND deleted_at IS NULL;
-- name: SetChannelBrand :one
UPDATE channel_instances SET brand_id=sqlc.narg(brand_id) WHERE organization_id=sqlc.arg(organization_id) AND uuid=sqlc.arg(uuid) AND deleted_at IS NULL RETURNING *;
-- name: BrandPublicArticles :many
SELECT a.uuid,a.slug,a.locale,v.title,left(v.body,800)::text AS excerpt
FROM knowledge_articles a JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
WHERE a.organization_id=$1 AND a.brand_id=$2 AND a.deleted_at IS NULL AND v.visibility='public' AND a.locale=$3
AND (sqlc.arg(q)::text='' OR v.title ILIKE '%'||sqlc.arg(q)||'%' OR v.body ILIKE '%'||sqlc.arg(q)||'%')
ORDER BY v.created_at DESC,a.id DESC LIMIT 50;
-- name: BrandPublicArticle :one
SELECT a.uuid,a.slug,a.locale,v.title,v.body
FROM knowledge_articles a JOIN knowledge_article_versions v ON v.organization_id=a.organization_id AND v.article_id=a.id AND v.version=a.published_version
WHERE a.organization_id=$1 AND a.brand_id=$2 AND a.deleted_at IS NULL AND v.visibility='public' AND a.locale=$3 AND a.slug=$4;
-- public help endpoint resolves active organization by slug and brand UUID before scoped article reads
-- name: ResolvePublicBrand :one
SELECT b.* FROM brands b JOIN organizations o ON o.id=b.organization_id
WHERE o.slug=$1 AND b.uuid=$2 AND o.deleted_at IS NULL AND o.status='active' AND (o.access_ends_at IS NULL OR o.access_ends_at>now());
-- public help endpoint resolves active organization and brand by configured subdomain or organization slug plus brand help path
-- name: ResolvePublicBrandIdentity :one
SELECT b.* FROM brands b JOIN organizations o ON o.id=b.organization_id
WHERE o.deleted_at IS NULL AND o.status='active' AND (o.access_ends_at IS NULL OR o.access_ends_at>now())
AND ((sqlc.arg(subdomain)::text<>'' AND b.help_subdomain=sqlc.arg(subdomain)::text)
OR (sqlc.arg(subdomain)::text='' AND o.slug=sqlc.arg(organization_slug)::text AND b.help_path=sqlc.arg(help_path)::text AND b.help_path<>''));
