-- CF-13: guided set-up of a new organization (kurulum sihirbazı). The owner
-- opens the organization, connects a first channel, invites the team and
-- installs the web widget, aiming at a first customer message within ten
-- minutes.
--
-- Channels, members, invitations and conversations are counted from their
-- own tables; steps holds the steps confirmed by hand (e.g. "widget
-- installed"). finished_at is set when the owner completes or dismisses
-- the checklist; the panel stops showing it then. Only organizations
-- opened through self-serve sign-up get a row, so existing organizations
-- never see the wizard.

CREATE TABLE organization_onboarding (
    organization_id BIGINT PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    steps           TEXT[]      NOT NULL DEFAULT '{}',
    finished_at     TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_organization_onboarding_updated_at
    BEFORE UPDATE ON organization_onboarding
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
