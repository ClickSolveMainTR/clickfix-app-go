DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.custom_fields.read', 'tenant.custom_fields.manage',
        'tenant.custom_objects.read', 'tenant.custom_objects.write'
    )
);
DELETE FROM permissions WHERE slug IN (
    'tenant.custom_fields.read', 'tenant.custom_fields.manage',
    'tenant.custom_objects.read', 'tenant.custom_objects.write'
);

DROP INDEX IF EXISTS idx_companies_custom_fields;
DROP TABLE IF EXISTS custom_object_records;
DROP TABLE IF EXISTS custom_field_definitions;
DROP TABLE IF EXISTS custom_object_types;
