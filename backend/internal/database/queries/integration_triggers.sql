-- name: IntegrationRecordTrigger :exec
WITH allocated AS (
 INSERT INTO integration_event_cursors(organization_id,last_sequence) VALUES(sqlc.arg(organization_id),1)
 ON CONFLICT(organization_id) DO UPDATE SET last_sequence=integration_event_cursors.last_sequence+1
 RETURNING organization_id,last_sequence
)
INSERT INTO integration_trigger_events(organization_id,sequence,event_id,event_name,payload)
SELECT organization_id,last_sequence,sqlc.arg(event_id),sqlc.arg(event_name),sqlc.arg(payload) FROM allocated
ON CONFLICT(organization_id,event_id) DO NOTHING;
-- name: IntegrationPollTriggers :many
SELECT sequence,event_id,event_name,payload,created_at FROM integration_trigger_events
WHERE organization_id=$1 AND event_name=$2 AND sequence>sqlc.arg(after_sequence)::bigint
ORDER BY sequence LIMIT sqlc.arg(row_limit);
-- name: IntegrationTriggerCursor :one
SELECT last_sequence FROM integration_event_cursors WHERE organization_id=$1;
-- name: IntegrationTriggerOldest :one
SELECT pruned_sequence FROM integration_event_cursors WHERE organization_id=$1;
-- name: IntegrationPurgeTriggers :exec
WITH removed AS (DELETE FROM integration_trigger_events WHERE created_at<now()-interval '30 days' RETURNING organization_id,sequence),
 floors AS (SELECT organization_id,max(sequence) AS floor FROM removed GROUP BY organization_id)
 UPDATE integration_event_cursors c SET pruned_sequence=GREATEST(c.pruned_sequence,f.floor) FROM floors f WHERE c.organization_id=f.organization_id;
