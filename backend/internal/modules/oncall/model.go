// Package oncall implements organization rotas, escalation and HTTP uptime checks.
package oncall

import (
	"time"

	"github.com/google/uuid"
)

type Layer struct {
	StartsAt        time.Time   `json:"starts_at"`
	EndsAt          *time.Time  `json:"ends_at,omitempty"`
	RotationMinutes int         `json:"rotation_minutes"`
	Agents          []uuid.UUID `json:"agents"`
}
type Override struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Agent    uuid.UUID `json:"agent"`
}
type RotaInput struct {
	Name      string     `json:"name"`
	Timezone  string     `json:"timezone"`
	Layers    []Layer    `json:"layers"`
	Overrides []Override `json:"overrides"`
}
type Step struct {
	AfterMinutes int        `json:"after_minutes"`
	Agent        *uuid.UUID `json:"agent,omitempty"`
	Rota         *uuid.UUID `json:"rota,omitempty"`
	Team         *uuid.UUID `json:"team,omitempty"`
	Channels     []string   `json:"channels"`
}
type PolicyInput struct {
	Name                string `json:"name"`
	Steps               []Step `json:"steps"`
	UrgentConversations bool   `json:"urgent_conversations"`
	Incidents           bool   `json:"incidents"`
	Enabled             bool   `json:"enabled"`
}
type MonitorInput struct {
	Name            string     `json:"name"`
	URL             string     `json:"url"`
	IntervalSeconds int32      `json:"interval_seconds"`
	ExpectedStatus  int32      `json:"expected_status"`
	Keyword         string     `json:"keyword"`
	Component       uuid.UUID  `json:"component_uuid"`
	Policy          *uuid.UUID `json:"policy_uuid,omitempty"`
	Enabled         bool       `json:"enabled"`
}

// Agents uses half-open windows. Later layers and the latest matching override take precedence.
func Agents(layers []Layer, overrides []Override, at time.Time) []uuid.UUID {
	out := []uuid.UUID{}
	for _, l := range layers {
		if at.Before(l.StartsAt) || (l.EndsAt != nil && !at.Before(*l.EndsAt)) || len(l.Agents) == 0 || l.RotationMinutes < 1 {
			continue
		}
		n := int(at.Sub(l.StartsAt)/time.Minute) / l.RotationMinutes
		out = []uuid.UUID{l.Agents[n%len(l.Agents)]}
	}
	for _, o := range overrides {
		if !at.Before(o.StartsAt) && at.Before(o.EndsAt) {
			out = []uuid.UUID{o.Agent}
		}
	}
	return out
}
