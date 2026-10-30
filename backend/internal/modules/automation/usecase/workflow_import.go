package usecase

import (
	"context"
	"encoding/json"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	routingmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/model"
	routing "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
)

func (s *Workflows) ImportRouting(ctx context.Context, org, actor int64, id uuid.UUID) (db.Workflow, error) {
	rule, err := s.Store.Q.GetRoutingRuleByUUID(ctx, db.GetRoutingRuleByUUIDParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return db.Workflow{}, err
	}
	var conditions routingmodel.Conditions
	if err = json.Unmarshal(rule.Conditions, &conditions); err != nil {
		return db.Workflow{}, err
	}
	settings, err := routing.New(s.Store.Pool, s.Store.Q, nil, nil).Settings(ctx, org)
	if err != nil {
		return db.Workflow{}, err
	}
	snap := routing.WorkflowSnapshot{Settings: settings, Skills: rule.Skills}
	if rule.TeamID.Valid {
		teams, e := s.Store.Q.ListRoutingTeams(ctx, org)
		if e != nil {
			return db.Workflow{}, e
		}
		for _, t := range teams {
			if t.ID == rule.TeamID.Int64 {
				v := t.Uuid
				snap.Team = &v
			}
		}
	}
	raw, _ := json.Marshal(snap)
	all := []model.Condition{}
	addIn := func(field string, values []string) {
		if len(values) > 0 {
			v := []any{}
			for _, x := range values {
				v = append(v, x)
			}
			all = append(all, model.Condition{Field: field, Operator: "in", Value: v})
		}
	}
	addIn("channel_type", conditions.ChannelTypes)
	addIn("priority", conditions.Priorities)
	if len(conditions.TagUUIDs) > 0 {
		anyTags := []model.Condition{}
		for _, tag := range conditions.TagUUIDs {
			anyTags = append(anyTags, model.Condition{Field: "tag_uuids", Operator: "contains", Value: tag.String()})
		}
		all = append(all, model.Condition{Any: anyTags})
	}
	c := model.Condition{All: all}
	graph := model.Graph{SchemaVersion: 1, Nodes: []model.Node{{ID: "trigger", Type: "trigger", Trigger: events.ConversationCreated},
		{ID: "condition", Type: "condition", Condition: &c}, {ID: "route", Type: "action", Action: &model.Action{Type: "route", Content: string(raw)}}, {ID: "end", Type: "end"}},
		Edges: []model.Edge{{From: "trigger", Port: "next", To: "condition", Input: "in"}, {From: "condition", Port: "true", To: "route", Input: "in"}, {From: "condition", Port: "false", To: "end", Input: "in"}, {From: "route", Port: "next", To: "end", Input: "in"}}}
	row, err := s.Save(ctx, org, actor, uuid.Nil, model.WorkflowInput{Name: rule.Name, Graph: graph})
	if err != nil {
		return row, err
	}
	row, err = s.publish(ctx, org, actor, row.Uuid, row.Revision, false)
	if err != nil {
		return row, err
	}
	// Import is opt-in and published but disabled, so existing routing never runs twice by surprise.
	return row, nil
}
