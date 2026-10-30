-- Data-loss note: removes workflow graphs, versions and history.
ALTER TABLE workflows DROP CONSTRAINT fk_workflow_published;
DROP TABLE workflow_steps,workflow_runs,workflow_versions,workflows;
-- Catalog definitions referenced by billing configuration or immutable financial history are retained.
-- Published plan entitlements cannot be deleted (BILLING_VERSION_IMMUTABLE).
DELETE FROM billing_features f WHERE f.key IN ('workflows.enabled','workflows.count','workflows.runs_daily')
 AND NOT EXISTS(SELECT 1 FROM billing_plan_entitlements e WHERE e.feature_id=f.id)
 AND NOT EXISTS(SELECT 1 FROM billing_entitlement_overrides o WHERE o.feature_id=f.id)
 AND NOT EXISTS(SELECT 1 FROM billing_price_tiers t WHERE t.feature_id=f.id)
 AND NOT EXISTS(SELECT 1 FROM billing_addons a WHERE a.feature_id=f.id)
 AND NOT EXISTS(SELECT 1 FROM billing_usage_records r WHERE r.feature_id=f.id)
 AND NOT EXISTS(SELECT 1 FROM billing_usage_totals t WHERE t.feature_id=f.id)
 AND NOT EXISTS(SELECT 1 FROM billing_usage_settlements s WHERE s.feature_id=f.id);
