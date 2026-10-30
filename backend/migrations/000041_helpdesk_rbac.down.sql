-- Permissions created by 000037_inbox_permissions (CF-8) stay.
-- Data-loss note: members and invitations holding a removed template role
-- fall back to 'member'; grants of the helpdesk permissions and the paid
-- seat flag of every role are dropped.
UPDATE organization_members
SET role_id = (SELECT id FROM roles WHERE slug = 'member' AND scope = 'tenant' AND organization_id IS NULL)
WHERE role_id IN (
    SELECT id FROM roles
    WHERE slug IN ('supervisor', 'agent', 'light_agent', 'billing', 'viewer')
      AND scope = 'tenant' AND organization_id IS NULL
);

UPDATE organization_invitations
SET role_id = (SELECT id FROM roles WHERE slug = 'member' AND scope = 'tenant' AND organization_id IS NULL)
WHERE role_id IN (
    SELECT id FROM roles
    WHERE slug IN ('supervisor', 'agent', 'light_agent', 'billing', 'viewer')
      AND scope = 'tenant' AND organization_id IS NULL
);

DELETE FROM organization_member_roles
WHERE role_id IN (
    SELECT id FROM roles
    WHERE slug IN ('supervisor', 'agent', 'light_agent', 'billing', 'viewer')
      AND scope = 'tenant' AND organization_id IS NULL
);

-- Keep the primary role in organization_member_roles.
INSERT INTO organization_member_roles (organization_id, member_id, role_id)
SELECT organization_id, id, role_id FROM organization_members
ON CONFLICT DO NOTHING;

DELETE FROM roles
WHERE slug IN ('supervisor', 'agent', 'light_agent', 'billing', 'viewer')
  AND scope = 'tenant' AND organization_id IS NULL;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.conversations.read_all', 'tenant.conversations.note', 'tenant.conversations.assign', 'tenant.conversations.delete', 'tenant.conversations.export',
        'tenant.tickets.read', 'tenant.tickets.read_all', 'tenant.tickets.write', 'tenant.tickets.assign',
        'tenant.tickets.delete', 'tenant.tickets.export',
        'tenant.contacts.delete', 'tenant.contacts.export',
        'tenant.contacts.import', 'tenant.teams.read', 'tenant.teams.write', 'tenant.reports.read', 'tenant.reports.export',
        'tenant.ai.use', 'tenant.ai.read', 'tenant.ai.write', 'tenant.automations.read', 'tenant.automations.write',
        'tenant.knowledge.read', 'tenant.knowledge.write', 'tenant.knowledge.publish',
        'tenant.macros.read', 'tenant.macros.write', 'tenant.sla.read', 'tenant.sla.write'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.conversations.read_all', 'tenant.conversations.note', 'tenant.conversations.assign', 'tenant.conversations.delete', 'tenant.conversations.export',
    'tenant.tickets.read', 'tenant.tickets.read_all', 'tenant.tickets.write', 'tenant.tickets.assign',
    'tenant.tickets.delete', 'tenant.tickets.export',
    'tenant.contacts.delete', 'tenant.contacts.export',
    'tenant.contacts.import', 'tenant.teams.read', 'tenant.teams.write', 'tenant.reports.read', 'tenant.reports.export',
    'tenant.ai.use', 'tenant.ai.read', 'tenant.ai.write', 'tenant.automations.read', 'tenant.automations.write',
    'tenant.knowledge.read', 'tenant.knowledge.write', 'tenant.knowledge.publish',
    'tenant.macros.read', 'tenant.macros.write', 'tenant.sla.read', 'tenant.sla.write'
);

ALTER TABLE roles DROP COLUMN IF EXISTS is_billable;
