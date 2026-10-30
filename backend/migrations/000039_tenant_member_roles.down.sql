-- Data-loss note: drops extra member / invitation roles (each keeps its
-- primary role_id), who accepted / revoked invitations and resend counters.
DROP TRIGGER IF EXISTS trg_organization_invitation_roles_scope_check ON organization_invitation_roles;
DROP TABLE IF EXISTS organization_invitation_roles;

DROP INDEX IF EXISTS uq_organization_invitations_open;
CREATE INDEX idx_organization_invitations_pending
    ON organization_invitations (organization_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

ALTER TABLE organization_invitations
    DROP CONSTRAINT IF EXISTS uq_organization_invitations_organization_id,
    DROP COLUMN IF EXISTS send_count,
    DROP COLUMN IF EXISTS last_sent_at,
    DROP COLUMN IF EXISTS revoked_by,
    DROP COLUMN IF EXISTS accepted_by;

DROP TRIGGER IF EXISTS trg_organization_members_primary_role_sync ON organization_members;
DROP FUNCTION IF EXISTS organization_members_primary_role_sync();
DROP TRIGGER IF EXISTS trg_organization_member_roles_scope_check ON organization_member_roles;
DROP TABLE IF EXISTS organization_member_roles;
DROP FUNCTION IF EXISTS organization_member_roles_scope_check();

ALTER TABLE organization_members
    DROP CONSTRAINT IF EXISTS uq_organization_members_organization_id;
