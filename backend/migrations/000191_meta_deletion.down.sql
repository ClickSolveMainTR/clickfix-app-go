-- Data-loss note: public deletion receipts and their request links are removed.
DROP TABLE meta_deletion_links;
DROP TABLE meta_deletion_receipts;
ALTER TABLE data_subject_requests DROP CONSTRAINT IF EXISTS data_subject_requests_org_uuid;
