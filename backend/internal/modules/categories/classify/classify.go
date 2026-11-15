// Package classify is the single classification contract of the knowledge
// layer (CF-218). Every record kind (conversation, archive record, knowledge
// unit, suggestion) is categorized through a Classifier; callers never talk
// to embeddings or the AI provider for categorization themselves.
//
// Phase 1 is a Chain of stages: embedding similarity first, the AI when the
// embedding is unsure, and a human (needs_review) when confidence stays low.
// The organization's own model (CF-219) is one more Stage put in front of the
// chain with Chain.Prepend; it abstains (Decided=false) when unsure and the
// chain falls through to the phase-1 stages.
package classify

import (
	"context"
	"errors"
	"strings"
)

// Sources of a category decision, stored with every record.
const (
	SourceEmbedding = "embedding"
	SourceAI        = "ai"
	SourceHuman     = "human"
	// SourceModel is the organization's own model (CF-219).
	SourceModel = "model"
)

// Record kinds that carry a category.
const (
	SubjectConversation  = "conversation"
	SubjectArchiveRecord = "archive_record"
	SubjectKnowledgeUnit = "knowledge_unit"
	SubjectSuggestion    = "suggestion"
)

// SubjectTypes lists every record kind.
var SubjectTypes = []string{SubjectConversation, SubjectArchiveRecord, SubjectKnowledgeUnit, SubjectSuggestion}

// ValidSubjectType reports whether t is a known record kind.
func ValidSubjectType(t string) bool {
	for _, s := range SubjectTypes {
		if s == t {
			return true
		}
	}
	return false
}

// ErrNoCandidates means the organization (or department) has no active
// category to choose from.
var ErrNoCandidates = errors.New("classify: no categories")

// Input is one record to categorize. Text is the record's content (it is
// masked before it reaches a provider). DepartmentID 0 is organization-wide.
type Input struct {
	OrganizationID int64
	DepartmentID   int64
	SubjectType    string
	Text           string
	Locale         string
	// ActorID is the user the AI run is recorded for (0 = system).
	ActorID int64
}

// Candidate is one category the record may take (organization-wide nodes
// plus the record's department branch).
type Candidate struct {
	ID           int64
	ParentID     int64
	DepartmentID int64
	Name         string
	Slug         string
	Description  string
	// Path is the slash-joined names from the root ("Fatura / İade").
	Path string
}

// Prediction is one stage's answer.
type Prediction struct {
	CategoryID int64
	Confidence float64
	Source     string
	// Classifier names the stage and its version ("embedding/v1").
	Classifier string
	// Decided: the stage is confident enough; later stages are skipped.
	Decided bool
}

// Result is the chain's decision for a record.
type Result struct {
	Prediction
	// NeedsReview: no stage was confident; an agent is asked to confirm or
	// correct (the best guess, if any, is kept as a suggestion).
	NeedsReview bool
}

// Stage is one classifier step. It returns Decided=false to abstain or to
// pass a low-confidence guess to the next stage.
type Stage interface {
	Name() string
	Predict(ctx context.Context, in Input, candidates []Candidate) (Prediction, error)
}

// Classifier is what callers use (CF-217's archive pipeline, the inbox
// consumer, the HTTP "classify" route).
type Classifier interface {
	Classify(ctx context.Context, in Input, candidates []Candidate) (Result, error)
}

// Thresholds are the confidence bars of the chain.
type Thresholds struct {
	// Review: a final confidence below this asks an agent.
	Review float64
}

// DefaultThresholds are the phase-1 bars.
var DefaultThresholds = Thresholds{Review: 0.6}

// Chain runs stages in order and stops at the first decided prediction.
// A stage error (provider down, AI disabled, no credits) is not fatal: the
// chain moves on, and with no decision the record goes to review.
type Chain struct {
	Stages     []Stage
	Thresholds Thresholds
	// OnStageError observes stage failures (logging); optional.
	OnStageError func(stage string, err error)
}

// Prepend puts a stage (the CF-219 model) in front of the chain.
func (c *Chain) Prepend(s Stage) { c.Stages = append([]Stage{s}, c.Stages...) }

// Classify implements Classifier.
func (c *Chain) Classify(ctx context.Context, in Input, candidates []Candidate) (Result, error) {
	if len(candidates) == 0 {
		return Result{}, ErrNoCandidates
	}
	if strings.TrimSpace(in.Text) == "" {
		return Result{NeedsReview: true}, nil
	}
	valid := make(map[int64]bool, len(candidates))
	for _, cand := range candidates {
		valid[cand.ID] = true
	}
	th := c.Thresholds
	if th.Review <= 0 {
		th = DefaultThresholds
	}
	var best Prediction
	for _, st := range c.Stages {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		p, err := st.Predict(ctx, in, candidates)
		if err != nil {
			if c.OnStageError != nil {
				c.OnStageError(st.Name(), err)
			}
			continue
		}
		if p.CategoryID == 0 || !valid[p.CategoryID] {
			continue
		}
		p.Confidence = clamp(p.Confidence)
		if p.Decided {
			return Result{Prediction: p, NeedsReview: p.Confidence < th.Review}, nil
		}
		if p.Confidence > best.Confidence {
			best = p
		}
	}
	best.Decided = false
	return Result{Prediction: best, NeedsReview: true}, nil
}

func clamp(v float64) float64 {
	switch {
	case v != v || v < 0: // NaN or negative
		return 0
	case v > 1:
		return 1
	}
	return v
}

// Paths fills Candidate.Path from the parent links.
func Paths(candidates []Candidate) []Candidate {
	byID := make(map[int64]Candidate, len(candidates))
	for _, c := range candidates {
		byID[c.ID] = c
	}
	out := make([]Candidate, len(candidates))
	for i, c := range candidates {
		parts := []string{c.Name}
		seen := map[int64]bool{c.ID: true}
		for p := c.ParentID; p != 0 && !seen[p]; {
			parent, ok := byID[p]
			if !ok {
				break
			}
			seen[p] = true
			parts = append([]string{parent.Name}, parts...)
			p = parent.ParentID
		}
		c.Path = strings.Join(parts, " / ")
		out[i] = c
	}
	return out
}
