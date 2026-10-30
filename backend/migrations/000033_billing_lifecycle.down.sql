-- Data-loss note: discards transition audit, cache generations and newly seeded permissions/features.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN
 ('platform.plans.read','platform.plans.write','platform.subscriptions.read','platform.subscriptions.write','platform.entitlements.read','platform.entitlements.write','tenant.billing.read','tenant.billing.write'));
DELETE FROM permissions WHERE slug IN
 ('platform.plans.read','platform.plans.write','platform.subscriptions.read','platform.subscriptions.write','platform.entitlements.read','platform.entitlements.write','tenant.billing.read','tenant.billing.write');
DROP TRIGGER trg_billing_subscription_revision ON billing_subscriptions;
DROP TRIGGER trg_billing_item_revision ON billing_subscription_items;
DROP TRIGGER trg_billing_override_revision ON billing_entitlement_overrides;
DROP FUNCTION billing_bump_revision();
DROP TRIGGER trg_billing_addon_revision ON billing_addons;
DROP FUNCTION billing_addon_revision();
DROP TABLE billing_subscription_transitions;
ALTER TABLE organizations DROP COLUMN billing_revision;
-- New feature definitions are retained when commercial data references them.
DELETE FROM billing_features f WHERE key IN ('channels.whatsapp','channels.email','contacts','api.calls','engineering.enabled','status_page.enabled')
 AND NOT EXISTS (SELECT 1 FROM billing_plan_entitlements e WHERE e.feature_id=f.id)
 AND NOT EXISTS (SELECT 1 FROM billing_entitlement_overrides o WHERE o.feature_id=f.id)
 AND NOT EXISTS (SELECT 1 FROM billing_addons a WHERE a.feature_id=f.id)
 AND NOT EXISTS (SELECT 1 FROM billing_price_tiers t WHERE t.feature_id=f.id)
 AND NOT EXISTS (SELECT 1 FROM billing_usage_records u WHERE u.feature_id=f.id)
 AND NOT EXISTS (SELECT 1 FROM billing_usage_totals u WHERE u.feature_id=f.id);
ALTER TABLE billing_subscriptions DROP CONSTRAINT billing_subscriptions_status_check;
UPDATE billing_subscriptions SET status=CASE WHEN status='pending_payment' THEN 'paused' ELSE 'grace' END
 WHERE status IN ('pending_payment','suspended');
ALTER TABLE billing_subscriptions ADD CONSTRAINT billing_subscriptions_status_check CHECK
 (status IN ('trialing','active','past_due','grace','paused','canceled','expired'));
DROP INDEX uq_billing_subscriptions_live;
CREATE UNIQUE INDEX uq_billing_subscriptions_live ON billing_subscriptions(organization_id)
 WHERE status IN ('trialing','active','past_due','grace','paused');
