-- CF-14: time-limited support access. An organization owner / admin grants
-- ClickFix support staff access to the organization for a limited time
-- (read-only or full); platform staff with platform.support.access then work
-- inside it under their own account. Every action is recorded in the
-- organization's audit log with via_support = true.

CREATE TABLE support_access_grants (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    mode            VARCHAR(16)  NOT NULL,
    reason          TEXT         NOT NULL DEFAULT '',
    granted_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    starts_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ  NOT NULL,
    revoked_at      TIMESTAMPTZ  NULL,
    revoked_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_support_access_grants_uuid UNIQUE (uuid),
    CONSTRAINT chk_support_access_grants_mode CHECK (mode IN ('read_only', 'full')),
    CONSTRAINT chk_support_access_grants_window CHECK (expires_at > starts_at)
);

CREATE INDEX idx_support_access_grants_organization_created
    ON support_access_grants (organization_id, created_at DESC);
CREATE INDEX idx_support_access_grants_open
    ON support_access_grants (expires_at)
    WHERE revoked_at IS NULL;

INSERT INTO permissions (name, slug) VALUES
    ('Read support access grants', 'tenant.support_access.read'),
    ('Grant and revoke support access', 'tenant.support_access.manage'),
    ('Enter organizations with a support access grant', 'platform.support.access')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.support_access.read', 'tenant.support_access.manage')
WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug IN ('owner', 'admin')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'platform.support.access'
WHERE r.scope = 'platform' AND r.slug = 'super_admin'
ON CONFLICT DO NOTHING;
