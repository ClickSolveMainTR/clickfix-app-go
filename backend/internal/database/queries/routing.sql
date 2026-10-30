-- CF-53 / CF-54: agent capacity and automatic routing.

-- name: ListCapacityLimits :many
SELECT a.user_id, u.uuid AS user_uuid, a.channel_type, a.max_open
FROM agent_capacity a
LEFT JOIN users u ON u.id = a.user_id
WHERE a.organization_id = $1
ORDER BY a.user_id NULLS FIRST, a.channel_type;

-- name: DeleteCapacityLimits :exec
DELETE FROM agent_capacity
WHERE organization_id = sqlc.arg(organization_id) AND COALESCE(user_id, 0) = sqlc.arg(user_id)::bigint;

-- name: InsertCapacityLimit :exec
INSERT INTO agent_capacity (organization_id, user_id, channel_type, max_open)
VALUES (sqlc.arg(organization_id), sqlc.narg(user_id), sqlc.arg(channel_type), sqlc.arg(max_open));

-- name: ListRoutingAgents :many
-- Members who may answer customers, with presence and skills.
SELECT u.id AS user_id, u.uuid AS user_uuid, u.name, u.surname, m.skills,
       COALESCE(p.status, 'offline')::text AS status, p.last_seen_at
FROM organization_members m
JOIN users u ON u.id = m.user_id
LEFT JOIN agent_presence p ON p.organization_id = m.organization_id AND p.user_id = m.user_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND u.status = 'active'
  AND EXISTS (
      SELECT 1 FROM role_permissions rp
      JOIN permissions pe ON pe.id = rp.permission_id AND pe.slug = 'tenant.conversations.reply'
      WHERE rp.role_id = m.role_id
         OR rp.role_id IN (SELECT r.role_id FROM organization_member_roles r
                           WHERE r.organization_id = m.organization_id AND r.member_id = m.id))
ORDER BY u.id;

-- name: ListRoutingTeamMembers :many
SELECT team_id, user_id FROM team_members WHERE organization_id = $1 ORDER BY team_id, user_id;

-- name: ListRoutingTeams :many
SELECT id, uuid, name, skills FROM teams WHERE organization_id = $1 ORDER BY lower(name);

-- name: CountOpenConversationsByAgent :many
SELECT assignee_user_id::bigint AS user_id, channel_type, COUNT(*)::int AS open
FROM conversations
WHERE organization_id = $1 AND assignee_user_id IS NOT NULL AND status = 'open' AND deleted_at IS NULL
GROUP BY assignee_user_id, channel_type;

-- name: CountUnassignedOpenConversations :one
SELECT COUNT(*)::int FROM conversations
WHERE organization_id = $1 AND assignee_user_id IS NULL AND status = 'open' AND deleted_at IS NULL;

-- name: GetRoutingSettings :one
SELECT * FROM routing_settings WHERE organization_id = $1;

-- name: UpsertRoutingSettings :one
INSERT INTO routing_settings (organization_id, enabled, strategy, respect_capacity, vip_account_owner, reassign_on_offline, unanswered_minutes, updated_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(enabled), sqlc.arg(strategy), sqlc.arg(respect_capacity), sqlc.arg(vip_account_owner),
        sqlc.arg(reassign_on_offline), sqlc.narg(unanswered_minutes), NOW())
ON CONFLICT (organization_id) DO UPDATE SET
    enabled = EXCLUDED.enabled, strategy = EXCLUDED.strategy, respect_capacity = EXCLUDED.respect_capacity,
    vip_account_owner = EXCLUDED.vip_account_owner, reassign_on_offline = EXCLUDED.reassign_on_offline,
    unanswered_minutes = EXCLUDED.unanswered_minutes, updated_at = NOW()
RETURNING *;

-- name: ListRoutingRules :many
SELECT r.*, t.uuid AS team_uuid, t.name AS team_name
FROM routing_rules r
LEFT JOIN teams t ON t.organization_id = r.organization_id AND t.id = r.team_id
WHERE r.organization_id = $1
ORDER BY r.position, r.id;

-- name: GetRoutingRuleByUUID :one
SELECT * FROM routing_rules WHERE organization_id = $1 AND uuid = $2;

-- name: NextRoutingRulePosition :one
SELECT COALESCE(MAX(position), 0)::int + 1 FROM routing_rules WHERE organization_id = $1;

-- name: InsertRoutingRule :one
INSERT INTO routing_rules (organization_id, name, position, is_active, conditions, team_id, skills)
VALUES (sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(position), sqlc.arg(is_active), sqlc.arg(conditions),
        sqlc.narg(team_id), sqlc.arg(skills))
RETURNING *;

-- name: UpdateRoutingRule :one
UPDATE routing_rules
SET name = sqlc.arg(name), position = sqlc.arg(position), is_active = sqlc.arg(is_active),
    conditions = sqlc.arg(conditions), team_id = sqlc.narg(team_id), skills = sqlc.arg(skills)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteRoutingRule :execrows
DELETE FROM routing_rules WHERE organization_id = $1 AND uuid = $2;

-- name: GetTeamByUUIDForRouting :one
SELECT id, uuid, name FROM teams WHERE organization_id = $1 AND uuid = $2;

-- name: GetRoutingCursor :one
SELECT last_user_id FROM routing_cursors WHERE organization_id = $1 AND scope_key = $2;

-- name: UpsertRoutingCursor :exec
INSERT INTO routing_cursors (organization_id, scope_key, last_user_id, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (organization_id, scope_key) DO UPDATE SET last_user_id = EXCLUDED.last_user_id, updated_at = NOW();

-- name: InsertRoutingEvent :exec
INSERT INTO routing_events (organization_id, conversation_id, from_user_id, to_user_id, team_id, rule_id, reason, strategy, note)
VALUES (sqlc.arg(organization_id), sqlc.arg(conversation_id), sqlc.narg(from_user_id), sqlc.narg(to_user_id),
        sqlc.narg(team_id), sqlc.narg(rule_id), sqlc.arg(reason), sqlc.arg(strategy), sqlc.arg(note));

-- name: ListRoutingEvents :many
SELECT e.id, e.reason, e.strategy, e.note, e.created_at, c.uuid AS conversation_uuid, c.number,
       fu.uuid AS from_user_uuid, COALESCE(fu.name || ' ' || fu.surname, '')::text AS from_name,
       tu.uuid AS to_user_uuid, COALESCE(tu.name || ' ' || tu.surname, '')::text AS to_name,
       t.name AS team_name
FROM routing_events e
JOIN conversations c ON c.organization_id = e.organization_id AND c.id = e.conversation_id
LEFT JOIN users fu ON fu.id = e.from_user_id
LEFT JOIN users tu ON tu.id = e.to_user_id
LEFT JOIN teams t ON t.organization_id = e.organization_id AND t.id = e.team_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(conversation_uuid)::uuid IS NULL OR c.uuid = sqlc.narg(conversation_uuid)::uuid)
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(max_rows);

-- name: SetConversationTeam :exec
UPDATE conversations SET assignee_team_id = sqlc.narg(team_id)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ListRoutingConversationTagUUIDs :many
SELECT t.uuid FROM conversation_tags ct
JOIN tags t ON t.organization_id = ct.organization_id AND t.id = ct.tag_id
WHERE ct.organization_id = $1 AND ct.conversation_id = $2;

-- name: GetCompanyAccountOwner :one
SELECT account_owner_id FROM companies WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: ListQueuedConversations :many
-- Unassigned open conversations of organizations with routing on.
SELECT c.organization_id, c.id FROM conversations c
JOIN routing_settings s ON s.organization_id = c.organization_id AND s.enabled
WHERE c.assignee_user_id IS NULL AND c.status = 'open' AND c.deleted_at IS NULL
ORDER BY c.created_at
LIMIT sqlc.arg(max_rows);

-- name: ListOrganizationQueue :many
SELECT id FROM conversations
WHERE organization_id = $1 AND assignee_user_id IS NULL AND status = 'open' AND deleted_at IS NULL
ORDER BY created_at
LIMIT sqlc.arg(max_rows);

-- name: ListUnansweredRouted :many
-- Routed conversations still waiting on their assignee past the limit; a
-- later manual reassignment (latest routing event to someone else) wins.
SELECT c.organization_id, c.id, c.assignee_user_id::bigint AS assignee_user_id
FROM conversations c
JOIN routing_settings s ON s.organization_id = c.organization_id AND s.enabled AND s.unanswered_minutes IS NOT NULL
WHERE c.status = 'open' AND c.deleted_at IS NULL AND c.assignee_user_id IS NOT NULL AND c.waiting_since IS NOT NULL
  AND c.waiting_since < sqlc.arg(now)::timestamptz - make_interval(mins => s.unanswered_minutes)
  AND EXISTS (
      SELECT 1 FROM routing_events e
      WHERE e.organization_id = c.organization_id AND e.conversation_id = c.id
        AND e.to_user_id = c.assignee_user_id
        AND e.created_at < sqlc.arg(now)::timestamptz - make_interval(mins => s.unanswered_minutes)
        AND e.id = (SELECT MAX(e2.id) FROM routing_events e2
                    WHERE e2.organization_id = c.organization_id AND e2.conversation_id = c.id))
ORDER BY c.waiting_since
LIMIT sqlc.arg(max_rows);

-- name: ListWaitingConversationsOfAgent :many
SELECT id FROM conversations
WHERE organization_id = $1 AND assignee_user_id = $2 AND status = 'open' AND waiting_since IS NOT NULL AND deleted_at IS NULL
ORDER BY waiting_since
LIMIT 200;
