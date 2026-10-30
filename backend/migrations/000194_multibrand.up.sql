-- Brands use public UUID references scoped by organization, including composite FKs.
CREATE TABLE brands (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
 logo_attachment_id UUID,
 primary_color TEXT NOT NULL DEFAULT '#2563eb',
 secondary_color TEXT NOT NULL DEFAULT '#ffffff',
 support_email TEXT NOT NULL DEFAULT '',
 sender_name TEXT NOT NULL DEFAULT '',
 reply_to TEXT NOT NULL DEFAULT '',
 help_subdomain TEXT NOT NULL DEFAULT '',
 help_path TEXT NOT NULL DEFAULT '',
 widget_defaults JSONB NOT NULL DEFAULT '{}',
 signature TEXT NOT NULL DEFAULT '',
 business_hours JSONB NOT NULL DEFAULT '{}',
 sla_policy_id UUID,
 ai_persona TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,uuid),
 UNIQUE(organization_id,name)
);
CREATE INDEX idx_brands_organization ON brands(organization_id);
CREATE UNIQUE INDEX uq_brands_help_subdomain ON brands(lower(help_subdomain)) WHERE help_subdomain <> '';
CREATE UNIQUE INDEX uq_brands_help_path ON brands(organization_id,help_path) WHERE help_path <> '';
ALTER TABLE attachments ADD CONSTRAINT uq_attachments_org_uuid UNIQUE(organization_id,uuid);
ALTER TABLE sla_policies ADD CONSTRAINT uq_sla_policies_org_uuid UNIQUE(organization_id,uuid);
ALTER TABLE brands ADD FOREIGN KEY(organization_id,logo_attachment_id) REFERENCES attachments(organization_id,uuid);
ALTER TABLE brands ADD FOREIGN KEY(organization_id,sla_policy_id) REFERENCES sla_policies(organization_id,uuid);
CREATE TRIGGER trg_brands_updated BEFORE UPDATE ON brands FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE channel_instances ADD COLUMN brand_id UUID;
ALTER TABLE channel_instances ADD FOREIGN KEY(organization_id,brand_id) REFERENCES brands(organization_id,uuid);
CREATE INDEX idx_channel_instances_brand ON channel_instances(organization_id,brand_id);
ALTER TABLE conversations ADD COLUMN brand_id UUID;
ALTER TABLE conversations ADD FOREIGN KEY(organization_id,brand_id) REFERENCES brands(organization_id,uuid);
CREATE INDEX idx_conversations_brand ON conversations(organization_id,brand_id,last_message_at DESC);
ALTER TABLE knowledge_articles ADD COLUMN brand_id UUID;
ALTER TABLE knowledge_articles ADD FOREIGN KEY(organization_id,brand_id) REFERENCES brands(organization_id,uuid);
CREATE INDEX idx_knowledge_articles_brand ON knowledge_articles(organization_id,brand_id);
-- A conversation keeps the original brand even if its channel is later reassigned.
CREATE FUNCTION inherit_conversation_brand() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.brand_id IS NULL AND NEW.channel_instance_id IS NOT NULL THEN
  SELECT brand_id INTO NEW.brand_id FROM channel_instances WHERE organization_id=NEW.organization_id AND id=NEW.channel_instance_id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER trg_conversation_brand BEFORE INSERT ON conversations FOR EACH ROW EXECUTE FUNCTION inherit_conversation_brand();
CREATE TABLE brand_email_templates (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 brand_id UUID NOT NULL,
 code TEXT NOT NULL CHECK(length(code) BETWEEN 1 AND 80),
 subject TEXT NOT NULL DEFAULT '',
 body TEXT NOT NULL CHECK(length(body) <= 20000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,brand_id) REFERENCES brands(organization_id,uuid) ON DELETE CASCADE,
 UNIQUE(organization_id,brand_id,code)
);
CREATE TRIGGER trg_brand_email_templates_updated BEFORE UPDATE ON brand_email_templates FOR EACH ROW EXECUTE FUNCTION set_updated_at();
INSERT INTO billing_features(key,kind,label_tr,label_en,unit,aggregation,period)
 VALUES('brands','limit','Marka sayısı','Brands','brand','gauge','none') ON CONFLICT(key) DO NOTHING;
-- Published plan versions are immutable (trg_billing_entitlement_guard); a new feature still has to
-- reach them, so the guard is lifted only for this insert.
ALTER TABLE billing_plan_entitlements DISABLE TRIGGER trg_billing_entitlement_guard;
-- Existing plans gain one brand; administrators may publish higher limits or org overrides.
INSERT INTO billing_plan_entitlements(plan_version_id,feature_id,kind,value_limit)
 SELECT v.id,f.id,'limit',1 FROM billing_plan_versions v CROSS JOIN billing_features f WHERE f.key='brands'
 ON CONFLICT(plan_version_id,feature_id) DO NOTHING;
ALTER TABLE billing_plan_entitlements ENABLE TRIGGER trg_billing_entitlement_guard;
