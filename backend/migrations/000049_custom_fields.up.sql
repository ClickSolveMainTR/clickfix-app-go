-- CF-7: organization-defined custom fields and custom objects.
-- Values live in the owning row's custom_fields JSONB (contacts, companies,
-- conversations; tickets add the same column) keyed by definition key.

CREATE TABLE custom_object_types (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    key             TEXT         NOT NULL,
    singular_label  TEXT         NOT NULL,
    plural_label    TEXT         NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    icon            TEXT         NOT NULL DEFAULT '',
    archived_at     TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_custom_object_types_uuid UNIQUE (uuid),
    CONSTRAINT uq_custom_object_types_org_key UNIQUE (organization_id, key),
    CONSTRAINT chk_custom_object_types_key CHECK (key ~ '^[a-z][a-z0-9_]{0,62}$')
);

CREATE INDEX idx_custom_object_types_organization ON custom_object_types (organization_id);

CREATE TRIGGER trg_custom_object_types_set_updated_at
    BEFORE UPDATE ON custom_object_types
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE custom_field_definitions (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    entity_type     TEXT         NOT NULL,
    object_type_id  BIGINT       NULL REFERENCES custom_object_types (id) ON DELETE CASCADE,
    key             TEXT         NOT NULL,
    label           TEXT         NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    field_type      TEXT         NOT NULL,
    required        BOOLEAN      NOT NULL DEFAULT FALSE,
    options         JSONB        NOT NULL DEFAULT '[]'::jsonb,
    config          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    default_value   JSONB        NULL,
    condition       JSONB        NULL,
    position        INT          NOT NULL DEFAULT 0,
    archived_at     TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_custom_field_definitions_uuid UNIQUE (uuid),
    CONSTRAINT uq_custom_field_definitions_key UNIQUE NULLS NOT DISTINCT (organization_id, entity_type, object_type_id, key),
    CONSTRAINT chk_custom_field_definitions_key CHECK (key ~ '^[a-z][a-z0-9_]{0,62}$'),
    CONSTRAINT chk_custom_field_definitions_entity CHECK (
        (entity_type IN ('contact', 'company', 'conversation', 'ticket') AND object_type_id IS NULL)
        OR (entity_type = 'object' AND object_type_id IS NOT NULL)
    ),
    CONSTRAINT chk_custom_field_definitions_type CHECK (field_type IN (
        'text', 'textarea', 'number', 'money', 'date', 'boolean',
        'select', 'multiselect', 'url', 'email', 'phone', 'user'
    ))
);

CREATE INDEX idx_custom_field_definitions_scope
    ON custom_field_definitions (organization_id, entity_type, object_type_id, position);

CREATE TRIGGER trg_custom_field_definitions_set_updated_at
    BEFORE UPDATE ON custom_field_definitions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE custom_object_records (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    object_type_id  BIGINT       NOT NULL REFERENCES custom_object_types (id) ON DELETE CASCADE,
    display_name    TEXT         NOT NULL,
    custom_fields   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_custom_object_records_uuid UNIQUE (uuid)
);

CREATE INDEX idx_custom_object_records_type
    ON custom_object_records (organization_id, object_type_id, created_at DESC);
CREATE INDEX idx_custom_object_records_custom_fields
    ON custom_object_records USING GIN (custom_fields jsonb_path_ops);

CREATE TRIGGER trg_custom_object_records_set_updated_at
    BEFORE UPDATE ON custom_object_records
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- contacts and conversations got their GIN index in CF-25; companies did not.
CREATE INDEX idx_companies_custom_fields ON companies USING GIN (custom_fields jsonb_path_ops);

INSERT INTO permissions (name, slug) VALUES
    ('Read custom field definitions', 'tenant.custom_fields.read'),
    ('Manage custom fields and object types', 'tenant.custom_fields.manage'),
    ('Read custom object records', 'tenant.custom_objects.read'),
    ('Write custom object records', 'tenant.custom_objects.write')
ON CONFLICT (slug) DO NOTHING;

-- Every support role renders fields and reads records; roles that edit
-- conversations and tickets also edit records; owners and admins define them.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.custom_fields.read', 'tenant.custom_objects.read')
WHERE r.scope = 'tenant' AND r.organization_id IS NULL
  AND r.slug IN ('owner', 'admin', 'member', 'supervisor', 'agent', 'light_agent', 'viewer')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.custom_objects.write'
WHERE r.scope = 'tenant' AND r.organization_id IS NULL
  AND r.slug IN ('owner', 'admin', 'member', 'supervisor', 'agent')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.custom_fields.manage'
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin')
ON CONFLICT DO NOTHING;
