package model

import (
	"encoding/json"
	"github.com/google/uuid"
)

// Condition is a bounded recursive all/any tree or one field comparison.
// Empty all matches every conversation; empty any matches none.
type Condition struct {
	All      []Condition `json:"all,omitempty"`
	Any      []Condition `json:"any,omitempty"`
	Field    string      `json:"field,omitempty"`
	Operator string      `json:"operator,omitempty"`
	Value    any         `json:"value,omitempty"`
}
type Action struct {
	Type       string    `json:"type"`
	Value      string    `json:"value,omitempty"`
	Content    string    `json:"content,omitempty"`
	TargetUUID uuid.UUID `json:"target_uuid,omitempty"`
}
type RuleInput struct {
	Name            string    `json:"name"`
	Trigger         string    `json:"trigger"`
	Conditions      Condition `json:"conditions"`
	Actions         []Action  `json:"actions"`
	Position        int32     `json:"position"`
	Enabled         bool      `json:"enabled"`
	StopOnMatch     bool      `json:"stop_on_match"`
	ScheduleMinutes int32     `json:"schedule_minutes"`
}

// MarshalJSON preserves empty all/any groups; omitempty on an empty slice
// would turn an unconditional rule into an invalid empty leaf after storage.
func (c Condition) MarshalJSON() ([]byte, error) {
	if c.All != nil {
		return json.Marshal(map[string]any{"all": c.All})
	}
	if c.Any != nil {
		return json.Marshal(map[string]any{"any": c.Any})
	}
	type leaf Condition
	return json.Marshal(leaf(c))
}
