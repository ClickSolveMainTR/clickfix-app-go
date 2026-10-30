// Package model defines organization AI agent policies and structured results.
package model

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("aiagents: invalid input")
var ErrConflict = errors.New("aiagents: conflicting state")

type Config struct {
	Mode            string   `json:"mode"`
	TriageEnabled   bool     `json:"triage_enabled"`
	Persona         string   `json:"persona"`
	Channels        []string `json:"channels"`
	Categories      []string `json:"categories"`
	Locale          string   `json:"locale"`
	Timezone        string   `json:"timezone"`
	StartMinute     int      `json:"start_minute"`
	EndMinute       int      `json:"end_minute"`
	Weekdays        []int    `json:"weekdays"`
	Confidence      float64  `json:"confidence_threshold"`
	HandoffKeywords []string `json:"handoff_keywords"`
}

func DefaultConfig() Config {
	return Config{Mode: "off", Locale: "tr", Timezone: "Europe/Istanbul", Confidence: 0.85, Channels: []string{}, Categories: []string{}, Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, EndMinute: 1440, HandoffKeywords: []string{"insan", "temsilci", "human", "agent"}}
}
func (c Config) Validate() error {
	if !slices.Contains([]string{"off", "summary", "draft", "autonomous"}, c.Mode) || len(c.Persona) > 3000 || len(c.Channels) > 20 || len(c.Categories) > 50 || len(c.HandoffKeywords) > 30 {
		return ErrInvalid
	}
	if c.Locale != "tr" && c.Locale != "en" {
		return ErrInvalid
	}
	if c.Confidence < 0.5 || c.Confidence > 1 || c.StartMinute < 0 || c.EndMinute > 1440 || c.StartMinute >= c.EndMinute {
		return ErrInvalid
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return ErrInvalid
	}
	for _, day := range c.Weekdays {
		if day < 0 || day > 6 {
			return ErrInvalid
		}
	}
	for _, value := range append(append(append([]string{}, c.Channels...), c.Categories...), c.HandoffKeywords...) {
		if len(value) == 0 || len(value) > 100 {
			return ErrInvalid
		}
	}
	return nil
}
func (c Config) Active(at time.Time, channel string) bool {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil || !slices.Contains(c.Channels, channel) {
		return false
	}
	now := at.In(loc)
	minute := now.Hour()*60 + now.Minute()
	return slices.Contains(c.Weekdays, int(now.Weekday())) && minute >= c.StartMinute && minute < c.EndMinute
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
