-- name: InsertDomainEventLog :execrows
INSERT INTO domain_event_log (
    event_id, event_name, event_version, organization_id, actor_user_id,
    entity_type, entity_id, entity_uuid, payload, occurred_at
) VALUES (
    sqlc.arg(event_id),
    sqlc.arg(event_name),
    sqlc.arg(event_version),
    sqlc.narg(organization_id),
    sqlc.narg(actor_user_id),
    sqlc.narg(entity_type),
    sqlc.narg(entity_id),
    sqlc.narg(entity_uuid),
    sqlc.arg(payload),
    sqlc.arg(occurred_at)
)
ON CONFLICT (event_id) DO NOTHING;

-- name: ListDomainEventLogByOrganization :many
SELECT *
FROM domain_event_log
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(event_name)::text IS NULL OR event_name = sqlc.narg(event_name)::text)
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: ClaimEventConsumption :execrows
-- Returns 1 for the first delivery of an event to a consumer, 0 for a duplicate.
INSERT INTO event_consumptions (consumer, event_id)
VALUES (sqlc.arg(consumer), sqlc.arg(event_id))
ON CONFLICT (consumer, event_id) DO NOTHING;

-- name: PurgeEventConsumptionsBefore :execrows
DELETE FROM event_consumptions
WHERE processed_at < sqlc.arg(before);

-- name: UpsertEventDeadLetter :one
INSERT INTO event_dead_letters (consumer, event_id, event_name, organization_id, envelope, attempts, last_error)
VALUES (
    sqlc.arg(consumer),
    sqlc.arg(event_id),
    sqlc.arg(event_name),
    sqlc.narg(organization_id),
    sqlc.arg(envelope),
    sqlc.arg(attempts),
    sqlc.arg(last_error)
)
ON CONFLICT (consumer, event_id) DO UPDATE
SET attempts = event_dead_letters.attempts + EXCLUDED.attempts,
    last_error = EXCLUDED.last_error,
    envelope = EXCLUDED.envelope,
    replayed_at = NULL
RETURNING *;

-- name: GetEventDeadLetter :one
SELECT *
FROM event_dead_letters
WHERE id = $1;

-- name: ListOpenEventDeadLetters :many
SELECT *
FROM event_dead_letters
WHERE replayed_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: MarkEventDeadLetterReplayed :exec
UPDATE event_dead_letters
SET replayed_at = NOW()
WHERE id = $1;
