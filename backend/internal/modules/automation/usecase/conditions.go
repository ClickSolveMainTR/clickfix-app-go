package usecase

import (
	"encoding/json"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/model"
	"reflect"
	"slices"
	"strings"
)

var fields = []string{"uuid", "number", "status", "priority", "subject", "channel_type", "source", "inbox.uuid", "contact.uuid", "contact.name", "contact.email", "contact.phone", "company.uuid", "company.name", "assignee.uuid", "team_id", "tag_uuids", "unread_count", "message_count", "last_message_direction", "hours_since_created", "hours_since_updated", "hours_since_inbound", "hours_waiting", "sla_breached", "clock.hour", "clock.weekday", "screen.page_url", "screen.page_path", "screen.last_event", "screen.event_names"}
var operators = []string{"eq", "neq", "contains", "not_contains", "in", "not_in", "gt", "gte", "lt", "lte", "exists", "not_exists"}

func validateCondition(c model.Condition, depth int, n *int) error {
	*n++
	if depth > 8 || *n > 128 {
		return fmt.Errorf("condition tree exceeds depth 8 or 128 nodes")
	}
	groups := 0
	if c.All != nil {
		groups++
	}
	if c.Any != nil {
		groups++
	}
	if c.Field != "" {
		groups++
	}
	if groups != 1 {
		return fmt.Errorf("condition must contain exactly one of all, any or field")
	}
	if c.All != nil || c.Any != nil {
		if c.Operator != "" || c.Value != nil {
			return fmt.Errorf("groups cannot contain operators or values")
		}
		for _, sub := range append(append([]model.Condition{}, c.All...), c.Any...) {
			if err := validateCondition(sub, depth+1, n); err != nil {
				return err
			}
		}
		return nil
	}
	if !slices.Contains(fields, c.Field) && (!strings.HasPrefix(c.Field, "custom_fields.") || len(c.Field) > 180 || strings.TrimPrefix(c.Field, "custom_fields.") == "") {
		return fmt.Errorf("unknown condition field: %s", c.Field)
	}
	if !slices.Contains(operators, c.Operator) {
		return fmt.Errorf("unknown condition operator: %s", c.Operator)
	}
	if c.Operator == "in" || c.Operator == "not_in" {
		if _, ok := c.Value.([]any); !ok {
			return fmt.Errorf("in operator requires an array")
		}
	}
	if slices.Contains([]string{"gt", "gte", "lt", "lte"}, c.Operator) {
		if _, ok := c.Value.(float64); !ok {
			return fmt.Errorf("numeric comparison requires a number")
		}
	}
	return nil
}
func lookup(data map[string]any, path string) (any, bool) {
	var cur any = data
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, cur != nil
}
func matches(c model.Condition, data map[string]any) bool {
	if c.All != nil {
		for _, s := range c.All {
			if !matches(s, data) {
				return false
			}
		}
		return true
	}
	if c.Any != nil {
		for _, s := range c.Any {
			if matches(s, data) {
				return true
			}
		}
		return false
	}
	v, exists := lookup(data, c.Field)
	switch c.Operator {
	case "exists":
		return exists
	case "not_exists":
		return !exists
	}
	// Missing data never satisfies a comparison, including negated comparisons.
	if !exists {
		return false
	}
	switch c.Operator {
	case "eq":
		return reflect.DeepEqual(v, c.Value)
	case "neq":
		return !reflect.DeepEqual(v, c.Value)
	case "contains", "not_contains":
		found := false
		switch x := v.(type) {
		case string:
			if y, ok := c.Value.(string); ok {
				found = strings.Contains(x, y)
			}
		case []any:
			for _, item := range x {
				if reflect.DeepEqual(item, c.Value) {
					found = true
					break
				}
			}
		default:
			return false
		}
		if c.Operator == "not_contains" {
			return !found
		}
		return found
	case "in", "not_in":
		found := false
		for _, item := range c.Value.([]any) {
			if reflect.DeepEqual(item, v) {
				found = true
				break
			}
		}
		if c.Operator == "not_in" {
			return !found
		}
		return found
	case "gt", "gte", "lt", "lte":
		x, ok := v.(float64)
		y, yok := c.Value.(float64)
		if !ok || !yok {
			return false
		}
		switch c.Operator {
		case "gt":
			return x > y
		case "gte":
			return x >= y
		case "lt":
			return x < y
		case "lte":
			return x <= y
		}
	}
	return false
}
func decodeRule(conditions, actions []byte) (model.Condition, []model.Action, error) {
	var c model.Condition
	var a []model.Action
	if err := json.Unmarshal(conditions, &c); err != nil {
		return c, a, err
	}
	err := json.Unmarshal(actions, &a)
	return c, a, err
}

// ConditionFields lists the conversation fields a condition may test
// (custom_fields.<key> is accepted too).
func ConditionFields() []string { return slices.Clone(fields) }

// ConditionOperators lists the condition operators.
func ConditionOperators() []string { return slices.Clone(operators) }
