-- Data-loss note: drops the connected system settings, encrypted
-- credentials and the call log of every organization.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.integrations.read', 'tenant.integrations.manage', 'tenant.integrations.use'));
DELETE FROM permissions WHERE slug IN ('tenant.integrations.read', 'tenant.integrations.manage', 'tenant.integrations.use');
DROP TABLE IF EXISTS integration_calls;
DROP TABLE IF EXISTS integration_playbooks;
DROP TABLE IF EXISTS integration_actions;
DROP TABLE IF EXISTS integration_sources;
DROP TABLE IF EXISTS integration_connections;
