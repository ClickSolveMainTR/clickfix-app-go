DROP INDEX IF EXISTS idx_companies_sla_policy;
ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_sla_policy_fk;
UPDATE companies SET sla_policy_id = NULL WHERE sla_policy_id IS NOT NULL;
ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_sla_policy_fk;
UPDATE conversations
SET sla_policy_id = NULL, first_response_due_at = NULL, next_response_due_at = NULL,
    resolution_due_at = NULL, sla_breached_at = NULL
WHERE sla_policy_id IS NOT NULL;

DROP TABLE IF EXISTS conversation_sla_metrics;
DROP TABLE IF EXISTS sla_policy_targets;
DROP TABLE IF EXISTS sla_policies;
DROP TABLE IF EXISTS business_holidays;
DROP TABLE IF EXISTS public_holidays;
DROP TABLE IF EXISTS business_hours;
