-- name: OpsTranscript :many
SELECT sender_type,content FROM messages WHERE organization_id=$1 AND conversation_id=$2 AND NOT private AND deleted_at IS NULL AND kind NOT IN ('event','unsupported') ORDER BY id DESC LIMIT 100;
