package usecase

import (
	"context"
	"encoding/json"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/model"
	"github.com/google/uuid"
)

// WorkflowSnapshot freezes routing configuration; capacity and presence stay live.
type WorkflowSnapshot struct {
	Settings model.Settings `json:"settings"`
	Team     *uuid.UUID     `json:"team_uuid"`
	Skills   []string       `json:"skills"`
}

func (s *Service) RouteWorkflow(ctx context.Context, org int64, conversation uuid.UUID, raw string) error {
	var snap WorkflowSnapshot
	if json.Unmarshal([]byte(raw), &snap) != nil {
		return ErrInvalid
	}
	c, err := s.q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: conversation})
	if err != nil {
		return err
	}
	rule := &db.ListRoutingRulesRow{Skills: snap.Skills}
	if snap.Team != nil {
		team, e := s.q.GetTeamByUUIDForRouting(ctx, db.GetTeamByUUIDForRoutingParams{OrganizationID: org, Uuid: *snap.Team})
		if e != nil {
			return e
		}
		rule.TeamID.Int64, rule.TeamID.Valid = team.ID, true
	}
	_, err = s.route(ctx, org, c.ID, ReasonNew, 0, &snap.Settings, rule)
	return err
}
