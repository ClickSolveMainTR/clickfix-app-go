ALTER TABLE contacts DROP CONSTRAINT IF EXISTS contacts_job_title_chk, DROP COLUMN IF EXISTS job_title;
ALTER TABLE companies
    DROP CONSTRAINT IF EXISTS companies_tier_chk,
    DROP COLUMN IF EXISTS sla_policy_id,
    DROP COLUMN IF EXISTS account_owner_id,
    DROP COLUMN IF EXISTS tier;
