-- Data-loss note: drops every widget token (widgets fall back to "open the
-- app" until a new token is issued), every Live Activity push token and the
-- users' Live Activity switch.
DROP TABLE IF EXISTS mobile_surface_preferences;
DROP TABLE IF EXISTS live_activity_tokens;
DROP TABLE IF EXISTS widget_tokens;
