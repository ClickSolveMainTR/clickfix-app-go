-- Teams (CF-5). Every query is scoped by organization_id.

-- name: CreateTeam :one
INSERT INTO teams (organization_id, name, description, color, timezone, working_hours, skills)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(color),
    sqlc.narg(timezone), sqlc.narg(working_hours), sqlc.arg(skills)::text[]
)
RETURNING *;

-- name: UpdateTeam :one
UPDATE teams
SET name = sqlc.arg(name),
    description = sqlc.arg(description),
    color = sqlc.arg(color),
    timezone = sqlc.narg(timezone),
    working_hours = sqlc.narg(working_hours),
    skills = sqlc.arg(skills)::text[]
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid)
RETURNING *;

-- name: DeleteTeam :execrows
DELETE FROM teams
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: GetTeamByUUID :one
SELECT t.*,
       (SELECT COUNT(*) FROM team_members tm
        WHERE tm.organization_id = t.organization_id AND tm.team_id = t.id)::bigint AS member_count
FROM teams t
WHERE t.organization_id = sqlc.arg(organization_id) AND t.uuid = sqlc.arg(uuid);

-- name: TeamNameTaken :one
SELECT EXISTS (
    SELECT 1 FROM teams
    WHERE organization_id = sqlc.arg(organization_id)
      AND lower(name) = lower(sqlc.arg(name)::text)
      AND uuid <> sqlc.arg(except_uuid)::uuid
)::boolean;

-- name: ListTeams :many
SELECT t.*,
       (SELECT COUNT(*) FROM team_members tm
        WHERE tm.organization_id = t.organization_id AND tm.team_id = t.id)::bigint AS member_count
FROM teams t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL OR t.name ILIKE '%' || sqlc.narg(q) || '%' OR t.description ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(skill)::text IS NULL OR sqlc.narg(skill)::text = ANY(t.skills))
  AND (
    sqlc.narg(member_user_id)::bigint IS NULL
    OR EXISTS (
        SELECT 1 FROM team_members tm
        WHERE tm.organization_id = t.organization_id AND tm.team_id = t.id
          AND tm.user_id = sqlc.narg(member_user_id)::bigint
    )
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(t.name) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN lower(t.name) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN t.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN t.created_at END DESC,
  t.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountTeams :one
SELECT COUNT(*)::bigint
FROM teams t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL OR t.name ILIKE '%' || sqlc.narg(q) || '%' OR t.description ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(skill)::text IS NULL OR sqlc.narg(skill)::text = ANY(t.skills))
  AND (
    sqlc.narg(member_user_id)::bigint IS NULL
    OR EXISTS (
        SELECT 1 FROM team_members tm
        WHERE tm.organization_id = t.organization_id AND tm.team_id = t.id
          AND tm.user_id = sqlc.narg(member_user_id)::bigint
    )
  );

-- name: ListTeamMembers :many
SELECT tm.team_id, tm.role, tm.created_at,
       u.id AS user_id, u.uuid AS user_uuid, u.email, u.name, u.surname, u.status,
       om.skills
FROM team_members tm
INNER JOIN organization_members om ON om.organization_id = tm.organization_id AND om.user_id = tm.user_id
INNER JOIN users u ON u.id = tm.user_id AND u.deleted_at IS NULL
WHERE tm.organization_id = sqlc.arg(organization_id)
  AND tm.team_id = ANY(sqlc.arg(team_ids)::bigint[])
ORDER BY tm.team_id, (tm.role = 'lead') DESC, u.name, u.surname, u.id;

-- name: UpsertTeamMember :exec
INSERT INTO team_members (organization_id, team_id, user_id, role)
VALUES (sqlc.arg(organization_id), sqlc.arg(team_id), sqlc.arg(user_id), sqlc.arg(role))
ON CONFLICT (organization_id, team_id, user_id) DO UPDATE SET role = EXCLUDED.role;

-- name: DeleteTeamMember :execrows
DELETE FROM team_members
WHERE organization_id = sqlc.arg(organization_id)
  AND team_id = sqlc.arg(team_id)
  AND user_id = sqlc.arg(user_id);

-- name: GetOrganizationMemberUserID :one
-- The user id of a member of this organization, by user uuid.
SELECT om.user_id
FROM organization_members om
INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id) AND u.uuid = sqlc.arg(user_uuid);

-- name: ListTeamIDsForUser :many
-- Teams of a member: conversation / ticket visibility without read_all.
SELECT team_id FROM team_members
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id)
ORDER BY team_id;

-- name: ListTeamUUIDsForUser :many
SELECT t.uuid
FROM team_members tm
INNER JOIN teams t ON t.organization_id = tm.organization_id AND t.id = tm.team_id
WHERE tm.organization_id = sqlc.arg(organization_id) AND tm.user_id = sqlc.arg(user_id)
ORDER BY t.name;

-- name: SetMemberSkills :execrows
UPDATE organization_members
SET skills = sqlc.arg(skills)::text[]
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id);

-- name: ListOrganizationSkills :many
-- Every skill tag used in the organization (members and teams), for
-- autocomplete and routing rules.
SELECT DISTINCT skill::text AS skill
FROM (
    SELECT unnest(om.skills) AS skill FROM organization_members om WHERE om.organization_id = sqlc.arg(organization_id)
    UNION
    SELECT unnest(t.skills) AS skill FROM teams t WHERE t.organization_id = sqlc.arg(organization_id)
) s
ORDER BY 1;
