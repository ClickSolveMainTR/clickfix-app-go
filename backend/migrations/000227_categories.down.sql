-- Data-loss note: drops the category tree, every record's category and the
-- collected training labels of every organization.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.categories.read', 'tenant.categories.manage', 'tenant.categories.label', 'tenant.categories.export'));
DELETE FROM permissions WHERE slug IN ('tenant.categories.read', 'tenant.categories.manage', 'tenant.categories.label', 'tenant.categories.export');
DROP TABLE IF EXISTS category_labels;
DROP FUNCTION IF EXISTS category_labels_immutable();
DROP TABLE IF EXISTS category_assignments;
DROP TABLE IF EXISTS category_examples;
DROP TABLE IF EXISTS categories;
