-- CF-5: teams and team membership.
--
-- A team groups organization members (L1, L2, Billing, Technical ...). A
-- member can be in several teams; one or more members lead a team. Teams
-- carry their own working hours (NULL = the organization's business hours)
-- and skill tags; members carry skill tags too (language, product area) so
-- routing can match conversations to people. Conversations and tickets of a
-- team are visible to its members (tenant.conversations.read without
-- read_all, see orgctx.Scope.CanSeeAll).

CREATE TABLE teams (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            VARCHAR(120) NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    color           VARCHAR(7)   NOT NULL DEFAULT '',
    timezone        VARCHAR(64)  NULL,
    working_hours   JSONB        NULL,
    skills          TEXT[]       NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_teams_uuid UNIQUE (uuid),
    CONSTRAINT uq_teams_organization_id UNIQUE (organization_id, id),
    CONSTRAINT chk_teams_color CHECK (color = '' OR color ~ '^#[0-9A-Fa-f]{6}$')
);

CREATE UNIQUE INDEX uq_teams_organization_name ON teams (organization_id, lower(name));
CREATE INDEX idx_teams_organization_created ON teams (organization_id, created_at DESC);

CREATE TRIGGER trg_teams_set_updated_at
    BEFORE UPDATE ON teams
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE team_members (
    organization_id BIGINT      NOT NULL,
    team_id         BIGINT      NOT NULL,
    user_id         BIGINT      NOT NULL,
    role            VARCHAR(16) NOT NULL DEFAULT 'member',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_team_members PRIMARY KEY (organization_id, team_id, user_id),
    CONSTRAINT chk_team_members_role CHECK (role IN ('lead', 'member')),
    CONSTRAINT fk_team_members_team
        FOREIGN KEY (organization_id, team_id) REFERENCES teams (organization_id, id) ON DELETE CASCADE,
    -- Leaving the organization removes the member from every team.
    CONSTRAINT fk_team_members_member
        FOREIGN KEY (organization_id, user_id)
        REFERENCES organization_members (organization_id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_team_members_organization_user ON team_members (organization_id, user_id);

-- Skill tags of a member in the organization (routing).
ALTER TABLE organization_members
    ADD COLUMN skills TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX idx_organization_members_skills ON organization_members USING GIN (skills);
CREATE INDEX idx_teams_skills ON teams USING GIN (skills);
