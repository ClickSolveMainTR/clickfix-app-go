-- Data-loss note: deletes brand configuration, templates and brand assignments.
ALTER TABLE billing_plan_entitlements DISABLE TRIGGER trg_billing_entitlement_guard;
DELETE FROM billing_plan_entitlements WHERE feature_id IN (SELECT id FROM billing_features WHERE key='brands');
ALTER TABLE billing_plan_entitlements ENABLE TRIGGER trg_billing_entitlement_guard;
DELETE FROM billing_features WHERE key='brands';
DROP TABLE brand_email_templates;
DROP TRIGGER trg_conversation_brand ON conversations;
DROP FUNCTION inherit_conversation_brand();
ALTER TABLE knowledge_articles DROP COLUMN brand_id;
ALTER TABLE conversations DROP COLUMN brand_id;
ALTER TABLE channel_instances DROP COLUMN brand_id;
DROP TABLE brands;
ALTER TABLE attachments DROP CONSTRAINT uq_attachments_org_uuid;
ALTER TABLE sla_policies DROP CONSTRAINT uq_sla_policies_org_uuid;
