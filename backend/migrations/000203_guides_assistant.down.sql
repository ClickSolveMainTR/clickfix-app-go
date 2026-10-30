-- Data-loss note: drops guides (incl. admin edits), tour progress, assistant
-- threads/usage and product feedback.
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('tenant.assistant.use', 'platform.assistant.manage', 'platform.guides.manage', 'platform.product_feedback.read'));
DELETE FROM permissions WHERE slug IN ('tenant.assistant.use', 'platform.assistant.manage', 'platform.guides.manage', 'platform.product_feedback.read');
DROP TABLE IF EXISTS product_feedback;
DROP TABLE IF EXISTS assistant_usage;
DROP TABLE IF EXISTS assistant_messages;
DROP TABLE IF EXISTS assistant_threads;
DROP TABLE IF EXISTS assistant_settings;
DROP TABLE IF EXISTS guide_progress;
DROP TABLE IF EXISTS guides;
