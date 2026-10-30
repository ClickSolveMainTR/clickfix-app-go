-- CF-3: multi-role memberships and multi-role invitations (ported from
-- rentopia-app-go 000041 / 000058 without branch scoping).
--
-- organization_member_roles is the source of tenant permissions: a member's
-- permissions are the union over all of its roles. organization_members.role_id
-- stays the member's primary (display) role; it is always one of the member's
-- roles and the use case keeps it in sync.

ALTER TABLE organization_members
    -- Target of the composite foreign keys below.
    ADD CONSTRAINT uq_organization_members_organization_id UNIQUE (organization_id, id);

CREATE TABLE organization_member_roles (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    member_id       BIGINT      NOT NULL,
    role_id         BIGINT      NOT NULL REFERENCES roles (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_organization_member_roles PRIMARY KEY (organization_id, member_id, role_id),
    CONSTRAINT fk_organization_member_roles_member
        FOREIGN KEY (organization_id, member_id)
        REFERENCES organization_members (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_organization_member_roles_organization_role
    ON organization_member_roles (organization_id, role_id);

CREATE OR REPLACE FUNCTION organization_member_roles_scope_check()
RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roles r
        WHERE r.id = NEW.role_id
          AND r.scope = 'tenant'
          AND (r.organization_id IS NULL OR r.organization_id = NEW.organization_id)
    ) THEN
        RAISE EXCEPTION 'organization member role must be tenant-scoped for this organization'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_organization_member_roles_scope_check
    BEFORE INSERT OR UPDATE ON organization_member_roles
    FOR EACH ROW
    EXECUTE FUNCTION organization_member_roles_scope_check();

-- Every new membership starts with its primary role, whoever inserts it
-- (organization create, platform add member, invitation accept).
CREATE OR REPLACE FUNCTION organization_members_primary_role_sync()
RETURNS trigger AS $$
BEGIN
    INSERT INTO organization_member_roles (organization_id, member_id, role_id)
    VALUES (NEW.organization_id, NEW.id, NEW.role_id)
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_organization_members_primary_role_sync
    AFTER INSERT ON organization_members
    FOR EACH ROW
    EXECUTE FUNCTION organization_members_primary_role_sync();

INSERT INTO organization_member_roles (organization_id, member_id, role_id)
SELECT organization_id, id, role_id
FROM organization_members
ON CONFLICT DO NOTHING;

-- Invitations: several roles, who accepted / revoked, resend counters and one
-- open invitation per e-mail and organization.
ALTER TABLE organization_invitations
    ADD COLUMN accepted_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN revoked_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN last_sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN send_count INTEGER NOT NULL DEFAULT 1,
    ADD CONSTRAINT uq_organization_invitations_organization_id UNIQUE (organization_id, id);

UPDATE organization_invitations oi
SET revoked_at = NOW()
WHERE oi.accepted_at IS NULL AND oi.revoked_at IS NULL
  AND EXISTS (
    SELECT 1 FROM organization_invitations newer
    WHERE newer.organization_id = oi.organization_id
      AND newer.email = oi.email
      AND newer.accepted_at IS NULL AND newer.revoked_at IS NULL
      AND (newer.created_at, newer.id) > (oi.created_at, oi.id)
  );

DROP INDEX IF EXISTS idx_organization_invitations_pending;
CREATE UNIQUE INDEX uq_organization_invitations_open
    ON organization_invitations (organization_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

CREATE TABLE organization_invitation_roles (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    invitation_id   BIGINT      NOT NULL,
    role_id         BIGINT      NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_organization_invitation_roles PRIMARY KEY (organization_id, invitation_id, role_id),
    CONSTRAINT fk_organization_invitation_roles_invitation
        FOREIGN KEY (organization_id, invitation_id)
        REFERENCES organization_invitations (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_organization_invitation_roles_organization_role
    ON organization_invitation_roles (organization_id, role_id);

CREATE TRIGGER trg_organization_invitation_roles_scope_check
    BEFORE INSERT OR UPDATE ON organization_invitation_roles
    FOR EACH ROW
    EXECUTE FUNCTION organization_member_roles_scope_check();

INSERT INTO organization_invitation_roles (organization_id, invitation_id, role_id)
SELECT organization_id, id, role_id
FROM organization_invitations
ON CONFLICT DO NOTHING;
