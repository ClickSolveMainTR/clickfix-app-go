package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrWorkflowConflict = errors.New("workflow revision conflict")

// Workflows shares the rule engine's condition evaluator, principal and action adapters.
// Asynq's existing automation scan recovers due runs from Postgres, including waits.
type Workflows struct {
	Store        *repository.WorkflowStore
	Automation   *Service
	Entitlements entitlement.Engine
	ExtraAction  func(context.Context, db.AutomationRule, uuid.UUID, uuid.UUID, model.Action) (any, error)
}

func (s *Workflows) allowed(ctx context.Context, org int64) error {
	return entitlement.Require(ctx, s.Entitlements, org, "workflows.enabled")
}
func (s *Workflows) Save(ctx context.Context, org, actor int64, id uuid.UUID, in model.WorkflowInput) (db.Workflow, error) {
	var out db.Workflow
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 {
		return out, ErrInvalid
	}
	if err := ValidateGraph(in.Graph); err != nil {
		return out, err
	}
	if err := s.allowed(ctx, org); err != nil {
		return out, err
	}
	raw, err := json.Marshal(in.Graph)
	if err != nil {
		return out, err
	}
	if len(raw) > 128<<10 {
		return out, ErrInvalid
	}
	err = s.Store.Atomic(ctx, org, func(q *db.Queries) error {
		var e error
		if id == uuid.Nil {
			count, e := q.WorkflowCount(ctx, org)
			if e != nil {
				return e
			}
			if _, e = s.Entitlements.CheckLimit(ctx, org, "workflows.count", count, 1); e != nil {
				return e
			}
			out, e = q.WorkflowCreate(ctx, db.WorkflowCreateParams{OrganizationID: org, Name: in.Name, Draft: raw, CreatedBy: actor})
			return e
		}
		out, e = q.WorkflowUpdate(ctx, db.WorkflowUpdateParams{OrganizationID: org, Uuid: id, Name: in.Name, Draft: raw, Revision: in.Revision})
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrWorkflowConflict
		}
		return e
	})
	return out, err
}
func (s *Workflows) Publish(ctx context.Context, org, actor int64, id uuid.UUID, revision int32) (db.Workflow, error) {
	return s.publish(ctx, org, actor, id, revision, true)
}
func (s *Workflows) publish(ctx context.Context, org, actor int64, id uuid.UUID, revision int32, enabled bool) (db.Workflow, error) {
	var out db.Workflow
	if err := s.allowed(ctx, org); err != nil {
		return out, err
	}
	err := s.Store.Atomic(ctx, org, func(q *db.Queries) error {
		row, e := q.WorkflowGet(ctx, db.WorkflowGetParams{OrganizationID: org, Uuid: id})
		if e != nil {
			return e
		}
		if row.Revision != revision {
			return ErrWorkflowConflict
		}
		var g model.Graph
		if json.Unmarshal(row.Draft, &g) != nil {
			return ErrInvalid
		}
		if e = ValidateGraph(g); e != nil {
			return e
		}
		// Re-publishing the same revision is idempotent and does not create a second version.
		if row.PublishedVersion.Valid && row.PublishedVersion.Int32 == revision {
			out = row
			return nil
		}
		trigger := graphTrigger(g)
		_, e = q.WorkflowVersionCreate(ctx, db.WorkflowVersionCreateParams{OrganizationID: org, WorkflowID: row.ID,
			Version: revision, Graph: row.Draft, TriggerEvent: trigger.Trigger, ScheduleMinutes: trigger.ScheduleMinutes, CreatedBy: actor})
		if e != nil {
			return e
		}
		out, e = q.WorkflowPublish(ctx, db.WorkflowPublishParams{OrganizationID: org, Uuid: id, Enabled: enabled, Revision: revision})
		return e
	})
	return out, err
}
func (s *Workflows) Enable(ctx context.Context, org int64, id uuid.UUID, enabled bool) (db.Workflow, error) {
	if enabled {
		if err := s.allowed(ctx, org); err != nil {
			return db.Workflow{}, err
		}
	}
	return s.Store.Q.WorkflowEnable(ctx, db.WorkflowEnableParams{OrganizationID: org, Uuid: id, Enabled: enabled})
}
func (s *Workflows) Consume(ctx context.Context, e events.Event) error {
	if e.TenantID == nil {
		return nil
	}
	org := *e.TenantID
	id := uuid.Nil
	if e.EntityType == "conversation" && e.EntityUUID != nil {
		id = *e.EntityUUID
	}
	if id == uuid.Nil {
		if raw, ok := e.Payload["conversation_uuid"].(string); ok {
			id, _ = uuid.Parse(raw)
		}
	}
	if id == uuid.Nil {
		var internal int64
		switch v := e.Payload["conversation_id"].(type) {
		case int64:
			internal = v
		case float64:
			if v > 0 && v < 1<<53 {
				internal = int64(v)
			}
		}
		if internal > 0 {
			var err error
			id, err = s.Store.Q.AutomationConversationUUID(ctx, db.AutomationConversationUUIDParams{OrganizationID: org, ID: internal})
			if err != nil {
				return err
			}
		}
	}
	if id == uuid.Nil {
		return nil
	}
	rows, err := s.Store.Q.WorkflowMatching(ctx, db.WorkflowMatchingParams{OrganizationID: org, TriggerEvent: e.Name})
	if err != nil {
		return err
	}
	for _, r := range rows {
		_, err = s.Start(ctx, startRun{Org: org, Workflow: r.WorkflowID, Version: r.ID, WorkflowUUID: r.WorkflowUuid, Event: e.EventID, Conversation: id, Graph: r.Graph, Trace: eventTrace(e)})
		// A plan downgrade blocks workflow execution without poisoning unrelated outbox delivery.
		if err != nil && !errors.Is(err, entitlement.ErrDenied) {
			return err
		}
	}
	return nil
}

type startRun struct {
	Org, Workflow, Version            int64
	WorkflowUUID, Event, Conversation uuid.UUID
	Graph                             []byte
	Trace                             []string
}

func (s *Workflows) Start(ctx context.Context, in startRun) (db.WorkflowRun, error) {
	var out db.WorkflowRun
	if in.Event == uuid.Nil || in.Conversation == uuid.Nil {
		return out, ErrInvalid
	}
	if len(in.Trace) >= 8 || slices.Contains(in.Trace, in.WorkflowUUID.String()) {
		return out, nil
	}
	var g model.Graph
	if err := json.Unmarshal(in.Graph, &g); err != nil {
		return out, err
	}
	if err := s.allowed(ctx, in.Org); err != nil {
		return out, err
	}
	trace, _ := json.Marshal(append(append([]string{}, in.Trace...), in.WorkflowUUID.String()))
	err := s.Store.Atomic(ctx, in.Org, func(q *db.Queries) error {
		old, e := q.WorkflowRunExisting(ctx, db.WorkflowRunExistingParams{OrganizationID: in.Org, WorkflowID: in.Workflow, EventID: in.Event, ConversationUuid: in.Conversation})
		if e == nil {
			out = old
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		count, e := q.WorkflowDailyRuns(ctx, in.Org)
		if e != nil {
			return e
		}
		if _, e = s.Entitlements.CheckLimit(ctx, in.Org, "workflows.runs_daily", count, 1); e != nil {
			return e
		}
		minute, e := q.WorkflowMinuteRuns(ctx, in.Org)
		if e != nil {
			return e
		}
		// Hard safety ceiling applies even to unlimited plans. Replayed events are checked above.
		if minute >= 100 {
			return &entitlement.Error{Code: entitlement.CodeLimitExceeded, Feature: "workflows.rate_per_minute", Limit: 100, Used: minute}
		}
		out, e = q.WorkflowRunCreate(ctx, db.WorkflowRunCreateParams{OrganizationID: in.Org, WorkflowID: in.Workflow, VersionID: in.Version,
			EventID: in.Event, ConversationUuid: in.Conversation, NodeID: graphTrigger(g).ID, Trace: trace})
		return e
	})
	return out, err
}

// DryRun reads a real sample conversation, evaluates the draft and produces an inert trace.
// Waits are reported and stop simulation; no delay is slept and no action is executed.
func (s *Workflows) DryRun(ctx context.Context, row db.Workflow, actor int64, conversation uuid.UUID) ([]model.TraceStep, error) {
	var g model.Graph
	if err := json.Unmarshal(row.Draft, &g); err != nil {
		return nil, err
	}
	if err := ValidateGraph(g); err != nil {
		return nil, err
	}
	rule := db.AutomationRule{OrganizationID: row.OrganizationID, CreatedBy: actor, Uuid: row.Uuid, Name: row.Name}
	ctx, _, err := s.Automation.principal(ctx, rule)
	if err != nil {
		return nil, err
	}
	data, err := s.Automation.data(ctx, rule, conversation)
	if err != nil {
		return nil, err
	}
	trace := []model.TraceStep{}
	id := graphTrigger(g).ID
	for range MaxWorkflowSteps {
		n := graphNode(g, id)
		port := graphPort(n, data)
		status := "simulated"
		if n.Type == "action" {
			status = "would_execute"
			simulateAction(data, *n.Action)
		}
		if n.Type == "wait" {
			status = "would_wait"
		}
		trace = append(trace, model.TraceStep{NodeID: id, Type: n.Type, Port: port, Status: status})
		if n.Type == "end" || n.Type == "wait" {
			return trace, nil
		}
		id = graphNext(g, id, port)
	}
	return trace, fmt.Errorf("%w: step limit", ErrInvalid)
}
func (s *Workflows) Sweep(ctx context.Context) error {
	// Existing scheduler is Asynq-owned. Runs and their leases survive Redis loss.
	if err := s.schedule(ctx); err != nil {
		return err
	}
	for range 20 {
		rows, err := s.Store.Q.WorkflowRunDue(ctx)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		r := rows[0]
		for range MaxWorkflowSteps + 1 {
			if err = s.execute(ctx, r); err != nil {
				return err
			}
			r, err = s.Store.Q.WorkflowRunGet(ctx, db.WorkflowRunGetParams{OrganizationID: r.OrganizationID, Uuid: r.Uuid})
			if err != nil {
				return err
			}
			if r.Status != "pending" {
				break
			}
		}
	}
	return nil
}

func (s *Workflows) schedule(ctx context.Context) error {
	rows, err := s.Store.Q.WorkflowDueSchedules(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {

		event := uuid.NewSHA1(r.WorkflowUuid, []byte(r.NextScheduleAt.Time.UTC().Format(time.RFC3339Nano)))
		conversations, e := s.Store.Q.AutomationConversationPage(ctx, db.AutomationConversationPageParams{OrganizationID: r.OrganizationID, AfterID: r.ScheduleCursor})
		if e != nil {
			return e
		}
		after := r.ScheduleCursor
		for _, c := range conversations {
			_, e = s.Start(ctx, startRun{Org: r.OrganizationID, Workflow: r.WorkflowID, Version: r.ID, WorkflowUUID: r.WorkflowUuid, Event: event, Conversation: c.Uuid, Graph: r.Graph})
			if e != nil {
				var denied *entitlement.Error
				if !errors.As(e, &denied) {
					return e
				}
				if denied.Feature == "workflows.rate_per_minute" {
					break
				}
			}
			after = c.ID
		}
		if e = s.Store.Q.WorkflowScheduleCursor(ctx, db.WorkflowScheduleCursorParams{OrganizationID: r.OrganizationID, ID: r.WorkflowID, ScheduleCursor: after}); e != nil {
			return e
		}
		if len(conversations) == 100 || len(conversations) > 0 && after != conversations[len(conversations)-1].ID {
			continue
		}

		next := time.Now().UTC().Add(time.Duration(r.ScheduleMinutes) * time.Minute)
		if e := s.Store.Q.WorkflowScheduleAdvance(ctx, db.WorkflowScheduleAdvanceParams{OrganizationID: r.OrganizationID, ID: r.WorkflowID, NextScheduleAt: pgtype.Timestamptz{Time: next, Valid: true}}); e != nil {
			return e
		}
	}
	return nil
}
func (s *Workflows) checkpoint(ctx context.Context, r db.WorkflowRun, status, node, msg string, index int32, due time.Time) error {
	return s.Store.Q.WorkflowRunCheckpoint(ctx, db.WorkflowRunCheckpointParams{OrganizationID: r.OrganizationID, ID: r.ID,
		NodeID: node, StepIndex: index, Status: status, DueAt: pgtype.Timestamptz{Time: due, Valid: true}, Error: msg})
}
func (s *Workflows) execute(ctx context.Context, r db.WorkflowRun) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := s.allowed(ctx, r.OrganizationID); err != nil {
		return s.checkpoint(ctx, r, "blocked", r.NodeID, err.Error(), r.StepIndex, time.Now())
	}
	v, err := s.Store.Q.WorkflowVersionGet(ctx, db.WorkflowVersionGetParams{OrganizationID: r.OrganizationID, ID: r.VersionID})
	if err != nil {
		return err
	}
	var g model.Graph
	if err = json.Unmarshal(v.Graph, &g); err != nil {
		return err
	}
	rule := db.AutomationRule{OrganizationID: r.OrganizationID, CreatedBy: v.CreatedBy, Uuid: v.Uuid, Name: "Workflow"}
	ctx, p, err := s.Automation.principal(ctx, rule)
	if err != nil {
		return s.checkpoint(ctx, r, "failed", r.NodeID, err.Error(), r.StepIndex, time.Now())
	}
	trace := []string{}
	if err = json.Unmarshal(r.Trace, &trace); err != nil {
		return err
	}
	ctx = events.WithAutomationTrace(ctx, trace)
	n := graphNode(g, r.NodeID)
	if r.StepIndex >= MaxWorkflowSteps {
		return s.checkpoint(ctx, r, "failed", r.NodeID, "step/loop guard exceeded", r.StepIndex, time.Now())
	}
	if time.Since(r.CreatedAt.Time) > 30*24*time.Hour {
		return s.checkpoint(ctx, r, "failed", r.NodeID, "run age guard exceeded", r.StepIndex, time.Now())
	}
	// One node per durable checkpoint. Successful steps recover without replaying side effects.
	old, e := s.Store.Q.WorkflowStepGet(ctx, db.WorkflowStepGetParams{OrganizationID: r.OrganizationID, RunID: r.ID, StepIndex: r.StepIndex})
	var output struct {
		Port   string `json:"port"`
		Result any    `json:"result,omitempty"`
	}
	if e == nil {
		if old.Status != "succeeded" {
			if err = s.Store.Q.WorkflowStepFinish(ctx, db.WorkflowStepFinishParams{OrganizationID: r.OrganizationID, RunID: r.ID, StepIndex: r.StepIndex, Status: "uncertain", Output: []byte("{}"), Error: "execution interrupted before checkpoint"}); err != nil {
				return err
			}

			return s.checkpoint(ctx, r, "failed", r.NodeID, "uncertain action outcome; manual verification required", r.StepIndex, time.Now())
		}
		if err = json.Unmarshal(old.Output, &output); err != nil {
			return err
		}
	} else {
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if err = s.Store.Q.WorkflowStepStart(ctx, db.WorkflowStepStartParams{OrganizationID: r.OrganizationID, RunID: r.ID, StepIndex: r.StepIndex, NodeID: n.ID}); err != nil {
			return err
		}
		data, e := s.Automation.data(ctx, rule, r.ConversationUuid)
		if e == nil {
			output.Port = graphPort(n, data)
			if n.Type == "action" {
				key := uuid.NewSHA1(r.Uuid, []byte(fmt.Sprint(r.StepIndex)))
				switch n.Action.Type {
				case "run_ai_procedure", "create_engineering_issue", "send_template", "route":
					if s.ExtraAction == nil {
						e = fmt.Errorf("workflow action adapter unavailable")
					} else {
						output.Result, e = s.ExtraAction(ctx, rule, r.ConversationUuid, key, *n.Action)
					}
				default:
					e = s.Automation.action(ctx, p, rule, r.ConversationUuid, key, *n.Action)
				}
			}
		}
		status, msg := "succeeded", ""
		if e != nil {
			status, msg = "failed", e.Error()
		}
		raw, _ := json.Marshal(output)
		if err = s.Store.Q.WorkflowStepFinish(ctx, db.WorkflowStepFinishParams{OrganizationID: r.OrganizationID, RunID: r.ID, StepIndex: r.StepIndex, Status: status, Output: raw, Error: msg}); err != nil {
			return err
		}
		if e != nil {
			return s.checkpoint(ctx, r, "failed", r.NodeID, msg, r.StepIndex, time.Now())
		}
	}
	status, next, due := "pending", graphNext(g, n.ID, output.Port), time.Now().UTC()
	if n.Type == "end" {
		status, next = "succeeded", n.ID
	}
	if n.Type == "wait" {
		status = "waiting"
		due = old.CreatedAt.Time.Add(time.Duration(n.DelaySeconds) * time.Second)
		if old.CreatedAt.Time.IsZero() {
			due = time.Now().UTC().Add(time.Duration(n.DelaySeconds) * time.Second)
		}
	}
	return s.checkpoint(ctx, r, status, next, "", r.StepIndex+1, due)
}

func (s *Workflows) StartWebhook(ctx context.Context, v db.WorkflowMatchingRow, event, conversation uuid.UUID) (db.WorkflowRun, error) {
	return s.Start(ctx, startRun{Org: v.OrganizationID, Workflow: v.WorkflowID, Version: v.ID, WorkflowUUID: v.WorkflowUuid, Event: event, Conversation: conversation, Graph: v.Graph})
}

// ApplyWorkflowAction reuses the same authorized, event-producing inbox adapter as rules.
func (s *Service) ApplyWorkflowAction(ctx context.Context, rule db.AutomationRule, conversation, key uuid.UUID, a model.Action) error {
	ctx, p, err := s.principal(ctx, rule)
	if err != nil {
		return err
	}
	return s.action(ctx, p, rule, conversation, key, a)
}

// simulateAction applies only deterministic field changes to dry-run data. It never
// invokes an adapter or guesses a network/AI result.
func simulateAction(data map[string]any, a model.Action) {
	switch a.Type {
	case "set_status":
		data["status"] = a.Value
	case "set_priority":
		data["priority"] = a.Value
	case "assign_user":
		data["assignee"] = map[string]any{"uuid": a.TargetUUID.String()}
	case "add_tag", "remove_tag":
		tags, _ := data["tag_uuids"].([]any)
		out := []any{}
		for _, tag := range tags {
			if tag != a.TargetUUID.String() {
				out = append(out, tag)
			}
		}
		if a.Type == "add_tag" {
			out = append(out, a.TargetUUID.String())
		}
		data["tag_uuids"] = out
	}
}
