-- CF-100/101/105/106/108/113/114: scoped operations permissions.
INSERT INTO permissions(name,slug) VALUES
 ('Read feedback','tenant.feedback.read'),
 ('Create feedback','tenant.feedback.write'),
 ('Manage feedback and changelog','tenant.feedback.manage'),
 ('Read schedules','tenant.schedules.read'),
 ('Manage schedules','tenant.schedules.manage'),
 ('Use internal team chat','tenant.team_chat.use'),
 ('Read quality reviews','tenant.quality.read'),
 ('Manage quality reviews','tenant.quality.manage'),
 ('Read revenue reports','platform.revenue.read'),
 ('Read enterprise quotes','platform.quotes.read'),
 ('Manage enterprise quotes','platform.quotes.write') ON CONFLICT(slug) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id) SELECT r.id,p.id FROM roles r CROSS JOIN permissions p WHERE p.slug IN ('tenant.feedback.read','tenant.feedback.write','tenant.feedback.manage','tenant.schedules.read','tenant.schedules.manage','tenant.team_chat.use','tenant.quality.read','tenant.quality.manage','platform.revenue.read','platform.quotes.read','platform.quotes.write') AND ((r.scope='platform' AND r.slug='super_admin' AND p.slug LIKE 'platform.%') OR (r.scope='tenant' AND r.organization_id IS NULL AND r.slug IN ('owner','admin','supervisor') AND p.slug LIKE 'tenant.%') OR (r.scope='tenant' AND r.organization_id IS NULL AND r.slug IN ('member','agent','light_agent') AND p.slug IN ('tenant.feedback.read','tenant.feedback.write','tenant.schedules.read','tenant.team_chat.use'))) ON CONFLICT DO NOTHING;
