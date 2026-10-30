-- Global receipts contain no provider user id or contact data. The code is an
-- unguessable public capability; organization-scoped links use the CF-127 queue.
CREATE TABLE meta_deletion_receipts (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 code TEXT NOT NULL UNIQUE CHECK(code ~ '^[a-f0-9]{32}$'),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','completed')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 completed_at TIMESTAMPTZ
);
CREATE TRIGGER meta_deletion_receipts_updated BEFORE UPDATE ON meta_deletion_receipts FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX meta_deletion_receipts_pending ON meta_deletion_receipts(id) WHERE status='pending';
ALTER TABLE data_subject_requests ADD CONSTRAINT data_subject_requests_org_uuid UNIQUE(organization_id,uuid);
CREATE TABLE meta_deletion_links (
 id BIGSERIAL PRIMARY KEY,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 receipt_id BIGINT NOT NULL REFERENCES meta_deletion_receipts(id) ON DELETE CASCADE,
 request_uuid UUID NOT NULL,
 FOREIGN KEY(organization_id,request_uuid) REFERENCES data_subject_requests(organization_id,uuid) ON DELETE CASCADE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,receipt_id,request_uuid)
);
CREATE INDEX meta_deletion_links_org ON meta_deletion_links(organization_id);
CREATE INDEX meta_deletion_links_receipt ON meta_deletion_links(receipt_id);
