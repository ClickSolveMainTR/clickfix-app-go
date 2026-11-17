-- Data-loss note: drops every remote support session (including its audit
-- fields) and the organization settings. The remote_support.* plan features
-- stay in billing_features when usage was already metered against them
-- (billing_usage_records is append-only).
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.remote_support.use','tenant.remote_support.control','tenant.remote_support.settings','platform.remote_support.manage'));
DELETE FROM permissions WHERE slug IN ('tenant.remote_support.use','tenant.remote_support.control','tenant.remote_support.settings','platform.remote_support.manage');

DROP TABLE IF EXISTS remote_support_sessions;
DROP TABLE IF EXISTS remote_support_settings;

DELETE FROM billing_entitlement_overrides WHERE feature_id IN (SELECT id FROM billing_features WHERE key IN ('remote_support.enabled','remote_support.minutes'));
DELETE FROM billing_features f WHERE f.key IN ('remote_support.enabled','remote_support.minutes')
    AND NOT EXISTS (SELECT 1 FROM billing_plan_entitlements e WHERE e.feature_id = f.id)
    AND NOT EXISTS (SELECT 1 FROM billing_usage_records u WHERE u.feature_id = f.id)
    AND NOT EXISTS (SELECT 1 FROM billing_usage_totals t WHERE t.feature_id = f.id)
    AND NOT EXISTS (SELECT 1 FROM billing_usage_settlements s WHERE s.feature_id = f.id);
