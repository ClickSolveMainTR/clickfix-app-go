-- name: InsertAttachment :one
INSERT INTO attachments (
    uuid, organization_id, uploaded_by, owner_type, owner_id, filename, content_type,
    size_bytes, sha256, object_key, thumb_key, scan_status, scan_detail, scanned_at, expires_at
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.narg(uploaded_by), sqlc.narg(owner_type),
    sqlc.narg(owner_id), sqlc.arg(filename), sqlc.arg(content_type),
    sqlc.arg(size_bytes), sqlc.arg(sha256), sqlc.arg(object_key), sqlc.narg(thumb_key),
    sqlc.arg(scan_status), sqlc.narg(scan_detail), sqlc.narg(scanned_at), sqlc.narg(expires_at)
)
RETURNING *;

-- name: GetAttachmentByUUID :one
SELECT * FROM attachments
WHERE organization_id = $1 AND uuid = $2 AND deleted_at IS NULL AND (expires_at IS NULL OR expires_at>now());

-- name: ListAttachments :many
SELECT * FROM attachments
WHERE organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
  AND (sqlc.narg(owner_type)::text IS NULL OR owner_type = sqlc.narg(owner_type))
  AND (sqlc.narg(owner_id)::uuid IS NULL OR owner_id = sqlc.narg(owner_id))
  AND (sqlc.narg(q)::text IS NULL OR filename ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'filename' AND NOT sqlc.arg(sort_desc)::boolean THEN filename END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'filename' AND sqlc.arg(sort_desc)::boolean THEN filename END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'size_bytes' AND NOT sqlc.arg(sort_desc)::boolean THEN size_bytes END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'size_bytes' AND sqlc.arg(sort_desc)::boolean THEN size_bytes END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN created_at END DESC,
  id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountAttachments :one
SELECT COUNT(*)::bigint FROM attachments
WHERE organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
  AND (sqlc.narg(owner_type)::text IS NULL OR owner_type = sqlc.narg(owner_type))
  AND (sqlc.narg(owner_id)::uuid IS NULL OR owner_id = sqlc.narg(owner_id))
  AND (sqlc.narg(q)::text IS NULL OR filename ILIKE '%' || sqlc.narg(q) || '%');

-- name: SoftDeleteAttachment :one
UPDATE attachments SET deleted_at = NOW()
WHERE organization_id = $1 AND uuid = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SumAttachmentUsage :one
SELECT COALESCE(SUM(size_bytes), 0)::bigint AS used_bytes, COUNT(*)::bigint AS file_count
FROM attachments
WHERE organization_id = $1 AND deleted_at IS NULL;

-- name: LockAttachmentQuota :exec
SELECT pg_advisory_xact_lock(sqlc.arg(lock_key)::bigint);

-- name: ListUserUUIDsByIDs :many
SELECT id, uuid FROM users WHERE id = ANY(sqlc.arg(ids)::bigint[]);
