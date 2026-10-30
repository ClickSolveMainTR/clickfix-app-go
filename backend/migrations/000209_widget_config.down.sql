-- Data-loss note: widget drafts, version history and install records are
-- dropped; the live settings on channel_instances stay.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.widgets.publish');
DELETE FROM permissions WHERE slug = 'tenant.widgets.publish';
DROP TABLE IF EXISTS widget_installs;
DROP TABLE IF EXISTS widget_versions;
DROP TABLE IF EXISTS widget_drafts;
