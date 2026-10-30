-- name: UpsertChannelVoiceCall :exec
INSERT INTO channel_voice_calls (
 organization_id, channel_instance_id, external_id, provider, direction, status,
 from_number, to_number, duration_seconds, recording_message_id, last_event_at, internal_number, recording_url, agent_user_id, answered_at, ended_at, initiated_at, provider_final
) VALUES (
 sqlc.arg(organization_id), sqlc.arg(channel_instance_id), sqlc.arg(external_id), sqlc.arg(provider),
 sqlc.arg(direction), sqlc.arg(status), sqlc.arg(from_number), sqlc.arg(to_number), sqlc.arg(duration_seconds),
 (SELECT id FROM messages WHERE organization_id = sqlc.arg(organization_id)
   AND channel_instance_id = sqlc.arg(channel_instance_id) AND external_id = sqlc.arg(recording_external_id)::text),
 sqlc.arg(last_event_at), sqlc.arg(internal_number), sqlc.arg(recording_url),
 (SELECT user_id FROM organization_members WHERE organization_id=sqlc.arg(organization_id) AND sip_extension=sqlc.arg(internal_number)::text AND sip_extension<>'' LIMIT 1),
 sqlc.narg(answered_at), sqlc.narg(ended_at), sqlc.narg(initiated_at),sqlc.arg(provider_final)
)
ON CONFLICT (organization_id, channel_instance_id, external_id) DO UPDATE SET
 internal_number=COALESCE(NULLIF(EXCLUDED.internal_number,''),channel_voice_calls.internal_number),
 recording_url=COALESCE(NULLIF(EXCLUDED.recording_url,''),channel_voice_calls.recording_url),
 agent_user_id=COALESCE(EXCLUDED.agent_user_id,channel_voice_calls.agent_user_id),
 answered_at=COALESCE(channel_voice_calls.answered_at,EXCLUDED.answered_at),
 ended_at=COALESCE(EXCLUDED.ended_at,channel_voice_calls.ended_at),
 from_number=COALESCE(NULLIF(EXCLUDED.from_number,''),channel_voice_calls.from_number),
 to_number=COALESCE(NULLIF(EXCLUDED.to_number,''),channel_voice_calls.to_number),
 initiated_at=COALESCE(channel_voice_calls.initiated_at,EXCLUDED.initiated_at),
 provider_final=channel_voice_calls.provider_final OR EXCLUDED.provider_final,
 direction=CASE WHEN channel_voice_calls.provider_final AND (NOT EXCLUDED.provider_final OR sqlc.arg(is_report)::boolean) THEN channel_voice_calls.direction ELSE EXCLUDED.direction END,
 status = CASE WHEN EXCLUDED.status='recording' OR (channel_voice_calls.provider_final AND sqlc.arg(is_report)::boolean) THEN channel_voice_calls.status WHEN EXCLUDED.provider_final THEN EXCLUDED.status WHEN channel_voice_calls.provider_final THEN channel_voice_calls.status WHEN channel_voice_calls.status='in-progress' AND EXCLUDED.status IN ('queued','ringing') THEN channel_voice_calls.status WHEN channel_voice_calls.status='queued' AND EXCLUDED.status='ringing' THEN channel_voice_calls.status WHEN channel_voice_calls.status IN ('completed', 'failed', 'busy', 'no-answer', 'canceled')
     OR EXCLUDED.status = 'recording' THEN channel_voice_calls.status ELSE EXCLUDED.status END,
 duration_seconds = GREATEST(channel_voice_calls.duration_seconds, EXCLUDED.duration_seconds),
 recording_message_id = COALESCE(EXCLUDED.recording_message_id, channel_voice_calls.recording_message_id),
 last_event_at = GREATEST(channel_voice_calls.last_event_at, EXCLUDED.last_event_at);

-- name: ListChannelVoiceCalls :many
SELECT c.uuid, c.external_id, c.provider, c.direction, c.status, c.from_number, c.to_number,
 c.duration_seconds, c.internal_number, c.answered_at, c.ended_at, c.initiated_at, u.uuid AS agent_user_uuid, m.uuid AS recording_message_uuid, ''::text AS transcript,
 COALESCE(t.status,'unavailable')::text AS transcript_status,t.uuid AS transcript_uuid,
 c.last_event_at, c.created_at, c.updated_at
FROM channel_voice_calls c
LEFT JOIN users u ON u.id=c.agent_user_id AND u.deleted_at IS NULL
LEFT JOIN messages m ON m.organization_id = c.organization_id AND m.id = c.recording_message_id
LEFT JOIN recording_transcripts t ON t.organization_id=c.organization_id AND t.voice_call_id=c.id
 AND EXISTS(SELECT 1 FROM attachments a WHERE a.organization_id=t.organization_id AND a.uuid=t.attachment_uuid AND a.deleted_at IS NULL AND a.object_key<>'' AND (a.expires_at IS NULL OR a.expires_at>now()))
WHERE c.organization_id = sqlc.arg(organization_id) AND c.channel_instance_id = sqlc.arg(channel_instance_id)
ORDER BY c.created_at DESC, c.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountChannelVoiceCalls :one
SELECT count(*) FROM channel_voice_calls
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id);

-- name: GetChannelVoiceCall :one
SELECT * FROM channel_voice_calls
WHERE organization_id = sqlc.arg(organization_id) AND channel_instance_id = sqlc.arg(channel_instance_id)
 AND external_id = sqlc.arg(external_id);

-- name: ListPendingNetgsmRecordings :many
SELECT * FROM channel_voice_calls WHERE organization_id=sqlc.arg(organization_id) AND channel_instance_id=sqlc.arg(channel_instance_id)
 AND provider='netgsm' AND recording_url<>'' AND recording_message_id IS NULL ORDER BY id LIMIT 50;

-- name: ListPendingNetgsmSMS :many
SELECT external_id::text FROM messages WHERE organization_id=sqlc.arg(organization_id) AND channel_instance_id=sqlc.arg(channel_instance_id)
 AND direction='out' AND status='sent' AND external_id IS NOT NULL AND created_at > now()-interval '30 days' ORDER BY id LIMIT 100;
