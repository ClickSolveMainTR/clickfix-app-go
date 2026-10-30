-- Data-loss note: rollback removes transcripts and summaries.
DELETE FROM recording_transcripts;
DROP TRIGGER trg_conversation_transcript_erase ON conversations;
DROP TRIGGER trg_video_transcript_erase ON video_recordings;
DROP TRIGGER trg_attachment_transcript_erase ON attachments;
DROP TRIGGER trg_message_transcript_erase ON messages;
DROP TRIGGER trg_video_transcription ON video_recordings;
DROP TRIGGER trg_voice_transcription ON channel_voice_calls;
DROP TABLE recording_transcripts;
DROP FUNCTION erase_recording_transcript();
DROP FUNCTION recording_transcript_timeline();
DROP FUNCTION queue_recording_transcript();
ALTER TABLE channel_voice_calls DROP CONSTRAINT uq_voice_calls_org_id;
ALTER TABLE channel_voice_calls DROP CONSTRAINT channel_voice_calls_transcript_status_check;
UPDATE channel_voice_calls SET transcript_status='complete' WHERE transcript_status='completed';
UPDATE channel_voice_calls SET transcript_status='pending' WHERE transcript_status='processing';
ALTER TABLE channel_voice_calls ADD CONSTRAINT channel_voice_calls_transcript_status_check CHECK(transcript_status IN ('unavailable','pending','complete','failed'));
-- Retain accounting rows by mapping the removed task to the legacy embedding bucket.
UPDATE ai_runs SET task='embedding' WHERE task='transcribe';
ALTER TABLE ai_runs DROP CONSTRAINT ai_runs_task_check;
ALTER TABLE ai_runs ADD CONSTRAINT ai_runs_task_check CHECK(task IN ('triage','reply','complex','embedding'));
