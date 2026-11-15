// Package model holds the category taxonomy DTOs (CF-218).
package model

import (
	"time"

	"github.com/google/uuid"
)

// Resource is the activity / meta resource name.
const Resource = "tenant.categories"

// MaxDepth is the deepest level (0 = root).
const MaxDepth = 4

// Category is one node of the tree.
type Category struct {
	UUID uuid.UUID `json:"uuid"`
	// DepartmentUUID nil = organization-wide (CF-215 departments).
	DepartmentUUID *uuid.UUID `json:"department_uuid"`
	ParentUUID     *uuid.UUID `json:"parent_uuid"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Description    string     `json:"description"`
	Examples       []string   `json:"examples"`
	Position       int32      `json:"position"`
	Depth          int16      `json:"depth"`
	Archived       bool       `json:"archived"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// CategoryInput creates or patches a node. Pointer fields left nil keep the
// current value on PATCH; ParentUUID / DepartmentUUID take "" to clear.
type CategoryInput struct {
	Name           *string   `json:"name"`
	Slug           *string   `json:"slug"`
	Description    *string   `json:"description"`
	Examples       *[]string `json:"examples"`
	Position       *int32    `json:"position"`
	ParentUUID     *string   `json:"parent_uuid"`
	DepartmentUUID *string   `json:"department_uuid"`
	Archived       *bool     `json:"archived"`
}

// CategoryRef is the short form embedded in assignments and labels.
type CategoryRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

// Assignment is the current category of one record.
type Assignment struct {
	SubjectType string       `json:"subject_type"`
	SubjectUUID uuid.UUID    `json:"subject_uuid"`
	Category    *CategoryRef `json:"category"`
	Confidence  float64      `json:"confidence"`
	Source      string       `json:"source"`
	Classifier  string       `json:"classifier"`
	NeedsReview bool         `json:"needs_review"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// LabelInput is an agent's confirmation or correction.
type LabelInput struct {
	CategoryUUID uuid.UUID `json:"category_uuid"`
}

// LabelExport is one JSONL line of the training export (CF-219).
type LabelExport struct {
	UUID             uuid.UUID    `json:"uuid"`
	CreatedAt        time.Time    `json:"created_at"`
	SubjectType      string       `json:"subject_type"`
	DepartmentUUID   *uuid.UUID   `json:"department_uuid"`
	Action           string       `json:"action"`
	Text             string       `json:"text"`
	EmbeddingModel   string       `json:"embedding_model"`
	Embedding        []float32    `json:"embedding"`
	Category         CategoryRef  `json:"category"`
	PreviousCategory *CategoryRef `json:"previous_category"`
	PreviousSource   *string      `json:"previous_source"`
	PreviousConf     *float64     `json:"previous_confidence"`
	LabeledBy        uuid.UUID    `json:"labeled_by"`
}

// ReviewFilter narrows the review queue.
type ReviewFilter struct {
	SubjectType  string
	DepartmentID int64
	Limit        int32
	Offset       int32
}
