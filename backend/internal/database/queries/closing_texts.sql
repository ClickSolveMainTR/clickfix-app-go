-- CF-250: closing texts appended to agent replies.

-- name: ListClosingTexts :many
SELECT * FROM closing_texts
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY brand_id NULLS FIRST, position, lower(name), id;

-- name: ListClosingTextsForBrand :many
-- Organization-wide texts plus the ones of the conversation's brand.
SELECT * FROM closing_texts
WHERE organization_id = sqlc.arg(organization_id)
  AND (brand_id IS NULL OR brand_id = sqlc.narg(brand_id)::uuid)
ORDER BY brand_id NULLS LAST, position, lower(name), id;

-- name: GetClosingText :one
SELECT * FROM closing_texts WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: GetClosingTextByID :one
SELECT * FROM closing_texts WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: DefaultClosingText :one
-- The brand default wins over the organization default.
SELECT * FROM closing_texts
WHERE organization_id = sqlc.arg(organization_id) AND is_default
  AND (brand_id IS NULL OR brand_id = sqlc.narg(brand_id)::uuid)
ORDER BY brand_id NULLS LAST
LIMIT 1;

-- name: InsertClosingText :one
INSERT INTO closing_texts (organization_id, brand_id, name, body_tr, body_en, is_default, position, created_by)
VALUES (sqlc.arg(organization_id), sqlc.narg(brand_id), sqlc.arg(name), sqlc.arg(body_tr), sqlc.arg(body_en),
        sqlc.arg(is_default), sqlc.arg(position), sqlc.narg(created_by))
RETURNING *;

-- name: UpdateClosingText :one
UPDATE closing_texts SET brand_id = sqlc.narg(brand_id), name = sqlc.arg(name), body_tr = sqlc.arg(body_tr),
    body_en = sqlc.arg(body_en), is_default = sqlc.arg(is_default), position = sqlc.arg(position)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: ClearClosingTextDefault :exec
-- Drops the other default of the same organization/brand before one is set.
UPDATE closing_texts SET is_default = FALSE
WHERE organization_id = sqlc.arg(organization_id) AND is_default AND id <> sqlc.arg(keep_id)
  AND brand_id IS NOT DISTINCT FROM sqlc.narg(brand_id)::uuid;

-- name: DeleteClosingText :execrows
DELETE FROM closing_texts WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: GetClosingTextSettings :one
SELECT * FROM closing_text_settings WHERE organization_id = sqlc.arg(organization_id);

-- name: UpsertClosingTextSettings :one
INSERT INTO closing_text_settings (organization_id, apply_to_ai)
VALUES (sqlc.arg(organization_id), sqlc.arg(apply_to_ai))
ON CONFLICT (organization_id) DO UPDATE SET apply_to_ai = EXCLUDED.apply_to_ai, updated_at = NOW()
RETURNING *;

-- name: GetClosingTextChoice :one
SELECT c.closing_text_id, t.uuid AS closing_text_uuid
FROM closing_text_choices c
LEFT JOIN closing_texts t ON t.organization_id = c.organization_id AND t.id = c.closing_text_id
WHERE c.organization_id = sqlc.arg(organization_id) AND c.user_id = sqlc.arg(user_id);

-- name: SetClosingTextChoice :exec
INSERT INTO closing_text_choices (organization_id, user_id, closing_text_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(user_id), sqlc.narg(closing_text_id))
ON CONFLICT (organization_id, user_id) DO UPDATE SET closing_text_id = EXCLUDED.closing_text_id, updated_at = NOW();
