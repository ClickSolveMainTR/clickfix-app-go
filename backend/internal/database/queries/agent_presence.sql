-- name: GetAgentPresence :one
SELECT * FROM agent_presence
WHERE organization_id = $1 AND user_id = $2;

-- name: LockAgentPresence :one
SELECT * FROM agent_presence
WHERE organization_id = $1 AND user_id = $2
FOR UPDATE;

-- name: UpsertAgentPresence :one
INSERT INTO agent_presence (organization_id, user_id, status, manual_status, last_seen_at, changed_at)
VALUES (sqlc.arg(organization_id), sqlc.arg(user_id), sqlc.arg(status), sqlc.arg(manual_status), NOW(), NOW())
ON CONFLICT (organization_id, user_id) DO UPDATE
SET status = EXCLUDED.status,
    manual_status = EXCLUDED.manual_status,
    last_seen_at = NOW(),
    changed_at = CASE WHEN agent_presence.status = EXCLUDED.status THEN agent_presence.changed_at ELSE NOW() END
RETURNING *;

-- name: TouchAgentPresence :exec
UPDATE agent_presence SET last_seen_at = NOW()
WHERE organization_id = $1 AND user_id = $2;

-- name: ListOrganizationPresence :many
-- Every member of the organization with their presence (offline when none).
SELECT u.uuid AS user_uuid,
       u.name,
       u.surname,
       COALESCE(p.status, 'offline')::text AS status,
       p.last_seen_at,
       p.changed_at
FROM organization_members m
JOIN users u ON u.id = m.user_id
LEFT JOIN agent_presence p ON p.organization_id = m.organization_id AND p.user_id = m.user_id
WHERE m.organization_id = sqlc.arg(organization_id)
ORDER BY CASE COALESCE(p.status, 'offline') WHEN 'online' THEN 0 WHEN 'busy' THEN 1 WHEN 'away' THEN 2 ELSE 3 END,
         u.name, u.surname;

-- name: ListIdleAgentPresence :many
-- Agents still shown as present whose last heartbeat is older than the cutoff.
SELECT p.organization_id, p.user_id, o.uuid AS organization_uuid, u.uuid AS user_uuid
FROM agent_presence p
JOIN organizations o ON o.id = p.organization_id
JOIN users u ON u.id = p.user_id
WHERE p.status <> 'offline' AND p.last_seen_at < sqlc.arg(cutoff)
LIMIT 500;

-- name: SetAgentPresenceOffline :execrows
UPDATE agent_presence
SET status = 'offline', changed_at = NOW()
WHERE organization_id = $1 AND user_id = $2 AND status <> 'offline' AND last_seen_at < sqlc.arg(cutoff);

-- name: GetConversationOrganization :one
-- Owner of a conversation, for realtime channel authorization.
SELECT c.organization_id, o.uuid AS organization_uuid
FROM conversations c
JOIN organizations o ON o.id = c.organization_id
WHERE c.uuid = $1 AND c.deleted_at IS NULL;
