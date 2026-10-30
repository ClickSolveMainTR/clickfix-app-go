-- CF-127: explicit opt-in retention; zero days disables a category.
CREATE TABLE retention_policies (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_days INTEGER NOT NULL DEFAULT 0 CHECK(conversation_days BETWEEN 0 AND 36500),
 attachment_days INTEGER NOT NULL DEFAULT 0 CHECK(attachment_days BETWEEN 0 AND 36500),
 ai_log_days INTEGER NOT NULL DEFAULT 0 CHECK(ai_log_days BETWEEN 0 AND 36500),
 action TEXT NOT NULL DEFAULT 'anonymize' CHECK(action IN ('anonymize','delete')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_retention_policies_updated BEFORE UPDATE ON retention_policies FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE data_subject_requests (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 contact_id BIGINT NOT NULL, requested_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
 kind TEXT NOT NULL CHECK(kind IN ('export','erase')),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','completed')),
 object_key TEXT NOT NULL DEFAULT '', request_id TEXT NOT NULL DEFAULT '',
 expires_at TIMESTAMPTZ, completed_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,contact_id) REFERENCES contacts(organization_id,id)
);
CREATE INDEX idx_data_subject_requests_org ON data_subject_requests(organization_id,created_at DESC);
CREATE TRIGGER trg_data_subject_requests_updated BEFORE UPDATE ON data_subject_requests FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE contact_consents (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 contact_id BIGINT NOT NULL, purpose TEXT NOT NULL CHECK(length(purpose) BETWEEN 1 AND 100),
 granted BOOLEAN NOT NULL, notice_version TEXT NOT NULL CHECK(length(notice_version) BETWEEN 1 AND 100),
 source TEXT NOT NULL CHECK(length(source) BETWEEN 1 AND 100),
 recorded_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,contact_id) REFERENCES contacts(organization_id,id)
);
CREATE INDEX idx_contact_consents_org ON contact_consents(organization_id,contact_id,created_at DESC);
ALTER TABLE conversations ADD COLUMN anonymized_at TIMESTAMPTZ;
ALTER TABLE contacts ADD COLUMN erased_at TIMESTAMPTZ;
-- Bind conversation-originated AI logs at creation, including in-flight runs.
ALTER TABLE ai_runs ADD COLUMN conversation_id BIGINT,
 ADD COLUMN pii_erased_at TIMESTAMPTZ,
 ADD CONSTRAINT ai_runs_conversation_fk FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id);
CREATE INDEX idx_ai_runs_conversation ON ai_runs(organization_id,conversation_id) WHERE conversation_id IS NOT NULL;
-- A late provider completion or message webhook must not resurrect disposed data.
CREATE FUNCTION compliance_guard_content() RETURNS trigger AS $$
DECLARE disposed timestamptz;
BEGIN
 SELECT anonymized_at INTO disposed FROM conversations
 WHERE organization_id=NEW.organization_id AND id=NEW.conversation_id FOR SHARE;
 IF TG_TABLE_NAME='ai_runs' THEN
  IF disposed IS NOT NULL OR NEW.pii_erased_at IS NOT NULL THEN
   NEW.input=''; NEW.output=''; NEW.retrieved_chunks='[]'; NEW.tool_calls='[]'; NEW.pii_erased_at=COALESCE(NEW.pii_erased_at,now());
  END IF;
 ELSIF disposed IS NOT NULL THEN
  NEW.content=''; NEW.content_attributes='{}'; NEW.external_id=NULL; NEW.error=''; NEW.deleted_at=now();
  IF NEW.status IN ('pending','queued') THEN NEW.status='failed'; END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_ai_runs_privacy BEFORE INSERT OR UPDATE ON ai_runs FOR EACH ROW EXECUTE FUNCTION compliance_guard_content();
CREATE TRIGGER trg_messages_privacy BEFORE INSERT OR UPDATE ON messages FOR EACH ROW EXECUTE FUNCTION compliance_guard_content();
CREATE FUNCTION compliance_guard_tombstone() RETURNS trigger AS $$
BEGIN
 IF TG_TABLE_NAME='contacts' THEN
  IF OLD.erased_at IS NOT NULL THEN
   NEW.name=OLD.name; NEW.email=NULL; NEW.phone=NULL; NEW.locale=NULL; NEW.timezone=NULL;
   NEW.notes=''; NEW.custom_fields='{}'; NEW.job_title=''; NEW.company_id=NULL;
   NEW.blocked=true; NEW.deleted_at=OLD.deleted_at; NEW.erased_at=OLD.erased_at;
  END IF;
 ELSIF OLD.anonymized_at IS NOT NULL THEN
  NEW.subject=''; NEW.meta='{}'; NEW.custom_fields='{}'; NEW.last_message_preview='';
  NEW.external_thread_id=NULL; NEW.company_id=NULL; NEW.contact_id=OLD.contact_id;
  NEW.anonymized_at=OLD.anonymized_at; NEW.deleted_at=OLD.deleted_at;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_contacts_privacy BEFORE UPDATE ON contacts FOR EACH ROW EXECUTE FUNCTION compliance_guard_tombstone();
CREATE TRIGGER trg_conversations_privacy BEFORE UPDATE ON conversations FOR EACH ROW EXECUTE FUNCTION compliance_guard_tombstone();
CREATE FUNCTION compliance_guard_attachment() RETURNS trigger AS $$
DECLARE disposed timestamptz;
BEGIN
 IF NEW.object_key='' THEN RETURN NEW; END IF;
 IF NEW.owner_type IN ('conversation','ticket') THEN
  SELECT anonymized_at INTO disposed FROM conversations WHERE organization_id=NEW.organization_id AND uuid=NEW.owner_id FOR SHARE;
 ELSIF NEW.owner_type='message' THEN
  SELECT c.anonymized_at INTO disposed FROM conversations c JOIN messages m ON m.organization_id=c.organization_id AND m.conversation_id=c.id
  WHERE m.organization_id=NEW.organization_id AND m.uuid=NEW.owner_id FOR SHARE OF c;
 END IF;
 IF disposed IS NOT NULL THEN RAISE EXCEPTION 'disposed attachment owner' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_attachments_privacy BEFORE INSERT OR UPDATE ON attachments FOR EACH ROW EXECUTE FUNCTION compliance_guard_attachment();
CREATE FUNCTION compliance_guard_new_conversation() RETURNS trigger AS $$
DECLARE erased timestamptz;
BEGIN
 SELECT erased_at INTO erased FROM contacts WHERE organization_id=NEW.organization_id AND id=NEW.contact_id FOR SHARE;
 IF erased IS NOT NULL THEN
  RAISE EXCEPTION 'erased conversation subject' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_new_conversations_privacy BEFORE INSERT ON conversations FOR EACH ROW EXECUTE FUNCTION compliance_guard_new_conversation();
