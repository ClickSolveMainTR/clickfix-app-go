DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('tenant.attachments.read', 'tenant.attachments.write')
);
DELETE FROM permissions WHERE slug IN ('tenant.attachments.read', 'tenant.attachments.write');

DROP TABLE IF EXISTS attachments;
