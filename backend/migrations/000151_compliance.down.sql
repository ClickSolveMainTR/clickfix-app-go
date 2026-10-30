DROP TRIGGER IF EXISTS trg_attachments_privacy ON attachments;
DROP TRIGGER IF EXISTS trg_new_conversations_privacy ON conversations;
DROP FUNCTION IF EXISTS compliance_guard_attachment();
DROP FUNCTION IF EXISTS compliance_guard_new_conversation();
DROP TRIGGER IF EXISTS trg_contacts_privacy ON contacts;
DROP TRIGGER IF EXISTS trg_conversations_privacy ON conversations;
DROP FUNCTION IF EXISTS compliance_guard_tombstone();
DROP TRIGGER IF EXISTS trg_messages_privacy ON messages;
DROP TRIGGER IF EXISTS trg_ai_runs_privacy ON ai_runs;
DROP FUNCTION IF EXISTS compliance_guard_content();
ALTER TABLE ai_runs DROP COLUMN IF EXISTS conversation_id, DROP COLUMN IF EXISTS pii_erased_at;
-- Data-loss note: policy, consent and request audit history are removed.
ALTER TABLE contacts DROP COLUMN erased_at;
ALTER TABLE conversations DROP COLUMN anonymized_at;
DROP TABLE contact_consents;
DROP TABLE data_subject_requests;
DROP TABLE retention_policies;
