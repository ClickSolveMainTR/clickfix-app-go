-- Data-loss note: drops health scores, their history and the settings.
DELETE FROM notification_templates WHERE code = 'company.health_dropped';
DROP TABLE IF EXISTS company_health_history;
DROP TABLE IF EXISTS company_health;
DROP TABLE IF EXISTS company_health_settings;
