package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// CF-220: AI knowledge panel. Every section says whether its data exists
// for the scope; a missing source is shown as "no data", never as zero.

// Reasons a section has no data.
const (
	// ReasonSourceNotConnected: the layer that produces the data has not
	// landed yet (answer cache CF-216, categorization CF-218).
	ReasonSourceNotConnected = "source_not_connected"
	// ReasonDepartmentScopeUnavailable: the source only knows the
	// organization as a whole; department scope comes with CF-215.
	ReasonDepartmentScopeUnavailable = "department_scope_unavailable"
)

// ErrDepartmentScope is returned by an organization-only source for a
// department scope; the service turns it into ReasonDepartmentScopeUnavailable.
var ErrDepartmentScope = errors.New("analytics: source has no department scope")

// ErrDepartmentsUnavailable: a department was requested before departments exist.
var ErrDepartmentsUnavailable = errors.New("analytics: departments are not available yet")

// Department is a department of the organization (CF-215).
type Department struct {
	ID   int64     `json:"-"`
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// KnowledgeScope is the organization, an optional department and the
// [From, Until) window.
type KnowledgeScope struct {
	Organization int64
	Department   *Department
	From, Until  time.Time
}

// Availability says whether a section has data and, if not, why.
type Availability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// CacheStats is the answer cache's share of questions.
type CacheStats struct {
	Availability
	Lookups int64    `json:"lookups"`
	Hits    int64    `json:"hits"`
	HitRate *float64 `json:"hit_rate"`
}

// TaskUsage is one AI task's calls and cost.
type TaskUsage struct {
	Task         string  `json:"task"`
	Calls        int64   `json:"calls"`
	Failed       int64   `json:"failed"`
	CostMicros   int64   `json:"cost_micros"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// UsageStats are AI calls and their cost.
type UsageStats struct {
	Availability
	Calls      int64       `json:"calls"`
	Failed     int64       `json:"failed"`
	CostMicros int64       `json:"cost_micros"`
	ByTask     []TaskUsage `json:"by_task"`
}

// GapTopic is a topic the AI could not answer (handed off).
type GapTopic struct {
	Topic         string `json:"topic"`
	Conversations int64  `json:"conversations"`
}

// GapStats are the knowledge gaps.
type GapStats struct {
	Availability
	Topics []GapTopic `json:"topics"`
}

// SuggestionStats is the knowledge suggestion pipeline.
type SuggestionStats struct {
	Availability
	Pending  int64 `json:"pending"`
	Approved int64 `json:"approved"`
	Rejected int64 `json:"rejected"`
}

// Classification is one conversation's category and confidence (nil
// when the classifier gave none).
type Classification struct {
	Category   string
	Confidence *float64
}

// CategoryShare is one category's conversations and mean confidence.
type CategoryShare struct {
	Category      string   `json:"category"`
	Conversations int64    `json:"conversations"`
	AvgConfidence *float64 `json:"avg_confidence"`
}

// ConfidenceBucket counts classifications in [From, To).
type ConfidenceBucket struct {
	From  float64 `json:"from"`
	To    float64 `json:"to"`
	Count int64   `json:"count"`
}

// CategoryStats is the category distribution and classification confidence.
type CategoryStats struct {
	Availability
	Classified    int64              `json:"classified"`
	AvgConfidence *float64           `json:"avg_confidence"`
	LowConfidence int64              `json:"low_confidence"`
	Categories    []CategoryShare    `json:"categories"`
	Buckets       []ConfidenceBucket `json:"confidence_buckets"`
}

// HandoffStats is the share of AI conversations handed to a person.
type HandoffStats struct {
	Availability
	Conversations int64    `json:"conversations"`
	Handoffs      int64    `json:"handoffs"`
	Rate          *float64 `json:"rate"`
}

// EvaluationPoint is one completed evaluation run's metrics.
type EvaluationPoint struct {
	UUID              uuid.UUID `json:"uuid"`
	Trigger           string    `json:"trigger"`
	Model             string    `json:"model"`
	CreatedAt         time.Time `json:"created_at"`
	AnswerAccuracy    *float64  `json:"answer_accuracy"`
	SourceHitRate     *float64  `json:"source_hit_rate"`
	CacheHitRate      *float64  `json:"cache_hit_rate"`
	AvgLatencyMs      *float64  `json:"avg_latency_ms"`
	CostPerCaseMicros *float64  `json:"cost_per_case_micros"`
}

// EvaluationStats is the latest run and the one before it.
type EvaluationStats struct {
	Availability
	Latest   *EvaluationPoint `json:"latest"`
	Previous *EvaluationPoint `json:"previous"`
}

// KnowledgePanel is the AI knowledge panel of a scope.
type KnowledgePanel struct {
	From          time.Time       `json:"from"`
	Until         time.Time       `json:"until"`
	Department    *Department     `json:"department"`
	Departments   []Department    `json:"departments"`
	Cache         CacheStats      `json:"cache"`
	Usage         UsageStats      `json:"ai_usage"`
	KnowledgeGaps GapStats        `json:"knowledge_gaps"`
	Suggestions   SuggestionStats `json:"suggestions"`
	Categories    CategoryStats   `json:"categories"`
	Handoff       HandoffStats    `json:"handoff"`
	Evaluation    EvaluationStats `json:"evaluation"`
}
