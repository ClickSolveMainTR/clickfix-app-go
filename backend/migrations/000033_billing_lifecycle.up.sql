-- CF-19/20/22: lifecycle audit, cache generations and platform permissions.
ALTER TABLE billing_subscriptions DROP CONSTRAINT billing_subscriptions_status_check;
ALTER TABLE billing_subscriptions ADD CONSTRAINT billing_subscriptions_status_check CHECK
    (status IN ('pending_payment','trialing','active','past_due','grace','suspended','paused','canceled','expired'));
DROP INDEX uq_billing_subscriptions_live;
CREATE UNIQUE INDEX uq_billing_subscriptions_live ON billing_subscriptions(organization_id)
    WHERE status NOT IN ('canceled','expired');
ALTER TABLE organizations ADD COLUMN billing_revision BIGINT NOT NULL DEFAULT 0;
CREATE TABLE billing_subscription_transitions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    subscription_id BIGINT NOT NULL,
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    reason TEXT NOT NULL CHECK (length(trim(reason)) BETWEEN 1 AND 1000),
    actor_user_id BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, subscription_id) REFERENCES billing_subscriptions(organization_id,id)
);
CREATE INDEX idx_billing_transitions_organization ON billing_subscription_transitions(organization_id,subscription_id,id);
CREATE TRIGGER trg_billing_transitions_append_only BEFORE UPDATE OR DELETE ON billing_subscription_transitions
    FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();
CREATE FUNCTION billing_bump_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP <> 'INSERT' THEN
        UPDATE organizations SET billing_revision=billing_revision+1 WHERE id=OLD.organization_id;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        UPDATE organizations SET billing_revision=billing_revision+1 WHERE id=NEW.organization_id;
        RETURN NEW;
    END IF;
    RETURN OLD;
END $$;
CREATE TRIGGER trg_billing_subscription_revision AFTER INSERT OR UPDATE OR DELETE ON billing_subscriptions
    FOR EACH ROW EXECUTE FUNCTION billing_bump_revision();
CREATE TRIGGER trg_billing_item_revision AFTER INSERT OR UPDATE OR DELETE ON billing_subscription_items
    FOR EACH ROW EXECUTE FUNCTION billing_bump_revision();
CREATE TRIGGER trg_billing_override_revision AFTER INSERT OR UPDATE OR DELETE ON billing_entitlement_overrides
    FOR EACH ROW EXECUTE FUNCTION billing_bump_revision();
INSERT INTO billing_features(key,kind,label_tr,label_en,unit,aggregation,period) VALUES
 ('channels.whatsapp','limit','WhatsApp kanalları','WhatsApp channels','channel','gauge','none'),
 ('channels.email','limit','E-posta kanalları','Email channels','channel','gauge','none'),
 ('contacts','limit','Kişiler','Contacts','contact','gauge','none'),
 ('api.calls','limit','API çağrıları','API calls','request','sum','month'),
 ('engineering.enabled','boolean','Mühendislik köprüsü','Engineering bridge','','none','none'),
 ('status_page.enabled','boolean','Durum sayfası','Status page','','none','none') ON CONFLICT(key) DO NOTHING;
INSERT INTO permissions(name,slug) VALUES
 ('Read billing plans','platform.plans.read'),('Manage billing plans','platform.plans.write'),
 ('Read billing subscriptions','platform.subscriptions.read'),('Manage billing subscriptions','platform.subscriptions.write'),
 ('Read billing entitlements','platform.entitlements.read'),('Manage billing entitlements','platform.entitlements.write'),
 ('Read organization billing','tenant.billing.read'),('Manage organization billing','tenant.billing.write') ON CONFLICT(slug) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
 SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
 WHERE (r.slug='super_admin' AND r.scope='platform' AND p.slug IN
 ('platform.plans.read','platform.plans.write','platform.subscriptions.read','platform.subscriptions.write','platform.entitlements.read','platform.entitlements.write'))
 OR (r.scope='tenant' AND r.organization_id IS NULL AND r.slug IN ('owner','admin') AND p.slug IN ('tenant.billing.read','tenant.billing.write'))
 OR (r.scope='tenant' AND r.organization_id IS NULL AND r.slug='member' AND p.slug='tenant.billing.read')
 ON CONFLICT DO NOTHING;

CREATE FUNCTION billing_addon_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE organizations SET billing_revision=billing_revision+1 WHERE id IN
  (SELECT organization_id FROM billing_subscription_items WHERE addon_id=NEW.id);
 RETURN NEW;
END $$;
CREATE TRIGGER trg_billing_addon_revision AFTER UPDATE ON billing_addons
 FOR EACH ROW EXECUTE FUNCTION billing_addon_revision();
