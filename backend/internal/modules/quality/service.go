// Package quality implements rubric-based manual and guarded AI quality reviews.
package quality

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	aiusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"math"
	"strings"
)

type Service struct {
	Repo *repository.Repository
	AI   supportops.AI
}
type Criterion struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Weight   int    `json:"weight"`
	Critical bool   `json:"critical"`
}
type Score struct {
	Key       string `json:"key"`
	Value     int    `json:"value"`
	Rationale string `json:"rationale"`
}
type CardInput struct {
	Name      string      `json:"name"`
	Criteria  []Criterion `json:"criteria"`
	Threshold int32       `json:"threshold"`
	Auto      bool        `json:"auto_score"`
}

func ValidateCard(in CardInput) error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || len(in.Criteria) < 1 || len(in.Criteria) > 20 || in.Threshold < 0 || in.Threshold > 100 {
		return supportops.ErrInvalid
	}
	seen := map[string]bool{}
	total := 0
	for _, c := range in.Criteria {
		if c.Key == "" || len(c.Key) > 60 || seen[c.Key] || strings.TrimSpace(c.Label) == "" || len(c.Label) > 500 || c.Weight < 1 || c.Weight > 100 {
			return supportops.ErrInvalid
		}
		seen[c.Key] = true
		total += c.Weight
	}
	if total != 100 {
		return supportops.ErrInvalid
	}
	return nil
}

// Calculate rejects incomplete, duplicate or out-of-range model output; critical zero overrides the total.
func Calculate(criteria []Criterion, scores []Score) (float64, bool, error) {
	if len(criteria) != len(scores) {
		return 0, false, supportops.ErrInvalid
	}
	values := map[string]Score{}
	for _, s := range scores {
		if _, ok := values[s.Key]; ok || s.Value < 0 || s.Value > 100 || strings.TrimSpace(s.Rationale) == "" || len(s.Rationale) > 2000 {
			return 0, false, supportops.ErrInvalid
		}
		values[s.Key] = s
	}
	total := 0.0
	critical := false
	for _, c := range criteria {
		s, ok := values[c.Key]
		if !ok {
			return 0, false, supportops.ErrInvalid
		}
		total += float64(c.Weight*s.Value) / 100
		if c.Critical && s.Value == 0 {
			critical = true
		}
	}
	if critical {
		total = 0
	}
	return math.Round(total*100) / 100, critical, nil
}
func (s *Service) Card(ctx context.Context, org int64, in CardInput) (db.QaScorecard, error) {
	if err := ValidateCard(in); err != nil {
		return db.QaScorecard{}, err
	}
	b, _ := json.Marshal(in.Criteria)
	return s.Repo.Q.QACreateScorecard(ctx, db.QACreateScorecardParams{OrganizationID: org, Name: in.Name, Criteria: b, Threshold: in.Threshold, AutoScore: in.Auto})
}

type ReviewInput struct {
	Scorecard    uuid.UUID `json:"scorecard_uuid"`
	Conversation uuid.UUID `json:"conversation_uuid"`
	Scores       []Score   `json:"scores"`
}

func (s *Service) Review(ctx context.Context, org, actor int64, in ReviewInput, automatic bool) (db.QaReview, error) {
	if !automatic {
		return s.review(ctx, org, actor, in, false)
	}
	var result db.QaReview
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		if err := r.Q.QALockAutomatic(ctx, db.QALockAutomaticParams{OrganizationID: org, Scorecard: in.Scorecard, Conversation: in.Conversation}); err != nil {
			return err
		}
		var err error
		result, err = s.review(ctx, org, actor, in, true)
		return err
	})
	return result, err
}
func (s *Service) review(ctx context.Context, org, actor int64, in ReviewInput, automatic bool) (db.QaReview, error) {
	var out db.QaReview
	card, err := s.Repo.Q.QAGetScorecard(ctx, db.QAGetScorecardParams{OrganizationID: org, Uuid: in.Scorecard})
	if err != nil {
		return out, err
	}
	c, text, err := supportops.Transcript(ctx, s.Repo.Q, org, actor, in.Conversation)
	if err != nil {
		return out, err
	}
	if c.Status != "resolved" {
		return out, supportops.ErrConflict
	}
	var criteria []Criterion
	if err = json.Unmarshal(card.Criteria, &criteria); err != nil {
		return out, err
	}
	source := "manual"
	var run pgtype.UUID
	if automatic {
		if s.AI == nil {
			return out, supportops.ErrConflict
		}
		if _, err = s.Repo.Q.QAExistingAuto(ctx, db.QAExistingAutoParams{OrganizationID: org, ScorecardID: card.ID, ConversationID: c.ID}); err == nil {
			return out, supportops.ErrConflict
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		result, e := s.AI.RunAgent(ctx, org, actor, aimodel.RunInput{Task: "complex", Input: text, MaxTokens: 4096}, aiusecase.AgentOptions{Instructions: "Score this resolved conversation, whether handled by a human or AI agent, against this rubric: " + string(card.Criteria) + ". Return JSON {scores:[{key:string,value:integer 0..100,rationale:string}]}. Include each criterion exactly once with evidence. A critical failure receives zero."})
		if e != nil {
			return out, e
		}
		var resultScores struct {
			Scores []Score `json:"scores"`
		}
		if json.Unmarshal([]byte(result.Text), &resultScores) != nil {
			return out, supportops.ErrInvalid
		}
		in.Scores = resultScores.Scores
		source = "ai"
		run = pgtype.UUID{Bytes: result.UUID, Valid: true}
	}
	score, critical, err := Calculate(criteria, in.Scores)
	if err != nil {
		return out, err
	}
	var n pgtype.Numeric
	if err = n.Scan(fmt.Sprintf("%.2f", score)); err != nil {
		return out, err
	}
	b, _ := json.Marshal(in.Scores)
	err = s.Repo.Within(ctx, func(r *repository.Repository) error {
		current, err := r.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: org, ID: c.ID})
		if err != nil {
			return err
		}
		if current.Status != "resolved" {
			return supportops.ErrConflict
		}
		out, err = r.Q.QAInsertReview(ctx, db.QAInsertReviewParams{OrganizationID: org, ScorecardID: card.ID, ConversationID: c.ID, AgentID: c.AssigneeUserID, ReviewerID: pgtype.Int8{Int64: actor, Valid: actor > 0 && !automatic}, Source: source, Rubric: card.Criteria, Scores: b, Score: n, CriticalFailure: critical, NeedsReview: critical || score < float64(card.Threshold), AiRunUuid: run})
		if err != nil {
			return err
		}
		return r.Emit(ctx, events.New("quality.reviewed").WithOrganization(org).WithEntity("qa_review", &out.ID, &out.Uuid))
	})
	return out, err
}
func (s *Service) AutoResolved(ctx context.Context, ev events.Event) error {
	if ev.TenantID == nil || ev.EntityUUID == nil {
		return nil
	}
	org := *ev.TenantID
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: org, Permissions: []string{rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReadAll}})
	cards, err := s.Repo.Q.QAAutoCards(ctx, org)
	if err != nil {
		return err
	}
	for _, c := range cards {
		_, err = s.Review(ctx, org, 0, ReviewInput{Scorecard: c.Uuid, Conversation: *ev.EntityUUID}, true)
		if errors.Is(err, supportops.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdateCard affects future reviews; existing review snapshots remain unchanged.
func (s *Service) UpdateCard(ctx context.Context, org int64, id uuid.UUID, in CardInput) (db.QaScorecard, error) {
	if err := ValidateCard(in); err != nil {
		return db.QaScorecard{}, err
	}
	b, _ := json.Marshal(in.Criteria)
	return s.Repo.Q.QAUpdateScorecard(ctx, db.QAUpdateScorecardParams{OrganizationID: org, Uuid: id, Name: in.Name, Criteria: b, Threshold: in.Threshold, AutoScore: in.Auto})
}
