-- Data-loss note: drops every team, team membership and member skill tag.
DROP INDEX IF EXISTS idx_organization_members_skills;
ALTER TABLE organization_members DROP COLUMN IF EXISTS skills;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
