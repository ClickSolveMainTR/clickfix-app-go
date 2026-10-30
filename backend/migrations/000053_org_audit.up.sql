-- CF-12: organization audit log permissions. Owners and admins read and
-- export their organization's audit log (activity_events.organization_id).
INSERT INTO permissions (name, slug) VALUES
    ('Read the organization audit log', 'tenant.audit.read'),
    ('Export the organization audit log', 'tenant.audit.export')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.audit.read', 'tenant.audit.export')
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin')
ON CONFLICT DO NOTHING;

-- Tenant audit list: newest events of one organization, by actor and kind.
CREATE INDEX IF NOT EXISTS idx_activity_events_organization_actor_created
    ON activity_events (organization_id, actor_user_id, created_at DESC)
    WHERE organization_id IS NOT NULL;
