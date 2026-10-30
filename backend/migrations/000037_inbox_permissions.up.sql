-- CF-8: permissions of channels (inboxes), conversations and contacts.
INSERT INTO permissions (name, slug) VALUES
    ('Read channels', 'tenant.channels.read'),
    ('Connect and manage channels', 'tenant.channels.manage'),
    ('Read conversations', 'tenant.conversations.read'),
    ('Reply to conversations', 'tenant.conversations.reply'),
    ('Manage conversations', 'tenant.conversations.manage'),
    ('Read contacts', 'tenant.contacts.read'),
    ('Write contacts', 'tenant.contacts.write')
ON CONFLICT (slug) DO NOTHING;

-- Owners and admins get everything; members are agents: they read the
-- channels, work conversations and keep contacts up to date, but only
-- owners and admins connect or remove channels.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.channels.read', 'tenant.channels.manage', 'tenant.conversations.read', 'tenant.conversations.reply',
    'tenant.conversations.manage', 'tenant.contacts.read', 'tenant.contacts.write')
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.channels.read', 'tenant.conversations.read', 'tenant.conversations.reply',
    'tenant.conversations.manage', 'tenant.contacts.read', 'tenant.contacts.write')
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug = 'member'
ON CONFLICT DO NOTHING;
