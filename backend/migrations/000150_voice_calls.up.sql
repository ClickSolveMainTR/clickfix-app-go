-- Voice lifecycle log. Recording bytes stay in storage as message attachments;
-- the recording message FK supplies org-scoped access and storage URLs.
CREATE TABLE channel_voice_calls (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_instance_id BIGINT NOT NULL,
    external_id TEXT NOT NULL CHECK (char_length(external_id) BETWEEN 1 AND 255),
    provider TEXT NOT NULL CHECK (provider IN ('twilio', 'netgsm')),
    direction TEXT NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    status TEXT NOT NULL,
    from_number TEXT NOT NULL DEFAULT '',
    to_number TEXT NOT NULL DEFAULT '',
    duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
    recording_message_id BIGINT NULL,
    transcript TEXT NOT NULL DEFAULT '',
    transcript_status TEXT NOT NULL DEFAULT 'unavailable' CHECK (transcript_status IN ('unavailable', 'pending', 'complete', 'failed')),
    last_event_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT channel_voice_calls_instance_fk FOREIGN KEY (organization_id, channel_instance_id)
        REFERENCES channel_instances(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT channel_voice_calls_recording_fk FOREIGN KEY (organization_id, recording_message_id)
        REFERENCES messages(organization_id, id) ON DELETE SET NULL (recording_message_id),
    UNIQUE (organization_id, channel_instance_id, external_id)
);
CREATE INDEX idx_channel_voice_calls_org ON channel_voice_calls(organization_id, channel_instance_id, created_at DESC);
CREATE TRIGGER trg_channel_voice_calls_updated BEFORE UPDATE ON channel_voice_calls
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
