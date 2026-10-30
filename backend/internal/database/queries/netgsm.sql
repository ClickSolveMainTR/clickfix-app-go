-- name: GetMemberSIP :one
SELECT m.sip_extension,m.sip_username,m.sip_credentials_encrypted,u.uuid AS user_uuid,u.id AS user_id
FROM organization_members m JOIN users u ON u.id=m.user_id AND u.deleted_at IS NULL AND u.status='active'
WHERE m.organization_id=sqlc.arg(organization_id) AND u.uuid=sqlc.arg(user_uuid);

-- name: SetMemberSIP :execrows
UPDATE organization_members SET sip_extension=sqlc.arg(sip_extension),sip_username=sqlc.arg(sip_username),
 sip_credentials_encrypted=CASE WHEN sqlc.arg(clear_password)::boolean THEN '' ELSE COALESCE(NULLIF(sqlc.arg(ciphertext)::text,''),sip_credentials_encrypted) END
WHERE organization_id=sqlc.arg(organization_id) AND user_id=(SELECT id FROM users WHERE uuid=sqlc.arg(user_uuid) AND deleted_at IS NULL);

-- name: ClaimMissedNetgsmCall :many
WITH claimed AS (
 UPDATE channel_voice_calls SET missed_notified_at=now() WHERE organization_id=sqlc.arg(organization_id)
 AND channel_instance_id=sqlc.arg(channel_instance_id) AND external_id=sqlc.arg(external_id)
 AND direction='inbound' AND status IN ('no-answer','busy','failed') AND provider_final=true AND missed_notified_at IS NULL
 RETURNING agent_user_id
)
SELECT DISTINCT m.user_id FROM claimed c JOIN organization_members m ON m.organization_id=sqlc.arg(organization_id)
JOIN roles r ON r.id=m.role_id WHERE m.user_id=c.agent_user_id OR r.slug IN ('owner','admin');

-- name: MergeNetgsmProviderCall :exec
UPDATE channel_voice_calls SET external_id=sqlc.arg(external_id)
WHERE id=(SELECT c.id FROM channel_voice_calls c JOIN organization_members m ON m.organization_id=c.organization_id AND m.user_id=c.agent_user_id
 WHERE c.organization_id=sqlc.arg(organization_id) AND c.channel_instance_id=sqlc.arg(channel_instance_id)
 AND c.external_id LIKE 'client:%' AND m.sip_extension=sqlc.arg(internal_number)::text AND m.sip_extension<>''
 AND c.direction=sqlc.arg(direction)::text
 AND right(regexp_replace(CASE WHEN c.direction='outbound' THEN c.to_number ELSE c.from_number END,'[^0-9]','','g'),10)=sqlc.arg(phone)::text
 AND abs(extract(epoch from (c.initiated_at-sqlc.narg(initiated_at)::timestamptz)))<=120
 AND NOT EXISTS(SELECT 1 FROM channel_voice_calls p WHERE p.organization_id=c.organization_id AND p.channel_instance_id=c.channel_instance_id AND p.external_id=sqlc.arg(external_id)::text)
 ORDER BY abs(extract(epoch from(c.initiated_at-sqlc.narg(initiated_at)::timestamptz))),c.id LIMIT 1);

-- name: UpsertWebphoneEvent :exec
INSERT INTO channel_voice_calls(organization_id,channel_instance_id,external_id,provider,direction,status,from_number,to_number,internal_number,agent_user_id,duration_seconds,last_event_at,initiated_at,ended_at,client_event_id)
SELECT sqlc.arg(organization_id),sqlc.arg(channel_instance_id),
 COALESCE((SELECT c.external_id FROM channel_voice_calls c WHERE c.organization_id=sqlc.arg(organization_id) AND c.channel_instance_id=sqlc.arg(channel_instance_id)
 AND c.agent_user_id=sqlc.arg(user_id) AND c.direction=sqlc.arg(direction)::text
 AND right(regexp_replace(CASE WHEN c.direction='outbound' THEN c.to_number ELSE c.from_number END,'[^0-9]','','g'),10)=sqlc.arg(phone)::text
 AND (c.client_event_id IS NULL OR c.client_event_id=sqlc.arg(event_id)::uuid)
 AND abs(extract(epoch from(c.initiated_at-sqlc.arg(started_at)::timestamptz)))<=120
 ORDER BY abs(extract(epoch from(c.initiated_at-sqlc.arg(started_at)::timestamptz))),c.id LIMIT 1),'client:'||sqlc.arg(event_id)::text),
 'netgsm',sqlc.arg(direction),sqlc.arg(status),sqlc.arg(from_number),sqlc.arg(to_number),sqlc.arg(internal_number),sqlc.arg(user_id),sqlc.arg(duration_seconds),sqlc.arg(ended_at),sqlc.arg(started_at),sqlc.arg(ended_at),sqlc.arg(event_id)
ON CONFLICT(organization_id,channel_instance_id,external_id) DO UPDATE SET client_event_id=EXCLUDED.client_event_id;

-- name: LockNetgsmInstance :exec
SELECT pg_advisory_xact_lock(hashtextextended('netgsm:'||sqlc.arg(organization_id)::bigint::text||':'||sqlc.arg(channel_instance_id)::bigint::text,0));

-- name: ConsolidateNetgsmClientCall :exec
WITH candidate AS MATERIALIZED (
 SELECT c.id FROM channel_voice_calls c JOIN organization_members m ON m.organization_id=c.organization_id AND m.user_id=c.agent_user_id
 WHERE c.organization_id=sqlc.arg(organization_id) AND c.channel_instance_id=sqlc.arg(channel_instance_id)
 AND c.external_id LIKE 'client:%' AND m.sip_extension=sqlc.arg(internal_number)::text AND m.sip_extension<>''
 AND c.direction=sqlc.arg(direction)::text
 AND right(regexp_replace(CASE WHEN c.direction='outbound' THEN c.to_number ELSE c.from_number END,'[^0-9]','','g'),10)=sqlc.arg(phone)::text
 AND abs(extract(epoch from(c.initiated_at-sqlc.narg(initiated_at)::timestamptz)))<=120
 AND EXISTS(SELECT 1 FROM channel_voice_calls p WHERE p.organization_id=c.organization_id AND p.channel_instance_id=c.channel_instance_id AND p.external_id=sqlc.arg(external_id)::text AND p.client_event_id IS NULL)
 ORDER BY abs(extract(epoch from(c.initiated_at-sqlc.narg(initiated_at)::timestamptz))),c.id LIMIT 1
), removed AS (
 DELETE FROM channel_voice_calls WHERE id=(SELECT id FROM candidate) RETURNING client_event_id,agent_user_id,initiated_at
)
UPDATE channel_voice_calls target SET client_event_id=r.client_event_id,agent_user_id=COALESCE(target.agent_user_id,r.agent_user_id),initiated_at=COALESCE(target.initiated_at,r.initiated_at)
FROM removed r WHERE target.organization_id=sqlc.arg(organization_id) AND target.channel_instance_id=sqlc.arg(channel_instance_id) AND target.external_id=sqlc.arg(external_id)::text;
