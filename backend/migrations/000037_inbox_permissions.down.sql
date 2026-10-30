DELETE FROM permissions WHERE slug IN (
    'tenant.channels.read', 'tenant.channels.manage', 'tenant.conversations.read', 'tenant.conversations.reply',
    'tenant.conversations.manage', 'tenant.contacts.read', 'tenant.contacts.write');
