package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	kb "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type SourceInput struct {
	URL           string `json:"url"`
	Title         string `json:"title"`
	Locale        string `json:"locale"`
	MaxDepth      int32  `json:"max_depth"`
	IntervalHours int32  `json:"interval_hours"`
	// Department (CF-215): null or omitted is organization-wide.
	Department deptscope.Optional `json:"department_uuid"`
	// DepartmentID is the resolved Department (handler).
	DepartmentID pgtype.Int8 `json:"-"`
}
type SourceService struct {
	Agents  *Service
	Storage storage.Driver
	Fetch   PageFetcher
}

func (s *SourceService) validate(in SourceInput) error {
	if strings.TrimSpace(in.Title) == "" || len(in.Title) > 300 || (in.Locale != "tr" && in.Locale != "en") || in.MaxDepth < 0 || in.MaxDepth > 3 || in.IntervalHours < 1 || in.IntervalHours > 8760 {
		return model.ErrInvalid
	}
	return nil
}
func (s *SourceService) CreateURL(ctx context.Context, org, actor int64, in SourceInput) (db.KnowledgeSource, error) {
	if err := s.validate(in); err != nil {
		return db.KnowledgeSource{}, err
	}
	if _, err := toolURL(in.URL); err != nil {
		return db.KnowledgeSource{}, err
	}
	row, err := s.Agents.Repo.Q.AIKnowledgeSourceCreate(ctx, db.AIKnowledgeSourceCreateParams{OrganizationID: org, Kind: "url", Location: in.URL, Title: in.Title, Locale: in.Locale, AuthorID: actor, MaxDepth: in.MaxDepth, IntervalHours: in.IntervalHours})
	if err != nil {
		return row, err
	}
	return s.department(ctx, org, row, in.DepartmentID)
}

// department files a new source under its department (CF-215).
func (s *SourceService) department(ctx context.Context, org int64, row db.KnowledgeSource, dept pgtype.Int8) (db.KnowledgeSource, error) {
	if !dept.Valid {
		return row, nil
	}
	if _, err := s.Agents.Repo.Q.SetKnowledgeSourceDepartment(ctx, db.SetKnowledgeSourceDepartmentParams{OrganizationID: org, Uuid: row.Uuid, DepartmentID: dept}); err != nil {
		return row, err
	}
	row.DepartmentID = dept
	return row, nil
}

func (s *SourceService) Upload(ctx context.Context, org, actor int64, in SourceInput, name string, data []byte) (db.KnowledgeSource, error) {
	if err := s.validate(in); err != nil {
		return db.KnowledgeSource{}, err
	}
	if s.Storage == nil {
		return db.KnowledgeSource{}, ai.ErrUnavailable
	}
	if _, err := ioengine.ExtractKnowledgeDocument(ctx, name, data); err != nil {
		return db.KnowledgeSource{}, fmt.Errorf("%w: invalid knowledge document", model.ErrInvalid)
	}
	key := storage.ImportSourceObjectKey(uuid.NewString(), strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")))
	if err := s.Storage.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/octet-stream"}, key); err != nil {
		return db.KnowledgeSource{}, err
	}
	row, err := s.Agents.Repo.Q.AIKnowledgeSourceCreate(ctx, db.AIKnowledgeSourceCreateParams{OrganizationID: org, Kind: "file", Location: key, Title: in.Title, Locale: in.Locale, AuthorID: actor, MaxDepth: 0, IntervalHours: in.IntervalHours})
	if err != nil {
		_ = s.Storage.Delete(ctx, key)
		return row, err
	}
	return s.department(ctx, org, row, in.DepartmentID)
}

func (s *SourceService) Sync(ctx context.Context, org int64, id uuid.UUID) (err error) {
	source, err := s.Agents.Repo.Q.AIKnowledgeSourceClaim(ctx, db.AIKnowledgeSourceClaimParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	defer func() {
		finish, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		status, code := "completed", ""
		if err != nil {
			status, code = "failed", "SOURCE_SYNC_FAILED: verify source availability, robots, document size, or conflicting manual article edits"
		}
		e := s.Agents.Repo.Q.AIKnowledgeSourceFinish(finish, db.AIKnowledgeSourceFinishParams{OrganizationID: org, Uuid: id, Status: status, LastError: code})
		if e != nil {
			err = errors.Join(err, e)
		}
	}()
	documents := []CrawledDocument{}
	if source.Kind == "url" {
		documents, err = CrawlKnowledge(ctx, source.Location, int(source.MaxDepth), s.Fetch)
		if err != nil {
			return err
		}
	} else {
		if s.Storage == nil {
			return ai.ErrUnavailable
		}
		r, size, e := s.Storage.Download(ctx, source.Location)
		if e != nil {
			return e
		}
		defer func() { _ = r.Close() }()
		if size > ioengine.MaxKnowledgeFileBytes {
			return model.ErrInvalid
		}
		data, e := io.ReadAll(io.LimitReader(r, ioengine.MaxKnowledgeFileBytes+1))
		if e != nil {
			return e
		}
		text, e := ioengine.ExtractKnowledgeDocument(ctx, source.Location, data)
		if e != nil {
			return e
		}
		documents = append(documents, CrawledDocument{Location: source.Location, Title: source.Title, Body: text})
	}
	for _, document := range documents {
		if err = s.importDocument(ctx, source, document); err != nil {
			return err
		}
	}
	return nil
}
func (s *SourceService) importDocument(ctx context.Context, source db.KnowledgeSource, document CrawledDocument) error {
	if document.Title == "" {
		document.Title = source.Title
	}
	document.Body = ai.NewMasker().Mask(document.Body)
	document.Title = ai.NewMasker().Mask(document.Title)
	if ai.Guard(document.Body) != nil || ai.Guard(document.Title) != nil {
		return ai.ErrInjection
	}
	digestBytes := sha256.Sum256([]byte(document.Title + "\n" + document.Body))
	digest := hex.EncodeToString(digestBytes[:])
	return s.Agents.Repo.Within(ctx, func(r *billingrepo.Repository) error {
		// Lock the source to serialize source/document ownership and avoid duplicate
		// articles if a lease expires while an earlier worker is finishing.
		current, e := r.Q.AIKnowledgeSourceLock(ctx, db.AIKnowledgeSourceLockParams{OrganizationID: source.OrganizationID, Uuid: source.Uuid})
		if e != nil {
			return e
		}
		if !current.Enabled {
			return model.ErrConflict
		}
		old, e := r.Q.AIKnowledgeSourceDocument(ctx, db.AIKnowledgeSourceDocumentParams{OrganizationID: source.OrganizationID, SourceID: source.ID, Location: document.Location})
		create := errors.Is(e, pgx.ErrNoRows)
		if e != nil && !create {
			return e
		}
		if !create && old.Digest == digest {
			return nil
		}
		var article db.KnowledgeArticle
		if create {
			id := uuid.NewSHA1(source.Uuid, []byte(document.Location))
			article, e = r.Q.KnowledgeInsertArticle(ctx, db.KnowledgeInsertArticleParams{OrganizationID: source.OrganizationID, Uuid: id, Slug: "import-" + id.String(), Locale: source.Locale, Title: document.Title, Body: document.Body, Tags: []string{}, Visibility: "internal", AuthorID: source.AuthorID})
		} else {
			var previous db.KnowledgeArticle
			previous, e = r.Q.KnowledgeSourceArticleLock(ctx, db.KnowledgeSourceArticleLockParams{OrganizationID: source.OrganizationID, ID: old.ArticleID})
			if e != nil {
				return e
			}
			if previous.Version != old.ArticleVersion || previous.DeletedAt.Valid || previous.Visibility != "internal" {
				return model.ErrConflict
			}
			article, e = r.Q.KnowledgeUpdateArticle(ctx, db.KnowledgeUpdateArticleParams{OrganizationID: source.OrganizationID, Uuid: previous.Uuid, CollectionID: previous.CollectionID, Slug: previous.Slug, Title: document.Title, Body: document.Body, Tags: previous.Tags, Visibility: "internal", AuthorID: source.AuthorID})
		}
		if e != nil {
			return e
		}
		if e = r.Q.KnowledgeInsertVersion(ctx, db.KnowledgeInsertVersionParams{OrganizationID: source.OrganizationID, ArticleID: article.ID, Version: article.Version, Title: article.Title, Body: article.Body, Visibility: "internal", AuthorID: source.AuthorID}); e != nil {
			return e
		}
		if e = r.Q.AIKnowledgeSourceDocumentSave(ctx, db.AIKnowledgeSourceDocumentSaveParams{OrganizationID: source.OrganizationID, SourceID: source.ID, Location: document.Location, Digest: digest, ArticleID: article.ID, ArticleVersion: article.Version}); e != nil {
			return e
		}
		revision, e := r.Q.KnowledgeSetPublished(ctx, db.KnowledgeSetPublishedParams{OrganizationID: source.OrganizationID, ID: article.ID, PublishedVersion: pgtype.Int4{Int32: article.Version, Valid: true}})
		if e != nil {
			return e
		}
		return r.Emit(ctx, events.New(kb.ArticlePublished).WithTenant(source.OrganizationID).WithEntity("knowledge_articles", &article.ID, &article.Uuid).WithPayload(map[string]any{"article_uuid": article.Uuid.String(), "version": article.Version}).WithIdempotencyKey(fmt.Sprintf("source:%s:%s:%d", source.Uuid, article.Uuid, revision)))
	})
}
func (s *SourceService) Sweep(ctx context.Context) error {
	rows, err := s.Agents.Repo.Q.AIKnowledgeSourceDue(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, row := range rows {
		if err = s.Sync(ctx, row.OrganizationID, row.Uuid); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			failures = append(failures, err)
		}
	}
	organizations, e := s.Agents.Repo.Q.KnowledgeReviewDueOrganizations(ctx)
	if e != nil {
		failures = append(failures, e)
	} else {
		for _, org := range organizations {
			if e = s.Agents.SweepKnowledgeReviews(ctx, org, 6); e != nil {
				failures = append(failures, e)
			}
		}
	}
	return errors.Join(failures...)
}
