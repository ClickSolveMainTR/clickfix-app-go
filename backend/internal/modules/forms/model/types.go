// Package model holds the API shapes of dynamic request forms (CF-52).
package model

import (
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/engine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
)

// Channels a form can be offered on: the web widget pre-chat, the web
// (help center) ticket form, e-mail ticket creation and agent-created
// conversations.
var Channels = []string{"widget", "web", "email", "agent"}

// Form is a request form.
type Form struct {
	UUID        uuid.UUID      `json:"uuid"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	IsActive    bool           `json:"is_active"`
	Channels    []string       `json:"channels"`
	Position    int32          `json:"position"`
	Fields      []engine.Field `json:"fields"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// FormInput creates or replaces a form.
type FormInput struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	IsActive    *bool          `json:"is_active"`
	Channels    []string       `json:"channels"`
	Position    *int32         `json:"position"`
	Fields      []engine.Field `json:"fields"`
}

// PublicField is a form field as a client renders it.
type PublicField struct {
	Key         string                  `json:"key"`
	Source      string                  `json:"source"`
	Type        customfields.FieldType  `json:"type"`
	Label       string                  `json:"label"`
	Help        string                  `json:"help"`
	Required    bool                    `json:"required"`
	Options     []customfields.Option   `json:"options"`
	Placeholder string                  `json:"placeholder"`
	Condition   *customfields.Condition `json:"condition"`
}

// PublicForm is what a channel shows (no internal flags).
type PublicForm struct {
	UUID        uuid.UUID     `json:"uuid"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Fields      []PublicField `json:"fields"`
}

// ConversationForm is the form a conversation started with, its current
// conversation field values (by form key) and what still blocks resolving.
type ConversationForm struct {
	Form              *PublicForm    `json:"form"`
	Values            map[string]any `json:"values"`
	RequiredToResolve []string       `json:"required_to_resolve"`
	MissingToResolve  []string       `json:"missing_to_resolve"`
}

// Submission is a form submitted with a first message.
type Submission struct {
	FormUUID uuid.UUID      `json:"form_uuid"`
	Values   map[string]any `json:"values"`
}
