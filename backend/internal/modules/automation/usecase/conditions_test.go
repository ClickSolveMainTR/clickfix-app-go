package usecase

import (
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"strings"
	"testing"
)

func condition(t *testing.T, s string) model.Condition {
	t.Helper()
	var c model.Condition
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		t.Fatal(err)
	}
	return c
}
func TestConditionsCustomFieldsAndMissingData(t *testing.T) {
	data := map[string]any{"status": "pending", "hours_waiting": 49.0, "custom_fields": map[string]any{"plan": "enterprise", "seats": 25.0}, "tag_uuids": []any{"vip", "paid"}}
	tests := []struct {
		tree string
		want bool
	}{
		{`{"all":[{"field":"status","operator":"eq","value":"pending"},{"field":"hours_waiting","operator":"gt","value":48},{"any":[{"field":"custom_fields.plan","operator":"eq","value":"enterprise"},{"field":"tag_uuids","operator":"contains","value":"vip"}]}]}`, true},
		{`{"field":"custom_fields.seats","operator":"gte","value":25}`, true},
		{`{"field":"custom_fields.missing","operator":"neq","value":"x"}`, false},
		{`{"field":"custom_fields.missing","operator":"not_in","value":["x"]}`, false},
		{`{"field":"custom_fields.missing","operator":"not_exists"}`, true},
		{`{"field":"status","operator":"gt","value":10}`, false},
		{`{"field":"status","operator":"in","value":["pending","open"]}`, true},
		{`{"all":[]}`, true}, {`{"any":[]}`, false},
	}
	for _, tc := range tests {
		c := condition(t, tc.tree)
		n := 0
		if err := validateCondition(c, 0, &n); err != nil {
			t.Fatal(err)
		}
		if got := matches(c, data); got != tc.want {
			t.Errorf("%s: got %t", tc.tree, got)
		}
	}
}
func TestInvalidConditionTrees(t *testing.T) {
	for _, raw := range []string{`{}`, `{"all":[],"any":[]}`, `{"field":"password_hash","operator":"eq","value":"x"}`, `{"field":"status","operator":"in","value":"pending"}`, `{"field":"status","operator":"sql","value":"x"}`, `{"field":"hours_waiting","operator":"gt","value":"48"}`} {
		n := 0
		if err := validateCondition(condition(t, raw), 0, &n); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	raw := strings.Repeat(`{"all":[`, 10) + `{"all":[]}` + strings.Repeat(`]}`, 10)
	n := 0
	if validateCondition(condition(t, raw), 0, &n) == nil {
		t.Fatal("unbounded recursion")
	}
}
func TestRuleRejectsUnknownActionsAndSchedules(t *testing.T) {
	in := model.RuleInput{Name: "pending", Trigger: "scheduled", ScheduleMinutes: 60, Conditions: condition(t, `{"all":[]}`), Actions: []model.Action{{Type: "set_status", Value: "resolved"}}}
	if err := Validate(in); err != nil {
		t.Fatal(err)
	}
	in.ScheduleMinutes = 0
	if Validate(in) == nil {
		t.Fatal("scheduled rule without scan interval")
	}
	in.ScheduleMinutes = 60
	in.Actions[0].Type = "arbitrary_sql"
	if Validate(in) == nil {
		t.Fatal("unknown action")
	}
}
func TestEmptyConditionGroupsSurviveStorage(t *testing.T) {
	for _, raw := range []string{`{"all":[]}`, `{"any":[]}`} {
		c := condition(t, raw)
		stored, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		restored := condition(t, string(stored))
		n := 0
		if err = validateCondition(restored, 0, &n); err != nil {
			t.Fatalf("%s became %s: %v", raw, stored, err)
		}
		if matches(c, map[string]any{}) != matches(restored, map[string]any{}) {
			t.Fatal("changed meaning after storage")
		}
	}
}

func TestScreenContextConditions(t *testing.T) {
	data := map[string]any{"screen": map[string]any{"page_path": "/faturalar/F-1", "last_event": "payment_failed", "event_names": []any{"opened", "payment_failed"}}}
	n := 0
	for _, c := range []model.Condition{
		{Field: "screen.page_path", Operator: "contains", Value: "/faturalar"},
		{Field: "screen.last_event", Operator: "eq", Value: "payment_failed"},
	} {
		if err := validateCondition(c, 0, &n); err != nil {
			t.Fatalf("%s rejected: %v", c.Field, err)
		}
		if !matches(c, data) {
			t.Fatalf("%s did not match", c.Field)
		}
	}
	if matches(model.Condition{Field: "screen.last_event", Operator: "eq", Value: "opened"}, data) {
		t.Fatal("wrong event matched")
	}
	if err := Validate(model.RuleInput{Name: "x", Trigger: "conversation.created", Conditions: model.Condition{All: []model.Condition{}},
		Actions: []model.Action{{Type: "run_system_action", Value: "fatura_tekrar_gonder"}}}); err != nil {
		t.Fatalf("system action rejected: %v", err)
	}
	if err := Validate(model.RuleInput{Name: "x", Trigger: "conversation.created", Conditions: model.Condition{All: []model.Condition{}},
		Actions: []model.Action{{Type: "run_system_action", Value: "Bad Key"}}}); err == nil {
		t.Fatal("bad system action key accepted")
	}
}
