-- CF-48: canned responses and macros. Visible to an agent: organization
-- scope, team scope (all teams until CF-5 scopes it) and their own personal ones.

-- name: ListCannedResponses :many
SELECT * FROM canned_responses
WHERE organization_id = sqlc.arg(organization_id)
  AND (scope <> 'personal' OR owner_user_id = sqlc.arg(viewer_id)::bigint)
  AND (sqlc.narg(q)::text IS NULL OR title ILIKE '%' || sqlc.narg(q) || '%' OR shortcut ILIKE sqlc.narg(q) || '%'
       OR content ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY usage_count DESC, lower(title), id
LIMIT 200;

-- name: GetCannedResponse :one
SELECT * FROM canned_responses
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid)
  AND (scope <> 'personal' OR owner_user_id = sqlc.arg(viewer_id)::bigint);

-- name: InsertCannedResponse :one
INSERT INTO canned_responses (organization_id, scope, owner_user_id, team_id, title, shortcut, category, content, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(scope), sqlc.narg(owner_user_id), sqlc.narg(team_id), sqlc.arg(title),
        sqlc.arg(shortcut), sqlc.arg(category), sqlc.arg(content), sqlc.narg(created_by))
RETURNING *;

-- name: UpdateCannedResponse :one
UPDATE canned_responses SET title = sqlc.arg(title), shortcut = sqlc.arg(shortcut), category = sqlc.arg(category),
    content = sqlc.arg(content)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteCannedResponse :execrows
DELETE FROM canned_responses WHERE organization_id = $1 AND id = $2;

-- name: TouchCannedResponse :exec
UPDATE canned_responses SET usage_count = usage_count + 1, last_used_at = NOW()
WHERE organization_id = $1 AND id = $2;

-- name: ListMacros :many
SELECT * FROM macros
WHERE organization_id = sqlc.arg(organization_id)
  AND (scope <> 'personal' OR owner_user_id = sqlc.arg(viewer_id)::bigint)
ORDER BY usage_count DESC, lower(name), id
LIMIT 200;

-- name: GetMacro :one
SELECT * FROM macros
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid)
  AND (scope <> 'personal' OR owner_user_id = sqlc.arg(viewer_id)::bigint);

-- name: InsertMacro :one
INSERT INTO macros (organization_id, scope, owner_user_id, team_id, name, description, actions, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(scope), sqlc.narg(owner_user_id), sqlc.narg(team_id), sqlc.arg(name),
        sqlc.arg(description), sqlc.arg(actions), sqlc.narg(created_by))
RETURNING *;

-- name: UpdateMacro :one
UPDATE macros SET name = sqlc.arg(name), description = sqlc.arg(description), actions = sqlc.arg(actions)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteMacro :execrows
DELETE FROM macros WHERE organization_id = $1 AND id = $2;

-- name: TouchMacro :exec
UPDATE macros SET usage_count = usage_count + 1, last_used_at = NOW()
WHERE organization_id = $1 AND id = $2;
