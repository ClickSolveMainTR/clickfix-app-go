-- CF-150: web widget editor drafts, published versions with rollback and
-- install detection, plus the permission to publish widget settings.

-- One autosaved draft per widget; publishing copies it to
-- channel_instances.settings and records a version.
CREATE TABLE widget_drafts (
    id                  BIGSERIAL PRIMARY KEY,
    organization_id     BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_instance_id BIGINT NOT NULL,
    settings            JSONB NOT NULL DEFAULT '{}'::jsonb,
    base_version        INTEGER NOT NULL DEFAULT 0,
    updated_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT widget_drafts_instance_uq UNIQUE (channel_instance_id),
    FOREIGN KEY (organization_id, channel_instance_id) REFERENCES channel_instances(organization_id, id) ON DELETE CASCADE
);
CREATE TRIGGER trg_widget_drafts_updated BEFORE UPDATE ON widget_drafts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Immutable published snapshots (version 1, 2, … per widget).
CREATE TABLE widget_versions (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id     BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_instance_id BIGINT NOT NULL,
    version             INTEGER NOT NULL CHECK (version > 0),
    settings            JSONB NOT NULL,
    note                TEXT NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    rollback_of         INTEGER,
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT widget_versions_instance_version_uq UNIQUE (channel_instance_id, version),
    FOREIGN KEY (organization_id, channel_instance_id) REFERENCES channel_instances(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX widget_versions_org_idx ON widget_versions (organization_id, channel_instance_id, version DESC);

-- Where the widget was seen running (install checker): one row per page
-- origin, refreshed at most every few minutes by bootstrap.
CREATE TABLE widget_installs (
    id                  BIGSERIAL PRIMARY KEY,
    organization_id     BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_instance_id BIGINT NOT NULL,
    origin              TEXT NOT NULL CHECK (char_length(origin) <= 300),
    sdk_version         TEXT NOT NULL DEFAULT '' CHECK (char_length(sdk_version) <= 40),
    last_page_url       TEXT NOT NULL DEFAULT '' CHECK (char_length(last_page_url) <= 500),
    hits                BIGINT NOT NULL DEFAULT 1,
    first_seen_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT widget_installs_instance_origin_uq UNIQUE (channel_instance_id, origin),
    FOREIGN KEY (organization_id, channel_instance_id) REFERENCES channel_instances(organization_id, id) ON DELETE CASCADE
);

-- Existing widgets start their history with what is live today.
INSERT INTO widget_versions (organization_id, channel_instance_id, version, settings, note)
SELECT organization_id, id, 1, settings, 'initial'
FROM channel_instances
WHERE channel_type = 'widget' AND deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO permissions (name, slug) VALUES ('Publish web widget settings', 'tenant.widgets.publish')
ON CONFLICT (slug) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin')
  AND p.slug = 'tenant.widgets.publish'
ON CONFLICT DO NOTHING;
