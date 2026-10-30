-- name: TranscriptGet :one
SELECT t.* FROM recording_transcripts t
LEFT JOIN attachments a ON a.organization_id=t.organization_id AND a.uuid=t.attachment_uuid
LEFT JOIN video_recordings v ON v.organization_id=t.organization_id AND v.id=t.video_recording_id
WHERE t.organization_id=$1 AND t.uuid=$2
AND (t.attachment_uuid IS NULL OR (a.deleted_at IS NULL AND a.object_key<>'' AND (a.expires_at IS NULL OR a.expires_at>now())))
AND (t.video_recording_id IS NULL OR (v.status='complete' AND (v.expires_at IS NULL OR v.expires_at>now())));
-- name: TranscriptPending :many
SELECT organization_id,uuid FROM recording_transcripts WHERE status='pending' ORDER BY id LIMIT 100;
-- name: TranscriptClaim :one
UPDATE recording_transcripts SET status='processing' WHERE organization_id=$1 AND uuid=$2 AND status='pending' RETURNING *;
-- name: TranscriptFinish :execrows
UPDATE recording_transcripts SET status=$3,transcript=$4,segments=$5,summary=$6,summary_status=$7,error_code=$8
WHERE organization_id=$1 AND uuid=$2 AND status='processing';
-- name: TranscriptVoiceSource :one
SELECT COALESCE(NULLIF((m.content_attributes->>'recording_duration_seconds')::bigint,0),c.duration_seconds)::bigint AS duration_seconds,a.uuid,a.filename FROM channel_voice_calls c JOIN recording_transcripts t ON t.organization_id=c.organization_id AND t.voice_call_id=c.id
JOIN messages m ON m.organization_id=t.organization_id AND m.id=t.source_message_id
JOIN attachments a ON a.organization_id=t.organization_id AND a.uuid=t.attachment_uuid
WHERE c.organization_id=$1 AND t.uuid=$2 AND a.deleted_at IS NULL AND a.object_key<>'' AND (a.expires_at IS NULL OR a.expires_at>now());
-- name: TranscriptVideoSource :one
SELECT v.uuid,v.duration_seconds FROM video_recordings v JOIN recording_transcripts t ON t.organization_id=v.organization_id AND t.video_recording_id=v.id
WHERE v.organization_id=$1 AND t.uuid=$2 AND v.status='complete' AND (v.expires_at IS NULL OR v.expires_at>now());
-- name: TranscriptSyncVoice :exec
UPDATE channel_voice_calls c SET transcript_status=t.status FROM recording_transcripts t
WHERE c.organization_id=$1 AND t.organization_id=c.organization_id AND t.uuid=$2 AND t.voice_call_id=c.id;
-- name: TranscriptExpire :exec
DELETE FROM recording_transcripts t WHERE t.organization_id IS NOT NULL AND (
EXISTS(SELECT 1 FROM attachments a WHERE a.organization_id=t.organization_id AND a.uuid=t.attachment_uuid AND (a.deleted_at IS NOT NULL OR a.object_key='' OR a.expires_at<=now()))
OR EXISTS(SELECT 1 FROM video_recordings v WHERE v.organization_id=t.organization_id AND v.id=t.video_recording_id AND (v.status='deleted' OR v.expires_at<=now())));
-- name: TranscriptStale :exec
UPDATE recording_transcripts SET status='failed',summary_status=CASE WHEN summary_status='processing' THEN 'failed' ELSE summary_status END,error_code='TRANSCRIPTION_INTERRUPTED'
WHERE organization_id IS NOT NULL AND status='processing' AND updated_at<now()-interval '15 minutes';
-- name: TranscriptConversation :many
SELECT t.* FROM recording_transcripts t
LEFT JOIN attachments a ON a.organization_id=t.organization_id AND a.uuid=t.attachment_uuid
LEFT JOIN video_recordings v ON v.organization_id=t.organization_id AND v.id=t.video_recording_id
WHERE t.organization_id=$1 AND t.conversation_id=$2
AND (t.attachment_uuid IS NULL OR (a.deleted_at IS NULL AND a.object_key<>'' AND (a.expires_at IS NULL OR a.expires_at>now())))
AND (t.video_recording_id IS NULL OR (v.status='complete' AND (v.expires_at IS NULL OR v.expires_at>now())))
ORDER BY t.id DESC LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);
-- name: TranscriptByVideo :one
SELECT t.uuid FROM recording_transcripts t JOIN video_recordings v ON v.organization_id=t.organization_id AND v.id=t.video_recording_id
WHERE t.organization_id=$1 AND v.uuid=$2;

-- name: TranscriptConversationCount :one
SELECT count(*) FROM recording_transcripts t
LEFT JOIN attachments a ON a.organization_id=t.organization_id AND a.uuid=t.attachment_uuid
LEFT JOIN video_recordings v ON v.organization_id=t.organization_id AND v.id=t.video_recording_id
WHERE t.organization_id=$1 AND t.conversation_id=$2
AND (t.attachment_uuid IS NULL OR (a.deleted_at IS NULL AND a.object_key<>'' AND (a.expires_at IS NULL OR a.expires_at>now())))
AND (t.video_recording_id IS NULL OR (v.status='complete' AND (v.expires_at IS NULL OR v.expires_at>now())))
;
