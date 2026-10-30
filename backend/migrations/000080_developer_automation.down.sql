-- Data-loss note: removes automation configuration, execution logs, webhooks, deliveries and API credentials.
DROP TABLE public_api_idempotency,public_api_keys,outbound_webhook_deliveries,outbound_webhooks,automation_executions,automation_rules;
ALTER TABLE conversations DROP CONSTRAINT uq_conversations_org_uuid;
