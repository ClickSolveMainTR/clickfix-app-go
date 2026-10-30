package model

import (
	"time"

	"github.com/google/uuid"
)

type Filters struct {
	Channel string     `json:"channel,omitempty"`
	Team    *uuid.UUID `json:"team,omitempty"`
	Agent   *uuid.UUID `json:"agent,omitempty"`
}
type Definition struct {
	Dimensions []string `json:"dimensions"`
	Metrics    []string `json:"metrics"`
	Filters    Filters  `json:"filters"`
	ChartType  string   `json:"chart_type"`
	Timezone   string   `json:"timezone"`
}
type Input struct {
	Name       string     `json:"name"`
	Definition Definition `json:"definition"`
}
type Evaluation struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}
type ScheduleInput struct {
	ReportUUID uuid.UUID   `json:"report_uuid"`
	Frequency  string      `json:"frequency"`
	Timezone   string      `json:"timezone"`
	Format     string      `json:"format"`
	Recipients []uuid.UUID `json:"recipients"`
	Enabled    bool        `json:"enabled"`
}

var Metrics = []string{"volume", "first_response_seconds", "resolution_seconds", "csat_average", "qa_score", "sla_breaches"}
