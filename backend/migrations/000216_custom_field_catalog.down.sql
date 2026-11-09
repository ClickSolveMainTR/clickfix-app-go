-- Data-loss note: unknown-key warnings, received unknown values, article
-- audiences and the visibility/sensitive/widget_access settings are dropped.
-- Select values rewritten from labels to option ids stay as ids.
DROP TABLE IF EXISTS knowledge_article_audiences;
DROP TABLE IF EXISTS custom_field_unknown_values;
DROP TABLE IF EXISTS custom_field_unknown_keys;
ALTER TABLE custom_field_definitions
    DROP CONSTRAINT IF EXISTS chk_custom_field_definitions_widget_access,
    DROP COLUMN IF EXISTS widget_access,
    DROP COLUMN IF EXISTS sensitive,
    DROP COLUMN IF EXISTS visibility;
