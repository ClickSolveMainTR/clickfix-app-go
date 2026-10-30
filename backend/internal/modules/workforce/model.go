// Package workforce provides seasonal staffing estimates and bounded activity timesheets.
package workforce

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/google/uuid"
	"math"
	"time"
)

type ForecastInput struct {
	Since         time.Time  `json:"since"`
	Until         time.Time  `json:"until"`
	Channel       string     `json:"channel"`
	Team          *uuid.UUID `json:"team_uuid"`
	Alpha         float64    `json:"alpha"`
	HandleSeconds float64    `json:"handle_seconds"`
	SLASeconds    float64    `json:"sla_seconds"`
	TargetSLA     float64    `json:"target_sla"`
	Granularity   string     `json:"granularity"`
}
type Bucket struct {
	Start           time.Time `json:"start"`
	Volume          float64   `json:"volume"`
	Required        int       `json:"required_agents"`
	Scheduled       int64     `json:"scheduled_agents"`
	Gap             int64     `json:"gap"`
	CapacityLimited bool      `json:"capacity_limited"`
}
type Daily struct {
	Date            string  `json:"date"`
	Volume          float64 `json:"volume"`
	Required        int     `json:"required_agent_hours"`
	Scheduled       int64   `json:"scheduled_agent_hours"`
	Gap             int64   `json:"gap_agent_hours"`
	CapacityLimited bool    `json:"capacity_limited"`
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (in *ForecastInput) validate(now time.Time) error {
	if in.Alpha == 0 {
		in.Alpha = .3
	}
	if in.HandleSeconds == 0 {
		in.HandleSeconds = 300
	}
	if in.SLASeconds == 0 {
		in.SLASeconds = 30
	}
	if in.TargetSLA == 0 {
		in.TargetSLA = .8
	}
	if in.Granularity == "" {
		in.Granularity = "hour"
	}
	if !in.Until.After(in.Since) || in.Until.Sub(in.Since) > 31*24*time.Hour || in.Since.Before(now.Truncate(time.Hour)) || in.Since.After(now.Add(90*24*time.Hour)) || !in.Since.Equal(in.Since.Truncate(time.Hour)) || !in.Until.Equal(in.Until.Truncate(time.Hour)) || !finite(in.Alpha) || in.Alpha <= 0 || in.Alpha > 1 || !finite(in.HandleSeconds) || in.HandleSeconds < 1 || in.HandleSeconds > 86400 || !finite(in.SLASeconds) || in.SLASeconds < 1 || in.SLASeconds > 86400 || !finite(in.TargetSLA) || in.TargetSLA <= 0 || in.TargetSLA >= 1 || len(in.Channel) > 40 || (in.Team != nil && *in.Team == uuid.Nil) || (in.Granularity != "hour" && in.Granularity != "day") {
		return supportops.ErrInvalid
	}
	return nil
}

// RequiredAgents uses the stable Erlang-B recurrence to obtain Erlang C.
// A hard limit bounds CPU; limited=true explicitly marks unmet SLA at the ceiling.
func RequiredAgents(volume, handle, wait, target float64) (agents int, limited bool) {
	if volume <= 0 {
		return 0, false
	}
	a := volume * handle / 3600
	b := 1.0
	for n := 1; n <= 10000; n++ {
		b = a * b / (float64(n) + a*b)
		if float64(n) <= a {
			continue
		}
		rho := a / float64(n)
		c := b / (1 - rho + rho*b)
		if 1-c*math.Exp(-(float64(n)-a)*wait/handle) >= target {
			return n, false
		}
	}
	return 10000, true
}

// Seasonal includes zero-volume hours and smooths each local weekday/hour independently.
func Seasonal(since, until time.Time, loc *time.Location, counts map[time.Time]float64, alpha float64) [168]float64 {
	var values [168]float64
	var seen [168]bool
	for t := since; t.Before(until); t = t.Add(time.Hour) {
		local := t.In(loc)
		k := int(local.Weekday())*24 + local.Hour()
		v := counts[t]
		if !seen[k] {
			values[k] = v
			seen[k] = true
		} else {
			values[k] = alpha*v + (1-alpha)*values[k]
		}
	}
	return values
}
