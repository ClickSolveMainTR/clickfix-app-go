// Package usecase implements ordered organization automation rules on the transactional event bus.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/repository"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	notifmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	notify "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	hooks "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrInvalid = errors.New("invalid automation rule")

type Service struct {
	Workflows     *Workflows
	Store         *repository.Store
	Conversations *conv.Service
	Bridge        *apibridge.Bridge
	Resolver      orgctx.Resolver
	Webhooks      *hooks.Service
	Notifications *notify.Service
	// Fields exposes contact/company custom fields to conditions and keeps
	// fields hidden from workflows out of the data map (CF-196).
	Fields FieldSource
}

func TriggerNames() []string {
	out := []string{"scheduled"}
	for _, d := range events.DomainDefinitions() {
		if strings.HasPrefix(d.Name, "conversation.") || strings.HasPrefix(d.Name, "message.") || strings.HasPrefix(d.Name, "sla.") {
			out = append(out, d.Name)
		}
	}
	return out
}

func Validate(in model.RuleInput) error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || !slices.Contains(TriggerNames(), in.Trigger) || len(in.Actions) == 0 || len(in.Actions) > 20 || in.ScheduleMinutes < 0 || in.ScheduleMinutes > 525600 {
		return ErrInvalid
	}
	if (in.Trigger == "scheduled") != (in.ScheduleMinutes > 0) {
		return fmt.Errorf("%w: scheduled rules require schedule_minutes > 0", ErrInvalid)
	}
	n := 0
	if err := validateCondition(in.Conditions, 0, &n); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	for _, a := range in.Actions {
		switch a.Type {
		case "assign_user", "add_tag", "remove_tag", "call_webhook", "notify":
			if a.TargetUUID == uuid.Nil {
				return fmt.Errorf("%w: %s requires target_uuid", ErrInvalid, a.Type)
			}
		case "set_status":
			if !slices.Contains(convmodel.Statuses, a.Value) || a.Value == convmodel.StatusSnoozed {
				return ErrInvalid
			}
		case "set_priority":
			if !slices.Contains(convmodel.Priorities, a.Value) {
				return ErrInvalid
			}
		case "add_note", "send_canned_reply":
			if strings.TrimSpace(a.Content) == "" || len([]rune(a.Content)) > 10000 {
				return ErrInvalid
			}
		default:
			return fmt.Errorf("%w: unsupported action %s", ErrInvalid, a.Type)
		}
	}
	return nil
}

func (s *Service) Save(ctx context.Context, org, actor int64, id uuid.UUID, in model.RuleInput) (db.AutomationRule, error) {
	if err := Validate(in); err != nil {
		return db.AutomationRule{}, err
	}
	c, _ := json.Marshal(in.Conditions)
	a, _ := json.Marshal(in.Actions)
	if id == uuid.Nil {
		return s.Store.CreateAutomationRule(ctx, db.CreateAutomationRuleParams{OrganizationID: org, CreatedBy: actor, Name: in.Name, TriggerEvent: in.Trigger, Conditions: c, Actions: a, Position: in.Position, Enabled: in.Enabled, StopOnMatch: in.StopOnMatch, ScheduleMinutes: in.ScheduleMinutes})
	}
	return s.Store.UpdateAutomationRule(ctx, db.UpdateAutomationRuleParams{CreatedBy: actor, OrganizationID: org, Uuid: id, Name: in.Name, TriggerEvent: in.Trigger, Conditions: c, Actions: a, Position: in.Position, Enabled: in.Enabled, StopOnMatch: in.StopOnMatch, ScheduleMinutes: in.ScheduleMinutes})
}

func (s *Service) Register(bus events.ReliableBus) {
	for _, n := range TriggerNames() {
		if n != "scheduled" {
			bus.SubscribeReliable(n, s.Consume)
		}
	}
	for old := range events.DomainAliases() {
		bus.SubscribeReliable(old, func(ctx context.Context, e events.Event) error {
			for _, expanded := range events.ExpandDomainAlias(e) {
				if err := s.Consume(ctx, expanded); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

func eventTrace(e events.Event) []string {
	out := []string{}
	switch t := e.Payload["_automation_trace"].(type) {
	case []string:
		return t
	case []any:
		for _, v := range t {
			if x, ok := v.(string); ok {
				out = append(out, x)
			}
		}
	}
	return out
}

func (s *Service) Consume(ctx context.Context, e events.Event) error {
	if s.Workflows != nil {
		if err := s.Workflows.Consume(ctx, e); err != nil {
			return err
		}
	}
	if e.TenantID == nil {
		return nil
	}
	org := *e.TenantID
	id := uuid.Nil
	if e.EntityUUID != nil && e.EntityType == "conversation" {
		id = *e.EntityUUID
	}
	if id == uuid.Nil {
		if raw, ok := e.Payload["conversation_uuid"].(string); ok {
			id, _ = uuid.Parse(raw)
		}
	}
	if id == uuid.Nil {
		var convID int64
		switch v := e.Payload["conversation_id"].(type) {
		case int64:
			convID = v
		case float64:
			convID = int64(v)
		}
		if convID > 0 {
			var err error
			id, err = s.Store.AutomationConversationUUID(ctx, db.AutomationConversationUUIDParams{OrganizationID: org, ID: convID})
			if err != nil {
				return err
			}
		}
	}
	if id == uuid.Nil {
		return nil
	}
	rules, err := s.Store.MatchAutomationRules(ctx, db.MatchAutomationRulesParams{OrganizationID: org, TriggerEvent: e.Name})
	if err != nil {
		return err
	}
	trace := eventTrace(e)
	if len(trace) >= 8 {
		return nil
	}
	for _, rule := range rules {
		if slices.Contains(trace, rule.Uuid.String()) {
			continue
		}
		matched, err := s.run(ctx, rule, e.EventID, id, trace, false)
		if err != nil {
			return err
		}
		if matched && rule.StopOnMatch {
			break
		}
	}
	return nil
}

func (s *Service) principal(ctx context.Context, rule db.AutomationRule) (context.Context, apibridge.Principal, error) {
	o, err := s.Store.GetOrganizationByID(ctx, rule.OrganizationID)
	if err != nil {
		return ctx, apibridge.Principal{}, err
	}
	u, err := s.Store.GetUserByID(ctx, rule.CreatedBy)
	if err != nil {
		return ctx, apibridge.Principal{}, err
	}
	if u.Status != "active" || u.DeletedAt.Valid {
		return ctx, apibridge.Principal{}, fmt.Errorf("rule owner is inactive")
	}
	if s.Resolver == nil {
		return ctx, apibridge.Principal{}, fmt.Errorf("organization resolver unavailable")
	}
	scope, err := s.Resolver.Resolve(ctx, o.Uuid.String(), u.ID)
	if err != nil {
		return ctx, apibridge.Principal{}, err
	}
	if !scope.Has(rbac.PermTenantAutomationWrite) || !scope.Has(rbac.PermTenantConversationsRead) {
		return ctx, apibridge.Principal{}, fmt.Errorf("rule owner lacks automation or conversation read permission")
	}
	return orgctx.WithScope(ctx, scope), apibridge.Principal{OrganizationUUID: o.Uuid, UserUUID: u.Uuid}, nil
}

func (s *Service) data(ctx context.Context, rule db.AutomationRule, id uuid.UUID) (map[string]any, error) {
	d, err := s.Conversations.Get(ctx, rule.OrganizationID, rule.CreatedBy, id)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out["hours_since_created"] = now.Sub(d.CreatedAt).Hours()
	out["hours_since_updated"] = now.Sub(d.UpdatedAt).Hours()
	out["sla_breached"] = d.SLABreachedAt != nil
	if d.LastInboundAt != nil {
		out["hours_since_inbound"] = now.Sub(*d.LastInboundAt).Hours()
	}
	if d.WaitingSince != nil {
		out["hours_waiting"] = now.Sub(*d.WaitingSince).Hours()
	}
	out["clock"] = map[string]any{"hour": float64(now.Hour()), "weekday": float64(now.Weekday())}
	var company *uuid.UUID
	if d.Company != nil {
		company = &d.Company.UUID
	}
	if err := s.addCustomFields(ctx, rule.OrganizationID, out, d.Contact.UUID, company); err != nil {
		return nil, err
	}
	return out, nil
}

// DryRun evaluates current data with current permissions and never claims an execution or runs actions.
func (s *Service) DryRun(ctx context.Context, rule db.AutomationRule, id uuid.UUID) (bool, error) {
	return s.run(ctx, rule, uuid.Nil, id, nil, true)
}

func (s *Service) run(ctx context.Context, rule db.AutomationRule, event, id uuid.UUID, trace []string, dry bool) (bool, error) {
	ctx, p, err := s.principal(ctx, rule)
	if err != nil {
		return false, err
	}
	c, actions, err := decodeRule(rule.Conditions, rule.Actions)
	if err != nil {
		return false, err
	}
	data, err := s.data(ctx, rule, id)
	if err != nil {
		return false, err
	}
	matched := matches(c, data)
	if dry {
		return matched, nil
	}
	execution, err := s.Store.ClaimAutomationExecution(ctx, db.ClaimAutomationExecutionParams{OrganizationID: rule.OrganizationID, RuleID: rule.ID, EventID: event, ConversationUuid: id, Matched: matched})
	if errors.Is(err, pgx.ErrNoRows) {
		old, readErr := s.Store.GetAutomationExecution(ctx, db.GetAutomationExecutionParams{OrganizationID: rule.OrganizationID, RuleID: rule.ID, EventID: event, ConversationUuid: id})
		if readErr != nil {
			return false, readErr
		}
		if old.Status == "running" {
			return false, fmt.Errorf("automation execution is already leased")
		}
		return old.Matched, nil
	}
	if err != nil {
		return false, err
	}
	matched = execution.Matched
	ctx = events.WithAutomationTrace(ctx, append(append([]string{}, trace...), rule.Uuid.String()))
	index := execution.ActionIndex
	if matched {
		for i := int(index); i < len(actions); i++ {
			key := uuid.NewSHA1(execution.Uuid, []byte(strconv.Itoa(i)))
			if err = s.action(ctx, p, rule, id, key, actions[i]); err != nil {
				break
			}
			index = safeconv.ToInt32(i + 1)
			if err = s.Store.CheckpointAutomationExecution(ctx, db.CheckpointAutomationExecutionParams{OrganizationID: rule.OrganizationID, ID: execution.ID, ActionIndex: index}); err != nil {
				break
			}
		}
	}
	status := "succeeded"
	msg := ""
	if !matched {
		status = "skipped"
	}
	if err != nil {
		status = "failed"
		msg = err.Error()
	}
	finishErr := s.Store.FinishAutomationExecution(ctx, db.FinishAutomationExecutionParams{OrganizationID: rule.OrganizationID, ID: execution.ID, Status: status, Matched: matched, Error: msg, ActionIndex: index})
	return matched, errors.Join(err, finishErr)
}

func (s *Service) action(ctx context.Context, p apibridge.Principal, rule db.AutomationRule, id, key uuid.UUID, a model.Action) error {
	base := "/v1/tenant/conversations/" + id.String()
	method := http.MethodPatch
	var body any
	path := base
	switch a.Type {
	case "set_status":
		body = map[string]any{"status": a.Value}
	case "set_priority":
		body = map[string]any{"priority": a.Value}
	case "assign_user":
		body = map[string]any{"assignee": a.TargetUUID}
	case "add_tag":
		method = http.MethodPost
		path += "/tags"
		body = map[string]any{"uuid": a.TargetUUID}
	case "remove_tag":
		method = http.MethodDelete
		path += "/tags/" + a.TargetUUID.String()
	case "add_note", "send_canned_reply":
		method = http.MethodPost
		path += "/messages"
		body = map[string]any{"content": a.Content, "private": a.Type == "add_note", "client_id": key}
	case "call_webhook":
		if !orgctx.Must(ctx).Has(rbac.PermTenantWebhooksWrite) {
			return fmt.Errorf("missing webhook management permission")
		}
		return s.Webhooks.Enqueue(ctx, rule.OrganizationID, a.TargetUUID, key, "automation.executed", map[string]any{"id": key, "event": "automation.executed", "rule_uuid": rule.Uuid, "conversation_uuid": id})
	case "notify":
		u, err := s.Store.GetOrganizationAgentByUUID(ctx, db.GetOrganizationAgentByUUIDParams{OrganizationID: rule.OrganizationID, Uuid: a.TargetUUID})
		if err != nil {
			return err
		}
		if s.Notifications == nil {
			return fmt.Errorf("notifications unavailable")
		}
		_, err = s.Notifications.Enqueue(ctx, notifmodel.EnqueueInput{TenantID: &rule.OrganizationID, UserID: &u.ID, Channels: []string{"inapp"}, Title: rule.Name, Body: a.Content, SourceEvent: "automation.executed", Payload: map[string]any{"execution_key": key, "conversation_uuid": id}})
		return err
	default:
		return ErrInvalid
	}
	res, err := s.Bridge.JSON(ctx, p, method, path, body)
	if err != nil {
		return err
	}
	if res.Status >= 300 {
		return fmt.Errorf("automation action %s failed (HTTP %d)", a.Type, res.Status)
	}
	return nil
}

func (s *Service) Sweep(ctx context.Context) error {
	if s.Workflows != nil {
		if err := s.Workflows.Sweep(ctx); err != nil {
			return err
		}
	}
	rules, err := s.Store.ClaimScheduledAutomationRules(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, rule := range rules {
		runErr := s.scanRule(ctx, rule)
		finishErr := s.Store.CompleteScheduledAutomationRule(ctx, db.CompleteScheduledAutomationRuleParams{OrganizationID: rule.OrganizationID, ID: rule.ID, Succeeded: runErr == nil})
		failures = append(failures, runErr, finishErr)
	}
	return errors.Join(failures...)
}

func (s *Service) scanRule(ctx context.Context, rule db.AutomationRule) error {
	var after int64
	// The due timestamp stays fixed until the full scan succeeds, so retries
	// resume the same execution ids and completed actions remain checkpointed.
	event := uuid.NewSHA1(rule.Uuid, []byte(rule.NextRunAt.Time.UTC().Format(time.RFC3339Nano)))
	// Resume checkpointed work even when an earlier action resolved the
	// conversation and it no longer appears in the active scan.
	pending, err := s.Store.PendingAutomationExecutions(ctx, db.PendingAutomationExecutionsParams{OrganizationID: rule.OrganizationID, RuleID: rule.ID, EventID: event})
	if err != nil {
		return err
	}
	for _, id := range pending {
		if _, err = s.run(ctx, rule, event, id, nil, false); err != nil {
			return err
		}
	}
	for {
		rows, err := s.Store.AutomationConversationPage(ctx, db.AutomationConversationPageParams{OrganizationID: rule.OrganizationID, AfterID: after})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if _, err = s.run(ctx, rule, event, row.Uuid, nil, false); err != nil {
				return err
			}
			after = row.ID
		}
		if len(rows) < 100 {
			return nil
		}
		if err = s.Store.HeartbeatScheduledAutomationRule(ctx, db.HeartbeatScheduledAutomationRuleParams{OrganizationID: rule.OrganizationID, ID: rule.ID}); err != nil {
			return err
		}
	}
}
