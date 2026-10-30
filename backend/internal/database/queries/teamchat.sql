-- name: ChatCreateThread :one
INSERT INTO internal_threads(organization_id,kind,title,direct_key,conversation_id,created_by) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(organization_id,direct_key) DO UPDATE SET direct_key=excluded.direct_key RETURNING *;
-- name: ChatAddMember :exec
INSERT INTO internal_thread_members(organization_id,thread_id,user_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;
-- name: ChatGetThread :one
SELECT t.* FROM internal_threads t WHERE t.organization_id=$1 AND t.uuid=$2 AND EXISTS(SELECT 1 FROM internal_thread_members m WHERE m.organization_id=t.organization_id AND m.thread_id=t.id AND m.user_id=$3) FOR UPDATE;
-- name: ChatListThreads :many
SELECT t.uuid,t.kind,t.title,t.closed_at,t.updated_at,
 (SELECT count(*) FROM internal_messages x WHERE x.organization_id=t.organization_id AND x.thread_id=t.id AND x.created_at>COALESCE(m.last_read_at,'epoch'))::bigint AS unread
FROM internal_threads t JOIN internal_thread_members m ON m.organization_id=t.organization_id AND m.thread_id=t.id
WHERE t.organization_id=$1 AND m.user_id=$2 ORDER BY t.created_at DESC,t.id DESC LIMIT $3 OFFSET $4;
-- name: ChatThreadMembers :many
SELECT u.id,u.uuid FROM internal_thread_members m JOIN users u ON u.id=m.user_id
JOIN organization_members om ON om.organization_id=m.organization_id AND om.user_id=m.user_id
WHERE m.organization_id=$1 AND m.thread_id=$2 AND u.status='active' ORDER BY u.id;
-- name: ChatMessages :many
SELECT x.uuid,x.body,x.is_ai,x.client_id,x.created_at,u.uuid AS sender_uuid FROM internal_messages x JOIN users u ON u.id=x.sender_id WHERE x.organization_id=$1 AND x.thread_id=$2 ORDER BY x.id DESC LIMIT $3 OFFSET $4;
-- name: ChatPost :one
INSERT INTO internal_messages(organization_id,thread_id,sender_id,body,client_id,is_ai) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(organization_id,thread_id,client_id) DO UPDATE SET client_id=excluded.client_id RETURNING *;
-- name: ChatTouch :exec
UPDATE internal_threads SET updated_at=now() WHERE organization_id=$1 AND id=$2;
-- name: ChatRead :exec
UPDATE internal_thread_members SET last_read_at=now() WHERE organization_id=$1 AND thread_id=$2 AND user_id=$3;
-- name: ChatClose :exec
UPDATE internal_threads SET closed_at=now() WHERE organization_id=$1 AND id=$2;
-- name: ChatCountThreads :one
SELECT count(*) FROM internal_threads t JOIN internal_thread_members m ON m.organization_id=t.organization_id AND m.thread_id=t.id WHERE t.organization_id=$1 AND m.user_id=$2;
