package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
)

// FieldSource lists custom field definitions (customfields usecase).
type FieldSource interface {
	Definitions(ctx context.Context, orgID int64, entity string, objectTypeID *int64) ([]customfields.Definition, error)
}

// Audience is who the widget offers an article to (CF-196): every
// condition must hold for the visitor's contact/company list fields, by
// option id. No conditions: everyone.
type Audience struct {
	Conditions []customfields.FieldMatch `json:"conditions"`
}

// ArticleAudience returns an article's audience.
func (s *Service) ArticleAudience(ctx context.Context, org int64, id uuid.UUID) (Audience, error) {
	row, err := s.Repo.Q.KnowledgeArticleAudience(ctx, db.KnowledgeArticleAudienceParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return Audience{}, repository.Normalize(err)
	}
	out := Audience{Conditions: []customfields.FieldMatch{}}
	_ = json.Unmarshal(row.Conditions, &out.Conditions)
	return out, nil
}

// SetArticleAudience checks the conditions against the catalog (labels
// resolve to option ids) and stores them. It applies at once.
func (s *Service) SetArticleAudience(ctx context.Context, org int64, id uuid.UUID, in Audience) (Audience, error) {
	row, err := s.Repo.Q.KnowledgeArticleAudience(ctx, db.KnowledgeArticleAudienceParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return Audience{}, repository.Normalize(err)
	}
	out := Audience{Conditions: []customfields.FieldMatch{}}
	if len(in.Conditions) > 0 {
		if s.Fields == nil {
			return Audience{}, model.ErrInvalid
		}
		defs := map[string][]customfields.Definition{}
		for _, e := range customfields.MatchEntities {
			if defs[e], err = s.Fields.Definitions(ctx, org, e, nil); err != nil {
				return Audience{}, err
			}
		}
		matches, errs := customfields.NormalizeMatches(in.Conditions, func(e string) []customfields.Definition { return defs[e] })
		if len(errs) > 0 {
			return Audience{}, fmt.Errorf("%w: %s: %s", model.ErrInvalid, errs[0].Field, errs[0].Message)
		}
		out.Conditions = matches
	}
	raw, err := json.Marshal(out.Conditions)
	if err != nil {
		return Audience{}, err
	}
	if err := s.Repo.Q.SaveKnowledgeArticleAudience(ctx, db.SaveKnowledgeArticleAudienceParams{OrganizationID: org, ArticleID: row.ID, Conditions: raw}); err != nil {
		return Audience{}, err
	}
	return out, nil
}
