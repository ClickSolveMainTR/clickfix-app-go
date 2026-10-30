-- CF-44: conversations are assigned to teams (CF-5). The column existed
-- since CF-25 without a foreign key; a team that is deleted leaves its
-- conversations unassigned to a team.
UPDATE conversations c SET assignee_team_id = NULL
WHERE assignee_team_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM teams t WHERE t.organization_id = c.organization_id AND t.id = c.assignee_team_id);

ALTER TABLE conversations
    ADD CONSTRAINT conversations_team_fk FOREIGN KEY (organization_id, assignee_team_id)
        REFERENCES teams (organization_id, id) ON DELETE SET NULL (assignee_team_id);
