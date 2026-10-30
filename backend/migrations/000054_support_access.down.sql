-- Data-loss note: drops the support access history of every organization.
DELETE FROM permissions WHERE slug IN ('tenant.support_access.read', 'tenant.support_access.manage', 'platform.support.access');
DROP TABLE IF EXISTS support_access_grants;
