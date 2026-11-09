package automation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Automation rule writes (CF-188): create a rule from a sentence, change,
// turn on/off and delete it. Rules are the single-step form of workflows:
// one trigger, a condition tree, a list of actions.

// ruleRow is GET /v1/tenant/automations/{uuid} (conditions and actions
// are JSON in bytea columns, sent base64).
type ruleRow struct {
	UUID            uuid.UUID `json:"uuid"`
	Name            string    `json:"name"`
	TriggerEvent    string    `json:"trigger_event"`
	Conditions      []byte    `json:"conditions"`
	Actions         []byte    `json:"actions"`
	Position        int32     `json:"position"`
	Enabled         bool      `json:"enabled"`
	StopOnMatch     bool      `json:"stop_on_match"`
	ScheduleMinutes int32     `json:"schedule_minutes"`
}

func (r ruleRow) input() (model.RuleInput, error) {
	in := model.RuleInput{Name: r.Name, Trigger: r.TriggerEvent, Position: r.Position, Enabled: r.Enabled,
		StopOnMatch: r.StopOnMatch, ScheduleMinutes: r.ScheduleMinutes}
	if err := json.Unmarshal(r.Conditions, &in.Conditions); err != nil {
		return in, err
	}
	return in, json.Unmarshal(r.Actions, &in.Actions)
}

func getRule(ctx context.Context, api capability.API, id string) (ruleRow, error) {
	var row ruleRow
	if _, err := uuid.Parse(id); err != nil {
		return row, capability.Invalid("rule_uuid must be a UUID")
	}
	r, err := api.Call(ctx, http.MethodGet, rulesPath+"/"+id, nil)
	if err != nil {
		return row, err
	}
	decErr := r.Decode(&row)
	return row, decErr
}

// applyRuleParams copies the given fields onto in.
func applyRuleParams(in *model.RuleInput, p capability.Params) error {
	raw, _ := json.Marshal(p)
	var f struct {
		Name            *string          `json:"name"`
		Trigger         *string          `json:"trigger"`
		Conditions      *model.Condition `json:"conditions"`
		Actions         *[]model.Action  `json:"actions"`
		Enabled         *bool            `json:"enabled"`
		StopOnMatch     *bool            `json:"stop_on_match"`
		ScheduleMinutes *int32           `json:"schedule_minutes"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return capability.Invalid("the rule does not fit: %s", err.Error())
	}
	if f.Name != nil {
		in.Name = strings.TrimSpace(*f.Name)
	}
	if f.Trigger != nil {
		in.Trigger = *f.Trigger
	}
	if f.Conditions != nil {
		in.Conditions = *f.Conditions
	}
	if f.Actions != nil {
		in.Actions = *f.Actions
	}
	if f.Enabled != nil {
		in.Enabled = *f.Enabled
	}
	if f.StopOnMatch != nil {
		in.StopOnMatch = *f.StopOnMatch
	}
	if f.ScheduleMinutes != nil {
		in.ScheduleMinutes = *f.ScheduleMinutes
	}
	return nil
}

// ruleBody is the API body of a rule. model.Condition drops an empty
// "all" list (omitempty), which the API reads as no group at all; an empty
// condition is sent as {"all":[]} explicitly.
func ruleBody(in model.RuleInput) map[string]any {
	var cond any = in.Conditions
	c := in.Conditions
	if c.Any == nil && c.Field == "" && len(c.All) == 0 {
		cond = json.RawMessage(`{"all":[]}`)
	}
	return map[string]any{"name": in.Name, "trigger": in.Trigger, "conditions": cond, "actions": in.Actions,
		"position": in.Position, "enabled": in.Enabled, "stop_on_match": in.StopOnMatch, "schedule_minutes": in.ScheduleMinutes}
}

func validateRule(in model.RuleInput) error {
	for _, a := range in.Actions {
		if _, ok := DraftActions[a.Type]; !ok {
			return capability.Invalid("unsupported action %q; use one of %s", a.Type, strings.Join(draftActionTypes(), ", "))
		}
	}
	if err := usecase.Validate(in); err != nil {
		msg := strings.TrimPrefix(strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()), ": ")
		if msg == "" {
			msg = "check the name, trigger, condition fields/operators and each action's value (workflows_reference)"
		}
		return capability.Invalid("the rule is not valid: %s", msg)
	}
	return nil
}

func describeRule(e capability.Exec, in model.RuleInput) []capability.Detail {
	d := draft{Name: in.Name, Trigger: in.Trigger, ScheduleMinutes: in.ScheduleMinutes, Actions: in.Actions}
	if c := in.Conditions; c.Any != nil || c.Field != "" || len(c.All) > 0 {
		d.Condition = &c
	}
	out := describe(e, d)
	state := t(e, "kapalı", "off")
	if in.Enabled {
		state = t(e, "açık", "on")
	}
	return append(out, capability.Detail{Label: t(e, "Durum", "State"), Value: state})
}

// simulateRule runs the rule's dry run on the latest conversation.
func simulateRule(ctx context.Context, e capability.Exec, id string) capability.Check {
	conv := latestConversation(ctx, e.API)
	if conv == "" {
		return capability.Check{Status: capability.VerifyOK, Message: t(e, "Kaydedildi; deneme için henüz konuşma yok.", "Saved; no conversation to test on yet.")}
	}
	r, err := e.API.Call(ctx, http.MethodPost, rulesPath+"/"+id+"/test", map[string]any{"conversation_uuid": conv})
	if err != nil {
		return capability.Check{Status: capability.VerifySkipped}
	}
	var out struct {
		Matched bool `json:"matched"`
	}
	if r.Decode(&out) != nil {
		return capability.Check{Status: capability.VerifySkipped}
	}
	if out.Matched {
		return capability.Check{Status: capability.VerifyOK, Message: t(e, "Son konuşmada koşullar sağlanıyor; eylemler çalışırdı.", "The conditions match the latest conversation; the actions would run.")}
	}
	return capability.Check{Status: capability.VerifyOK, Message: t(e, "Son konuşmada koşullar sağlanmıyor.", "The conditions do not match the latest conversation.")}
}

var ruleIDField = map[string]any{"type": "string", "title": "Rule", "pattern": "^[0-9a-fA-F-]{36}$"}

func ruleFields(required bool) map[string]any {
	props := map[string]any{
		"name":             map[string]any{"type": "string", "title": "Name", "minLength": 1, "maxLength": 200},
		"trigger":          map[string]any{"type": "string", "title": "Trigger", "enum": usecase.TriggerNames()},
		"conditions":       map[string]any{"type": "object", "title": "Conditions", "description": conditionDoc + " Use {\"all\":[]} for none."},
		"actions":          map[string]any{"type": "array", "title": "Actions", "minItems": 1, "maxItems": 20, "items": actionItemSchema()},
		"enabled":          map[string]any{"type": "boolean", "title": "Enabled"},
		"stop_on_match":    map[string]any{"type": "boolean", "title": "Stop other rules"},
		"schedule_minutes": map[string]any{"type": "integer", "title": "Every (minutes)", "minimum": 1, "maximum": 525600},
	}
	if !required {
		props["rule_uuid"] = ruleIDField
	}
	return props
}

func actionItemSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"type"},
		"properties": map[string]any{
			"type":        map[string]any{"type": "string", "enum": draftActionTypes()},
			"value":       map[string]any{"type": "string", "maxLength": 200},
			"content":     map[string]any{"type": "string", "maxLength": 10000},
			"target_uuid": map[string]any{"type": "string", "pattern": "^[0-9a-fA-F-]{36}$"},
		}}
}

// RuleWriteCapabilities are the confirmed rule changes.
func RuleWriteCapabilities() []*capability.Capability {
	routes := []string{"/rules"}
	newRule := func(p capability.Params) (model.RuleInput, error) {
		in := model.RuleInput{Conditions: model.Condition{All: []model.Condition{}}, Enabled: true}
		if err := applyRuleParams(&in, p); err != nil {
			return in, err
		}
		return in, validateRule(in)
	}
	return []*capability.Capability{
		{
			Name: "automations.create", Module: "automation", Kind: capability.Write, Permission: rbac.PermTenantAutomationWrite, Routes: routes,
			Title: map[string]string{"tr": "Otomasyon kuralı oluştur", "en": "Create an automation rule"},
			Description: "Create an automation rule from the user's sentence: a trigger (e.g. message.received, sla.breached, scheduled), " +
				"a condition tree and up to 20 actions. Use workflows_reference for valid values. For multi-step flows with waits use workflows_create.",
			Undo: "/rules",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "trigger", "actions"},
				"properties": ruleFields(true)},
			Validate: func(p capability.Params) error { _, err := newRule(p); return err },
			DryRun: func(_ context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				in, err := newRule(p)
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: t(e, "“%s” kuralı geçerli; kaydedilince son konuşmada denenecek.",
					"“%s” is valid; after saving it is tested on the latest conversation.", in.Name), Details: describeRule(e, in)}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				in, err := newRule(p)
				if err != nil {
					return capability.Result{}, err
				}
				r, err := e.API.Call(ctx, http.MethodPost, rulesPath, ruleBody(in))
				if err != nil {
					return capability.Result{}, err
				}
				var row ruleRow
				if err := r.Decode(&row); err != nil {
					return capability.Result{}, err
				}
				route := "/rules/" + row.UUID.String()
				return capability.Result{Summary: row.UUID.String(), Details: describeRule(e, in), Route: route, Focus: &capability.Focus{Route: route}}, nil
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, res capability.Result) capability.Check {
				if _, err := getRule(ctx, e.API, res.Summary); err != nil {
					return capability.Check{Status: capability.VerifyFailed}
				}
				return simulateRule(ctx, e, res.Summary)
			},
		},
		{
			Name: "automations.update", Module: "automation", Kind: capability.Write, Permission: rbac.PermTenantAutomationWrite, Routes: routes,
			Title:       map[string]string{"tr": "Otomasyon kuralını güncelle", "en": "Update an automation rule"},
			Description: "Change fields of an existing rule (rule_uuid from automations_list); fields left out stay as they are.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"rule_uuid"},
				"properties": ruleFields(false)},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				row, err := getRule(ctx, e.API, str(p, "rule_uuid"))
				if err != nil {
					return capability.Result{}, err
				}
				in, err := row.input()
				if err != nil {
					return capability.Result{}, err
				}
				if err := applyRuleParams(&in, p); err != nil {
					return capability.Result{}, err
				}
				if err := validateRule(in); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: t(e, "“%s” kuralı güncellenecek.", "Rule “%s” will be updated.", row.Name), Details: describeRule(e, in)}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := str(p, "rule_uuid")
				row, err := getRule(ctx, e.API, id)
				if err != nil {
					return capability.Result{}, err
				}
				in, err := row.input()
				if err != nil {
					return capability.Result{}, err
				}
				if err := applyRuleParams(&in, p); err != nil {
					return capability.Result{}, err
				}
				if err := validateRule(in); err != nil {
					return capability.Result{}, err
				}
				r, err := e.API.Call(ctx, http.MethodPut, rulesPath+"/"+id, ruleBody(in))
				if err != nil {
					return capability.Result{}, err
				}
				if err := r.Err(); err != nil {
					return capability.Result{}, err
				}
				route := "/rules/" + id
				return capability.Result{Summary: id, Details: describeRule(e, in), Route: route, Focus: &capability.Focus{Route: route}}, nil
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, res capability.Result) capability.Check {
				return simulateRule(ctx, e, res.Summary)
			},
		},
		{
			Name: "automations.delete", Module: "automation", Kind: capability.Destructive, Permission: rbac.PermTenantAutomationWrite, Routes: routes,
			Title:       map[string]string{"tr": "Otomasyon kuralını sil", "en": "Delete an automation rule"},
			Description: "Delete an automation rule for good. To pause it instead, propose automations_update with enabled=false.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"rule_uuid"},
				"properties": map[string]any{"rule_uuid": ruleIDField}},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				row, err := getRule(ctx, e.API, str(p, "rule_uuid"))
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: t(e, "“%s” kuralı kalıcı olarak silinecek; geri alınamaz.", "Rule “%s” will be deleted for good.", row.Name)}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := str(p, "rule_uuid")
				row, err := getRule(ctx, e.API, id)
				if err != nil {
					return capability.Result{}, err
				}
				r, err := e.API.Call(ctx, http.MethodDelete, rulesPath+"/"+id, nil)
				if err != nil {
					return capability.Result{}, err
				}
				if err := r.Err(); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: row.Name, Route: "/rules"}, nil
			},
			Verify: func(ctx context.Context, e capability.Exec, p capability.Params, _ capability.Result) capability.Check {
				_, err := getRule(ctx, e.API, str(p, "rule_uuid"))
				var api *capability.APIError
				if errors.As(err, &api) && api.Status == http.StatusNotFound {
					return capability.Check{Status: capability.VerifyOK}
				}
				return capability.Check{Status: capability.VerifyFailed}
			},
		},
	}
}
