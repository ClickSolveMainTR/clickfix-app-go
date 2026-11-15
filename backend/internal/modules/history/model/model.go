// Package model holds the history pipeline's (CF-217) file schema, inputs and errors.
package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound    = errors.New("history: not found")
	ErrInvalid     = errors.New("history: invalid")
	ErrConflict    = errors.New("history: conflict")
	ErrForbidden   = errors.New("history: not allowed")
	ErrUnavailable = errors.New("history: unavailable")
)

// SchemaVersion is the documented file schema (docs/gecmis-kayitlar.md).
const SchemaVersion = "clickfix.history/v1"

// ConsentVersion names the consent text the uploader accepted.
const ConsentVersion = "2026-11"

// Limits of one uploaded file.
const (
	MaxUploadBytes    = 50 << 20
	MaxUnpackedBytes  = 200 << 20
	MaxTickets        = 20000
	MaxMessages       = 200000
	MaxMessageRunes   = 20000
	MaxFieldRunes     = 500
	MaxSubjectRunes   = 500
	MaxCaseTextRunes  = 4000
	MaxDraftCases     = 8
	ConflictThreshold = 0.80
)

// Bundle is one export in the documented schema: contacts, tickets and the
// messages of those tickets. Every field except the ids, ticket subject or
// message body is optional.
type Bundle struct {
	Schema       string    `json:"schema"`
	SourceSystem string    `json:"source_system"`
	Locale       string    `json:"locale"`
	Agents       []Agent   `json:"agents"`
	Contacts     []Contact `json:"contacts"`
	Tickets      []Ticket  `json:"tickets"`
	Messages     []Message `json:"messages"`
}

type Agent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Contact struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Company   string `json:"company"`
	TaxNumber string `json:"tax_number"`
	Address   string `json:"address"`
}

type Ticket struct {
	ID         string     `json:"id"`
	ContactID  string     `json:"contact_id"`
	Subject    string     `json:"subject"`
	Status     string     `json:"status"`
	Category   string     `json:"category"`
	Resolution string     `json:"resolution"`
	Locale     string     `json:"locale"`
	CreatedAt  *time.Time `json:"created_at"`
	ClosedAt   *time.Time `json:"closed_at"`
}

type Message struct {
	TicketID  string     `json:"ticket_id"`
	Author    string     `json:"author"`
	AgentID   string     `json:"agent_id"`
	Body      string     `json:"body"`
	CreatedAt *time.Time `json:"created_at"`
}

// Solved reports whether a ticket status counts as resolved history.
func Solved(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "solved", "closed", "resolved", "cozuldu", "çözüldü", "kapandi", "kapandı":
		return true
	}
	return false
}

// UploadInput is the metadata sent with an uploaded file. Both consents are
// recorded on the run; analysis consent is required to process at all.
type UploadInput struct {
	ConsentAnalysis bool
	ConsentTraining bool
	BrandUUID       string
	DepartmentID    int64
}

// SettingsInput is the organization's pipeline configuration.
type SettingsInput struct {
	EvidenceThreshold      int32   `json:"evidence_threshold"`
	ClusterSimilarity      float32 `json:"cluster_similarity"`
	ResolutionSimilarity   float32 `json:"resolution_similarity"`
	RejectionSimilarity    float32 `json:"rejection_similarity"`
	RawRetentionHours      int32   `json:"raw_retention_hours"`
	ArchiveRetentionDays   int32   `json:"archive_retention_days"`
	NightlyEnabled         bool    `json:"nightly_enabled"`
	NightlyConsentAnalysis bool    `json:"nightly_consent_analysis"`
	NightlyConsentTraining bool    `json:"nightly_consent_training"`
}

func (in SettingsInput) Validate() error {
	sim := func(v float32) bool { return v >= 0.5 && v <= 0.99 }
	switch {
	case in.EvidenceThreshold < 2 || in.EvidenceThreshold > 100:
		return fmt.Errorf("%w: evidence_threshold must be 2-100", ErrInvalid)
	case !sim(in.ClusterSimilarity) || !sim(in.ResolutionSimilarity) || !sim(in.RejectionSimilarity):
		return fmt.Errorf("%w: similarities must be 0.5-0.99", ErrInvalid)
	case in.RawRetentionHours < 1 || in.RawRetentionHours > 720:
		return fmt.Errorf("%w: raw_retention_hours must be 1-720", ErrInvalid)
	case in.ArchiveRetentionDays < 0 || in.ArchiveRetentionDays > 3650:
		return fmt.Errorf("%w: archive_retention_days must be 0-3650", ErrInvalid)
	case in.NightlyEnabled && !in.NightlyConsentAnalysis:
		return fmt.Errorf("%w: the nightly run needs analysis consent", ErrInvalid)
	}
	return nil
}

// DefaultSettings mirrors the column defaults of history_settings.
func DefaultSettings() SettingsInput {
	return SettingsInput{EvidenceThreshold: 3, ClusterSimilarity: 0.80, ResolutionSimilarity: 0.75, RejectionSimilarity: 0.85, RawRetentionHours: 24}
}

// Decision values of a review.
const (
	DecisionApprove = "approve"
	DecisionReject  = "reject"
	DecisionCorrect = "correct"
)

// DecisionInput is a department lead's review of a suggestion. A reason is
// required for every decision; a correction carries the corrected text.
type DecisionInput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Locale   string `json:"locale"`
}

func (in DecisionInput) Validate() error {
	reason := strings.TrimSpace(in.Reason)
	if len([]rune(reason)) < 3 || len([]rune(reason)) > 2000 {
		return fmt.Errorf("%w: reason must be 3-2000 characters", ErrInvalid)
	}
	if in.Locale != "tr" && in.Locale != "en" {
		return fmt.Errorf("%w: locale must be tr or en", ErrInvalid)
	}
	switch in.Decision {
	case DecisionApprove, DecisionReject:
	case DecisionCorrect:
		if t := strings.TrimSpace(in.Title); t == "" || len([]rune(t)) > 300 || strings.TrimSpace(in.Body) == "" || len([]rune(in.Body)) > 30000 {
			return fmt.Errorf("%w: a correction needs a title (1-300) and a body (1-30000)", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: decision must be approve, reject or correct", ErrInvalid)
	}
	return nil
}
