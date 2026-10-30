-- name: MetaDeletionReceipt :one
INSERT INTO meta_deletion_receipts(code) VALUES($1) RETURNING *;
-- name: MetaDeletionLink :exec
INSERT INTO meta_deletion_links(organization_id,receipt_id,request_uuid) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;
-- name: MetaDeletionStatus :one
SELECT code,status FROM meta_deletion_receipts WHERE code=$1;
-- name: MetaDeletionRequests :many
SELECT l.organization_id,l.request_uuid FROM meta_deletion_links l JOIN meta_deletion_receipts r ON r.id=l.receipt_id WHERE r.code=$1 ORDER BY l.organization_id,l.id;
-- name: MetaDeletionComplete :exec
UPDATE meta_deletion_receipts SET status='completed',completed_at=COALESCE(completed_at,now()) WHERE code=$1;
-- name: MetaDeletionPending :many
SELECT code FROM meta_deletion_receipts WHERE status='pending' ORDER BY id LIMIT 100;
-- name: MetaSubjectContacts :many
-- Authenticated provider callback only; result organizations scope all further work.
SELECT DISTINCT i.organization_id,c.uuid FROM contact_identities i
JOIN contacts c ON c.organization_id=i.organization_id AND c.id=i.contact_id
WHERE i.external_id=sqlc.arg(subject)::text AND i.channel_type=ANY(sqlc.arg(channel_types)::text[])
ORDER BY i.organization_id,c.uuid;
-- name: MetaSubjectInstances :many
SELECT organization_id,id FROM channel_instances
WHERE channel_type=ANY(sqlc.arg(channel_types)::text[])
AND (external_id=sqlc.arg(subject)::text OR settings->>'meta_user_id'=sqlc.arg(subject)::text)
ORDER BY organization_id,id;
-- name: MetaDeauthorizeInstance :exec
UPDATE channel_instances SET status='disconnected',credentials_enc='',last_error='',last_error_at=NULL,
 settings=CASE WHEN sqlc.arg(erase)::boolean THEN settings-'meta_user_id' ELSE settings END
WHERE organization_id=$1 AND id=$2;
