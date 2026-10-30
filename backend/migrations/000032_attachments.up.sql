-- CF-11: organization-scoped file attachments. Bytes live in object storage
-- (platform/storage); this table is the metadata, ownership and quota ledger.
-- owner_type/owner_id is a polymorphic reference so conversations, messages and
-- tickets can attach files later without schema changes.
CREATE TABLE attachments (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    uploaded_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    owner_type      TEXT         NULL,
    owner_id        UUID         NULL,
    filename        TEXT         NOT NULL,
    content_type    TEXT         NOT NULL,
    size_bytes      BIGINT       NOT NULL,
    sha256          TEXT         NOT NULL,
    object_key      TEXT         NOT NULL,
    thumb_key       TEXT         NULL,
    scan_status     TEXT         NOT NULL DEFAULT 'skipped',
    scan_detail     TEXT         NULL,
    scanned_at      TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ  NULL,
    CONSTRAINT attachments_uuid_uq UNIQUE (uuid),
    CONSTRAINT attachments_size_chk CHECK (size_bytes >= 0),
    CONSTRAINT attachments_scan_status_chk CHECK (scan_status IN ('pending', 'clean', 'infected', 'skipped', 'error')),
    CONSTRAINT attachments_owner_chk CHECK ((owner_type IS NULL) = (owner_id IS NULL))
);

CREATE INDEX idx_attachments_org_created
    ON attachments (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_attachments_org_owner
    ON attachments (organization_id, owner_type, owner_id)
    WHERE deleted_at IS NULL AND owner_type IS NOT NULL;

INSERT INTO permissions (name, slug) VALUES
    ('Read tenant attachments', 'tenant.attachments.read'),
    ('Write tenant attachments', 'tenant.attachments.write')
ON CONFLICT (slug) DO NOTHING;

-- Every tenant template role can read and upload; agents attach files to
-- conversations and tickets. Deleting is covered by write.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.attachments.read', 'tenant.attachments.write')
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin', 'member')
ON CONFLICT DO NOTHING;
