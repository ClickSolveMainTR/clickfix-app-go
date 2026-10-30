-- name: EngineeringCreateProject :one
INSERT INTO engineering_projects(organization_id,name) VALUES($1,$2) RETURNING *;
-- name: EngineeringListProjects :many
SELECT * FROM engineering_projects WHERE organization_id=$1 ORDER BY name;
-- name: EngineeringCreateRelease :one
INSERT INTO engineering_releases(organization_id,version,changelog_url) VALUES($1,$2,$3) RETURNING *;
-- name: EngineeringListReleases :many
SELECT * FROM engineering_releases WHERE organization_id=$1 ORDER BY id DESC LIMIT 100;
-- name: EngineeringRelease :one
SELECT * FROM engineering_releases WHERE organization_id=$1 AND uuid=$2;
-- name: EngineeringPublishRelease :one
UPDATE engineering_releases SET released_at=COALESCE(released_at,now()) WHERE organization_id=$1 AND uuid=$2 RETURNING *;
-- name: EngineeringCreateIssue :one
INSERT INTO engineering_issues(organization_id,title,description,status,priority,labels,cycle,project_id,assignee_user_id,created_by,context,resolved_at)
VALUES(sqlc.arg(organization_id),sqlc.arg(title),sqlc.arg(description),sqlc.arg(status),sqlc.arg(priority),sqlc.arg(labels),sqlc.arg(cycle),sqlc.narg(project_id),sqlc.narg(assignee_user_id),sqlc.narg(created_by),sqlc.arg(context),CASE WHEN sqlc.arg(status)::text='done' THEN now() ELSE NULL END) RETURNING *;
-- name: EngineeringIssue :one
SELECT * FROM engineering_issues WHERE organization_id=$1 AND uuid=$2;
-- name: EngineeringLockIssue :one
SELECT * FROM engineering_issues WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: EngineeringListIssues :many
SELECT * FROM engineering_issues WHERE organization_id=sqlc.arg(organization_id)
 AND (sqlc.arg(status)::text='' OR status=sqlc.arg(status))
 AND (sqlc.arg(priority)::text='' OR priority=sqlc.arg(priority))
 AND (sqlc.arg(q)::text='' OR title ILIKE '%'||sqlc.arg(q)||'%' OR description ILIKE '%'||sqlc.arg(q)||'%')
ORDER BY updated_at DESC,id DESC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
-- name: EngineeringCountIssues :one
SELECT count(*) FROM engineering_issues WHERE organization_id=sqlc.arg(organization_id)
 AND (sqlc.arg(status)::text='' OR status=sqlc.arg(status))
 AND (sqlc.arg(priority)::text='' OR priority=sqlc.arg(priority))
 AND (sqlc.arg(q)::text='' OR title ILIKE '%'||sqlc.arg(q)||'%' OR description ILIKE '%'||sqlc.arg(q)||'%');
-- name: EngineeringUpdateIssue :one
UPDATE engineering_issues SET title=sqlc.arg(title), description=sqlc.arg(description), status=sqlc.arg(status),priority=sqlc.arg(priority), labels=sqlc.arg(labels), cycle=sqlc.arg(cycle), project_id=sqlc.narg(project_id),assignee_user_id=sqlc.narg(assignee_user_id),release_id=sqlc.narg(release_id), revision=revision+1,
resolved_at=CASE WHEN sqlc.arg(status)::text='done' THEN COALESCE(resolved_at,now()) ELSE NULL END
WHERE organization_id=sqlc.arg(organization_id) AND id=sqlc.arg(id) AND revision=sqlc.arg(expected_revision) RETURNING *;
-- name: EngineeringAssignee :one
SELECT u.id FROM users u JOIN organization_members m ON m.user_id=u.id WHERE m.organization_id=$1 AND u.uuid=$2 AND u.status='active';
-- name: EngineeringProject :one
SELECT * FROM engineering_projects WHERE organization_id=$1 AND uuid=$2;
-- name: EngineeringLink :one
INSERT INTO engineering_conversations(organization_id,issue_id,conversation_id,notify_customer) VALUES($1,$2,$3,$4)
ON CONFLICT(organization_id,issue_id,conversation_id) DO UPDATE SET notify_customer=EXCLUDED.notify_customer RETURNING *;
-- name: EngineeringUnlink :execrows
DELETE FROM engineering_conversations WHERE organization_id=$1 AND issue_id=$2 AND conversation_id=$3;
-- name: EngineeringLinks :many
SELECT ec.*,c.uuid AS conversation_uuid,c.subject,c.channel_type,c.contact_id,c.company_id,ct.custom_fields AS contact_fields,ct.blocked AS contact_blocked,c.channel_instance_id
FROM engineering_conversations ec JOIN conversations c ON c.organization_id=ec.organization_id AND c.id=ec.conversation_id
JOIN contacts ct ON ct.organization_id=c.organization_id AND ct.id=c.contact_id
WHERE ec.organization_id=$1 AND ec.issue_id=$2 AND c.deleted_at IS NULL AND ct.deleted_at IS NULL ORDER BY ec.id;
-- name: EngineeringImpact :many
SELECT r.currency,sum(r.mrr_minor)::bigint AS mrr_minor,count(*)::bigint AS companies FROM company_revenue r
WHERE r.organization_id=$1 AND r.company_id IN (
 SELECT DISTINCT c.company_id FROM engineering_conversations ec JOIN conversations c ON c.organization_id=ec.organization_id AND c.id=ec.conversation_id
 WHERE ec.organization_id=$1 AND ec.issue_id=$2 AND c.deleted_at IS NULL
) GROUP BY r.currency ORDER BY r.currency;
-- name: EngineeringAddComment :one
INSERT INTO engineering_comments(organization_id,issue_id,author_user_id,body,source,external_id) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(organization_id,issue_id,source,external_id) WHERE external_id<>'' DO UPDATE SET body=EXCLUDED.body RETURNING *;
-- name: EngineeringComments :many
SELECT * FROM engineering_comments WHERE organization_id=$1 AND issue_id=$2 ORDER BY id LIMIT 500;
-- name: EngineeringCreateDelivery :exec
INSERT INTO engineering_deliveries(organization_id,issue_id,conversation_id,release_id,generation,content,actor_user_id,resolution_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(organization_id,issue_id,conversation_id,generation) DO NOTHING;
-- name: EngineeringClaimDeliveries :many
WITH picked AS (SELECT id FROM engineering_deliveries WHERE status='pending' AND available_at<=now() ORDER BY id LIMIT 50 FOR UPDATE SKIP LOCKED)
UPDATE engineering_deliveries d SET available_at=now()+interval '5 minutes',attempts=attempts+1 FROM picked WHERE d.id=picked.id RETURNING d.*;
-- name: EngineeringFinishDelivery :exec
UPDATE engineering_deliveries SET status=$3,message_uuid=$4,last_error=$5 WHERE organization_id=$1 AND id=$2;
-- name: EngineeringDeliveries :many
SELECT * FROM engineering_deliveries WHERE organization_id=$1 AND issue_id=$2 ORDER BY id DESC LIMIT 500;
-- name: EngineeringIntegrationSave :one
INSERT INTO engineering_integrations(organization_id,provider,credentials_enc,config,enabled) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(organization_id,provider) DO UPDATE SET credentials_enc=EXCLUDED.credentials_enc, config=EXCLUDED.config,enabled=EXCLUDED.enabled RETURNING *;
-- name: EngineeringIntegration :one
SELECT * FROM engineering_integrations WHERE organization_id=$1 AND provider=$2;
-- name: EngineeringIntegrationByUUID :one
SELECT * FROM engineering_integrations WHERE uuid=$1 AND enabled=true;
-- name: EngineeringIntegrations :many
SELECT * FROM engineering_integrations WHERE organization_id=$1 ORDER BY provider;
-- name: EngineeringExternalSave :one
INSERT INTO engineering_external_issues(organization_id,integration_id,issue_id,external_id,external_key,external_url,synced_revision)
VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(organization_id,integration_id,issue_id) DO UPDATE SET external_id=EXCLUDED.external_id,external_key=EXCLUDED.external_key,external_url=EXCLUDED.external_url,synced_revision=EXCLUDED.synced_revision,last_error='' RETURNING *;
-- name: EngineeringExternalIssue :one
SELECT * FROM engineering_external_issues WHERE organization_id=$1 AND integration_id=$2 AND issue_id=$3;
-- name: EngineeringExternalByID :one
SELECT ei.*,i.uuid AS issue_uuid FROM engineering_external_issues ei JOIN engineering_issues i ON i.organization_id=ei.organization_id AND i.id=ei.issue_id
WHERE ei.organization_id=$1 AND ei.integration_id=$2 AND ei.external_id=$3;
-- name: EngineeringIssueExternals :many
SELECT * FROM engineering_external_issues WHERE organization_id=$1 AND issue_id=$2;
-- name: EngineeringReceipt :execrows
INSERT INTO engineering_webhook_receipts(organization_id,integration_id,delivery_key) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;
-- name: EngineeringOAuthState :exec
INSERT INTO engineering_oauth_states(organization_id,integration_id,user_id,state_hash,expires_at) VALUES($1,$2,$3,$4,$5);
-- name: EngineeringConsumeState :one
DELETE FROM engineering_oauth_states WHERE organization_id=$1 AND integration_id=$2 AND user_id=$3 AND state_hash=$4 AND expires_at>now() RETURNING id;
-- name: EngineeringCreateEscalation :one
INSERT INTO engineering_escalations(organization_id,conversation_id,tier,team_id,reason,due_at,actor_user_id) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: EngineeringEscalations :many
SELECT * FROM engineering_escalations WHERE organization_id=$1 AND conversation_id=$2 ORDER BY id DESC LIMIT 100;
-- name: EngineeringReleaseIssues :many
SELECT * FROM engineering_issues WHERE organization_id=$1 AND release_id=$2 ORDER BY id FOR UPDATE;
-- name: EngineeringIssueRefs :one
SELECT p.uuid AS project_uuid,r.uuid AS release_uuid,u.uuid AS assignee_uuid
FROM engineering_issues i
LEFT JOIN engineering_projects p ON p.organization_id=i.organization_id AND p.id=i.project_id
LEFT JOIN engineering_releases r ON r.organization_id=i.organization_id AND r.id=i.release_id
LEFT JOIN users u ON u.id=i.assignee_user_id
WHERE i.organization_id=$1 AND i.id=$2;
-- name: EngineeringLinkCurrent :one
SELECT notify_customer FROM engineering_conversations WHERE organization_id=$1 AND issue_id=$2 AND conversation_id=$3;
-- name: EngineeringHandoffConversation :exec
UPDATE conversations SET assignee_team_id=$3, assignee_user_id=NULL, status='open',status_id=NULL,resolved_at=NULL,closed_at=NULL,snoozed_until=NULL,resolution_due_at=$4,sla_breached_at=NULL
WHERE organization_id=$1 AND id=$2;
-- name: EngineeringDeliveryAllowed :one
SELECT EXISTS(SELECT 1 FROM engineering_deliveries d
JOIN engineering_conversations ec ON ec.organization_id=d.organization_id AND ec.issue_id=d.issue_id AND ec.conversation_id=d.conversation_id
JOIN conversations c ON c.organization_id=d.organization_id AND c.id=d.conversation_id
JOIN contacts ct ON ct.organization_id=c.organization_id AND ct.id=c.contact_id
JOIN engineering_issues i ON i.organization_id=d.organization_id AND i.id=d.issue_id
LEFT JOIN engineering_releases er ON er.organization_id=d.organization_id AND er.id=d.release_id
WHERE d.organization_id=$1 AND d.uuid=$2 AND ec.notify_customer=true AND c.deleted_at IS NULL AND ct.deleted_at IS NULL
AND ((i.status='done' AND d.resolution_at=i.resolved_at) OR er.released_at IS NOT NULL)
AND ct.blocked=false
AND COALESCE(ct.custom_fields->>'engineering_notifications','true') <> 'false'
AND COALESCE(ct.custom_fields->>'notifications_opt_out','false') <> 'true');
-- name: EngineeringLockIntegration :one
SELECT * FROM engineering_integrations WHERE organization_id=$1 AND provider=$2 FOR UPDATE;
-- name: EngineeringExternalError :exec
UPDATE engineering_external_issues SET last_error=$3 WHERE organization_id=$1 AND id=$2;
-- name: EngineeringCommentSyncMark :exec
INSERT INTO engineering_comment_sync(organization_id,comment_id,integration_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;
-- name: EngineeringCommentsToSync :many
SELECT c.* FROM engineering_comments c WHERE c.organization_id=$1 AND c.issue_id=$2 AND c.source='native'
AND NOT EXISTS(SELECT 1 FROM engineering_comment_sync cs WHERE cs.organization_id=c.organization_id AND cs.comment_id=c.id AND cs.integration_id=$3)
ORDER BY c.id LIMIT 100;
-- name: EngineeringAllExternal :many
-- Platform sweep; rows carry organization_id for every subsequent scoped call.
SELECT ei.*,i.uuid AS issue_uuid,intg.provider FROM engineering_external_issues ei
JOIN engineering_issues i ON i.organization_id=ei.organization_id AND i.id=ei.issue_id
JOIN engineering_integrations intg ON intg.organization_id=ei.organization_id AND intg.id=ei.integration_id
WHERE intg.enabled=true AND intg.provider<>'sentry' ORDER BY ei.updated_at LIMIT 100;
-- name: EngineeringWebhookSyncRevision :exec
UPDATE engineering_external_issues SET synced_revision=$4 WHERE organization_id=$1 AND integration_id=$2 AND issue_id=$3;

-- name: EngineeringCompanyCount :one
SELECT count(DISTINCT c.company_id) FROM engineering_conversations ec JOIN conversations c ON c.organization_id=ec.organization_id AND c.id=ec.conversation_id
WHERE ec.organization_id=$1 AND ec.issue_id=$2 AND c.deleted_at IS NULL;
-- name: EngineeringLatestEscalation :one
SELECT * FROM engineering_escalations WHERE organization_id=$1 AND conversation_id=$2 ORDER BY id DESC LIMIT 1;

-- name: EngineeringAttachError :one
INSERT INTO engineering_error_events(organization_id,issue_id,conversation_id,sentry_issue_id,event_id,event_url,summary) VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(organization_id,conversation_id,event_id) DO UPDATE SET summary=EXCLUDED.summary,event_url=EXCLUDED.event_url WHERE engineering_error_events.issue_id=EXCLUDED.issue_id RETURNING *;
-- name: EngineeringErrorEvents :many
SELECT e.*,i.uuid AS issue_uuid FROM engineering_error_events e JOIN engineering_issues i ON i.organization_id=e.organization_id AND i.id=e.issue_id
WHERE e.organization_id=$1 AND e.conversation_id=$2 ORDER BY e.id DESC LIMIT 100;

-- name: EngineeringPurgeOAuthStates :exec
DELETE FROM engineering_oauth_states WHERE expires_at < now();
