-- CF-229: the daily AI limits (calls, budget, concurrency) are platform
-- entitlements; ai_policies may only lower them. NULL in ai_policies means
-- "follow the platform limit".

-- Catalog defaults are the former ai_policies defaults, so organizations that
-- never configured anything keep the same limits until a plan grant or an
-- organization override (unlimited included) says otherwise.
INSERT INTO billing_features(key,kind,label_tr,label_en,unit,aggregation,period,default_limit) VALUES
 ('ai.daily_calls','limit','Günlük AI çağrısı','Daily AI calls','call','gauge','none',100),
 ('ai.daily_budget_micros','limit','Günlük AI bütçesi (mikro USD)','Daily AI budget (micro USD)','micro_usd','gauge','none',10000000),
 ('ai.concurrency','limit','Eşzamanlı AI çağrısı','Concurrent AI calls','call','gauge','none',3)
ON CONFLICT(key) DO NOTHING;

-- A tenant value above the former default was chosen on purpose (or set by a
-- platform hotfix): keep it as an organization override so the new platform
-- default does not lower it.
INSERT INTO billing_entitlement_overrides(organization_id,feature_id,kind,value_limit,reason)
SELECT p.organization_id, f.id, 'limit', v.value, 'CF-229: ai_policies değeri korundu'
FROM ai_policies p
CROSS JOIN LATERAL (VALUES
 ('ai.daily_calls', p.daily_calls::bigint, 100::bigint),
 ('ai.daily_budget_micros', p.daily_budget_micros, 10000000::bigint),
 ('ai.concurrency', p.concurrency::bigint, 3::bigint)) AS v(key,value,old_default)
JOIN billing_features f ON f.key = v.key
WHERE v.value > v.old_default
ON CONFLICT(organization_id,feature_id) DO NOTHING;

ALTER TABLE ai_policies
 ALTER COLUMN daily_calls DROP NOT NULL, ALTER COLUMN daily_calls DROP DEFAULT,
 ALTER COLUMN concurrency DROP NOT NULL, ALTER COLUMN concurrency DROP DEFAULT,
 ALTER COLUMN daily_budget_micros DROP NOT NULL, ALTER COLUMN daily_budget_micros DROP DEFAULT;

-- Values at or above the former default now come from the platform (above:
-- through the override just written). Lower values stay as tenant limits.
UPDATE ai_policies SET
 daily_calls = CASE WHEN daily_calls >= 100 THEN NULL ELSE daily_calls END,
 concurrency = CASE WHEN concurrency >= 3 THEN NULL ELSE concurrency END,
 daily_budget_micros = CASE WHEN daily_budget_micros >= 10000000 THEN NULL ELSE daily_budget_micros END;
