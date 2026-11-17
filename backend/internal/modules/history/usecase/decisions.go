package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// DecisionResult is the reviewed suggestion and the remembered decision.
type DecisionResult struct {
	Suggestion db.KnowledgeSuggestion `json:"suggestion"`
	Decision   db.HistoryDecision     `json:"decision"`
}

// Decide records a department lead's review of a pending suggestion. Approve
// and correct create an internal article draft (publishing stays a separate
// decision in the knowledge base); every decision is stored with its reason
// and an embedding in the decision memory.
func (s *Service) Decide(ctx context.Context, org, actor int64, id uuid.UUID, in model.DecisionInput) (DecisionResult, error) {
	var out DecisionResult
	if err := in.Validate(); err != nil {
		return out, err
	}
	current, err := s.Q.HistorySuggestionGet(ctx, db.HistorySuggestionGetParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return out, mapDB(err)
	}
	if current.Status != "pending" {
		return out, fmt.Errorf("%w: the suggestion was already reviewed", model.ErrConflict)
	}
	ok, err := s.Directory.CanReview(ctx, org, actor, current.DepartmentID)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, model.ErrForbidden
	}
	title, body := current.Title, current.Body
	if in.Decision == model.DecisionCorrect {
		title, body = strings.TrimSpace(in.Title), in.Body
	}
	// What is remembered is what was proposed: a later draft close to a
	// rejected proposal is suppressed even when worded differently.
	vec, err := s.embedOne(orgctx.WithOrganizationID(ctx, org), current.Title+"\n"+current.Body)
	if err != nil {
		return out, err
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		locked, err := q.HistorySuggestionLock(ctx, db.HistorySuggestionLockParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return mapDB(err)
		}
		if locked.Status != "pending" || locked.Title != current.Title || locked.Body != current.Body {
			return fmt.Errorf("%w: the suggestion changed; reload it", model.ErrConflict)
		}
		status, decision, article := "rejected", "rejected", pgtype.Int8{}
		if in.Decision != model.DecisionReject {
			status, decision = "approved", "approved"
			if in.Decision == model.DecisionCorrect {
				decision = "corrected"
			}
			a, err := q.KnowledgeInsertArticle(ctx, db.KnowledgeInsertArticleParams{OrganizationID: org, Uuid: uuid.New(), Slug: "history-" + id.String(), Locale: in.Locale,
				Title: title, Body: body, Tags: []string{"history"}, Visibility: "internal", AuthorID: actor})
			if err != nil {
				return mapDB(err)
			}
			if err := q.KnowledgeInsertVersion(ctx, db.KnowledgeInsertVersionParams{OrganizationID: org, ArticleID: a.ID, Version: a.Version, Title: a.Title, Body: a.Body, Visibility: a.Visibility, AuthorID: actor}); err != nil {
				return err
			}
			article = pgtype.Int8{Int64: a.ID, Valid: true}
		}
		out.Suggestion, err = q.HistorySuggestionReview(ctx, db.HistorySuggestionReviewParams{OrganizationID: org, ID: locked.ID, Status: status,
			ReviewedBy: pgtype.Int8{Int64: actor, Valid: true}, ArticleID: article, Title: title, Body: body})
		if err != nil {
			return mapDB(err)
		}
		correction := ""
		if decision == "corrected" {
			correction = title + "\n" + body
		}
		out.Decision, err = q.HistoryDecisionCreate(ctx, db.HistoryDecisionCreateParams{OrganizationID: org, SuggestionID: pgtype.Int8{Int64: locked.ID, Valid: true},
			ClusterID: locked.ClusterID, DepartmentID: locked.DepartmentID, Decision: decision, Reason: strings.TrimSpace(in.Reason),
			Title: current.Title, Body: current.Body, Correction: correction, Embedding: vec, EmbeddingModel: s.Embedder.Model(),
			DecidedBy: pgtype.Int8{Int64: actor, Valid: true}})
		return err
	})
	return out, err
}

// EngineDrafter writes drafts through the AI core, so budgets, policy,
// masking and the run log apply. Its input is already anonymized.
type EngineDrafter struct{ Engine *core.Service }

var draftSchema = map[string]any{
	"type": "object", "additionalProperties": false, "required": []string{"title", "body"},
	"properties": map[string]any{"title": map[string]any{"type": "string", "maxLength": 300}, "body": map[string]any{"type": "string", "maxLength": 30000}},
}

func (d EngineDrafter) Draft(ctx context.Context, org int64, locale, input string) (Draft, error) {
	if d.Engine == nil {
		return Draft{}, ai.ErrNotConfigured
	}
	run, err := d.Engine.RunAgent(ctx, org, 0, coremodel.RunInput{Task: "complex", Input: input, Locale: locale, MaxTokens: 4096},
		core.AgentOptions{Instructions: draftInstructions + " Exclude ALL private organization context.", Schema: draftSchema})
	if errors.Is(err, ai.ErrNotConfigured) || errors.Is(err, coremodel.ErrDisabled) {
		// No provider or AI is off for this organization: the cluster keeps
		// its evidence and the next run tries again.
		return Draft{}, nil
	}
	if err != nil {
		return Draft{}, err
	}
	var out Draft
	if err := json.Unmarshal([]byte(run.Text), &out); err != nil {
		return Draft{}, ai.ErrInvalidOutput
	}
	return out, nil
}
