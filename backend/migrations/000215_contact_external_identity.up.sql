-- CF-195: the customer's own ids as first-class fields, separately for the
-- person and the company (customer team decision, 9 Nov 2026).
--
-- contacts.external_id: the customer's user id (widget identify user_id,
-- contacts API). companies.external_id already exists (CF-25); it becomes
-- unique per organization as well. verified_at: the last time the contact
-- proved its identity with user_hash; NULL for contacts that only told us
-- who they are.
ALTER TABLE contacts
    ADD COLUMN external_id TEXT NULL,
    ADD COLUMN verified_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT contacts_external_id_chk CHECK (external_id IS NULL OR char_length(external_id) BETWEEN 1 AND 200);

-- Backfill from the widget's verified identities ("user:<id>"). One contact
-- per id: the one seen most recently wins; other contacts keep no
-- external id (they stay reachable through their identities).
WITH verified AS (
    SELECT DISTINCT ON (ci.organization_id, substr(ci.external_id, 6))
           ci.organization_id, ci.contact_id, substr(ci.external_id, 6) AS ext,
           COALESCE(ci.last_seen_at, ci.created_at) AS seen
    FROM contact_identities ci
    JOIN contacts c ON c.organization_id = ci.organization_id AND c.id = ci.contact_id
    WHERE ci.channel_type = 'widget' AND ci.external_id LIKE 'user:_%'
      AND char_length(ci.external_id) <= 205
      AND c.deleted_at IS NULL AND c.merged_into_id IS NULL
    ORDER BY ci.organization_id, substr(ci.external_id, 6), COALESCE(ci.last_seen_at, ci.created_at) DESC, ci.id DESC
), one_per_contact AS (
    SELECT DISTINCT ON (organization_id, contact_id) organization_id, contact_id, ext, seen
    FROM verified
    ORDER BY organization_id, contact_id, seen DESC
)
UPDATE contacts c SET external_id = v.ext, verified_at = v.seen
FROM one_per_contact v
WHERE c.organization_id = v.organization_id AND c.id = v.contact_id;

CREATE UNIQUE INDEX uq_contacts_org_external_id
    ON contacts (organization_id, external_id)
    WHERE deleted_at IS NULL AND external_id IS NOT NULL;

-- Companies: duplicates from before (identify created them by external id,
-- agents could type the same id twice) keep the id on the oldest company
-- only, so the unique index can be built.
UPDATE companies co SET external_id = NULL
WHERE co.external_id IS NOT NULL AND co.deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM companies o
              WHERE o.organization_id = co.organization_id AND o.external_id = co.external_id
                AND o.deleted_at IS NULL AND o.id < co.id);
UPDATE companies SET external_id = NULL WHERE external_id IS NOT NULL AND btrim(external_id) = '';

CREATE UNIQUE INDEX uq_companies_org_external_id
    ON companies (organization_id, external_id)
    WHERE deleted_at IS NULL AND external_id IS NOT NULL;

-- Erased contacts (CF compliance) keep neither id.
CREATE OR REPLACE FUNCTION compliance_guard_tombstone() RETURNS trigger AS $$
BEGIN
 IF TG_TABLE_NAME='contacts' THEN
  IF OLD.erased_at IS NOT NULL THEN
   NEW.name=OLD.name; NEW.email=NULL; NEW.phone=NULL; NEW.locale=NULL; NEW.timezone=NULL;
   NEW.notes=''; NEW.custom_fields='{}'; NEW.job_title=''; NEW.company_id=NULL;
   NEW.external_id=NULL; NEW.verified_at=NULL;
   NEW.blocked=true; NEW.deleted_at=OLD.deleted_at; NEW.erased_at=OLD.erased_at;
  END IF;
 ELSIF OLD.anonymized_at IS NOT NULL THEN
  NEW.subject=''; NEW.meta='{}'; NEW.custom_fields='{}'; NEW.last_message_preview='';
  NEW.external_thread_id=NULL; NEW.company_id=NULL; NEW.contact_id=OLD.contact_id;
  NEW.anonymized_at=OLD.anonymized_at; NEW.deleted_at=OLD.deleted_at;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
