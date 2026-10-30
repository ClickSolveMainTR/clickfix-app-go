-- name: EnsureEmailIMAPState :exec
INSERT INTO channel_email_imap_states (organization_id, channel_instance_id)
SELECT ci.organization_id, ci.id FROM channel_instances ci
WHERE ci.organization_id = sqlc.arg(organization_id) AND ci.id = sqlc.arg(channel_instance_id) AND ci.deleted_at IS NULL
ON CONFLICT (organization_id, channel_instance_id) DO NOTHING;

-- name: GetEmailIMAPState :one
SELECT * FROM channel_email_imap_states
WHERE organization_id = $1 AND channel_instance_id = $2;

-- name: SaveEmailIMAPCursor :exec
UPDATE channel_email_imap_states SET mailbox_key = sqlc.arg(mailbox_key),
 uid_validity = sqlc.arg(uid_validity), initial_uid = sqlc.arg(initial_uid), last_uid = sqlc.arg(last_uid)
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id);

-- name: FinishEmailIMAPPoll :exec
UPDATE channel_email_imap_states SET failures = sqlc.arg(failures), next_attempt_at = sqlc.arg(next_attempt_at)
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id);

-- name: TryEmailIMAPLock :one
-- Transaction-scoped platform worker lock, key derived from the instance id.
SELECT pg_try_advisory_xact_lock(sqlc.arg(lock_key)::bigint)::boolean;
