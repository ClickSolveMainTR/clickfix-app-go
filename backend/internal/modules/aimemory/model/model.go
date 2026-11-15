// Package model holds the CF-216 knowledge-unit, answer-cache and
// case-memory types shared by the customer AI and the agent assistant.
package model

import (
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalid  = errors.New("aimemory: invalid input")
	ErrNotFound = errors.New("aimemory: not found")
	ErrConflict = errors.New("aimemory: conflict")
)

// SuggestionReviewed is emitted when a knowledge suggestion is approved or
// rejected; the knowledge layer rebuilds the suggestion's units.
const SuggestionReviewed = "knowledge.suggestion_reviewed"

// Audiences. The customer AI sees public units and approved, anonymized case
// summaries only; the agent assistant also sees internal units and pending
// summaries.
const (
	AudienceCustomer = "customer"
	AudienceAgent    = "agent"
)

// Lookup outcomes. Exact repeats cost no AI call, semantic hits cost one
// embedding call and no generation call.
const (
	OutcomeExact    = "exact"
	OutcomeSemantic = "semantic"
	OutcomeMiss     = "miss"
)

// Scope is organization → brand → department. A nil brand or department means
// the caller has none, so only organization-wide rows qualify at that level.
type Scope struct {
	BrandID      *uuid.UUID
	DepartmentID *int64
}

// Key identifies the scope in cache and gap rows ("<brand>:<department>").
func (s Scope) Key() string {
	key := "-"
	if s.BrandID != nil {
		key = s.BrandID.String()
	}
	if s.DepartmentID != nil {
		return key + ":" + strconv.FormatInt(*s.DepartmentID, 10)
	}
	return key + ":-"
}

type Query struct {
	Organization int64
	Audience     string
	Locale       string
	Scope        Scope
	Question     string
	// ExcludeConversationID keeps the asking conversation out of its own
	// case-memory results.
	ExcludeConversationID int64
}

func (q Query) Validate() error {
	if q.Organization < 1 || (q.Audience != AudienceCustomer && q.Audience != AudienceAgent) || (q.Locale != "tr" && q.Locale != "en") {
		return ErrInvalid
	}
	return nil
}

// UnitHit is an approved answer from a knowledge unit.
type UnitHit struct {
	UUID        uuid.UUID  `json:"uuid"`
	Question    string     `json:"question"`
	Answer      string     `json:"answer"`
	Category    string     `json:"category"`
	Visibility  string     `json:"visibility"`
	ArticleUUID *uuid.UUID `json:"article_uuid,omitempty"`
	Score       float64    `json:"score"`
}

// CustomerCase is what the customer AI may read from case memory: anonymized
// problem and resolution text only. It carries no conversation, contact or
// company reference by construction.
type CustomerCase struct {
	Problem    string  `json:"problem"`
	Resolution string  `json:"resolution"`
	Category   string  `json:"category"`
	Score      float64 `json:"score"`
}

// AgentCase adds the review state and, when the viewer may open it, the
// source conversation.
type AgentCase struct {
	UUID             uuid.UUID  `json:"uuid"`
	Problem          string     `json:"problem"`
	Resolution       string     `json:"resolution"`
	Category         string     `json:"category"`
	Status           string     `json:"status"`
	Score            float64    `json:"score"`
	ConversationUUID *uuid.UUID `json:"conversation_uuid,omitempty"`
	// SourceConversationID decides whether the viewer may see the link; it
	// is never serialized.
	SourceConversationID int64 `json:"-"`
}

type Result struct {
	Outcome        string         `json:"outcome"`
	Unit           *UnitHit       `json:"unit,omitempty"`
	CustomerCases  []CustomerCase `json:"-"`
	AgentCases     []AgentCase    `json:"cases,omitempty"`
	EmbeddingCalls int            `json:"embedding_calls"`
}

// Hit reports whether a unit answered the question.
func (r Result) Hit() bool { return r.Unit != nil }

type AudienceStats struct {
	Audience     string  `json:"audience"`
	ExactHits    int64   `json:"exact_hits"`
	SemanticHits int64   `json:"semantic_hits"`
	Misses       int64   `json:"misses"`
	Lookups      int64   `json:"lookups"`
	HitRate      float64 `json:"hit_rate"`
}

type Stats struct {
	Since     time.Time       `json:"since"`
	Audiences []AudienceStats `json:"audiences"`
	Units     int64           `json:"units"`
	Cached    int64           `json:"cached_questions"`
}
