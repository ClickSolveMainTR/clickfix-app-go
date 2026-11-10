-- CF-198: automatic customer replies with one source per text. The
-- acknowledgement ("we got it, estimated reply time ..."), the out-of-hours
-- message and the AI-to-human handoff notice are organization-wide; the
-- working hours they follow are the SLA module's default schedule.
CREATE TABLE IF NOT EXISTS auto_replies (
    organization_id BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    settings JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
DROP TRIGGER IF EXISTS trg_auto_replies_updated_at ON auto_replies;
CREATE TRIGGER trg_auto_replies_updated_at BEFORE UPDATE ON auto_replies
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- Automatic replies are looked up per conversation (once per conversation,
-- or once per closed window).
CREATE INDEX IF NOT EXISTS idx_messages_auto_reply ON messages(organization_id, conversation_id, created_at)
    WHERE sender_type = 'bot' AND content_attributes ? 'auto_reply';
