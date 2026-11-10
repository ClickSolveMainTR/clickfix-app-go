-- Data-loss note: revoking and re-creating app keys after a rollback is the
-- only way back; the keys themselves were never stored.
DROP TABLE IF EXISTS widget_app_keys;
