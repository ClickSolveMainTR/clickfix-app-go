-- name: CompliancePolicy :one
SELECT * FROM retention_policies WHERE organization_id=$1;
-- name: ComplianceSavePolicy :one
INSERT INTO retention_policies(organization_id,conversation_days,attachment_days,ai_log_days,action)
VALUES($1,$2,$3,$4,$5) ON CONFLICT(organization_id) DO UPDATE SET
conversation_days=excluded.conversation_days,attachment_days=excluded.attachment_days,
ai_log_days=excluded.ai_log_days,action=excluded.action RETURNING *;
-- name: CompliancePolicies :many
SELECT * FROM retention_policies ORDER BY organization_id;
-- name: ComplianceContact :one
SELECT * FROM contacts WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: ComplianceRequest :one
INSERT INTO data_subject_requests(organization_id,contact_id,requested_by,kind,request_id)
VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: ComplianceGetRequest :one
SELECT * FROM data_subject_requests WHERE organization_id=$1 AND uuid=$2;
-- name: ComplianceListRequests :many
SELECT r.*,c.uuid AS contact_uuid,c.name AS contact_name,c.email AS contact_email,c.erased_at AS contact_erased_at,
COALESCE(u.name,'')::text AS requester_name,COALESCE(u.surname,'')::text AS requester_surname
FROM data_subject_requests r JOIN contacts c ON c.organization_id=r.organization_id AND c.id=r.contact_id
LEFT JOIN users u ON u.id=r.requested_by
WHERE r.organization_id=$1 ORDER BY r.created_at DESC,r.id DESC LIMIT $2 OFFSET $3;
-- name: ComplianceRequestCount :one
SELECT count(*) FROM data_subject_requests WHERE organization_id=$1;
-- name: ComplianceLockRequest :one
SELECT * FROM data_subject_requests WHERE organization_id=$1 AND uuid=$2 FOR UPDATE;
-- name: ComplianceClaimRequest :exec
UPDATE data_subject_requests SET status='processing' WHERE organization_id=$1 AND uuid=$2;
-- name: ComplianceCompleteRequest :exec
UPDATE data_subject_requests SET status='completed',object_key=$3,completed_at=now(),expires_at=now()+interval '7 days'
WHERE organization_id=$1 AND uuid=$2;
-- name: CompliancePendingRequests :many
SELECT organization_id,uuid,request_id FROM data_subject_requests WHERE status<>'completed' ORDER BY id LIMIT 200;
-- name: ComplianceExpiredExports :many
SELECT * FROM data_subject_requests WHERE object_key<>'' AND expires_at < now() ORDER BY id LIMIT 200;
-- name: ComplianceClearExport :exec
UPDATE data_subject_requests SET object_key='' WHERE organization_id=$1 AND uuid=$2;
-- name: ComplianceConversations :many
SELECT * FROM conversations WHERE organization_id=$1 AND contact_id=$2 ORDER BY id FOR UPDATE;
-- name: ComplianceMessages :many
SELECT m.* FROM messages m JOIN conversations c ON c.organization_id=m.organization_id AND c.id=m.conversation_id
WHERE m.organization_id=$1 AND c.contact_id=$2 ORDER BY m.id LIMIT $3 OFFSET $4;
-- name: ComplianceAIRuns :many
SELECT DISTINCT a.* FROM ai_runs a JOIN ai_agent_jobs j ON j.organization_id=a.organization_id AND j.run_uuid=a.uuid
JOIN conversations c ON c.organization_id=j.organization_id AND c.id=j.conversation_id
WHERE a.organization_id=$1 AND c.contact_id=$2
UNION SELECT a.* FROM ai_runs a JOIN conversations c ON c.organization_id=a.organization_id AND c.id=a.conversation_id WHERE a.organization_id=$1 AND c.contact_id=$2 ORDER BY id;
-- name: ComplianceAttachments :many
SELECT a.* FROM attachments a WHERE a.organization_id=$1 AND (
(a.owner_type IN ('conversation','ticket') AND a.owner_id IN(SELECT c.uuid FROM conversations c WHERE c.organization_id=$1 AND c.contact_id=$2))
OR (a.owner_type='message' AND a.owner_id IN(SELECT m.uuid FROM messages m JOIN conversations c ON c.organization_id=m.organization_id AND c.id=m.conversation_id WHERE m.organization_id=$1 AND c.contact_id=$2))) ORDER BY a.id;
-- name: ComplianceConsents :many
SELECT * FROM contact_consents WHERE organization_id=$1 AND contact_id=$2 ORDER BY id DESC LIMIT $3 OFFSET $4;
-- name: ComplianceConsentCount :one
SELECT count(*) FROM contact_consents WHERE organization_id=$1 AND contact_id=$2;
-- name: ComplianceConsent :one
INSERT INTO contact_consents(organization_id,contact_id,purpose,granted,notice_version,source,recorded_by)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: ComplianceEraseContact :exec
UPDATE contacts SET name='Silinmiş kişi',email=NULL,phone=NULL,locale=NULL,timezone=NULL,notes='',custom_fields='{}',
job_title='',company_id=NULL,last_seen_at=NULL,blocked=true,deleted_at=now(),erased_at=now()
WHERE organization_id=$1 AND id=$2;
-- name: ComplianceEraseIdentities :exec
DELETE FROM contact_identities WHERE organization_id=$1 AND contact_id=$2;
-- name: ComplianceRetentionConversations :many
SELECT * FROM conversations WHERE organization_id=$1 AND status='closed' AND closed_at < now()-make_interval(days=>sqlc.arg(days)::int)
AND anonymized_at IS NULL ORDER BY id LIMIT 200 FOR UPDATE;
-- name: ComplianceScrubConversation :exec
UPDATE conversations SET subject='',custom_fields='{}',meta='{}',last_message_preview='',external_thread_id=NULL,company_id=NULL,contact_id=sqlc.arg(anonymous_contact_id),anonymized_at=now(),
deleted_at=CASE WHEN sqlc.arg(erase)::boolean THEN now() ELSE deleted_at END
WHERE organization_id=$1 AND id=$2;
-- name: ComplianceScrubMessages :exec
UPDATE messages SET content='',content_attributes='{}',sender_contact_id=NULL,external_id=NULL,error='',deleted_at=now(),
status=CASE WHEN status IN ('pending','queued') THEN 'failed' ELSE status END
WHERE organization_id=$1 AND conversation_id=$2;
-- name: ComplianceScrubAI :exec
UPDATE ai_runs a SET input='',output='',retrieved_chunks='[]',tool_calls='[]',pii_erased_at=now()
WHERE a.organization_id=$1 AND (a.conversation_id=$2 OR a.uuid IN(SELECT j.run_uuid FROM ai_agent_jobs j WHERE j.organization_id=$1 AND j.conversation_id=$2));
-- name: ComplianceScrubJobs :exec
UPDATE ai_agent_jobs SET result='{}',reason='',status=CASE WHEN status='running' THEN 'skipped' ELSE status END
WHERE organization_id=$1 AND conversation_id=$2;
-- name: ComplianceScrubTools :exec
UPDATE ai_tool_calls SET error_code='' WHERE organization_id=$1 AND conversation_id=$2;
-- name: ComplianceRetentionAI :exec
UPDATE ai_runs SET input='',output='',retrieved_chunks='[]',tool_calls='[]',pii_erased_at=now()
WHERE organization_id=$1 AND status<>'running' AND created_at<now()-make_interval(days=>sqlc.arg(days)::int);
-- name: ComplianceRetentionAttachments :many
SELECT * FROM attachments WHERE organization_id=$1 AND created_at<now()-make_interval(days=>sqlc.arg(days)::int)
AND object_key<>'' ORDER BY id LIMIT 200;
-- name: CompliancePurgeAttachment :exec
UPDATE attachments SET filename='erased',sha256='',object_key='',thumb_key=NULL,deleted_at=now(),scan_detail=NULL
WHERE organization_id=$1 AND id=$2;
-- name: ComplianceSubjectByID :one
SELECT * FROM contacts WHERE organization_id=$1 AND id=$2 FOR UPDATE;
-- name: ComplianceAllConsents :many
SELECT * FROM contact_consents WHERE organization_id=$1 AND contact_id=$2 ORDER BY id FOR UPDATE;
-- name: ComplianceConversationAttachments :many
SELECT a.* FROM attachments a WHERE a.organization_id=$1 AND (
(a.owner_type IN ('conversation','ticket') AND a.owner_id IN(SELECT c.uuid FROM conversations c WHERE c.organization_id=$1 AND c.id=$2))
OR (a.owner_type='message' AND a.owner_id IN(SELECT m.uuid FROM messages m WHERE m.organization_id=$1 AND m.conversation_id=$2))) ORDER BY a.id;
-- name: ComplianceScrubDerived :exec
WITH sides AS (DELETE FROM side_conversations WHERE organization_id=$1 AND conversation_id=$2),
ratings AS (UPDATE conversation_ratings SET comment='',contact_id=NULL WHERE organization_id=$1 AND conversation_id=$2),
knowledge AS (DELETE FROM knowledge_suggestions WHERE organization_id=$1 AND conversation_id=$2),
qa AS (UPDATE qa_reviews SET rubric='{}',scores='{}' WHERE organization_id=$1 AND conversation_id=$2),
threads AS (UPDATE internal_threads SET title='Silindi' WHERE organization_id=$1 AND conversation_id=$2),
internal AS (UPDATE internal_messages m SET body='Silindi' WHERE m.organization_id=$1 AND m.thread_id IN(SELECT t.id FROM internal_threads t WHERE t.organization_id=$1 AND t.conversation_id=$2))
UPDATE ai_tool_calls a SET error_code='' WHERE a.organization_id=$1 AND a.conversation_id=$2;
-- name: ComplianceSubjectExports :many
SELECT * FROM data_subject_requests WHERE organization_id=$1 AND contact_id=$2 AND object_key<>'';
-- name: ComplianceConversationPublicID :one
SELECT uuid FROM conversations WHERE organization_id=$1 AND id=$2;
-- name: ComplianceConversationMessageIDs :many
SELECT uuid FROM messages WHERE organization_id=$1 AND conversation_id=$2;
-- name: ComplianceAnonymousContact :one
INSERT INTO contacts(organization_id,name,blocked,erased_at,deleted_at) VALUES($1,'Anonim kişi',true,now(),now()) RETURNING id;
