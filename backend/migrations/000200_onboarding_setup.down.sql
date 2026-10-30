DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.onboarding.manage');
DELETE FROM permissions WHERE slug = 'tenant.onboarding.manage';
DROP TABLE IF EXISTS onboarding_setup_items;
DROP TABLE IF EXISTS onboarding_setup_runs;
