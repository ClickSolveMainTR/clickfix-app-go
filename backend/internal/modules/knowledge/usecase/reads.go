package usecase

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) Article(ctx context.Context, org int64, id uuid.UUID) (db.KnowledgeArticle, error) {
	a, err := s.Repo.Q.KnowledgeArticle(ctx, db.KnowledgeArticleParams{OrganizationID: org, Uuid: id})
	return a, repository.Normalize(err)
}

// ArticleFilter narrows the admin article list. Empty strings mean "any".
type ArticleFilter struct {
	Q          string
	Locale     string
	Visibility string
	// State is draft (never published), published, or changed (published
	// with a newer unpublished draft).
	State      string
	Collection *uuid.UUID
	// Department (CF-215): "" any, "none" organization-wide, "id" the one in
	// DepartmentID. Scope is the member's visibility.
	DepartmentMode string
	DepartmentID   pgtype.Int8
	Scope          deptscope.Scope
}

func (f ArticleFilter) Validate() error {
	if f.Locale != "" && f.Locale != "tr" && f.Locale != "en" {
		return model.ErrInvalid
	}
	if f.Visibility != "" && f.Visibility != "public" && f.Visibility != "internal" {
		return model.ErrInvalid
	}
	if f.DepartmentMode != "" && f.DepartmentMode != "none" && f.DepartmentMode != "id" {
		return model.ErrInvalid
	}
	switch f.State {
	case "", "draft", "published", "changed":
	default:
		return model.ErrInvalid
	}
	return nil
}

func (s *Service) Articles(ctx context.Context, org int64, f ArticleFilter, limit, offset int32) ([]db.KnowledgeArticlesRow, int64, error) {
	if err := f.Validate(); err != nil {
		return nil, 0, err
	}
	collection := pgtype.UUID{}
	if f.Collection != nil {
		collection = pgtype.UUID{Bytes: *f.Collection, Valid: true}
	}
	restrict, viewer := f.Scope.Args()
	rows, err := s.Repo.Q.KnowledgeArticles(ctx, db.KnowledgeArticlesParams{OrganizationID: org, Q: f.Q, Locale: f.Locale, Visibility: f.Visibility, CollectionUuid: collection, State: f.State, Limit: limit, Offset: offset,
		DeptRestrict: restrict, ViewerDepartments: viewer, DepartmentMode: f.DepartmentMode, DepartmentID: f.DepartmentID})
	if err != nil {
		return nil, 0, err
	}
	count, err := s.Repo.Q.KnowledgeArticleCount(ctx, db.KnowledgeArticleCountParams{OrganizationID: org, Q: f.Q, Locale: f.Locale, Visibility: f.Visibility, CollectionUuid: collection, State: f.State,
		DeptRestrict: restrict, ViewerDepartments: viewer, DepartmentMode: f.DepartmentMode, DepartmentID: f.DepartmentID})
	return rows, count, err
}

func (s *Service) Versions(ctx context.Context, org int64, id uuid.UUID, limit, offset int32) ([]db.KnowledgeArticleVersion, int64, error) {
	a, err := s.Article(ctx, org, id)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.Repo.Q.KnowledgeVersions(ctx, db.KnowledgeVersionsParams{OrganizationID: org, ArticleID: a.ID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	count, err := s.Repo.Q.KnowledgeCountVersions(ctx, db.KnowledgeCountVersionsParams{OrganizationID: org, ArticleID: a.ID})
	return rows, count, err
}

func (s *Service) Collection(ctx context.Context, org int64, id uuid.UUID) (db.KnowledgeCollection, error) {
	row, err := s.Repo.Q.KnowledgeCollection(ctx, db.KnowledgeCollectionParams{OrganizationID: org, Uuid: id})
	return row, repository.Normalize(err)
}
