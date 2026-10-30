DROP INDEX IF EXISTS idx_activity_events_organization_actor_created;
DELETE FROM permissions WHERE slug IN ('tenant.audit.read', 'tenant.audit.export');
