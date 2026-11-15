-- Data-loss note: platform grants and overrides for the three features are
-- dropped; tenants that followed the platform get the former defaults back,
-- and the overrides written by the up migration fold back into ai_policies.
UPDATE ai_policies p SET
 daily_calls = COALESCE(p.daily_calls, (SELECT LEAST(o.value_limit,100000)::int FROM billing_entitlement_overrides o JOIN billing_features f ON f.id=o.feature_id WHERE f.key='ai.daily_calls' AND o.organization_id=p.organization_id AND NOT o.unlimited), 100),
 concurrency = COALESCE(p.concurrency, (SELECT LEAST(o.value_limit,20)::int FROM billing_entitlement_overrides o JOIN billing_features f ON f.id=o.feature_id WHERE f.key='ai.concurrency' AND o.organization_id=p.organization_id AND NOT o.unlimited), 3),
 daily_budget_micros = COALESCE(p.daily_budget_micros, (SELECT o.value_limit FROM billing_entitlement_overrides o JOIN billing_features f ON f.id=o.feature_id WHERE f.key='ai.daily_budget_micros' AND o.organization_id=p.organization_id AND NOT o.unlimited AND o.value_limit>0), 10000000);
ALTER TABLE ai_policies
 ALTER COLUMN daily_calls SET DEFAULT 100, ALTER COLUMN daily_calls SET NOT NULL,
 ALTER COLUMN concurrency SET DEFAULT 3, ALTER COLUMN concurrency SET NOT NULL,
 ALTER COLUMN daily_budget_micros SET DEFAULT 10000000, ALTER COLUMN daily_budget_micros SET NOT NULL;
DELETE FROM billing_entitlement_overrides WHERE feature_id IN (SELECT id FROM billing_features WHERE key IN ('ai.daily_calls','ai.daily_budget_micros','ai.concurrency'));
ALTER TABLE billing_plan_entitlements DISABLE TRIGGER trg_billing_entitlement_guard;
DELETE FROM billing_plan_entitlements WHERE feature_id IN (SELECT id FROM billing_features WHERE key IN ('ai.daily_calls','ai.daily_budget_micros','ai.concurrency'));
ALTER TABLE billing_plan_entitlements ENABLE TRIGGER trg_billing_entitlement_guard;
DELETE FROM billing_features WHERE key IN ('ai.daily_calls','ai.daily_budget_micros','ai.concurrency');
