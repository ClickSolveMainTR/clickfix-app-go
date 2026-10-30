// Package model defines the shared native/external issue contract.
package model

import (
	"encoding/json"
	"github.com/google/uuid"
	"time"
)

type Issue struct {
	UUID        uuid.UUID       `json:"uuid"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      string          `json:"status"`
	Priority    string          `json:"priority"`
	Labels      []string        `json:"labels"`
	Cycle       string          `json:"cycle"`
	Project     *uuid.UUID      `json:"project_uuid"`
	Assignee    *uuid.UUID      `json:"assignee_uuid"`
	Release     *uuid.UUID      `json:"release_uuid"`
	Context     json.RawMessage `json:"context"`
	Revision    int32           `json:"revision"`
	ResolvedAt  *time.Time      `json:"resolved_at"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type CreateInput struct {
	AISummary    bool            `json:"ai_summary"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Status       string          `json:"status"`
	Priority     string          `json:"priority"`
	Labels       []string        `json:"labels"`
	Cycle        string          `json:"cycle"`
	Project      *uuid.UUID      `json:"project_uuid"`
	Assignee     *uuid.UUID      `json:"assignee_uuid"`
	Conversation *uuid.UUID      `json:"conversation_uuid"`
	Context      json.RawMessage `json:"context"`
}

// PatchInput requires the revision the editor saw to prevent lost updates.
type PatchInput struct {
	Revision    int32     `json:"revision"`
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	Status      *string   `json:"status"`
	Priority    *string   `json:"priority"`
	Labels      *[]string `json:"labels"`
	Cycle       *string   `json:"cycle"`
	// Empty string clears a reference; omitted keeps it.
	Project  *string `json:"project_uuid"`
	Assignee *string `json:"assignee_uuid"`
	Release  *string `json:"release_uuid"`
}

type LinkInput struct {
	Conversation   uuid.UUID `json:"conversation_uuid"`
	NotifyCustomer bool      `json:"notify_customer"`
}
type EscalationInput struct {
	Tier       string    `json:"tier"`
	Team       uuid.UUID `json:"team_uuid"`
	Reason     string    `json:"reason"`
	SLASeconds int32     `json:"sla_seconds"`
}
type Preview struct {
	Conversation uuid.UUID `json:"conversation_uuid"`
	Channel      string    `json:"channel"`
	Eligible     bool      `json:"eligible"`
	Reason       string    `json:"reason"`
	Content      string    `json:"content"`
}
