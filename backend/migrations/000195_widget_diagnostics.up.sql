CREATE TABLE widget_diagnostics (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_id BIGINT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('screenshot','recording','technical_context')),
 attachment_id UUID,
 duration_ms INTEGER NOT NULL DEFAULT 0 CHECK(duration_ms BETWEEN 0 AND 60000),
 context JSONB NOT NULL DEFAULT '{}',
 expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '30 days',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,attachment_id) REFERENCES attachments(organization_id,uuid),
 CHECK((kind='technical_context' AND attachment_id IS NULL AND duration_ms=0) OR (kind IN ('screenshot','recording') AND attachment_id IS NOT NULL))
);
CREATE INDEX idx_widget_diagnostics_conversation ON widget_diagnostics(organization_id,conversation_id,created_at DESC);
CREATE INDEX idx_widget_diagnostics_expiry ON widget_diagnostics(expires_at);
CREATE TRIGGER trg_widget_diagnostics_updated BEFORE UPDATE ON widget_diagnostics FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- Retention is also enforced by attachment streaming, including previously signed URLs.
ALTER TABLE attachments ADD COLUMN expires_at TIMESTAMPTZ;

-- Diagnostic context follows conversation erasure immediately, even before retention runs.
CREATE FUNCTION erase_conversation_diagnostics() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.anonymized_at IS NOT NULL AND OLD.anonymized_at IS NULL THEN
  DELETE FROM widget_diagnostics WHERE organization_id=NEW.organization_id AND conversation_id=NEW.id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER trg_conversation_diagnostics_erasure AFTER UPDATE OF anonymized_at ON conversations
 FOR EACH ROW EXECUTE FUNCTION erase_conversation_diagnostics();
