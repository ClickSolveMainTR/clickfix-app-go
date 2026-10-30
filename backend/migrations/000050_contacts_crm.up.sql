-- CF-84: B2B fields of companies and contacts.
-- tier: the customer's plan / segment ("enterprise"); account_owner_id: the
-- CSM who owns the account; sla_policy_id: the contracted SLA (CF-46 adds
-- the policies table and its foreign key). job_title: the contact's role
-- at the company.
ALTER TABLE companies
    ADD COLUMN tier             TEXT   NOT NULL DEFAULT '',
    ADD COLUMN account_owner_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN sla_policy_id    BIGINT NULL,
    ADD CONSTRAINT companies_tier_chk CHECK (char_length(tier) <= 64);

ALTER TABLE contacts
    ADD COLUMN job_title TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT contacts_job_title_chk CHECK (char_length(job_title) <= 120);
