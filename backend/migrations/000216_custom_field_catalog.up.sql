-- CF-196: custom fields are live the moment they are defined.
--   * visibility / sensitive / widget_access on each definition: which
--     surfaces read the value, whether it is masked and kept out of AI,
--     exports and reports, and whether the widget SDK may write it by key
--     (no attribute mapping and no publish step).
--   * custom_field_unknown_keys: keys the widget sent that no field defines,
--     shown as "tanımsız alan geldi" warnings with a one-click definition.
--   * custom_field_unknown_values: the received values of those keys per
--     contact, applied to custom_fields when the field gets defined.
--   * knowledge_article_audiences: catalog conditions (option ids) that pick
--     which widget visitors are offered the article.
--   * select values stored as an option label ("Pro Paket") or with another
--     letter case are rewritten to the option id ("pro").

ALTER TABLE custom_field_definitions
    ADD COLUMN visibility    JSONB   NOT NULL DEFAULT '{"agent":true,"ai":true,"workflow":true,"reports":true}'::jsonb,
    ADD COLUMN sensitive     BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN widget_access TEXT    NOT NULL DEFAULT 'verified',
    ADD CONSTRAINT chk_custom_field_definitions_widget_access CHECK (widget_access IN ('none', 'verified', 'anonymous'));

CREATE TABLE custom_field_unknown_keys (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    entity_type         TEXT         NOT NULL DEFAULT 'contact',
    key                 TEXT         NOT NULL,
    source              TEXT         NOT NULL DEFAULT 'widget',
    channel_instance_id BIGINT       NULL REFERENCES channel_instances (id) ON DELETE SET NULL,
    value_kind          TEXT         NOT NULL DEFAULT 'text',
    hits                BIGINT       NOT NULL DEFAULT 1,
    last_contact_id     BIGINT       NULL REFERENCES contacts (id) ON DELETE SET NULL,
    first_seen_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_seen_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    dismissed_at        TIMESTAMPTZ  NULL,
    resolved_at         TIMESTAMPTZ  NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_custom_field_unknown_keys_uuid UNIQUE (uuid),
    CONSTRAINT uq_custom_field_unknown_keys_key UNIQUE (organization_id, entity_type, key),
    CONSTRAINT chk_custom_field_unknown_keys_entity CHECK (entity_type IN ('contact', 'company')),
    CONSTRAINT chk_custom_field_unknown_keys_source CHECK (source IN ('widget', 'api')),
    CONSTRAINT chk_custom_field_unknown_keys_kind CHECK (value_kind IN ('text', 'number', 'boolean', 'date')),
    CONSTRAINT chk_custom_field_unknown_keys_key CHECK (char_length(key) BETWEEN 1 AND 100)
);

CREATE INDEX idx_custom_field_unknown_keys_open
    ON custom_field_unknown_keys (organization_id, last_seen_at DESC)
    WHERE dismissed_at IS NULL AND resolved_at IS NULL;

CREATE TRIGGER trg_custom_field_unknown_keys_set_updated_at
    BEFORE UPDATE ON custom_field_unknown_keys
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE custom_field_unknown_values (
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    contact_id      BIGINT       NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    key             TEXT         NOT NULL,
    value           JSONB        NOT NULL,
    verified        BOOLEAN      NOT NULL DEFAULT FALSE,
    received_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_custom_field_unknown_values PRIMARY KEY (contact_id, key),
    CONSTRAINT chk_custom_field_unknown_values_key CHECK (char_length(key) BETWEEN 1 AND 100)
);

CREATE INDEX idx_custom_field_unknown_values_key ON custom_field_unknown_values (organization_id, key);

CREATE TABLE knowledge_article_audiences (
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    article_id      BIGINT       NOT NULL REFERENCES knowledge_articles (id) ON DELETE CASCADE,
    conditions      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_knowledge_article_audiences PRIMARY KEY (article_id)
);

CREATE INDEX idx_knowledge_article_audiences_org ON knowledge_article_audiences (organization_id);

-- Free-text select values → option ids (exact id rows are left alone).
UPDATE contacts c
SET custom_fields = c.custom_fields || jsonb_build_object(d.key, o.value)
FROM custom_field_definitions d
CROSS JOIN LATERAL jsonb_to_recordset(d.options) AS o(value TEXT, label TEXT)
WHERE d.organization_id = c.organization_id AND d.entity_type = 'contact' AND d.field_type = 'select'
  AND jsonb_typeof(c.custom_fields -> d.key) = 'string'
  AND (lower(trim(c.custom_fields ->> d.key)) = lower(o.value) OR lower(trim(c.custom_fields ->> d.key)) = lower(trim(o.label)))
  AND NOT EXISTS (SELECT 1 FROM jsonb_to_recordset(d.options) AS x(value TEXT) WHERE x.value = c.custom_fields ->> d.key);

UPDATE companies c
SET custom_fields = c.custom_fields || jsonb_build_object(d.key, o.value)
FROM custom_field_definitions d
CROSS JOIN LATERAL jsonb_to_recordset(d.options) AS o(value TEXT, label TEXT)
WHERE d.organization_id = c.organization_id AND d.entity_type = 'company' AND d.field_type = 'select'
  AND jsonb_typeof(c.custom_fields -> d.key) = 'string'
  AND (lower(trim(c.custom_fields ->> d.key)) = lower(o.value) OR lower(trim(c.custom_fields ->> d.key)) = lower(trim(o.label)))
  AND NOT EXISTS (SELECT 1 FROM jsonb_to_recordset(d.options) AS x(value TEXT) WHERE x.value = c.custom_fields ->> d.key);

UPDATE conversations c
SET custom_fields = c.custom_fields || jsonb_build_object(d.key, o.value)
FROM custom_field_definitions d
CROSS JOIN LATERAL jsonb_to_recordset(d.options) AS o(value TEXT, label TEXT)
WHERE d.organization_id = c.organization_id AND d.entity_type = 'conversation' AND d.field_type = 'select'
  AND jsonb_typeof(c.custom_fields -> d.key) = 'string'
  AND (lower(trim(c.custom_fields ->> d.key)) = lower(o.value) OR lower(trim(c.custom_fields ->> d.key)) = lower(trim(o.label)))
  AND NOT EXISTS (SELECT 1 FROM jsonb_to_recordset(d.options) AS x(value TEXT) WHERE x.value = c.custom_fields ->> d.key);
