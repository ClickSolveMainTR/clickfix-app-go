-- Data-loss note: removes the invitation e-mail templates, the organization
-- export / bulk permissions with their grants and the regional defaults of
-- every organization.
DELETE FROM notification_templates WHERE code = 'organizations.invitation';

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'platform.organizations.export',
        'platform.organizations.bulk.activate',
        'platform.organizations.bulk.suspend'
    )
);

DELETE FROM permissions WHERE slug IN (
    'platform.organizations.export',
    'platform.organizations.bulk.activate',
    'platform.organizations.bulk.suspend'
);

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS chk_organizations_country_code,
    DROP CONSTRAINT IF EXISTS chk_organizations_currency,
    DROP CONSTRAINT IF EXISTS chk_organizations_locale,
    DROP COLUMN IF EXISTS country_code,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS locale,
    DROP COLUMN IF EXISTS timezone;
