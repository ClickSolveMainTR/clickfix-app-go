-- name: OncallCreateRota :one
INSERT INTO oncall_rotas(organization_id,name,timezone,layers,overrides) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: OncallUpdateRota :one
UPDATE oncall_rotas SET name=$3,timezone=$4,layers=$5,overrides=$6 WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: OncallRota :one
SELECT * FROM oncall_rotas WHERE organization_id=$1 AND uuid=$2;
-- name: OncallRotas :many
SELECT * FROM oncall_rotas WHERE organization_id=$1 ORDER BY id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: OncallRotaCount :one
SELECT count(*) FROM oncall_rotas WHERE organization_id=$1;
-- name: OncallDeleteRota :execrows
DELETE FROM oncall_rotas WHERE organization_id=$1 AND uuid=$2;
-- name: OncallCreatePolicy :one
INSERT INTO escalation_policies(organization_id,name,steps,urgent_conversations,incidents,enabled) VALUES($1,$2,$3,$4,$5,$6) RETURNING *;
-- name: OncallUpdatePolicy :one
UPDATE escalation_policies SET name=$3,steps=$4,urgent_conversations=$5,incidents=$6,enabled=$7 WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: OncallPolicy :one
SELECT * FROM escalation_policies WHERE organization_id=$1 AND uuid=$2;
-- name: OncallPolicies :many
SELECT * FROM escalation_policies WHERE organization_id=$1 ORDER BY id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: OncallPolicyCount :one
SELECT count(*) FROM escalation_policies WHERE organization_id=$1;
-- name: OncallDeletePolicy :execrows
DELETE FROM escalation_policies WHERE organization_id=$1 AND uuid=$2;
-- name: OncallTeamMembers :many
SELECT u.uuid FROM team_members m JOIN teams t ON t.organization_id=m.organization_id AND t.id=m.team_id
JOIN organization_members om ON om.organization_id=m.organization_id AND om.user_id=m.user_id
JOIN users u ON u.id=m.user_id AND u.deleted_at IS NULL
WHERE m.organization_id=$1 AND t.uuid=$2 ORDER BY u.id LIMIT 501;
-- name: OncallCreateMonitor :one
INSERT INTO uptime_monitors(organization_id,name,url,interval_seconds,expected_status,keyword,component_id,policy_id,enabled)
SELECT c.organization_id,sqlc.arg(name),sqlc.arg(url),sqlc.arg(interval_seconds),sqlc.arg(expected_status),sqlc.arg(keyword),c.id,p.id,sqlc.arg(enabled)
FROM status_components c LEFT JOIN escalation_policies p ON p.organization_id=c.organization_id AND p.uuid=sqlc.narg(policy_uuid)::uuid
WHERE c.organization_id=sqlc.arg(organization_id) AND c.uuid=sqlc.arg(component_uuid) RETURNING *;
-- name: OncallUpdateMonitor :one
UPDATE uptime_monitors m SET name=sqlc.arg(name),url=sqlc.arg(url),interval_seconds=sqlc.arg(interval_seconds),expected_status=sqlc.arg(expected_status),keyword=sqlc.arg(keyword),enabled=sqlc.arg(enabled),component_id=c.id,policy_id=p.id
FROM status_components c LEFT JOIN escalation_policies p ON p.organization_id=c.organization_id AND p.uuid=sqlc.narg(policy_uuid)::uuid
WHERE m.organization_id=sqlc.arg(organization_id) AND m.uuid=sqlc.arg(uuid) AND c.organization_id=m.organization_id AND c.uuid=sqlc.arg(component_uuid) AND m.incident_id IS NULL RETURNING m.*;
-- name: OncallMonitors :many
SELECT m.uuid,m.name,m.url,m.interval_seconds,m.expected_status,m.keyword,m.enabled,m.next_check_at,m.last_checked_at,m.last_up,m.last_status,c.uuid AS component_uuid,p.uuid AS policy_uuid
FROM uptime_monitors m JOIN status_components c ON c.organization_id=m.organization_id AND c.id=m.component_id
LEFT JOIN escalation_policies p ON p.organization_id=m.organization_id AND p.id=m.policy_id
WHERE m.organization_id=$1 ORDER BY m.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: OncallMonitorCount :one
SELECT count(*) FROM uptime_monitors WHERE organization_id=$1;
-- name: OncallDeleteMonitor :execrows
DELETE FROM uptime_monitors WHERE organization_id=$1 AND uuid=$2;
-- name: OncallMonitorDue :many
SELECT m.organization_id,m.id FROM uptime_monitors m WHERE m.enabled AND m.next_check_at<=now() ORDER BY m.next_check_at LIMIT 20;
-- name: OncallLockMonitor :one
SELECT * FROM uptime_monitors WHERE organization_id=$1 AND id=$2 AND enabled AND next_check_at<=now() FOR UPDATE SKIP LOCKED;
-- name: OncallFinishMonitor :exec
UPDATE uptime_monitors SET next_check_at=now()+interval_seconds*interval '1 second',last_checked_at=now(),last_up=$3,last_status=$4,incident_id=$5 WHERE organization_id=$1 AND id=$2;
-- name: OncallInsertResult :exec
INSERT INTO uptime_results(organization_id,monitor_id,up,status_code,latency_ms) VALUES($1,$2,$3,$4,$5);
-- name: OncallResults :many
SELECT r.uuid,r.up,r.status_code,r.latency_ms,r.checked_at FROM uptime_results r JOIN uptime_monitors m ON m.organization_id=r.organization_id AND m.id=r.monitor_id
WHERE r.organization_id=$1 AND m.uuid=$2 ORDER BY r.checked_at DESC,r.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: OncallResultCount :one
SELECT count(*) FROM uptime_results r JOIN uptime_monitors m ON m.organization_id=r.organization_id AND m.id=r.monitor_id WHERE r.organization_id=$1 AND m.uuid=$2;
-- name: OncallPurgeResults :exec
DELETE FROM uptime_results WHERE checked_at<now()-interval '30 days';
-- name: OncallOpenIncidentAlert :exec
INSERT INTO oncall_alerts(organization_id,policy_id,incident_id)
SELECT p.organization_id,p.id,i.id FROM escalation_policies p JOIN status_incidents i ON i.organization_id=p.organization_id
WHERE p.organization_id=$1 AND p.enabled AND p.id=$2 AND i.id=$3 AND i.status<>'resolved'
ON CONFLICT(organization_id,policy_id,incident_id) DO NOTHING;
-- name: OncallOpenSourceAlerts :exec
INSERT INTO oncall_alerts(organization_id,policy_id,incident_id,conversation_id)
SELECT p.organization_id,p.id,i.id,c.id FROM escalation_policies p
LEFT JOIN status_incidents i ON i.organization_id=p.organization_id AND i.id=sqlc.narg(incident_id)::bigint AND i.status<>'resolved'
LEFT JOIN conversations c ON c.organization_id=p.organization_id AND c.id=sqlc.narg(conversation_id)::bigint AND c.priority='urgent' AND c.status NOT IN ('resolved','closed') AND c.deleted_at IS NULL
WHERE p.organization_id=sqlc.arg(organization_id) AND p.enabled AND ((p.incidents AND i.id IS NOT NULL) OR (p.urgent_conversations AND c.id IS NOT NULL))
ON CONFLICT DO NOTHING;
-- name: OncallCloseSourceAlerts :exec
UPDATE oncall_alerts a SET resolved_at=now() WHERE a.organization_id=$1 AND a.resolved_at IS NULL AND (
 EXISTS(SELECT 1 FROM status_incidents i WHERE i.organization_id=a.organization_id AND i.id=a.incident_id AND i.status='resolved') OR
 EXISTS(SELECT 1 FROM conversations c WHERE c.organization_id=a.organization_id AND c.id=a.conversation_id AND (c.status IN ('resolved','closed') OR c.priority<>'urgent' OR c.deleted_at IS NOT NULL)));
-- name: OncallAlertsDue :many
SELECT a.organization_id,a.id FROM oncall_alerts a WHERE a.acknowledged_at IS NULL AND a.resolved_at IS NULL AND a.next_notify_at<=now() ORDER BY a.next_notify_at LIMIT 100;
-- name: OncallLockAlert :one
SELECT a.*,p.steps,p.enabled AS policy_enabled FROM oncall_alerts a JOIN escalation_policies p ON p.organization_id=a.organization_id AND p.id=a.policy_id
WHERE a.organization_id=$1 AND a.id=$2 AND a.acknowledged_at IS NULL AND a.resolved_at IS NULL AND a.next_notify_at<=now() FOR UPDATE OF a SKIP LOCKED;
-- name: OncallAdvanceAlert :exec
UPDATE oncall_alerts SET next_step=next_step+1,next_notify_at=$3 WHERE organization_id=$1 AND id=$2;
-- name: OncallAcknowledge :execrows
UPDATE oncall_alerts SET acknowledged_at=now(),acknowledged_by=$3 WHERE organization_id=$1 AND uuid=$2 AND acknowledged_at IS NULL AND resolved_at IS NULL;
-- name: OncallAlerts :many
SELECT a.uuid,a.next_step,a.next_notify_at,a.acknowledged_at,a.resolved_at,a.created_at,p.uuid AS policy_uuid,i.uuid AS incident_uuid,c.uuid AS conversation_uuid
FROM oncall_alerts a JOIN escalation_policies p ON p.organization_id=a.organization_id AND p.id=a.policy_id
LEFT JOIN status_incidents i ON i.organization_id=a.organization_id AND i.id=a.incident_id
LEFT JOIN conversations c ON c.organization_id=a.organization_id AND c.id=a.conversation_id
WHERE a.organization_id=$1 ORDER BY a.id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: OncallAlertCount :one
SELECT count(*) FROM oncall_alerts WHERE organization_id=$1;
-- name: OncallAlertActive :one
SELECT a.uuid FROM oncall_alerts a JOIN escalation_policies p ON p.organization_id=a.organization_id AND p.id=a.policy_id
WHERE a.organization_id=$1 AND a.uuid=$2 AND p.enabled AND a.acknowledged_at IS NULL AND a.resolved_at IS NULL;
-- name: OncallLinkComponent :exec
INSERT INTO status_incident_components(organization_id,incident_id,component_id) VALUES($1,$2,$3);
-- name: OncallIncidentByID :one
SELECT * FROM status_incidents WHERE organization_id=$1 AND id=$2;
-- name: OncallSetNextAlert :exec
UPDATE oncall_alerts SET next_notify_at=$3 WHERE organization_id=$1 AND id=$2;
