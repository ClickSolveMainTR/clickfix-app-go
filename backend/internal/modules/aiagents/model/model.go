// Package model defines organization AI agent policies and structured results.
package model

import (
	"errors"
	"slices"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("aiagents: invalid input")
var ErrConflict = errors.New("aiagents: conflicting state")

// Config is the organization's AI agent policy. The agent has no schedule
// of its own (CF-198): BusinessHoursOnly limits it to the organization's
// working hours, which the SLA module's schedule defines.
type Config struct {
	Mode          string   `json:"mode"`
	TriageEnabled bool     `json:"triage_enabled"`
	Persona       string   `json:"persona"`
	Channels      []string `json:"channels"`
	Categories    []string `json:"categories"`
	Locale        string   `json:"locale"`
	// BusinessHoursOnly: outside the organization's working hours the agent
	// hands off instead of answering (false: around the clock).
	BusinessHoursOnly bool     `json:"business_hours_only"`
	Confidence        float64  `json:"confidence_threshold"`
	HandoffKeywords   []string `json:"handoff_keywords"`
}

func DefaultConfig() Config {
	return Config{Mode: "off", Locale: "tr", Confidence: 0.85, Channels: []string{}, Categories: []string{}, HandoffKeywords: []string{"insan", "temsilci", "human", "agent"}}
}
func (c Config) Validate() error {
	if !slices.Contains([]string{"off", "summary", "draft", "autonomous"}, c.Mode) || len(c.Persona) > 3000 || len(c.Channels) > 20 || len(c.Categories) > 50 || len(c.HandoffKeywords) > 30 {
		return ErrInvalid
	}
	if c.Locale != "tr" && c.Locale != "en" {
		return ErrInvalid
	}
	if c.Confidence < 0.5 || c.Confidence > 1 {
		return ErrInvalid
	}
	for _, value := range append(append(append([]string{}, c.Channels...), c.Categories...), c.HandoffKeywords...) {
		if len(value) == 0 || len(value) > 100 {
			return ErrInvalid
		}
	}
	return nil
}

// Handoff reasons of an inactive agent (CF-243): the run log and the
// conversation's AI jobs tell a channel left out of the agent's scope apart
// from a message outside working hours.
const (
	ReasonChannelNotEnabled    = "Channel not enabled for AI agent"
	ReasonOutsideBusinessHours = "Outside business hours"
)

// Active reports whether the agent handles channel now. open is the
// organization's working-hours state (businesshours.Source); it only
// matters with BusinessHoursOnly.
func (c Config) Active(channel string, open bool) bool {
	return c.InactiveReason(channel, open) == ""
}

// InactiveReason is why the agent does not handle channel now, or "" when
// it does (see Active).
func (c Config) InactiveReason(channel string, open bool) string {
	if !slices.Contains(c.Channels, channel) {
		return ReasonChannelNotEnabled
	}
	if !open && c.BusinessHoursOnly {
		return ReasonOutsideBusinessHours
	}
	return ""
}

type Triage struct {
	SuggestedAssignee    *uuid.UUID          `json:"suggested_assignee_uuid,omitempty"`
	SimilarConversations []ConversationMatch `json:"similar_conversations"`
	OpenIssues           []IssueMatch        `json:"open_issues"`
	Language             string              `json:"language"`
	Intent               string              `json:"intent"`
	Category             string              `json:"category"`
	Sentiment            string              `json:"sentiment"`
	Urgency              string              `json:"urgency"`
	Priority             string              `json:"priority"`
	Summary              string              `json:"summary"`
	Tags                 []string            `json:"suggested_tags"`
	Team                 string              `json:"suggested_team"`
	Confidence           float64             `json:"confidence"`
}

type Reply struct {
	Text       string      `json:"reply"`
	Summary    string      `json:"summary"`
	Confidence float64     `json:"confidence"`
	Handoff    bool        `json:"handoff"`
	Citations  []uuid.UUID `json:"citation_ids"`
	// UnitUUID is the knowledge unit that answered without a generation
	// call, Memory the cache outcome ("exact" or "semantic"; CF-216).
	UnitUUID *uuid.UUID `json:"unit_uuid,omitempty"`
	Memory   string     `json:"memory,omitempty"`
}

type CopilotInput struct {
	Operation string `json:"operation"`
	Text      string `json:"text"`
	Locale    string `json:"locale"`
	Tone      string `json:"tone"`
}

type ConversationMatch struct {
	UUID    uuid.UUID `json:"uuid"`
	Subject string    `json:"subject"`
	Status  string    `json:"status"`
	Score   float32   `json:"score"`
}
type IssueMatch struct {
	UUID  uuid.UUID `json:"uuid"`
	Title string    `json:"title"`
	Score float64   `json:"score"`
}
