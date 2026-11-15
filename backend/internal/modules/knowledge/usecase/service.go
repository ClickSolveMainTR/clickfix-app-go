// Package usecase ports ClickFix Laravel's org/draft/visibility scoped knowledge behavior.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ArticlePublished   = "knowledge.article_published"
	ArticleUnpublished = "knowledge.article_unpublished"
)

type Service struct {
	Repo *repository.Store
	// Fields checks article audiences against the custom field catalog.
	Fields FieldSource
}

func New(repo *repository.Store) *Service { return &Service{Repo: repo} }
func (s *Service) Collections(ctx context.Context, org int64, limit, offset int32) ([]db.KnowledgeCollection, int64, error) {
	rows, err := s.Repo.Q.KnowledgeCollections(ctx, db.KnowledgeCollectionsParams{OrganizationID: org, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	count, err := s.Repo.Q.KnowledgeCountCollections(ctx, org)
	return rows, count, err
}

func (s *Service) SaveCollection(ctx context.Context, org int64, id uuid.UUID, in model.CollectionInput) (db.KnowledgeCollection, error) {
	out := db.KnowledgeCollection{}
	if err := in.Validate(); err != nil {
		return out, err
	}
	err := s.Repo.Within(ctx, func(q *db.Queries) error {
		parent := pgtype.Int8{}
		if in.Parent != nil {
			p, e := q.KnowledgeCollection(ctx, db.KnowledgeCollectionParams{OrganizationID: org, Uuid: *in.Parent})
			if e != nil {
				return e
			}
			if p.Kind != "collection" || in.Kind != "category" {
				return model.ErrInvalid
			}
			parent = pgtype.Int8{Int64: p.ID, Valid: true}
		}
		if in.Kind == "category" && !parent.Valid {
			return model.ErrInvalid
		}
		old, oldErr := q.KnowledgeCollection(ctx, db.KnowledgeCollectionParams{OrganizationID: org, Uuid: id})
		if oldErr != nil && !errors.Is(oldErr, pgx.ErrNoRows) {
			return oldErr
		}
		if oldErr == nil && (old.Kind != in.Kind || old.ParentID != parent) {
			return model.ErrConflict
		}
		var e error
		out, e = q.KnowledgeSaveCollection(ctx, db.KnowledgeSaveCollectionParams{OrganizationID: org, Uuid: id, ParentID: parent, Name: in.Name, Slug: in.Slug, Kind: in.Kind})
		return e
	})
	return out, err
}

func (s *Service) DeleteCollection(ctx context.Context, org int64, id uuid.UUID) error {
	n, err := s.Repo.Q.KnowledgeDeleteCollection(ctx, db.KnowledgeDeleteCollectionParams{OrganizationID: org, Uuid: id})
	if err == nil && n == 0 {
		return model.ErrNotFound
	}
	return repository.Normalize(err)
}

func (s *Service) SaveArticle(ctx context.Context, org, actor int64, id uuid.UUID, in model.ArticleInput, create bool) (db.KnowledgeArticle, error) {
	out := db.KnowledgeArticle{}
	if err := in.Validate(); err != nil {
		return out, err
	}
	err := s.Repo.Within(ctx, func(q *db.Queries) error {
		collection := pgtype.Int8{}
		if in.Collection != nil {
			c, e := q.KnowledgeCollection(ctx, db.KnowledgeCollectionParams{OrganizationID: org, Uuid: *in.Collection})
			if e != nil {
				return e
			}
			collection = pgtype.Int8{Int64: c.ID, Valid: true}
		}
		if in.Tags == nil {
			in.Tags = []string{}
		}
		var e error
		if create {
			out, e = q.KnowledgeInsertArticle(ctx, db.KnowledgeInsertArticleParams{OrganizationID: org, Uuid: id, CollectionID: collection, Slug: in.Slug, Locale: in.Locale, Title: in.Title, Body: in.Body, Tags: in.Tags, Visibility: in.Visibility, AuthorID: actor})
		} else {
			old, err := q.KnowledgeLockArticle(ctx, db.KnowledgeLockArticleParams{OrganizationID: org, Uuid: id})
			if err != nil {
				return err
			}
			if old.DeletedAt.Valid {
				return model.ErrNotFound
			}
			if old.Version != in.Version || old.Locale != in.Locale {
				return model.ErrConflict
			}
			// Published slugs stay stable so old citations do not redirect to another document.
			if old.PublishedVersion.Valid && old.Slug != in.Slug {
				return model.ErrConflict
			}
			out, e = q.KnowledgeUpdateArticle(ctx, db.KnowledgeUpdateArticleParams{OrganizationID: org, Uuid: id, CollectionID: collection, Slug: in.Slug, Title: in.Title, Body: in.Body, Tags: in.Tags, Visibility: in.Visibility, AuthorID: actor})
		}
		if e != nil {
			return e
		}
		if create || in.Department.Set {
			if _, e = q.SetKnowledgeArticleDepartment(ctx, db.SetKnowledgeArticleDepartmentParams{OrganizationID: org, Uuid: id, DepartmentID: in.DepartmentID}); e != nil {
				return e
			}
			out.DepartmentID = in.DepartmentID
		}
		return q.KnowledgeInsertVersion(ctx, db.KnowledgeInsertVersionParams{OrganizationID: org, ArticleID: out.ID, Version: out.Version, Title: out.Title, Body: out.Body, Visibility: out.Visibility, AuthorID: actor})
	})
	return out, err
}

// Publish atomically switches the public snapshot and writes a replayable indexing event.
func (s *Service) Publish(ctx context.Context, org int64, id uuid.UUID, version int32, publish bool) error {
	return s.Repo.Within(ctx, func(q *db.Queries) error {
		a, err := q.KnowledgeLockArticle(ctx, db.KnowledgeLockArticleParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if a.DeletedAt.Valid {
			return model.ErrNotFound
		}
		if a.Version != version {
			return model.ErrConflict
		}
		v := pgtype.Int4{}
		name := ArticleUnpublished
		if publish {
			v = pgtype.Int4{Int32: version, Valid: true}
			name = ArticlePublished
		}
		revision, err := q.KnowledgeSetPublished(ctx, db.KnowledgeSetPublishedParams{OrganizationID: org, ID: a.ID, PublishedVersion: v})
		if err != nil {
			return err
		}
		return repository.Emit(ctx, q, events.New(name).WithOrganization(org).WithEntity("knowledge_articles", &a.ID, &a.Uuid).WithPayload(map[string]any{"article_uuid": id.String(), "version": version}).WithIdempotencyKey(fmt.Sprintf("%s:%d:%s:%d", id, version, name, revision)))
	})
}

func (s *Service) DeleteArticle(ctx context.Context, org int64, id uuid.UUID) error {
	return s.Repo.Within(ctx, func(q *db.Queries) error {
		a, err := q.KnowledgeLockArticle(ctx, db.KnowledgeLockArticleParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if a.DeletedAt.Valid {
			return model.ErrNotFound
		}
		if err = q.KnowledgeDeleteArticle(ctx, db.KnowledgeDeleteArticleParams{OrganizationID: org, ID: a.ID}); err != nil {
			return err
		}
		return repository.Emit(ctx, q, events.New(ArticleUnpublished).WithOrganization(org).WithPayload(map[string]any{"article_uuid": id.String(), "version": a.Version}).WithIdempotencyKey(id.String()+":delete:"+strconv.Itoa(int(a.Version))))
	})
}
