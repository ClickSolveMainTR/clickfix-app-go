-- CF-4: helpdesk permission catalog, support role templates and paid seats.
--
-- Slug convention: tenant.<resource>.<action>. Visibility of conversations
-- and tickets: `.read` alone = the member's own teams / assignments;
-- `.read_all` = every record of the organization (enforced by the owning
-- modules through orgctx.Scope.CanSeeAll).
--
-- roles.is_billable marks roles that take a paid seat. A member takes one
-- seat when at least one of its roles is billable.

ALTER TABLE roles
    ADD COLUMN is_billable BOOLEAN NOT NULL DEFAULT TRUE;

INSERT INTO permissions (name, slug) VALUES
    ('Read conversations (own teams)', 'tenant.conversations.read'),
    ('Read every conversation', 'tenant.conversations.read_all'),
    ('Reply to customers', 'tenant.conversations.reply'),
    ('Write internal notes', 'tenant.conversations.note'),
    ('Assign conversations', 'tenant.conversations.assign'),
    ('Manage conversations (status, tags, merge)', 'tenant.conversations.manage'),
    ('Delete conversations', 'tenant.conversations.delete'),
    ('Export conversations', 'tenant.conversations.export'),
    ('Read tickets (own teams)', 'tenant.tickets.read'),
    ('Read every ticket', 'tenant.tickets.read_all'),
    ('Write tickets', 'tenant.tickets.write'),
    ('Assign tickets', 'tenant.tickets.assign'),
    ('Delete tickets', 'tenant.tickets.delete'),
    ('Export tickets', 'tenant.tickets.export'),
    ('Read contacts', 'tenant.contacts.read'),
    ('Write contacts', 'tenant.contacts.write'),
    ('Delete contacts', 'tenant.contacts.delete'),
    ('Export contacts', 'tenant.contacts.export'),
    ('Import contacts', 'tenant.contacts.import'),
    ('Read channels', 'tenant.channels.read'),
    ('Read teams', 'tenant.teams.read'),
    ('Manage teams', 'tenant.teams.write'),
    ('Read reports', 'tenant.reports.read'),
    ('Export reports', 'tenant.reports.export'),
    ('Use AI assistance', 'tenant.ai.use'),
    ('Read AI agents', 'tenant.ai.read'),
    ('Manage AI agents', 'tenant.ai.write'),
    ('Read automations', 'tenant.automations.read'),
    ('Manage automations', 'tenant.automations.write'),
    ('Read knowledge base', 'tenant.knowledge.read'),
    ('Write knowledge base', 'tenant.knowledge.write'),
    ('Publish knowledge base', 'tenant.knowledge.publish'),
    ('Read macros', 'tenant.macros.read'),
    ('Manage macros', 'tenant.macros.write'),
    ('Read SLA policies', 'tenant.sla.read'),
    ('Manage SLA policies', 'tenant.sla.write')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO roles (name, slug, description, is_system, scope, organization_id, is_billable) VALUES
    ('Supervisor', 'supervisor', 'Leads the support team: every conversation, assignment, reports, teams, knowledge base', true, 'tenant', NULL, true),
    ('Agent', 'agent', 'Answers conversations and tickets of their teams', true, 'tenant', NULL, true),
    ('Light agent', 'light_agent', 'Reads conversations of their teams and writes internal notes only (free seat)', true, 'tenant', NULL, false),
    ('Billing', 'billing', 'Manages subscription, invoices and payment methods (free seat)', true, 'tenant', NULL, false),
    ('Viewer', 'viewer', 'Read-only access to conversations, tickets, contacts and reports (free seat)', true, 'tenant', NULL, false)
ON CONFLICT (slug) WHERE scope = 'tenant' AND organization_id IS NULL DO NOTHING;

-- Free seats among the existing templates: the basic member role is billable
-- (it is the default role of invitations), owner and admin are billable.

-- owner: every tenant permission of the catalog.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.%'
WHERE r.slug = 'owner' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- admin: every tenant permission except role management.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.%' AND p.slug <> 'tenant.roles.write'
WHERE r.slug = 'admin' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.members.read', 'tenant.roles.read',
    'tenant.attachments.read', 'tenant.attachments.write',
    'tenant.conversations.read', 'tenant.conversations.read_all', 'tenant.conversations.reply',
    'tenant.conversations.note', 'tenant.conversations.assign', 'tenant.conversations.manage',
    'tenant.conversations.export',
    'tenant.tickets.read', 'tenant.tickets.read_all', 'tenant.tickets.write', 'tenant.tickets.assign',
    'tenant.tickets.export',
    'tenant.contacts.read', 'tenant.contacts.write', 'tenant.contacts.export', 'tenant.contacts.import',
    'tenant.channels.read', 'tenant.teams.read', 'tenant.teams.write',
    'tenant.reports.read', 'tenant.reports.export',
    'tenant.ai.use', 'tenant.ai.read', 'tenant.automations.read',
    'tenant.knowledge.read', 'tenant.knowledge.write', 'tenant.knowledge.publish',
    'tenant.macros.read', 'tenant.macros.write', 'tenant.sla.read'
)
WHERE r.slug = 'supervisor' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.members.read',
    'tenant.attachments.read', 'tenant.attachments.write',
    'tenant.conversations.read', 'tenant.conversations.reply', 'tenant.conversations.note',
    'tenant.conversations.assign', 'tenant.conversations.manage',
    'tenant.tickets.read', 'tenant.tickets.write', 'tenant.tickets.assign',
    'tenant.contacts.read', 'tenant.contacts.write',
    'tenant.channels.read', 'tenant.teams.read',
    'tenant.ai.use', 'tenant.knowledge.read', 'tenant.macros.read', 'tenant.sla.read'
)
WHERE r.slug = 'agent' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.members.read',
    'tenant.attachments.read', 'tenant.attachments.write',
    'tenant.conversations.read', 'tenant.conversations.note',
    'tenant.tickets.read', 'tenant.contacts.read', 'tenant.teams.read',
    'tenant.knowledge.read', 'tenant.macros.read'
)
WHERE r.slug = 'light_agent' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.members.read',
    'tenant.billing.read', 'tenant.billing.write', 'tenant.reports.read'
)
WHERE r.slug = 'billing' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.members.read', 'tenant.attachments.read',
    'tenant.conversations.read', 'tenant.conversations.read_all',
    'tenant.tickets.read', 'tenant.tickets.read_all',
    'tenant.contacts.read', 'tenant.channels.read', 'tenant.teams.read',
    'tenant.reports.read', 'tenant.knowledge.read', 'tenant.sla.read'
)
WHERE r.slug = 'viewer' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
