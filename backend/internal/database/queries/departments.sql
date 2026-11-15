-- Departments (CF-215). Every query is scoped by organization_id.

-- name: CreateDepartment :one
INSERT INTO departments (
    organization_id, brand_id, name, description, color, widget_visible, sort_order, sla_policy_id,
    timezone, working_hours, ai_enabled, ai_instructions, routing_hint
) VALUES (
    sqlc.arg(organization_id), sqlc.narg(brand_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(color),
    sqlc.arg(widget_visible), sqlc.arg(sort_order), sqlc.narg(sla_policy_id), sqlc.narg(timezone),
    sqlc.narg(working_hours), sqlc.arg(ai_enabled), sqlc.arg(ai_instructions), sqlc.arg(routing_hint)
)
RETURNING *;

-- name: UpdateDepartment :one
UPDATE departments
SET brand_id = sqlc.narg(brand_id),
    name = sqlc.arg(name),
    description = sqlc.arg(description),
    color = sqlc.arg(color),
    widget_visible = sqlc.arg(widget_visible),
    sort_order = sqlc.arg(sort_order),
    sla_policy_id = sqlc.narg(sla_policy_id),
    timezone = sqlc.narg(timezone),
    working_hours = sqlc.narg(working_hours),
    ai_enabled = sqlc.arg(ai_enabled),
    ai_instructions = sqlc.arg(ai_instructions),
    routing_hint = sqlc.arg(routing_hint)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid)
RETURNING *;

-- name: DeleteDepartment :execrows
DELETE FROM departments WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: GetDepartmentByUUID :one
SELECT * FROM departments WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: GetDepartmentByID :one
SELECT * FROM departments WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: DepartmentNameTaken :one
SELECT EXISTS (
    SELECT 1 FROM departments
    WHERE organization_id = sqlc.arg(organization_id)
      AND lower(name) = lower(sqlc.arg(name)::text)
      AND uuid <> sqlc.arg(except_uuid)::uuid
)::boolean;

-- name: ListDepartments :many
SELECT d.*,
       (SELECT COUNT(*) FROM teams t WHERE t.organization_id = d.organization_id AND t.department_id = d.id)::bigint AS team_count
FROM departments d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL OR d.name ILIKE '%' || sqlc.narg(q) || '%' OR d.description ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(brand_id)::uuid IS NULL OR d.brand_id = sqlc.narg(brand_id)::uuid)
  AND (NOT sqlc.arg(only_ids)::boolean OR d.id = ANY(sqlc.arg(ids)::bigint[]))
ORDER BY d.sort_order, lower(d.name), d.id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountDepartments :one
SELECT COUNT(*)::bigint FROM departments d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL OR d.name ILIKE '%' || sqlc.narg(q) || '%' OR d.description ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(brand_id)::uuid IS NULL OR d.brand_id = sqlc.narg(brand_id)::uuid)
  AND (NOT sqlc.arg(only_ids)::boolean OR d.id = ANY(sqlc.arg(ids)::bigint[]));

-- name: ListDepartmentRefs :many
-- Uuid and name of departments by id (DTOs of teams, knowledge, conversations).
SELECT id, uuid, name, color FROM departments
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ListDepartmentTeams :many
SELECT t.department_id::bigint AS department_id, t.uuid, t.name, t.color
FROM teams t
WHERE t.organization_id = sqlc.arg(organization_id) AND t.department_id = ANY(sqlc.arg(department_ids)::bigint[])
ORDER BY lower(t.name), t.id;

-- name: ListDepartmentLeads :many
-- Leads of a department are the leads of its teams.
SELECT DISTINCT t.department_id::bigint AS department_id, u.uuid, u.email, u.name, u.surname
FROM team_members tm
INNER JOIN teams t ON t.organization_id = tm.organization_id AND t.id = tm.team_id
INNER JOIN users u ON u.id = tm.user_id AND u.deleted_at IS NULL
WHERE tm.organization_id = sqlc.arg(organization_id) AND tm.role = 'lead'
  AND t.department_id = ANY(sqlc.arg(department_ids)::bigint[])
ORDER BY 1, u.name, u.surname;

-- name: ListUserDepartments :many
-- Departments of a member (through their teams) and whether they lead one.
SELECT t.department_id::bigint AS department_id, bool_or(tm.role = 'lead')::boolean AS lead
FROM team_members tm
INNER JOIN teams t ON t.organization_id = tm.organization_id AND t.id = tm.team_id
WHERE tm.organization_id = sqlc.arg(organization_id) AND tm.user_id = sqlc.arg(user_id) AND t.department_id IS NOT NULL
GROUP BY t.department_id
ORDER BY t.department_id;

-- name: SetTeamDepartment :execrows
UPDATE teams SET department_id = sqlc.narg(department_id)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: DepartmentUsage :one
-- Knowledge a department still owns (it cannot be deleted while it does).
SELECT
    (SELECT COUNT(*) FROM knowledge_articles a WHERE a.organization_id = sqlc.arg(organization_id) AND a.department_id = sqlc.arg(department_id))::bigint AS articles,
    (SELECT COUNT(*) FROM knowledge_sources s WHERE s.organization_id = sqlc.arg(organization_id) AND s.department_id = sqlc.arg(department_id))::bigint AS sources,
    (SELECT COUNT(*) FROM business_context_sections b WHERE b.organization_id = sqlc.arg(organization_id) AND b.department_id = sqlc.arg(department_id))::bigint AS context_sections,
    (SELECT COUNT(*) FROM knowledge_suggestions k WHERE k.organization_id = sqlc.arg(organization_id) AND k.department_id = sqlc.arg(department_id))::bigint AS suggestions;

-- name: MoveDepartmentArticles :exec
UPDATE knowledge_articles SET department_id = sqlc.narg(to_department_id)
WHERE organization_id = sqlc.arg(organization_id) AND department_id = sqlc.arg(from_department_id);

-- name: MoveDepartmentSources :exec
UPDATE knowledge_sources SET department_id = sqlc.narg(to_department_id)
WHERE organization_id = sqlc.arg(organization_id) AND department_id = sqlc.arg(from_department_id);

-- name: MoveDepartmentContext :exec
UPDATE business_context_sections SET department_id = sqlc.narg(to_department_id)
WHERE organization_id = sqlc.arg(organization_id) AND department_id = sqlc.arg(from_department_id);

-- name: MoveDepartmentSuggestions :exec
UPDATE knowledge_suggestions SET department_id = sqlc.narg(to_department_id)
WHERE organization_id = sqlc.arg(organization_id) AND department_id = sqlc.arg(from_department_id);

-- name: ListWidgetDepartments :many
-- The widget's department picker: visible departments of the widget's brand
-- and departments without a brand.
SELECT uuid, name, description, color FROM departments
WHERE organization_id = sqlc.arg(organization_id) AND widget_visible
  AND (brand_id IS NULL OR brand_id = sqlc.narg(brand_id)::uuid)
ORDER BY sort_order, lower(name), id;

-- name: ListRoutingDepartments :many
-- Candidates of the AI department prediction.
SELECT id, uuid, name, description, routing_hint, brand_id FROM departments
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY sort_order, lower(name), id
LIMIT 50;

-- name: GetConversationDepartment :one
SELECT c.id, c.uuid, c.brand_id, c.department_id, c.department_source, c.department_confidence, c.department_routed_at,
       c.assignee_team_id, c.status
FROM conversations c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.uuid = sqlc.arg(uuid) AND c.deleted_at IS NULL;

-- name: GetConversationDepartmentByID :one
SELECT c.id, c.uuid, c.brand_id, c.department_id, c.department_source, c.department_confidence, c.department_routed_at,
       c.assignee_team_id, c.status
FROM conversations c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.id = sqlc.arg(id) AND c.deleted_at IS NULL;

-- name: SetConversationDepartment :execrows
UPDATE conversations
SET department_id = sqlc.narg(department_id),
    department_source = sqlc.narg(department_source),
    department_confidence = sqlc.narg(department_confidence),
    department_routed_at = COALESCE(department_routed_at, now())
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: TeamDepartmentByID :one
SELECT uuid, department_id FROM teams WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: MarkConversationRouted :execrows
-- Claims the one-time routing of a conversation (AI prediction).
UPDATE conversations SET department_routed_at = now()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND department_routed_at IS NULL;

-- name: InsertDepartmentTransfer :one
INSERT INTO conversation_department_transfers (
    organization_id, conversation_id, from_department_id, to_department_id, actor_kind, actor_user_id,
    confidence, reason, context_summary
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(conversation_id), sqlc.narg(from_department_id), sqlc.narg(to_department_id),
    sqlc.arg(actor_kind), sqlc.narg(actor_user_id), sqlc.narg(confidence), sqlc.arg(reason), sqlc.arg(context_summary)
)
RETURNING *;

-- name: ListDepartmentTransfers :many
SELECT tr.uuid, tr.actor_kind, tr.confidence, tr.reason, tr.context_summary, tr.created_at,
       fd.uuid AS from_uuid, fd.name AS from_name, td.uuid AS to_uuid, td.name AS to_name,
       u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname
FROM conversation_department_transfers tr
LEFT JOIN departments fd ON fd.organization_id = tr.organization_id AND fd.id = tr.from_department_id
LEFT JOIN departments td ON td.organization_id = tr.organization_id AND td.id = tr.to_department_id
LEFT JOIN users u ON u.id = tr.actor_user_id
WHERE tr.organization_id = sqlc.arg(organization_id) AND tr.conversation_id = sqlc.arg(conversation_id)
ORDER BY tr.created_at DESC, tr.id DESC
LIMIT 50;

-- name: GetDepartmentAI :one
SELECT id, uuid, name, ai_enabled, ai_instructions FROM departments
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- Knowledge scope of a single item (sharing, visibility checks).

-- name: KnowledgeArticleDepartment :one
SELECT id, department_id, title FROM knowledge_articles
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: KnowledgeSourceDepartment :one
SELECT id, department_id, title FROM knowledge_sources
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: BusinessContextDepartment :one
SELECT id, department_id, title FROM business_context_sections
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: KnowledgeSuggestionDepartment :one
SELECT id, department_id, title FROM knowledge_suggestions
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: SetKnowledgeArticleDepartment :execrows
UPDATE knowledge_articles SET department_id = sqlc.narg(department_id)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: SetKnowledgeSourceDepartment :execrows
UPDATE knowledge_sources SET department_id = sqlc.narg(department_id)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: SetBusinessContextDepartment :execrows
UPDATE business_context_sections SET department_id = sqlc.narg(department_id)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: SetKnowledgeSuggestionDepartment :execrows
UPDATE knowledge_suggestions SET department_id = sqlc.narg(department_id)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: DepartmentVisible :one
SELECT department_visible(sqlc.arg(organization_id)::bigint, sqlc.arg(resource_kind)::text, sqlc.arg(resource_id)::bigint,
    sqlc.narg(department_id)::bigint, sqlc.arg(viewer_departments)::bigint[])::boolean;

-- name: CreateDepartmentShare :one
INSERT INTO department_knowledge_shares (organization_id, department_id, resource_kind, resource_id, shared_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(department_id), sqlc.arg(resource_kind), sqlc.arg(resource_id), sqlc.narg(shared_by))
ON CONFLICT (organization_id, resource_kind, resource_id, department_id) DO UPDATE SET shared_by = department_knowledge_shares.shared_by
RETURNING *;

-- name: GetDepartmentShare :one
SELECT * FROM department_knowledge_shares
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: DeleteDepartmentShare :execrows
DELETE FROM department_knowledge_shares WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ListDepartmentShares :many
-- Items shared with a department (received) and items of the department
-- shared with others (given).
SELECT s.uuid, s.resource_kind, s.created_at,
       rd.uuid AS department_uuid, rd.name AS department_name,
       od.uuid AS owner_uuid, od.name AS owner_name,
       COALESCE(a.uuid, ks.uuid, bc.uuid)::uuid AS resource_uuid,
       COALESCE(a.title, ks.title, bc.title, '')::text AS resource_title
FROM department_knowledge_shares s
INNER JOIN departments rd ON rd.organization_id = s.organization_id AND rd.id = s.department_id
LEFT JOIN knowledge_articles a ON s.resource_kind = 'article' AND a.organization_id = s.organization_id AND a.id = s.resource_id
LEFT JOIN knowledge_sources ks ON s.resource_kind = 'source' AND ks.organization_id = s.organization_id AND ks.id = s.resource_id
LEFT JOIN business_context_sections bc ON s.resource_kind = 'context' AND bc.organization_id = s.organization_id AND bc.id = s.resource_id
LEFT JOIN departments od ON od.organization_id = s.organization_id AND od.id = COALESCE(a.department_id, ks.department_id, bc.department_id)
WHERE s.organization_id = sqlc.arg(organization_id)
  AND (s.department_id = sqlc.arg(department_id) OR COALESCE(a.department_id, ks.department_id, bc.department_id) = sqlc.arg(department_id))
ORDER BY s.created_at DESC, s.id DESC
LIMIT 200;

-- name: KnowledgeOwnerDepartment :one
-- The department a shared item belongs to (0 = organization-wide).
SELECT COALESCE(CASE sqlc.arg(resource_kind)::text
    WHEN 'article' THEN (SELECT a.department_id FROM knowledge_articles a WHERE a.organization_id = sqlc.arg(organization_id) AND a.id = sqlc.arg(resource_id))
    WHEN 'source' THEN (SELECT s.department_id FROM knowledge_sources s WHERE s.organization_id = sqlc.arg(organization_id) AND s.id = sqlc.arg(resource_id))
    ELSE (SELECT b.department_id FROM business_context_sections b WHERE b.organization_id = sqlc.arg(organization_id) AND b.id = sqlc.arg(resource_id))
END, 0)::bigint AS department_id;
