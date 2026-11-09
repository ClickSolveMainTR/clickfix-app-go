-- Data-loss note: contacts.external_id and verified_at are dropped; the
-- widget identities ("user:<id>") they were filled from stay. Company
-- external ids cleared as duplicates by the up migration are not restored.
CREATE OR REPLACE FUNCTION compliance_guard_tombstone() RETURNS trigger AS $$
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

DROP INDEX IF EXISTS uq_companies_org_external_id;
DROP INDEX IF EXISTS uq_contacts_org_external_id;
ALTER TABLE contacts
    DROP CONSTRAINT IF EXISTS contacts_external_id_chk,
    DROP COLUMN IF EXISTS verified_at,
    DROP COLUMN IF EXISTS external_id;
