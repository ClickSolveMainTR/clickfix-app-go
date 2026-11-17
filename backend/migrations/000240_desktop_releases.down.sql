-- Data-loss note: desktop_releases satırları (sürüm kayıtları ve updater
-- imzaları) silinir. Nesne depolamadaki paketler silinmez; desktop/releases/
-- altında kalır.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'platform.desktop_releases.manage');
DELETE FROM permissions WHERE slug = 'platform.desktop_releases.manage';

DROP TABLE IF EXISTS desktop_releases;
