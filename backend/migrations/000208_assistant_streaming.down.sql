-- Data-loss note: interrupted flags, guide preferences, shared setup claims and title usage are lost.
DELETE FROM assistant_usage WHERE kind = 'title';
ALTER TABLE assistant_usage DROP CONSTRAINT assistant_usage_kind_chk;
ALTER TABLE assistant_usage ADD CONSTRAINT assistant_usage_kind_chk CHECK (kind IN ('chat', 'guide_generate'));
ALTER TABLE users DROP COLUMN guides_auto_start;
ALTER TABLE assistant_messages DROP COLUMN interrupted;

DELETE FROM guide_progress WHERE guide_key='__setup__';
DROP FUNCTION IF EXISTS guide_progress_topic(TEXT);
