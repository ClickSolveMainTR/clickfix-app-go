// Package usecase is the history pipeline (CF-217): past support records
// become knowledge suggestions. Records are anonymized on the server, kept in
// a read-only archive outside the inbox, clustered by vector similarity and
// turned into a knowledge_suggestions row only when enough cases share the
// same resolution. Reviews are remembered so a rejected idea does not return.
package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the object storage the uploaded file lives in until the run ends.
type Store interface {
	Blobs
	Upload(ctx context.Context, file storage.File, path string) error
}

// Draft is a proposed article written from a cluster's agreeing cases.
type Draft struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Drafter writes a suggestion from already anonymized cases. The default
// implementation runs through the AI core (platform/ai); tests use a fake.
type Drafter interface {
	Draft(ctx context.Context, org int64, locale, input string) (Draft, error)
}

// Taxonomy assigns a category key to a case. CF-218 brings the shared
// category taxonomy; until then SourceTaxonomy keeps the source's own
// category (slugged), and cases without one share the "" category.
type Taxonomy interface {
	Categorize(ctx context.Context, org int64, r Record, c Case) (string, error)
}

type SourceTaxonomy struct{}

func (SourceTaxonomy) Categorize(_ context.Context, _ int64, r Record, _ Case) (string, error) {
	return r.Category, nil
}

// Directory answers department questions. CF-215 brings departments and
// their leads; until then OrganizationWide accepts no department id and lets
// every holder of tenant.history.review decide (NULL = organization-wide).
type Directory interface {
	ValidDepartment(ctx context.Context, org, department int64) (bool, error)
	CanReview(ctx context.Context, org, user int64, department pgtype.Int8) (bool, error)
}

type OrganizationWide struct{}

func (OrganizationWide) ValidDepartment(context.Context, int64, int64) (bool, error) {
	return false, nil
}
func (OrganizationWide) CanReview(context.Context, int64, int64, pgtype.Int8) (bool, error) {
	return true, nil
}

type Service struct {
	Pool       *pgxpool.Pool
	Q          *db.Queries
	Store      Store
	Embedder   ai.Embedder
	Drafter    Drafter
	Taxonomy   Taxonomy
	Directory  Directory
	Connectors map[string]Source
	Log        *slog.Logger
	Now        func() time.Time
	BatchSize  int
}

func New(pool *pgxpool.Pool, q *db.Queries, store Store, embedder ai.Embedder, drafter Drafter) *Service {
	return &Service{Pool: pool, Q: q, Store: store, Embedder: embedder, Drafter: drafter, Taxonomy: SourceTaxonomy{}, Directory: OrganizationWide{}, Connectors: map[string]Source{}, Log: slog.Default(), Now: time.Now, BatchSize: 32}
}

func (s *Service) now() time.Time { return s.Now().UTC() }

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error { return fn(s.Q.WithTx(tx)) })
}

// Settings returns the organization's settings (defaults when never saved).
func (s *Service) Settings(ctx context.Context, org int64) (db.HistorySetting, error) {
	row, err := s.Q.HistorySettingsGet(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		d := model.DefaultSettings()
		return db.HistorySetting{OrganizationID: org, EvidenceThreshold: d.EvidenceThreshold, ClusterSimilarity: d.ClusterSimilarity, ResolutionSimilarity: d.ResolutionSimilarity,
			RejectionSimilarity: d.RejectionSimilarity, RawRetentionHours: d.RawRetentionHours, ArchiveRetentionDays: d.ArchiveRetentionDays}, nil
	}
	return row, err
}

func (s *Service) SaveSettings(ctx context.Context, org, actor int64, in model.SettingsInput) (db.HistorySetting, error) {
	if err := in.Validate(); err != nil {
		return db.HistorySetting{}, err
	}
	return s.Q.HistorySettingsSave(ctx, db.HistorySettingsSaveParams{OrganizationID: org, EvidenceThreshold: in.EvidenceThreshold, ClusterSimilarity: in.ClusterSimilarity,
		ResolutionSimilarity: in.ResolutionSimilarity, RejectionSimilarity: in.RejectionSimilarity, RawRetentionHours: in.RawRetentionHours, ArchiveRetentionDays: in.ArchiveRetentionDays,
		NightlyEnabled: in.NightlyEnabled, NightlyConsentAnalysis: in.NightlyConsentAnalysis, NightlyConsentTraining: in.NightlyConsentTraining, UpdatedBy: pgtype.Int8{Int64: actor, Valid: actor > 0}})
}

// Upload validates an export, keeps it in private storage and queues a run.
// The file is parsed here only to reject a bad upload early; processing
// happens in the worker. Analysis consent is required; training consent is
// optional and recorded separately.
func (s *Service) Upload(ctx context.Context, org, actor int64, in model.UploadInput, name string, data []byte) (db.HistoryImport, error) {
	if !in.ConsentAnalysis {
		return db.HistoryImport{}, fmt.Errorf("%w: the data owner's consent to analysis is required", model.ErrInvalid)
	}
	if s.Store == nil {
		return db.HistoryImport{}, model.ErrUnavailable
	}
	b, err := ParseBundle(name, data)
	if err != nil {
		return db.HistoryImport{}, err
	}
	brand := pgtype.UUID{}
	if in.BrandUUID != "" {
		id, err := uuid.Parse(in.BrandUUID)
		if err != nil {
			return db.HistoryImport{}, fmt.Errorf("%w: brand_uuid is invalid", model.ErrInvalid)
		}
		brand = pgtype.UUID{Bytes: id, Valid: true}
	}
	department := pgtype.Int8{}
	if in.DepartmentID != 0 {
		ok, err := s.Directory.ValidDepartment(ctx, org, in.DepartmentID)
		if err != nil {
			return db.HistoryImport{}, err
		}
		if !ok {
			return db.HistoryImport{}, fmt.Errorf("%w: unknown department", model.ErrInvalid)
		}
		department = pgtype.Int8{Int64: in.DepartmentID, Valid: true}
	}
	if active, err := s.Q.HistoryImportActive(ctx, org); err != nil {
		return db.HistoryImport{}, err
	} else if active >= 3 {
		return db.HistoryImport{}, fmt.Errorf("%w: three runs are already waiting; wait for one to finish", model.ErrConflict)
	}
	settings, err := s.Settings(ctx, org)
	if err != nil {
		return db.HistoryImport{}, err
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ext != "zip" {
		ext = "json"
	}
	key := storage.HistoryRawObjectKey(org, uuid.NewString(), ext)
	if err := s.Store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/octet-stream"}, key); err != nil {
		return db.HistoryImport{}, err
	}
	run, err := s.Q.HistoryImportCreate(ctx, db.HistoryImportCreateParams{OrganizationID: org, BrandID: brand, DepartmentID: department, Source: "upload", FileName: truncate(filepath.Base(name), 255),
		ConsentAnalysis: true, ConsentTraining: in.ConsentTraining, ConsentBy: pgtype.Int8{Int64: actor, Valid: actor > 0}, RawKey: key,
		RawRetainUntil: pgtype.Timestamptz{Time: s.now().Add(time.Duration(settings.RawRetentionHours) * time.Hour), Valid: true},
		CreatedBy:      pgtype.Int8{Int64: actor, Valid: actor > 0}, TotalRecords: int32(len(b.Tickets))}) // #nosec G115 -- bounded by model.MaxTickets in validate
	if err != nil {
		_ = s.Store.Delete(context.WithoutCancel(ctx), key)
		return db.HistoryImport{}, mapDB(err)
	}
	return run, nil
}

// Cancel stops a waiting run at once and a running one at its next batch.
// What was archived before the stop stays until it is rolled back.
func (s *Service) Cancel(ctx context.Context, org int64, id uuid.UUID) (db.HistoryImport, error) {
	run, err := s.Q.HistoryImportRequestCancel(ctx, db.HistoryImportRequestCancelParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := s.Q.HistoryImportGet(ctx, db.HistoryImportGetParams{OrganizationID: org, Uuid: id}); getErr != nil {
			return run, mapDB(getErr)
		}
		return run, fmt.Errorf("%w: only a waiting or running run can be cancelled", model.ErrConflict)
	}
	return run, err
}

// Rollback removes what a finished run added: its archived cases, the
// pending suggestions that only its cases supported, and clusters left
// empty. Approved suggestions (now article drafts) and the decision memory
// are human decisions and stay.
func (s *Service) Rollback(ctx context.Context, org int64, id uuid.UUID) (db.HistoryImport, error) {
	var out db.HistoryImport
	var touched []int64
	err := s.tx(ctx, func(q *db.Queries) error {
		run, err := q.HistoryImportLock(ctx, db.HistoryImportLockParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return mapDB(err)
		}
		switch run.Status {
		case "completed", "failed", "cancelled":
		default:
			return fmt.Errorf("%w: cancel the run before rolling it back", model.ErrConflict)
		}
		if touched, err = q.HistoryClustersOfImport(ctx, db.HistoryClustersOfImportParams{OrganizationID: org, ImportID: run.ID}); err != nil {
			return err
		}
		if _, err = q.HistorySuggestionDeletePendingByImport(ctx, db.HistorySuggestionDeletePendingByImportParams{OrganizationID: org, ImportID: run.ID}); err != nil {
			return err
		}
		if _, err = q.HistoryCaseDeleteByImport(ctx, db.HistoryCaseDeleteByImportParams{OrganizationID: org, ImportID: run.ID}); err != nil {
			return err
		}
		if err = q.HistoryClusterRecount(ctx, db.HistoryClusterRecountParams{OrganizationID: org, Ids: touched}); err != nil {
			return err
		}
		if err = q.HistoryClusterDeleteEmpty(ctx, org); err != nil {
			return err
		}
		out, err = q.HistoryImportRolledBack(ctx, db.HistoryImportRolledBackParams{OrganizationID: org, ID: run.ID})
		return err
	})
	if err != nil {
		return out, err
	}
	settings, err := s.Settings(ctx, org)
	if err != nil {
		return out, err
	}
	for _, c := range touched {
		if _, err := s.refreshEvidence(ctx, org, c, settings); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
	}
	return out, nil
}

func mapDB(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	}
	if code := pgCode(err); code == "23503" || code == "23514" {
		return fmt.Errorf("%w: %s", model.ErrInvalid, "a referenced record is missing or a value is out of range")
	}
	return err
}
