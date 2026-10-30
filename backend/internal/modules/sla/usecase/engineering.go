package usecase

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/engine"
	"time"
)

// engineeringHandoff keeps an explicit escalation's resolution target when a
// later inbox event reconciles the ordinary organization policy. Other clocks
// still follow that policy; handoffs use elapsed time and pause with the inbox.
func engineeringHandoff(ctx context.Context, q *db.Queries, conv db.Conversation, stored map[string]engine.Clock, res engine.Result, at time.Time) (engine.Result, error) {
	handoff, err := q.EngineeringLatestEscalation(ctx, db.EngineeringLatestEscalationParams{OrganizationID: conv.OrganizationID, ConversationID: conv.ID})
	if isNoRows(err) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	target := handoff.DueAt.Time.Sub(handoff.CreatedAt.Time).Truncate(time.Second)
	if target <= 0 {
		return res, nil
	}
	resolution := map[string]engine.Clock{}
	if c, ok := stored[engine.Resolution]; ok {
		resolution[engine.Resolution] = c
	}
	if at.Before(handoff.CreatedAt.Time) {
		at = handoff.CreatedAt.Time
	}
	override := engine.Plan(toEngineConversation(conv), resolution, &engine.Policy{Targets: engine.Targets{Resolution: target}, WarningPercent: 80}, at)
	deletes := res.Delete[:0]
	for _, metric := range res.Delete {
		if metric != engine.Resolution {
			deletes = append(deletes, metric)
		}
	}
	res.Delete = deletes
	upserts := res.Upsert[:0]
	for _, clock := range res.Upsert {
		if clock.Metric != engine.Resolution {
			upserts = append(upserts, clock)
		}
	}
	res.Upsert = upserts
	transitions := res.Transitions[:0]
	for _, tr := range res.Transitions {
		if tr.Metric != engine.Resolution {
			transitions = append(transitions, tr)
		}
	}
	res.Transitions = transitions
	if c, ok := override.Clocks[engine.Resolution]; ok {
		c.PolicyID = nil
		res.Clocks[engine.Resolution] = c
	} else {
		delete(res.Clocks, engine.Resolution)
	}
	for _, clock := range override.Upsert {
		clock.PolicyID = nil
		res.Upsert = append(res.Upsert, clock)
	}
	for _, tr := range override.Transitions {
		tr.Clock.PolicyID = nil
		res.Transitions = append(res.Transitions, tr)
	}
	return res, nil
}
