package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/engine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// lockKey serializes routing per organization (capacity counts must not
// race between two conversations routed at the same moment).
func lockKey(orgID int64) int64 { return 0x52_4f_55_54<<32 | orgID&0xffffffff }

// Route assigns one conversation. from > 0 hands over a conversation from
// that agent (offline / unanswered; the agent is excluded); otherwise only
// open unassigned conversations are routed. It returns the new assignee (0
// when nobody is available; the conversation stays in its queue).
func (s *Service) Route(ctx context.Context, orgID, conversationID int64, reason string, from int64) (int64, error) {
	return s.route(ctx, orgID, conversationID, reason, from, nil, nil)
}

// route also accepts an explicitly imported workflow snapshot, sharing capacity and audit behavior.
func (s *Service) route(ctx context.Context, orgID, conversationID int64, reason string, from int64, override *model.Settings, imported *db.ListRoutingRulesRow) (int64, error) {
	if s.assigner == nil {
		return 0, nil
	}
	set, err := s.settings(ctx, orgID)
	if override != nil {
		set.Strategy = override.Strategy
		set.RespectCapacity = override.RespectCapacity
		set.VipAccountOwner = override.VIPAccountOwner
		set.Enabled = true
		err = nil
	}
	if err != nil || !set.Enabled {
		return 0, err
	}
	lock, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("routing: begin: %w", err)
	}
	defer func() { _ = lock.Rollback(ctx) }()
	if _, err := lock.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey(orgID)); err != nil {
		return 0, err
	}
	conv, err := s.q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: orgID, ID: conversationID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	if conv.Status != "open" {
		return 0, nil
	}
	if from > 0 {
		if conv.AssigneeUserID.Int64 != from {
			return 0, nil // reassigned meanwhile
		}
	} else if conv.AssigneeUserID.Valid {
		return 0, nil
	}

	team := conv.AssigneeTeamID
	rule := imported
	if !team.Valid && imported == nil {
		if rule, err = s.matchRule(ctx, conv); err != nil {
			return 0, err
		}
		if rule != nil && rule.TeamID.Valid {
			team = rule.TeamID
		}
	}
	if imported != nil && imported.TeamID.Valid && !team.Valid {
		team = imported.TeamID
	}
	snap, err := s.snapshot(ctx, s.q, orgID)
	if err != nil {
		return 0, err
	}
	req := engine.Request{Channel: conv.ChannelType, Strategy: set.Strategy, RespectCapacity: set.RespectCapacity}
	scope := "org"
	if team.Valid {
		req.Pool = snap.members[team.Int64]
		if req.Pool == nil {
			req.Pool = []int64{}
		}
		scope = "team:" + strconv.FormatInt(team.Int64, 10)
	}
	if rule != nil {
		req.Skills = rule.Skills
	}
	if from > 0 {
		req.Exclude = []int64{from}
	}
	if set.VipAccountOwner && conv.CompanyID.Valid {
		owner, err := s.q.GetCompanyAccountOwner(ctx, db.GetCompanyAccountOwnerParams{OrganizationID: orgID, ID: conv.CompanyID.Int64})
		if err == nil && owner.Valid {
			req.Preferred = owner.Int64
		}
	}
	if last, err := s.q.GetRoutingCursor(ctx, db.GetRoutingCursorParams{OrganizationID: orgID, ScopeKey: scope}); err == nil {
		req.LastUserID = last
	}
	pick, vip := engine.Pick(snap.agents, snap.limits, req)

	// A rule's team queue sticks even when nobody can take it yet.
	if team.Valid && !conv.AssigneeTeamID.Valid {
		if err := s.q.SetConversationTeam(ctx, db.SetConversationTeamParams{TeamID: team, OrganizationID: orgID, ID: conv.ID}); err != nil {
			return 0, err
		}
	}
	if pick == 0 {
		if reason == ReasonOffline {
			// The agent left: back to the queue rather than waiting on them.
			if err := s.assigner.Assign(ctx, orgID, conv.Uuid, nil); err != nil {
				return 0, err
			}
			if err := s.audit(ctx, conv, from, 0, team, rule, reason, set.Strategy, "no agent available; back to the queue"); err != nil {
				return 0, err
			}
		}
		return 0, lock.Commit(ctx)
	}
	i := slices.IndexFunc(snap.rows, func(r db.ListRoutingAgentsRow) bool { return r.UserID == pick })
	user := snap.rows[i].UserUuid
	if err := s.assigner.Assign(ctx, orgID, conv.Uuid, &user); err != nil {
		return 0, err
	}
	if vip {
		reason = ReasonVIP
	}
	if err := s.audit(ctx, conv, from, pick, team, rule, reason, set.Strategy, ""); err != nil {
		return 0, err
	}
	if !vip {
		if err := s.q.UpsertRoutingCursor(ctx, db.UpsertRoutingCursorParams{OrganizationID: orgID, ScopeKey: scope, LastUserID: pick}); err != nil {
			return 0, err
		}
	}
	return pick, lock.Commit(ctx)
}

func (s *Service) audit(ctx context.Context, conv db.Conversation, from, to int64, team pgtype.Int8, rule *db.ListRoutingRulesRow, reason, strategy, note string) error {
	p := db.InsertRoutingEventParams{
		OrganizationID: conv.OrganizationID, ConversationID: conv.ID, TeamID: team, Reason: reason, Strategy: strategy, Note: note,
	}
	if from > 0 {
		p.FromUserID = pgtype.Int8{Int64: from, Valid: true}
	}
	if to > 0 {
		p.ToUserID = pgtype.Int8{Int64: to, Valid: true}
	}
	if rule != nil && rule.ID > 0 {
		p.RuleID = pgtype.Int8{Int64: rule.ID, Valid: true}
	}
	return s.q.InsertRoutingEvent(ctx, p)
}

// matchRule returns the first active rule whose conditions fit.
func (s *Service) matchRule(ctx context.Context, conv db.Conversation) (*db.ListRoutingRulesRow, error) {
	rules, err := s.q.ListRoutingRules(ctx, conv.OrganizationID)
	if err != nil || len(rules) == 0 {
		return nil, err
	}
	var tags []uuid.UUID
	loaded := false
	for i := range rules {
		r := rules[i]
		if !r.IsActive {
			continue
		}
		var c model.Conditions
		if err := json.Unmarshal(r.Conditions, &c); err != nil {
			continue
		}
		if len(c.ChannelTypes) > 0 && !slices.Contains(c.ChannelTypes, conv.ChannelType) {
			continue
		}
		if len(c.Priorities) > 0 && !slices.Contains(c.Priorities, conv.Priority) {
			continue
		}
		if len(c.TagUUIDs) > 0 {
			if !loaded {
				if tags, err = s.q.ListRoutingConversationTagUUIDs(ctx, db.ListRoutingConversationTagUUIDsParams{OrganizationID: conv.OrganizationID, ConversationID: conv.ID}); err != nil {
					return nil, err
				}
				loaded = true
			}
			if !slices.ContainsFunc(c.TagUUIDs, func(u uuid.UUID) bool { return slices.Contains(tags, u) }) {
				continue
			}
		}
		return &r, nil
	}
	return nil, nil
}

// DrainQueue routes up to limit unassigned open conversations of an
// organization, oldest first, until nobody is available.
func (s *Service) DrainQueue(ctx context.Context, orgID int64, limit int32) int {
	ids, err := s.q.ListOrganizationQueue(ctx, db.ListOrganizationQueueParams{OrganizationID: orgID, MaxRows: limit})
	if err != nil {
		s.log.Warn("routing_queue_list_failed", "org", orgID, "error", err)
		return 0
	}
	n := 0
	for _, id := range ids {
		pick, err := s.Route(ctx, orgID, id, ReasonQueue, 0)
		if err != nil {
			s.log.Warn("routing_queue_route_failed", "org", orgID, "conversation", id, "error", err)
			continue
		}
		if pick > 0 {
			n++
		}
	}
	return n
}

// AgentOffline hands over the conversations waiting on an agent who went
// offline (when the organization asks for it).
func (s *Service) AgentOffline(ctx context.Context, orgID, userID int64) error {
	set, err := s.settings(ctx, orgID)
	if err != nil || !set.Enabled || !set.ReassignOnOffline {
		return err
	}
	ids, err := s.q.ListWaitingConversationsOfAgent(ctx, db.ListWaitingConversationsOfAgentParams{
		OrganizationID: orgID, AssigneeUserID: pgtype.Int8{Int64: userID, Valid: true},
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.Route(ctx, orgID, id, ReasonOffline, userID); err != nil {
			return err
		}
	}
	return nil
}

// Sweep drains the queues of organizations with routing on and hands over
// routed conversations left unanswered. Returns how many were assigned.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	n := 0
	queued, err := s.q.ListQueuedConversations(ctx, 200)
	if err != nil {
		return 0, err
	}
	full := map[int64]bool{}
	for _, c := range queued {
		if full[c.OrganizationID] {
			continue
		}
		pick, err := s.Route(ctx, c.OrganizationID, c.ID, ReasonQueue, 0)
		if err != nil {
			return n, err
		}
		if pick == 0 {
			full[c.OrganizationID] = true // nobody free there; try again next sweep
			continue
		}
		n++
	}
	stale, err := s.q.ListUnansweredRouted(ctx, db.ListUnansweredRoutedParams{Now: pgtype.Timestamptz{Time: s.now(), Valid: true}, MaxRows: 100})
	if err != nil {
		return n, err
	}
	for _, c := range stale {
		pick, err := s.Route(ctx, c.OrganizationID, c.ID, ReasonUnanswered, c.AssigneeUserID)
		if err != nil {
			return n, err
		}
		if pick > 0 {
			n++
		}
	}
	return n, nil
}

// HandleEvent reacts to conversation and presence events (durable consumer).
func (s *Service) HandleEvent(ctx context.Context, ev events.Event) error {
	if ev.TenantID == nil {
		return nil
	}
	org := *ev.TenantID
	if ev.Name == events.AgentPresenceChanged {
		p, err := events.DecodePayload[events.PresencePayload](ev)
		if err != nil {
			return err
		}
		switch p.Status {
		case "offline":
			return s.AgentOffline(ctx, org, p.UserID)
		case "online":
			s.DrainQueue(ctx, org, 20)
		}
		return nil
	}
	if ev.EntityType != "conversation" || ev.EntityID == nil {
		return nil
	}
	reason := ReasonNew
	if ev.Name != events.ConversationsCreated && ev.Name != events.ConversationCreated {
		reason = ReasonQueue
	}
	_, err := s.Route(ctx, org, *ev.EntityID, reason, 0)
	return err
}
