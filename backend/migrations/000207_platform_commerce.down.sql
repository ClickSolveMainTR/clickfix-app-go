-- Data-loss note: cms_user assignments removed by the up migration are not
-- restored; the role comes back empty with its two permissions.
INSERT INTO roles (name, slug, description, is_system)
VALUES ('CMS User', 'cms_user', 'Default CMS access for content users', false)
ON CONFLICT DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r
JOIN permissions p ON p.slug IN ('auth.session', 'notifications.read')
WHERE r.slug = 'cms_user' AND r.scope = 'platform'
ON CONFLICT DO NOTHING;

ALTER TABLE billing_credit_ledger DROP COLUMN actor_user_id, DROP COLUMN category;
ALTER TABLE organization_invitations DROP COLUMN platform_issued;
ALTER TABLE auth_settings DROP COLUMN signup_mode;
DELETE FROM notification_templates WHERE code = 'billing.wallet_low';
ALTER TABLE billing_wallet_settings DROP COLUMN low_balance_notified_at, DROP COLUMN alert_threshold;
ALTER TABLE billing_credit_packages DROP COLUMN sort_order, DROP COLUMN plan_ids, DROP COLUMN description;
ALTER TABLE billing_features DROP COLUMN default_unlimited, DROP COLUMN default_limit, DROP COLUMN default_bool;
