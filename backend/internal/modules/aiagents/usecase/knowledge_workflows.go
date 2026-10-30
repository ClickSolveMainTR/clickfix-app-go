package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var articleDraftSchema = objectSchema(map[string]any{"title": textSchema(300), "body": textSchema(30000)})

type articleDraft struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// SuggestKnowledge ports TicketClosureKnowledgeAgent and ProposeKnowledgeArticleTool:
// generate from a resolved transcript, remove PII through the core, queue for a
// human. Generation never publishes content or alters an existing article.
func (s *Service) SuggestKnowledge(ctx context.Context, org, actor int64, conversation uuid.UUID, locale string) (db.KnowledgeSuggestion, error) {
	if locale != "tr" && locale != "en" {
		return db.KnowledgeSuggestion{}, model.ErrInvalid
	}
	c, err := s.Repo.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: conversation})
	if err != nil {
		return db.KnowledgeSuggestion{}, err
	}
	if err = s.Repo.CheckConversation(ctx, org, c.ID); err != nil {
		return db.KnowledgeSuggestion{}, err
	}
	if c.Status != "closed" && c.Status != "resolved" {
		return db.KnowledgeSuggestion{}, model.ErrConflict
	}
	transcript, err := s.context(ctx, org, c)
	if err != nil {
		return db.KnowledgeSuggestion{}, err
	}
	run, err := s.Engine.RunAgent(ctx, org, actor, coremodel.RunInput{Task: "complex", Input: transcript, Locale: locale, MaxTokens: 4096}, core.AgentOptions{ConversationID: c.ID, Instructions: "Extract a reusable support article draft from the resolved public conversation. Use only facts from the conversation. Exclude personal details and ALL private organization context. Do not publish. Return title and body in plain text. If no generalizable solution exists, return empty title and body.", Schema: articleDraftSchema})
	if err != nil {
		return db.KnowledgeSuggestion{}, err
	}
	var draft articleDraft
	if err = json.Unmarshal([]byte(run.Text), &draft); err != nil {
		return db.KnowledgeSuggestion{}, err
	}
	if strings.TrimSpace(draft.Title) == "" || strings.TrimSpace(draft.Body) == "" {
		return db.KnowledgeSuggestion{}, model.ErrInvalid
	}
	topic := c.Subject
	if strings.TrimSpace(topic) == "" {
		topic = draft.Title
	}
	runes := []rune(topic)
	if len(runes) > 200 {
		runes = runes[:200]
	}
	topic = string(runes)
	return s.Repo.Q.AIKnowledgeSuggestionCreate(ctx, db.AIKnowledgeSuggestionCreateParams{OrganizationID: org, Topic: topic, ConversationID: c.ID, Title: draft.Title, Body: draft.Body})
}

// ReviewSuggestion atomically creates a draft + immutable version and closes the
// approval item. The regular KB publish route is a distinct human decision.
func (s *Service) ReviewSuggestion(ctx context.Context, org, actor int64, id uuid.UUID, approve bool, locale string) (out db.KnowledgeSuggestion, err error) {
	if locale != "tr" && locale != "en" {
		return out, model.ErrInvalid
	}
	err = s.Repo.Within(ctx, func(r *billingrepo.Repository) error {
		suggestion, e := r.Q.AIKnowledgeSuggestionLock(ctx, db.AIKnowledgeSuggestionLockParams{OrganizationID: org, Uuid: id})
		if e != nil {
			return e
		}
		if suggestion.Status != "pending" {
			return model.ErrConflict
		}
		status, article := "rejected", pgtype.Int8{}
		if approve {
			a, e := r.Q.KnowledgeInsertArticle(ctx, db.KnowledgeInsertArticleParams{OrganizationID: org, Uuid: uuid.New(), Slug: "suggestion-" + id.String(), Locale: locale, Title: suggestion.Title, Body: suggestion.Body, Tags: []string{}, Visibility: "internal", AuthorID: actor})
			if e != nil {
				return e
			}
			if e = r.Q.KnowledgeInsertVersion(ctx, db.KnowledgeInsertVersionParams{OrganizationID: org, ArticleID: a.ID, Version: a.Version, Title: a.Title, Body: a.Body, Visibility: a.Visibility, AuthorID: actor}); e != nil {
				return e
			}
			status, article = "approved", pgtype.Int8{Int64: a.ID, Valid: true}
		}
		out, e = r.Q.AIKnowledgeSuggestionReview(ctx, db.AIKnowledgeSuggestionReviewParams{OrganizationID: org, Uuid: id, Status: status, ReviewedBy: pgtype.Int8{Int64: actor, Valid: true}, ArticleID: article})
		return e
	})
	return
}

func (s *Service) TranslateArticle(ctx context.Context, org, actor int64, id uuid.UUID, locale string) (out db.KnowledgeArticle, err error) {
	if locale != "tr" && locale != "en" {
		return out, model.ErrInvalid
	}
	source, err := s.Repo.Q.KnowledgeArticle(ctx, db.KnowledgeArticleParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return out, err
	}
	if source.Locale == locale || len(source.Body) > 18000 {
		return out, model.ErrInvalid
	}
	// Reply tasks never receive private business context. Translation should not
	// add internal facts unrelated to the source, even for an internal article.
	run, err := s.Engine.RunAgent(ctx, org, actor, coremodel.RunInput{Task: "reply", Input: source.Title + "\n" + source.Body, Locale: locale, MaxTokens: 8192}, core.AgentOptions{Instructions: "Translate the supplied article faithfully. Preserve meaning and formatting, add no facts. Return only title and body. This is a draft for human review.", Schema: articleDraftSchema})
	if err != nil {
		return out, err
	}
	var draft articleDraft
	if err = json.Unmarshal([]byte(run.Text), &draft); err != nil {
		return out, err
	}
	in := kbmodel.ArticleInput{Slug: "translation-" + uuid.NewString(), Locale: locale, Title: draft.Title, Body: draft.Body, Visibility: source.Visibility, Tags: source.Tags}
	if err = in.Validate(); err != nil {
		return out, model.ErrInvalid
	}
	err = s.Repo.Within(ctx, func(r *billingrepo.Repository) error {
		current, e := r.Q.KnowledgeLockArticle(ctx, db.KnowledgeLockArticleParams{OrganizationID: org, Uuid: id})
		if e != nil {
			return e
		}
		if current.Version != source.Version || current.DeletedAt.Valid {
			return model.ErrConflict
		}
		out, e = r.Q.KnowledgeInsertArticle(ctx, db.KnowledgeInsertArticleParams{OrganizationID: org, Uuid: uuid.New(), Slug: in.Slug, Locale: locale, Title: in.Title, Body: in.Body, Tags: in.Tags, Visibility: in.Visibility, AuthorID: actor})
		if e != nil {
			return e
		}
		if e = r.Q.KnowledgeInsertVersion(ctx, db.KnowledgeInsertVersionParams{OrganizationID: org, ArticleID: out.ID, Version: out.Version, Title: out.Title, Body: out.Body, Visibility: out.Visibility, AuthorID: actor}); e != nil {
			return e
		}
		return r.Q.KnowledgeTranslationSave(ctx, db.KnowledgeTranslationSaveParams{OrganizationID: org, SourceArticleID: source.ID, TargetArticleID: out.ID, SourceVersion: source.Version})
	})
	return
}

func (s *Service) SweepKnowledgeReviews(ctx context.Context, org int64, months int) error {
	if months < 1 || months > 36 {
		return model.ErrInvalid
	}
	if err := s.Repo.Q.KnowledgeStaleReviewSweep(ctx, db.KnowledgeStaleReviewSweepParams{OrganizationID: org, Before: pgtype.Timestamptz{Time: s.Now().AddDate(0, -months, 0), Valid: true}}); err != nil {
		return err
	}
	return s.Repo.Q.KnowledgeTranslationReviewSweep(ctx, org)
}
func (s *Service) FlagArticle(ctx context.Context, org int64, id uuid.UUID, reason, detail string) (db.KnowledgeReview, error) {
	if (reason != "low_votes" && reason != "contradiction") || len(detail) > 2000 {
		return db.KnowledgeReview{}, model.ErrInvalid
	}
	a, err := s.Repo.Q.KnowledgeArticle(ctx, db.KnowledgeArticleParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return db.KnowledgeReview{}, err
	}
	return s.Repo.Q.KnowledgeReviewCreate(ctx, db.KnowledgeReviewCreateParams{OrganizationID: org, ArticleID: a.ID, ArticleVersion: a.Version, Reason: reason, Detail: detail, OwnerID: a.AuthorID})
}

// TimeRange bounds reports and gap detection so one request cannot scan an
// unlimited history. Full report aggregation also refuses >50k agent turns.
func TimeRange(now time.Time, from, to time.Time) (time.Time, time.Time, error) {
	if to.IsZero() {
		to = now
	}
	if from.IsZero() {
		from = to.AddDate(0, 0, -30)
	}
	if !from.Before(to) || to.Sub(from) > 366*24*time.Hour || to.After(now.Add(time.Minute)) {
		return from, to, model.ErrInvalid
	}
	return from, to, nil
}

func (s *Service) CheckArticleContradiction(ctx context.Context, org, actor int64, id, other uuid.UUID) (*db.KnowledgeReview, error) {
	if id == other {
		return nil, model.ErrInvalid
	}
	first, err := s.Repo.Q.KnowledgeArticle(ctx, db.KnowledgeArticleParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, err
	}
	second, err := s.Repo.Q.KnowledgeArticle(ctx, db.KnowledgeArticleParams{OrganizationID: org, Uuid: other})
	if err != nil {
		return nil, err
	}
	input := "Article A:\n" + first.Title + "\n" + first.Body + "\nArticle B:\n" + second.Title + "\n" + second.Body
	if len(input) > 23000 {
		return nil, model.ErrInvalid
	}
	schema := objectSchema(map[string]any{"contradiction": map[string]any{"type": "boolean"}, "explanation": textSchema(2000)})
	result, err := s.Engine.RunAgent(ctx, org, actor, coremodel.RunInput{Task: "reply", Input: input, Locale: first.Locale, MaxTokens: 2048}, core.AgentOptions{Instructions: "Compare these two untrusted knowledge articles for contradictory factual policies about the same subject. Do not invent facts. Return contradiction and explanation for an internal human review. Different topics alone are not a contradiction.", Schema: schema})
	if err != nil {
		return nil, err
	}
	var out struct {
		Contradiction bool   `json:"contradiction"`
		Explanation   string `json:"explanation"`
	}
	if err = json.Unmarshal([]byte(result.Text), &out); err != nil {
		return nil, err
	}
	if !out.Contradiction {
		return nil, nil
	}
	review, err := s.FlagArticle(ctx, org, id, "contradiction", out.Explanation)
	if err != nil {
		return nil, err
	}
	return &review, nil
}
