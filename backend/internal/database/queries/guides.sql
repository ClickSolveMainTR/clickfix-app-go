-- CF-141: module guides, per-user tour progress and product feedback.

-- name: GuidesForRoute :many
SELECT * FROM guides
WHERE surface = sqlc.arg(surface)
  AND locale = sqlc.arg(locale)
  AND is_published
  AND (sqlc.arg(route)::text = '' OR route = sqlc.arg(route)::text)
ORDER BY sort_order, key;

-- name: GuidesSetup :many
SELECT * FROM guides
WHERE surface = sqlc.arg(surface) AND locale = sqlc.arg(locale) AND is_published AND setup_order > 0
ORDER BY setup_order, key;

-- name: GuideByKey :one
SELECT * FROM guides WHERE key = sqlc.arg(key) AND locale = sqlc.arg(locale);

-- name: GuideKeyExists :one
SELECT EXISTS (SELECT 1 FROM guides WHERE key = $1)::boolean;

-- name: GuideByUUID :one
SELECT * FROM guides WHERE uuid = $1;

-- name: GuidesSearch :many
SELECT * FROM guides
WHERE locale = sqlc.arg(locale) AND is_published
  AND (title ILIKE '%' || sqlc.arg(query)::text || '%'
    OR summary ILIKE '%' || sqlc.arg(query)::text || '%'
    OR steps::text ILIKE '%' || sqlc.arg(query)::text || '%')
ORDER BY (surface = 'web') DESC, sort_order
LIMIT sqlc.arg(max_rows);

-- name: GuidesPlatformList :many
SELECT * FROM guides
WHERE (sqlc.arg(surface)::text = '' OR surface = sqlc.arg(surface)::text)
  AND (sqlc.arg(locale)::text = '' OR locale = sqlc.arg(locale)::text)
  AND (sqlc.arg(query)::text = '' OR key ILIKE '%' || sqlc.arg(query)::text || '%'
       OR title ILIKE '%' || sqlc.arg(query)::text || '%' OR route ILIKE '%' || sqlc.arg(query)::text || '%')
ORDER BY surface, route, sort_order, key, locale
LIMIT sqlc.arg(max_rows) OFFSET sqlc.arg(skip_rows);

-- name: GuidesPlatformCount :one
SELECT COUNT(*)::bigint FROM guides
WHERE (sqlc.arg(surface)::text = '' OR surface = sqlc.arg(surface)::text)
  AND (sqlc.arg(locale)::text = '' OR locale = sqlc.arg(locale)::text)
  AND (sqlc.arg(query)::text = '' OR key ILIKE '%' || sqlc.arg(query)::text || '%'
       OR title ILIKE '%' || sqlc.arg(query)::text || '%' OR route ILIKE '%' || sqlc.arg(query)::text || '%');

-- name: GuideCreate :one
INSERT INTO guides (key, locale, surface, route, kind, sort_order, setup_order, title, summary, steps, source, is_published, updated_by)
VALUES (sqlc.arg(key), sqlc.arg(locale), sqlc.arg(surface), sqlc.arg(route), sqlc.arg(kind), sqlc.arg(sort_order),
        sqlc.arg(setup_order), sqlc.arg(title), sqlc.arg(summary), sqlc.arg(steps), sqlc.arg(source), sqlc.arg(is_published),
        sqlc.narg(updated_by))
RETURNING *;

-- name: GuideUpdate :one
UPDATE guides SET
    surface = sqlc.arg(surface), route = sqlc.arg(route), kind = sqlc.arg(kind), sort_order = sqlc.arg(sort_order),
    setup_order = sqlc.arg(setup_order), title = sqlc.arg(title), summary = sqlc.arg(summary), steps = sqlc.arg(steps),
    is_published = sqlc.arg(is_published), source = CASE WHEN source = 'seed' THEN 'custom' ELSE source END,
    version = version + 1, updated_by = sqlc.narg(updated_by)
WHERE uuid = sqlc.arg(uuid)
RETURNING *;

-- name: GuideDelete :execrows
DELETE FROM guides WHERE uuid = $1;

-- Seed rows are refreshed only while nobody edited them (source stays 'seed').
-- name: GuideSeedUpsert :exec
INSERT INTO guides (key, locale, surface, route, kind, sort_order, setup_order, title, summary, steps, source, seed_version)
VALUES (sqlc.arg(key), sqlc.arg(locale), sqlc.arg(surface), sqlc.arg(route), sqlc.arg(kind), sqlc.arg(sort_order),
        sqlc.arg(setup_order), sqlc.arg(title), sqlc.arg(summary), sqlc.arg(steps), 'seed', sqlc.arg(seed_version))
ON CONFLICT (key, locale) DO UPDATE SET
    surface = EXCLUDED.surface, route = EXCLUDED.route, kind = EXCLUDED.kind, sort_order = EXCLUDED.sort_order,
    setup_order = EXCLUDED.setup_order, title = EXCLUDED.title, summary = EXCLUDED.summary, steps = EXCLUDED.steps,
    seed_version = EXCLUDED.seed_version, version = guides.version + 1
WHERE guides.source = 'seed' AND guides.seed_version < EXCLUDED.seed_version;

-- name: GuideProgressList :many
SELECT guide_key, status, step_index FROM guide_progress
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id);

-- name: GuideProgressUpsert :exec
INSERT INTO guide_progress (organization_id, user_id, guide_key, status, step_index)
VALUES (sqlc.arg(organization_id), sqlc.arg(user_id), sqlc.arg(guide_key), sqlc.arg(status), sqlc.arg(step_index))
ON CONFLICT (organization_id, user_id, guide_key) DO UPDATE SET
    status = EXCLUDED.status, step_index = EXCLUDED.step_index
WHERE guide_progress.status NOT IN ('completed', 'skipped')
   OR EXCLUDED.status IN ('completed', 'skipped') OR sqlc.arg(restart)::boolean;

-- name: GuideProgressReset :exec
DELETE FROM guide_progress WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id);

-- name: ProductFeedbackCreate :one
INSERT INTO product_feedback (organization_id, user_id, kind, message, route, surface, app_version, device, attachment_uuids)
VALUES (sqlc.arg(organization_id), sqlc.narg(user_id), sqlc.arg(kind), sqlc.arg(message), sqlc.arg(route),
        sqlc.arg(surface), sqlc.arg(app_version), sqlc.arg(device), sqlc.arg(attachment_uuids)::uuid[])
RETURNING uuid;

-- name: ProductFeedbackAttachmentsOwned :one
SELECT COUNT(*)::bigint FROM attachments
WHERE organization_id = sqlc.arg(organization_id) AND uploaded_by = sqlc.arg(user_id)
  AND deleted_at IS NULL AND uuid = ANY(sqlc.arg(ids)::uuid[]);

-- name: ProductFeedbackList :many
SELECT pf.uuid, pf.kind, pf.message, pf.route, pf.surface, pf.app_version, pf.device, pf.attachment_uuids,
       pf.status, pf.created_at, o.uuid AS organization_uuid, o.name AS organization_name,
       COALESCE(u.email, '')::text AS user_email
FROM product_feedback pf
JOIN organizations o ON o.id = pf.organization_id
LEFT JOIN users u ON u.id = pf.user_id
WHERE (sqlc.arg(kind)::text = '' OR pf.kind = sqlc.arg(kind)::text)
  AND (sqlc.arg(status)::text = '' OR pf.status = sqlc.arg(status)::text)
ORDER BY pf.created_at DESC
LIMIT sqlc.arg(max_rows) OFFSET sqlc.arg(skip_rows);

-- name: ProductFeedbackCount :one
SELECT COUNT(*)::bigint FROM product_feedback
WHERE (sqlc.arg(kind)::text = '' OR kind = sqlc.arg(kind)::text)
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text);

-- name: ProductFeedbackSetStatus :execrows
UPDATE product_feedback SET status = sqlc.arg(status) WHERE uuid = sqlc.arg(uuid);

-- name: GuideAutoStartClaim :execrows
WITH eligible AS (
 SELECT EXISTS (SELECT 1 FROM guides WHERE key=sqlc.arg(source_key) AND setup_order > 0) AS setup
 WHERE (SELECT guides_auto_start FROM users WHERE id=sqlc.arg(user_id))
 AND NOT EXISTS (SELECT 1 FROM guide_progress WHERE organization_id=sqlc.arg(organization_id) AND user_id=sqlc.arg(user_id)
   AND guide_progress_topic(guide_key)=sqlc.arg(guide_key) AND status <> 'new')
), setup_claim AS (
 INSERT INTO guide_progress (organization_id,user_id,guide_key,status,step_index)
 SELECT sqlc.arg(organization_id),sqlc.arg(user_id),'__setup__','in_progress',0 FROM eligible WHERE setup
 ON CONFLICT DO NOTHING RETURNING 1
)
INSERT INTO guide_progress (organization_id,user_id,guide_key,status,step_index)
SELECT sqlc.arg(organization_id),sqlc.arg(user_id),sqlc.arg(guide_key),'in_progress',0 FROM eligible
WHERE NOT setup OR EXISTS (SELECT 1 FROM setup_claim)
ON CONFLICT DO NOTHING;

-- name: GuidePreferencesGet :one
SELECT guides_auto_start FROM users WHERE id = $1;

-- name: GuidePreferencesSet :exec
UPDATE users SET guides_auto_start = sqlc.arg(auto_start) WHERE id = sqlc.arg(user_id);

-- name: GuideSetupSkip :exec
INSERT INTO guide_progress (organization_id, user_id, guide_key, status, step_index)
SELECT sqlc.arg(organization_id), sqlc.arg(user_id), guide_progress_topic(key), 'skipped', 0
FROM guides WHERE setup_order > 0 GROUP BY guide_progress_topic(key)
ON CONFLICT (organization_id, user_id, guide_key) DO UPDATE SET status = 'skipped'
WHERE guide_progress.status NOT IN ('completed','skipped');
