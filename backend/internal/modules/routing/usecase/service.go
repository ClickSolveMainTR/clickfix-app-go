// Package usecase implements automatic routing (CF-53) and agent capacity
// (CF-54).
//
// Route assigns one conversation: rules may send it to a team queue and
// require skills; a VIP company's account owner is tried first; otherwise
// the strategy (round-robin or load-balanced) picks among online agents
// (CF-10 presence) with room under their capacity limits. Every automatic
// assignment goes through the conversations service (timeline entry,
// events, notification) and is written to routing_events (audit trail).
// Routing runs on conversation events, on presence changes (an agent going
// offline hands over conversations waiting on them; coming online drains
// the queue) and from a periodic sweep (queue drain, unanswered reassign).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/engine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Routing reasons (routing_events.reason).
const (
	ReasonNew        = "new"
	ReasonQueue      = "queue"
	ReasonOffline    = "offline"
	ReasonUnanswered = "unanswered"
	ReasonVIP        = "vip"
)

// PresenceTimeout: an "online" agent without a heartbeat this long is
// treated as offline (matches CF-10 presence IdleTimeout).
const PresenceTimeout = 3 * time.Minute

// Errors returned by the service.
var (
	ErrNotFound = errors.New("routing: not found")
	ErrInvalid  = errors.New("routing: invalid request")
)

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Assigner changes a conversation's assignee through the conversations
// service (nil user unassigns).
type Assigner interface {
	Assign(ctx context.Context, orgID int64, conversation uuid.UUID, user *uuid.UUID) error
}

// Service is the routing use case.
type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	assigner Assigner
	log      *slog.Logger
	now      func() time.Time
}

// New builds the service; assigner may be nil in processes that only serve
// settings (routing then never assigns).
func New(pool *pgxpool.Pool, q *db.Queries, assigner Assigner, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{pool: pool, q: q, assigner: assigner, log: log, now: func() time.Time { return time.Now().UTC() }}
}

// WithClock replaces the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// ---------------------------------------------------------------------------
// Settings

var defaultSettings = db.RoutingSetting{
	Strategy: engine.RoundRobin, RespectCapacity: true, VipAccountOwner: true, ReassignOnOffline: true,
}

func (s *Service) settings(ctx context.Context, orgID int64) (db.RoutingSetting, error) {
	r, err := s.q.GetRoutingSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		d := defaultSettings
		d.OrganizationID = orgID
		return d, nil
	}
	return r, err
}

func toSettings(r db.RoutingSetting) model.Settings {
	out := model.Settings{
		Enabled: r.Enabled, Strategy: r.Strategy, RespectCapacity: r.RespectCapacity,
		VIPAccountOwner: r.VipAccountOwner, ReassignOnOffline: r.ReassignOnOffline,
	}
	if r.UnansweredMinutes.Valid {
		v := r.UnansweredMinutes.Int32
		out.UnansweredMinutes = &v
	}
	return out
}

// Settings returns the organization's routing settings (defaults when unset).
func (s *Service) Settings(ctx context.Context, orgID int64) (model.Settings, error) {
	r, err := s.settings(ctx, orgID)
	if err != nil {
		return model.Settings{}, err
	}
	return toSettings(r), nil
}

// SaveSettings replaces the routing settings. Turning routing on drains the
// queue right away.
func (s *Service) SaveSettings(ctx context.Context, orgID int64, in model.Settings) (model.Settings, error) {
	if in.Strategy != engine.RoundRobin && in.Strategy != engine.LoadBalanced {
		return model.Settings{}, invalidf("strategy must be round_robin or load_balanced")
	}
	var minutes pgtype.Int4
	if in.UnansweredMinutes != nil {
		if *in.UnansweredMinutes < 1 || *in.UnansweredMinutes > 1440 {
			return model.Settings{}, invalidf("unanswered_minutes must be between 1 and 1440")
		}
		minutes = pgtype.Int4{Int32: *in.UnansweredMinutes, Valid: true}
	}
	r, err := s.q.UpsertRoutingSettings(ctx, db.UpsertRoutingSettingsParams{
		OrganizationID: orgID, Enabled: in.Enabled, Strategy: in.Strategy, RespectCapacity: in.RespectCapacity,
		VipAccountOwner: in.VIPAccountOwner, ReassignOnOffline: in.ReassignOnOffline, UnansweredMinutes: minutes,
	})
	if err != nil {
		return model.Settings{}, err
	}
	if r.Enabled {
		s.DrainQueue(ctx, orgID, 50)
	}
	return toSettings(r), nil
}

// ---------------------------------------------------------------------------
// Rules

// Rules lists routing rules in evaluation order.
func (s *Service) Rules(ctx context.Context, orgID int64) ([]model.Rule, error) {
	rows, err := s.q.ListRoutingRules(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Rule, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRule(r))
	}
	return out, nil
}

func toRule(r db.ListRoutingRulesRow) model.Rule {
	out := model.Rule{UUID: r.Uuid, Name: r.Name, Position: r.Position, IsActive: r.IsActive, Skills: nonNil(r.Skills)}
	_ = json.Unmarshal(r.Conditions, &out.Conditions)
	c := &out.Conditions
	c.ChannelTypes, c.Priorities, c.TagUUIDs = nonNil(c.ChannelTypes), nonNil(c.Priorities), nonNil(c.TagUUIDs)
	if r.TeamUuid.Valid {
		out.Team = &model.TeamRef{UUID: uuid.UUID(r.TeamUuid.Bytes), Name: r.TeamName.String}
	}
	return out
}

func (s *Service) ruleParams(ctx context.Context, orgID int64, in *model.RuleInput) (pgtype.Int8, []byte, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 120 {
		return pgtype.Int8{}, nil, invalidf("name is required (max 120 characters)")
	}
	skills := []string{}
	for _, sk := range in.Skills {
		if sk = strings.TrimSpace(sk); sk != "" && len([]rune(sk)) <= 64 && !slices.Contains(skills, sk) {
			skills = append(skills, sk)
		}
	}
	in.Skills = skills
	for _, p := range in.Conditions.Priorities {
		if !slices.Contains([]string{"low", "normal", "high", "urgent"}, p) {
			return pgtype.Int8{}, nil, invalidf("unknown priority %q", p)
		}
	}
	var team pgtype.Int8
	if in.TeamUUID != nil {
		t, err := s.q.GetTeamByUUIDForRouting(ctx, db.GetTeamByUUIDForRoutingParams{OrganizationID: orgID, Uuid: *in.TeamUUID})
		if err != nil {
			return team, nil, invalidf("unknown team")
		}
		team = pgtype.Int8{Int64: t.ID, Valid: true}
	}
	if !team.Valid && len(in.Skills) == 0 {
		return team, nil, invalidf("a rule needs a team or skills")
	}
	cond, err := json.Marshal(in.Conditions)
	return team, cond, err
}

// CreateRule adds a rule (last unless a position is given).
func (s *Service) CreateRule(ctx context.Context, orgID int64, in model.RuleInput) (model.Rule, error) {
	team, cond, err := s.ruleParams(ctx, orgID, &in)
	if err != nil {
		return model.Rule{}, err
	}
	pos := int32(0)
	if in.Position != nil {
		pos = *in.Position
	} else if pos, err = s.q.NextRoutingRulePosition(ctx, orgID); err != nil {
		return model.Rule{}, err
	}
	r, err := s.q.InsertRoutingRule(ctx, db.InsertRoutingRuleParams{
		OrganizationID: orgID, Name: in.Name, Position: pos, IsActive: in.IsActive == nil || *in.IsActive,
		Conditions: cond, TeamID: team, Skills: in.Skills,
	})
	if err != nil {
		return model.Rule{}, err
	}
	return s.rule(ctx, orgID, r.Uuid)
}

// UpdateRule replaces a rule.
func (s *Service) UpdateRule(ctx context.Context, orgID int64, id uuid.UUID, in model.RuleInput) (model.Rule, error) {
	cur, err := s.q.GetRoutingRuleByUUID(ctx, db.GetRoutingRuleByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Rule{}, notFound(err)
	}
	team, cond, err := s.ruleParams(ctx, orgID, &in)
	if err != nil {
		return model.Rule{}, err
	}
	pos, active := cur.Position, cur.IsActive
	if in.Position != nil {
		pos = *in.Position
	}
	if in.IsActive != nil {
		active = *in.IsActive
	}
	if _, err := s.q.UpdateRoutingRule(ctx, db.UpdateRoutingRuleParams{
		OrganizationID: orgID, ID: cur.ID, Name: in.Name, Position: pos, IsActive: active,
		Conditions: cond, TeamID: team, Skills: in.Skills,
	}); err != nil {
		return model.Rule{}, err
	}
	return s.rule(ctx, orgID, id)
}

// DeleteRule removes a rule.
func (s *Service) DeleteRule(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := s.q.DeleteRoutingRule(ctx, db.DeleteRoutingRuleParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) rule(ctx context.Context, orgID int64, id uuid.UUID) (model.Rule, error) {
	rules, err := s.Rules(ctx, orgID)
	if err != nil {
		return model.Rule{}, err
	}
	for _, r := range rules {
		if r.UUID == id {
			return r, nil
		}
	}
	return model.Rule{}, ErrNotFound
}

// ---------------------------------------------------------------------------
// Capacity

func (s *Service) limits(ctx context.Context, orgID int64) (engine.Limits, map[uuid.UUID]int64, error) {
	rows, err := s.q.ListCapacityLimits(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	out := engine.Limits{}
	ids := map[uuid.UUID]int64{}
	for _, r := range rows {
		uid := int64(0)
		if r.UserID.Valid {
			uid = r.UserID.Int64
			ids[uuid.UUID(r.UserUuid.Bytes)] = uid
		}
		if out[uid] == nil {
			out[uid] = map[string]int{}
		}
		out[uid][r.ChannelType] = int(r.MaxOpen)
	}
	return out, ids, nil
}

// CapacityLimits returns the defaults and per-agent limits.
func (s *Service) CapacityLimits(ctx context.Context, orgID int64) (model.CapacityLimits, error) {
	l, ids, err := s.limits(ctx, orgID)
	if err != nil {
		return model.CapacityLimits{}, err
	}
	out := model.CapacityLimits{Defaults: toLimits(l[0]), Agents: map[uuid.UUID]model.Limits{}}
	for u, id := range ids {
		out.Agents[u] = toLimits(l[id])
	}
	return out, nil
}

func toLimits(m map[string]int) model.Limits {
	out := model.Limits{}
	for k, v := range m {
		out[k] = safeconv.ToInt32(v)
	}
	return out
}

// SetCapacityLimits replaces the limits of one agent or the defaults. A
// freed capacity drains the queue.
func (s *Service) SetCapacityLimits(ctx context.Context, orgID int64, in model.CapacityInput) (model.CapacityLimits, error) {
	var user pgtype.Int8
	if in.UserUUID != nil {
		agents, err := s.q.ListRoutingAgents(ctx, orgID)
		if err != nil {
			return model.CapacityLimits{}, err
		}
		i := slices.IndexFunc(agents, func(a db.ListRoutingAgentsRow) bool { return a.UserUuid == *in.UserUUID })
		if i < 0 {
			return model.CapacityLimits{}, invalidf("user is not an agent of this organization")
		}
		user = pgtype.Int8{Int64: agents[i].UserID, Valid: true}
	}
	for k, v := range in.Limits {
		if k == "" || len(k) > 32 || v < 0 || v > 1000 {
			return model.CapacityLimits{}, invalidf("limits must map a channel type ('*' for all) to 0-1000")
		}
	}
	err := s.inTx(ctx, func(ctx context.Context, q *db.Queries) error {
		if err := q.DeleteCapacityLimits(ctx, db.DeleteCapacityLimitsParams{OrganizationID: orgID, UserID: user.Int64}); err != nil {
			return err
		}
		for k, v := range in.Limits {
			if err := q.InsertCapacityLimit(ctx, db.InsertCapacityLimitParams{OrganizationID: orgID, UserID: user, ChannelType: k, MaxOpen: v}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.CapacityLimits{}, err
	}
	s.DrainQueue(ctx, orgID, 50)
	return s.CapacityLimits(ctx, orgID)
}

// snapshot loads agents with presence, open counts and limits.
type snapshot struct {
	rows    []db.ListRoutingAgentsRow
	agents  []engine.Agent
	limits  engine.Limits
	members map[int64][]int64 // team id → user ids
}

func (s *Service) snapshot(ctx context.Context, q *db.Queries, orgID int64) (snapshot, error) {
	var snap snapshot
	var err error
	if snap.rows, err = q.ListRoutingAgents(ctx, orgID); err != nil {
		return snap, err
	}
	counts, err := q.CountOpenConversationsByAgent(ctx, orgID)
	if err != nil {
		return snap, err
	}
	open := map[int64]map[string]int{}
	for _, c := range counts {
		if open[c.UserID] == nil {
			open[c.UserID] = map[string]int{}
		}
		open[c.UserID][c.ChannelType] = int(c.Open)
	}
	if snap.limits, _, err = s.limits(ctx, orgID); err != nil {
		return snap, err
	}
	members, err := q.ListRoutingTeamMembers(ctx, orgID)
	if err != nil {
		return snap, err
	}
	snap.members = map[int64][]int64{}
	for _, m := range members {
		snap.members[m.TeamID] = append(snap.members[m.TeamID], m.UserID)
	}
	cutoff := s.now().Add(-PresenceTimeout)
	for _, r := range snap.rows {
		available, err := q.ScheduleAvailable(ctx, db.ScheduleAvailableParams{OrganizationID: orgID, UserID: r.UserID, StartsAt: pgtype.Timestamptz{Time: s.now(), Valid: true}})
		if err != nil {
			return snap, err
		}
		online := available && r.Status == "online" && r.LastSeenAt.Valid && r.LastSeenAt.Time.After(cutoff)
		o := open[r.UserID]
		if o == nil {
			o = map[string]int{}
		}
		snap.agents = append(snap.agents, engine.Agent{UserID: r.UserID, Online: online, Skills: r.Skills, Open: o})
	}
	return snap, nil
}

// Capacity is the supervisor view: every agent with presence, open
// conversations per channel, limits and utilization, plus the queue.
func (s *Service) Capacity(ctx context.Context, orgID int64) (model.Capacity, error) {
	snap, err := s.snapshot(ctx, s.q, orgID)
	if err != nil {
		return model.Capacity{}, err
	}
	teams, err := s.q.ListRoutingTeams(ctx, orgID)
	if err != nil {
		return model.Capacity{}, err
	}
	queue, err := s.q.CountUnassignedOpenConversations(ctx, orgID)
	if err != nil {
		return model.Capacity{}, err
	}
	set, err := s.Settings(ctx, orgID)
	if err != nil {
		return model.Capacity{}, err
	}
	out := model.Capacity{Agents: []model.AgentLoad{}, Queue: int(queue), Settings: set}
	for i, r := range snap.rows {
		a := snap.agents[i]
		status := r.Status
		if !a.Online && status == "online" {
			status = "offline" // heartbeat lost; the presence sweep catches up
		}
		load := model.AgentLoad{
			UserUUID: r.UserUuid, Name: strings.TrimSpace(r.Name + " " + r.Surname), Status: status,
			Skills: nonNil(r.Skills), Teams: []model.TeamRef{}, Open: a.Open, OpenTotal: a.Total(), Limits: model.Limits{},
			Available: a.Online && snap.limits.HasRoom(a, engine.AnyChannel),
		}
		if r.LastSeenAt.Valid {
			t := r.LastSeenAt.Time.UTC()
			load.LastSeenAt = &t
		}
		for _, t := range teams {
			if slices.Contains(snap.members[t.ID], r.UserID) {
				load.Teams = append(load.Teams, model.TeamRef{UUID: t.Uuid, Name: t.Name})
			}
		}
		keys := map[string]bool{}
		for k := range snap.limits[0] {
			keys[k] = true
		}
		for k := range snap.limits[r.UserID] {
			keys[k] = true
		}
		for k := range keys {
			if v, ok := snap.limits.Limit(r.UserID, k); ok {
				load.Limits[k] = safeconv.ToInt32(v)
			}
		}
		if v, ok := snap.limits.Limit(r.UserID, engine.AnyChannel); ok && v > 0 {
			u := float64(load.OpenTotal) / float64(v)
			load.Utilization = &u
		}
		if a.Online {
			out.Online++
		}
		out.Agents = append(out.Agents, load)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Audit trail

// Events lists recent routing decisions (one conversation when given).
func (s *Service) Events(ctx context.Context, orgID int64, conversation *uuid.UUID, limit int32) ([]model.Event, error) {
	p := db.ListRoutingEventsParams{OrganizationID: orgID, MaxRows: limit}
	if conversation != nil {
		p.ConversationUuid = pgtype.UUID{Bytes: *conversation, Valid: true}
	}
	rows, err := s.q.ListRoutingEvents(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]model.Event, 0, len(rows))
	for _, r := range rows {
		e := model.Event{
			ConversationUUID: r.ConversationUuid, Number: r.Number, Reason: r.Reason, Strategy: r.Strategy,
			Note: r.Note, Team: r.TeamName.String, CreatedAt: r.CreatedAt.Time.UTC(),
		}
		if r.FromUserUuid.Valid {
			e.From = &model.AgentRef{UUID: uuid.UUID(r.FromUserUuid.Bytes), Name: strings.TrimSpace(r.FromName)}
		}
		if r.ToUserUuid.Valid {
			e.To = &model.AgentRef{UUID: uuid.UUID(r.ToUserUuid.Bytes), Name: strings.TrimSpace(r.ToName)}
		}
		out = append(out, e)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// helpers

func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context, q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("routing: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
