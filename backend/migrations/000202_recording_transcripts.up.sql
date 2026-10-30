ALTER TABLE ai_runs DROP CONSTRAINT ai_runs_task_check;
ALTER TABLE ai_runs ADD CONSTRAINT ai_runs_task_check CHECK(task IN ('triage','reply','complex','embedding','transcribe'));
ALTER TABLE channel_voice_calls DROP CONSTRAINT channel_voice_calls_transcript_status_check;
ALTER TABLE channel_voice_calls ADD CONSTRAINT channel_voice_calls_transcript_status_check CHECK(transcript_status IN ('unavailable','pending','processing','complete','completed','failed'));
ALTER TABLE channel_voice_calls ADD CONSTRAINT uq_voice_calls_org_id UNIQUE(organization_id,id);

CREATE TABLE recording_transcripts (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_id BIGINT,
 voice_call_id BIGINT,
 video_recording_id BIGINT,
 attachment_uuid UUID,
 source_message_id BIGINT,
 timeline_message_id BIGINT,
 run_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
 summary_run_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','completed','failed','unavailable')),
 transcript TEXT NOT NULL DEFAULT '',
 segments JSONB NOT NULL DEFAULT '[]',
 summary JSONB NOT NULL DEFAULT '{}',
 summary_status TEXT NOT NULL DEFAULT 'unavailable' CHECK(summary_status IN ('pending','processing','completed','failed','unavailable')),
 error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(num_nonnulls(voice_call_id,video_recording_id)=1),
 UNIQUE(organization_id,voice_call_id),
 UNIQUE(organization_id,video_recording_id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,voice_call_id) REFERENCES channel_voice_calls(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,video_recording_id) REFERENCES video_recordings(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,attachment_uuid) REFERENCES attachments(organization_id,uuid) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,source_message_id) REFERENCES messages(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,timeline_message_id) REFERENCES messages(organization_id,id) ON DELETE SET NULL(timeline_message_id)
);
CREATE INDEX idx_recording_transcripts_org ON recording_transcripts(organization_id,created_at);
CREATE INDEX idx_recording_transcripts_pending ON recording_transcripts(status,updated_at) WHERE status IN ('pending','processing');
CREATE TRIGGER trg_recording_transcripts_updated BEFORE UPDATE ON recording_transcripts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Durable completion bridge. A minutely Asynq sweep recovers lost queue writes.
CREATE FUNCTION queue_recording_transcript() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='channel_voice_calls' THEN
  IF NEW.recording_message_id IS NOT NULL THEN
   INSERT INTO recording_transcripts(organization_id,voice_call_id,source_message_id,conversation_id,attachment_uuid)
   SELECT NEW.organization_id,NEW.id,m.id,m.conversation_id,a.uuid FROM messages m
   JOIN attachments a ON a.organization_id=m.organization_id AND a.owner_type='message' AND a.owner_id=m.uuid
   JOIN conversations c ON c.organization_id=m.organization_id AND c.id=m.conversation_id
   WHERE m.organization_id=NEW.organization_id AND m.id=NEW.recording_message_id AND m.deleted_at IS NULL
    AND c.deleted_at IS NULL AND c.anonymized_at IS NULL AND a.deleted_at IS NULL AND a.object_key<>''
    AND (a.expires_at IS NULL OR a.expires_at>now())
   ORDER BY a.id LIMIT 1 ON CONFLICT DO NOTHING;
  END IF;
 ELSE
  IF NEW.status='complete' AND (NEW.expires_at IS NULL OR NEW.expires_at>now()) THEN
   INSERT INTO recording_transcripts(organization_id,video_recording_id,conversation_id)
   SELECT NEW.organization_id,NEW.id,r.conversation_id FROM video_rooms r
   LEFT JOIN conversations c ON c.organization_id=r.organization_id AND c.id=r.conversation_id
   WHERE r.organization_id=NEW.organization_id AND r.id=NEW.room_id
    AND (r.conversation_id IS NULL OR (c.deleted_at IS NULL AND c.anonymized_at IS NULL))
   ON CONFLICT DO NOTHING;
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER trg_voice_transcription AFTER INSERT OR UPDATE OF recording_message_id ON channel_voice_calls FOR EACH ROW EXECUTE FUNCTION queue_recording_transcript();
CREATE TRIGGER trg_video_transcription AFTER UPDATE OF status ON video_recordings FOR EACH ROW EXECUTE FUNCTION queue_recording_transcript();

CREATE FUNCTION recording_transcript_timeline() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE mid bigint;
BEGIN
 IF NEW.conversation_id IS NOT NULL THEN
  INSERT INTO messages(organization_id,conversation_id,direction,sender_type,kind,private,status,content_attributes)
  VALUES(NEW.organization_id,NEW.conversation_id,'internal','system','event',true,'recorded',jsonb_build_object('event','recording_transcript','transcript_uuid',NEW.uuid)) RETURNING id INTO mid;
  UPDATE recording_transcripts SET timeline_message_id=mid WHERE organization_id=NEW.organization_id AND id=NEW.id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER trg_transcript_timeline AFTER INSERT ON recording_transcripts FOR EACH ROW EXECUTE FUNCTION recording_transcript_timeline();

-- Delete derived text and masked run traces in the same transaction as erasure.
CREATE FUNCTION erase_recording_transcript() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='messages' THEN
  DELETE FROM recording_transcripts WHERE organization_id=NEW.organization_id AND source_message_id=NEW.id;
 ELSIF TG_TABLE_NAME='attachments' THEN
  DELETE FROM recording_transcripts WHERE organization_id=NEW.organization_id AND attachment_uuid=NEW.uuid;
 ELSIF TG_TABLE_NAME='video_recordings' THEN
  DELETE FROM recording_transcripts WHERE organization_id=NEW.organization_id AND video_recording_id=NEW.id;
 ELSIF TG_TABLE_NAME='conversations' THEN
  DELETE FROM recording_transcripts WHERE organization_id=NEW.organization_id AND conversation_id=NEW.id;
 ELSE
  UPDATE ai_runs SET input='',output='',retrieved_chunks='[]',tool_calls='[]',pii_erased_at=now()
   WHERE organization_id=OLD.organization_id AND uuid IN (OLD.run_uuid,OLD.summary_run_uuid);
  UPDATE channel_voice_calls SET transcript='',transcript_status='unavailable' WHERE organization_id=OLD.organization_id AND id=OLD.voice_call_id;
  UPDATE messages SET content='',content_attributes='{}',deleted_at=now() WHERE organization_id=OLD.organization_id AND id=OLD.timeline_message_id;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER trg_transcript_erase AFTER DELETE ON recording_transcripts FOR EACH ROW EXECUTE FUNCTION erase_recording_transcript();
CREATE TRIGGER trg_message_transcript_erase AFTER UPDATE OF deleted_at ON messages FOR EACH ROW WHEN (NEW.deleted_at IS NOT NULL) EXECUTE FUNCTION erase_recording_transcript();
CREATE TRIGGER trg_attachment_transcript_erase AFTER UPDATE OF deleted_at,object_key ON attachments FOR EACH ROW WHEN (NEW.deleted_at IS NOT NULL OR NEW.object_key='') EXECUTE FUNCTION erase_recording_transcript();
CREATE TRIGGER trg_video_transcript_erase AFTER UPDATE OF status ON video_recordings FOR EACH ROW WHEN (NEW.status='deleted') EXECUTE FUNCTION erase_recording_transcript();
CREATE TRIGGER trg_conversation_transcript_erase AFTER UPDATE OF anonymized_at,deleted_at ON conversations FOR EACH ROW WHEN (NEW.anonymized_at IS NOT NULL OR NEW.deleted_at IS NOT NULL) EXECUTE FUNCTION erase_recording_transcript();
