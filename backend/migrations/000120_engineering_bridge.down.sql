DROP TABLE IF EXISTS engineering_error_events;
DROP TABLE IF EXISTS engineering_comment_sync;
-- Data-loss note: removes engineering tracker, integrations and delivery history.
DROP TABLE IF EXISTS engineering_escalations CASCADE;
DROP TABLE IF EXISTS engineering_webhook_receipts CASCADE;
DROP TABLE IF EXISTS engineering_oauth_states CASCADE;
DROP TABLE IF EXISTS engineering_deliveries CASCADE;
DROP TABLE IF EXISTS engineering_external_issues CASCADE;
DROP TABLE IF EXISTS engineering_integrations CASCADE;
DROP TABLE IF EXISTS engineering_comments CASCADE;
DROP TABLE IF EXISTS engineering_conversations CASCADE;
DROP TABLE IF EXISTS engineering_issues CASCADE;
DROP TABLE IF EXISTS engineering_releases CASCADE;
DROP TABLE IF EXISTS engineering_projects CASCADE;
