package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrForbidden      = errors.New("forbidden")
	ErrInvalidRequest = errors.New("invalid request")
)

type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Service orchestrates import jobs.
type Service struct {
	q        *db.Queries
	storage  storage.Driver
	registry *ioengine.Registry
	queue    Enqueuer
	notifier Notifier
	activity *activity.Recorder
	log      *slog.Logger
	syncMode bool
}

func New(
	q *db.Queries,
	store storage.Driver,
	reg *ioengine.Registry,
	enq Enqueuer,
	notifier Notifier,
	rec *activity.Recorder,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		q: q, storage: store, registry: reg, queue: enq, notifier: notifier,
		activity: rec, log: log, syncMode: enq == nil,
	}
}

type ImportJobView struct {
	UUID           uuid.UUID               `json:"uuid"`
	Resource       string                  `json:"resource"`
	Format         string                  `json:"format"`
	Status         string                  `json:"status"`
	Mapping        map[string]string       `json:"mapping,omitempty"`
	Defaults       map[string]string       `json:"defaults,omitempty"`
	PreviewSummary ioengine.PreviewSummary `json:"preview_summary,omitempty"`
	Error          *string                 `json:"error,omitempty"`
	RollbackUntil  *time.Time              `json:"rollback_until,omitempty"`
	AppliedAt      *time.Time              `json:"applied_at,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
}

// Upload creates a job from file bytes.
func (s *Service) Upload(ctx context.Context, actorID int64, resource string, format ioengine.ImportFormat, locale string, filename string, r io.Reader) (ImportJobView, error) {
	if _, err := s.registry.Get(resource); err != nil {
		return ImportJobView{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	return s.upload(ctx, actorID, resource, format, locale, filename, r, func() (db.ImportJob, error) {
		return s.q.CreateImportJob(ctx, db.CreateImportJobParams{
			Resource: resource, ActorID: actorID, Format: string(format), Locale: locale,
			FileKey: pgtype.Text{},
		})
	})
}

// UploadTenant creates an organization-scoped import job (CF-84). Only
// tenant.* resources may be imported into an organization.
func (s *Service) UploadTenant(ctx context.Context, orgID, actorID int64, resource string, format ioengine.ImportFormat, locale string, filename string, r io.Reader) (ImportJobView, error) {
	if !strings.HasPrefix(resource, "tenant.") {
		return ImportJobView{}, fmt.Errorf("%w: not a tenant resource", ErrInvalidRequest)
	}
	if _, err := s.registry.Get(resource); err != nil {
		return ImportJobView{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	return s.upload(ctx, actorID, resource, format, locale, filename, r, func() (db.ImportJob, error) {
		return s.q.CreateTenantImportJob(ctx, db.CreateTenantImportJobParams{
			OrganizationID: pgtype.Int8{Int64: orgID, Valid: true},
			Resource:       resource, ActorID: actorID, Format: string(format), Locale: locale,
		})
	})
}

// ListTenantJobs lists the import jobs of one organization (optionally of
// one resource).
func (s *Service) ListTenantJobs(ctx context.Context, orgID int64, resource string, limit, offset int32) ([]ImportJobView, int64, error) {
	res := pgtype.Text{String: resource, Valid: resource != ""}
	org := pgtype.Int8{Int64: orgID, Valid: true}
	rows, err := s.q.ListTenantImportJobs(ctx, db.ListTenantImportJobsParams{
		OrganizationID: org, Resource: res, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountTenantImportJobs(ctx, db.CountTenantImportJobsParams{OrganizationID: org, Resource: res})
	if err != nil {
		return nil, 0, err
	}
	out := make([]ImportJobView, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapImportJob(r))
	}
	return out, total, nil
}

// GetTenantJob returns one job of the organization.
func (s *Service) GetTenantJob(ctx context.Context, orgID int64, jobUUID uuid.UUID) (ImportJobView, error) {
	row, err := s.q.GetImportJobByUUID(ctx, jobUUID)
	if err != nil || !row.OrganizationID.Valid || row.OrganizationID.Int64 != orgID {
		return ImportJobView{}, ErrNotFound
	}
	return mapImportJob(row), nil
}

func (s *Service) upload(ctx context.Context, actorID int64, resource string, format ioengine.ImportFormat, locale string, filename string, r io.Reader, create func() (db.ImportJob, error)) (ImportJobView, error) {
	switch format {
	case ioengine.ImportCSV, ioengine.ImportTSV, ioengine.ImportXLSX, ioengine.ImportJSON:
	default:
		return ImportJobView{}, fmt.Errorf("%w: unsupported format", ErrInvalidRequest)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return ImportJobView{}, err
	}
	if _, _, err := ioengine.ParseUpload(format, bytes.NewReader(data)); err != nil {
		return ImportJobView{}, fmt.Errorf("%w: file could not be read as %s", ErrInvalidRequest, format)
	}
	row, err := create()
	if err != nil {
		return ImportJobView{}, err
	}
	ext := string(format)
	if format == ioengine.ImportJSON {
		ext = "json"
	}
	key := storage.ImportSourceObjectKey(row.Uuid.String(), ext)
	if err := s.storage.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), Filename: filename,
	}, key); err != nil {
		return ImportJobView{}, err
	}
	updated, err := s.q.UpdateImportJobFileKey(ctx, db.UpdateImportJobFileKeyParams{
		Uuid: row.Uuid, FileKey: pgtype.Text{String: key, Valid: true},
	})
	if err != nil {
		return ImportJobView{}, err
	}
	if adapter, err := s.registry.Get(resource); err == nil {
		if headers, _, parseErr := ioengine.ParseUpload(format, bytes.NewReader(data)); parseErr == nil && len(headers) > 0 {
			suggested := ioengine.SuggestMapping(headers, adapter.ImportSchema(), i18n.Normalize(locale))
			if row.OrganizationID.Valid {
				// Tenant jobs keep every header (unmapped ones as "") so
				// the mapping UI can offer them without parsing the file
				// in the browser (XLSX).
				for _, h := range headers {
					if _, ok := suggested[h]; !ok && h != "" {
						suggested[h] = ""
					}
				}
			}
			if len(suggested) > 0 {
				mb, _ := json.Marshal(suggested)
				if mapped, mapErr := s.q.UpdateImportJobMapping(ctx, db.UpdateImportJobMappingParams{
					Uuid: updated.Uuid, MappingJson: mb, DefaultsJson: []byte("{}"),
				}); mapErr == nil {
					updated = mapped
				}
			}
		}
	}
	if s.activity != nil {
		uid := actorID
		s.activity.Record(ctx, &uid, "import.uploaded", resource, &updated.Uuid, map[string]any{
			"format": format,
		}, nil)
	}
	return mapImportJob(updated), nil
}

// SampleFile builds localized sample document.
func (s *Service) Sample(ctx context.Context, resource string, format ioengine.ImportFormat, locale string) ([]byte, string, error) {
	adapter, err := s.registry.Get(resource)
	if err != nil {
		return nil, "", err
	}
	fields := adapter.ImportSchema()
	if len(fields) == 0 {
		return nil, "", ErrInvalidRequest
	}
	return ioengine.EncodeSample(format, locale, fields, adapters.SampleRows(resource))
}

// UpdateMapping saves column mapping and defaults.
func (s *Service) UpdateMapping(ctx context.Context, jobUUID uuid.UUID, actorID int64, mapping, defaults map[string]string) (ImportJobView, error) {
	job, err := s.getOwned(ctx, jobUUID, actorID)
	if err != nil {
		return ImportJobView{}, err
	}
	mb, _ := json.Marshal(mapping)
	dbb, _ := json.Marshal(defaults)
	row, err := s.q.UpdateImportJobMapping(ctx, db.UpdateImportJobMappingParams{
		Uuid: jobUUID, MappingJson: mb, DefaultsJson: dbb,
	})
	if err != nil {
		return ImportJobView{}, err
	}
	_ = job
	return mapImportJob(row), nil
}

// Preview dry-runs mapped rows.
func (s *Service) Preview(ctx context.Context, jobUUID uuid.UUID, actorID int64) (ImportJobView, error) {
	job, err := s.getOwned(ctx, jobUUID, actorID)
	if err != nil {
		return ImportJobView{}, err
	}
	summary, err := s.buildPreview(ctx, job)
	if err != nil {
		return ImportJobView{}, err
	}
	pb, _ := json.Marshal(summary)
	row, err := s.q.UpdateImportJobPreview(ctx, db.UpdateImportJobPreviewParams{Uuid: jobUUID, PreviewJson: pb})
	if err != nil {
		return ImportJobView{}, err
	}
	return mapImportJob(row), nil
}

// Confirm queues apply task.
func (s *Service) Confirm(ctx context.Context, jobUUID uuid.UUID, actorID int64) (ImportJobView, error) {
	if _, err := s.getOwned(ctx, jobUUID, actorID); err != nil {
		return ImportJobView{}, err
	}
	row, err := s.q.QueueImportJob(ctx, jobUUID)
	if err != nil {
		return ImportJobView{}, err
	}
	if s.syncMode {
		_ = s.ProcessImport(ctx, row.ID)
		updated, _ := s.q.GetImportJobByID(ctx, row.ID)
		return mapImportJob(updated), nil
	}
	task, err := queue.NewImportProcessTask(row.ID)
	if err != nil {
		return ImportJobView{}, err
	}
	if _, err := s.queue.Enqueue(queue.WithRequestID(ctx, task), asynq.Queue(queue.QueueImports)); err != nil {
		return ImportJobView{}, err
	}
	return mapImportJob(row), nil
}

// ProcessImport applies rows in worker.
func (s *Service) ProcessImport(ctx context.Context, jobID int64) error {
	job, err := s.q.MarkImportJobApplying(ctx, jobID)
	if err != nil {
		return err
	}
	ctx = jobContext(ctx, job)
	adapter, err := s.registry.Get(job.Resource)
	if err != nil {
		return s.failImport(ctx, jobID, err.Error())
	}
	rows, mapping, defaults, err := s.loadMappedRows(ctx, job)
	if err != nil {
		return s.failImport(ctx, jobID, err.Error())
	}
	created, updated, failed := 0, 0, 0
	var applyErrs []ioengine.RowError
	for i, row := range rows {
		res, err := adapter.ApplyRow(ctx, row, defaults)
		if err != nil {
			failed++
			applyErrs = append(applyErrs, ioengine.RowError{Index: i + 1, Error: err.Error()})
			continue
		}
		if !res.OK {
			failed++
			applyErrs = append(applyErrs, ioengine.RowError{Index: i + 1, Error: res.Error})
			continue
		}
		if res.Op == "create" {
			created++
		} else {
			updated++
		}
		prev, _ := json.Marshal(res.Previous)
		_, _ = s.q.InsertImportChange(ctx, db.InsertImportChangeParams{
			JobID: jobID, EntityType: res.EntityType,
			EntityUuid: uuid.MustParse(res.EntityUUID), Op: res.Op,
			PreviousJson: prev,
		})
	}
	summary := ioengine.PreviewSummary{Total: len(rows), Valid: created + updated, Invalid: failed, Errors: applyErrs}
	pb, _ := json.Marshal(summary)
	applied, err := s.q.MarkImportJobApplied(ctx, db.MarkImportJobAppliedParams{ID: jobID, PreviewJson: pb})
	if err != nil {
		return err
	}
	if s.notifier != nil {
		uid := job.ActorID
		loc := i18n.Normalize(job.Locale)
		_, _ = s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			UserID: &uid, Channels: []string{notifmodel.ChannelInapp, notifmodel.ChannelEmail},
			TemplateCode: "imports.applied", Language: job.Locale,
			ContentVars: map[string]string{
				"resource": i18n.ResourceLabel(loc, job.Resource), "url": "/platform/settings/imports/" + job.Uuid.String(),
				"created": fmt.Sprintf("%d", created), "updated": fmt.Sprintf("%d", updated), "failed": fmt.Sprintf("%d", failed),
			},
			DedupeKey: "imports.applied:" + job.Uuid.String(),
			TemplateVars: map[string]string{
				"resource": i18n.ResourceLabel(loc, job.Resource),
				"created":  fmt.Sprintf("%d", created),
				"updated":  fmt.Sprintf("%d", updated),
				"failed":   fmt.Sprintf("%d", failed),
			},
			SourceEvent: "imports.applied",
		})
	}
	if s.activity != nil {
		uid := job.ActorID
		s.activity.Record(ctx, &uid, "import.applied", job.Resource, &job.Uuid, map[string]any{
			"created": created, "updated": updated, "failed": failed,
		}, nil)
	}
	_ = applied
	_ = mapping
	return nil
}

// Rollback reverts an applied import within window.
func (s *Service) Rollback(ctx context.Context, jobUUID uuid.UUID, actorID int64) (ImportJobView, error) {
	job, err := s.getOwned(ctx, jobUUID, actorID)
	if err != nil {
		return ImportJobView{}, err
	}
	if job.Status != "applied" {
		return ImportJobView{}, ErrInvalidRequest
	}
	if job.RollbackUntil.Valid && time.Now().After(job.RollbackUntil.Time) {
		return ImportJobView{}, ErrInvalidRequest
	}
	adapter, err := s.registry.Get(job.Resource)
	if err != nil {
		return ImportJobView{}, err
	}
	changes, err := s.q.ListImportChangesForJob(ctx, job.ID)
	if err != nil {
		return ImportJobView{}, err
	}
	rctx := jobContext(ctx, job)
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		var prev map[string]any
		_ = json.Unmarshal(ch.PreviousJson, &prev)
		if err := adapter.RevertRow(rctx, ch.EntityType, ch.EntityUuid.String(), prev); err != nil {
			s.log.WarnContext(ctx, "import rollback row failed", "job", job.Uuid, "entity", ch.EntityUuid, "err", err)
		}
	}
	row, err := s.q.MarkImportJobRolledBack(ctx, job.ID)
	if err != nil {
		return ImportJobView{}, err
	}
	if s.activity != nil {
		uid := actorID
		s.activity.Record(ctx, &uid, "import.rolled_back", job.Resource, &job.Uuid, map[string]any{}, nil)
	}
	return mapImportJob(row), nil
}

func (s *Service) ListJobs(ctx context.Context, actorID int64, admin bool, limit, offset int32) ([]ImportJobView, int64, error) {
	var rows []db.ImportJob
	var total int64
	var err error
	if admin {
		rows, err = s.q.ListAllImportJobs(ctx, db.ListAllImportJobsParams{LimitCount: limit, OffsetCount: offset})
		total, _ = s.q.CountAllImportJobs(ctx)
	} else {
		rows, err = s.q.ListImportJobsForActor(ctx, db.ListImportJobsForActorParams{
			ActorID: actorID, LimitCount: limit, OffsetCount: offset,
		})
		total, _ = s.q.CountImportJobsForActor(ctx, actorID)
	}
	if err != nil {
		return nil, 0, err
	}
	out := make([]ImportJobView, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapImportJob(r))
	}
	return out, total, nil
}

func (s *Service) GetJob(ctx context.Context, jobUUID uuid.UUID, actorID int64, admin bool) (ImportJobView, error) {
	row, err := s.q.GetImportJobByUUID(ctx, jobUUID)
	if err != nil {
		return ImportJobView{}, ErrNotFound
	}
	if !admin && row.ActorID != actorID {
		return ImportJobView{}, ErrForbidden
	}
	return mapImportJob(row), nil
}

func (s *Service) buildPreview(ctx context.Context, job db.ImportJob) (ioengine.PreviewSummary, error) {
	rows, _, defaults, err := s.loadMappedRows(ctx, job)
	if err != nil {
		return ioengine.PreviewSummary{}, err
	}
	ctx = jobContext(ctx, job)
	var previewer ioengine.RowPreviewer
	if adapter, err := s.registry.Get(job.Resource); err == nil {
		previewer, _ = adapter.(ioengine.RowPreviewer)
	}
	summary := ioengine.PreviewSummary{Total: len(rows)}
	previews := make([]ioengine.RowPreview, 0, min(len(rows), 50))
	for i, row := range rows {
		op := ""
		if previewer != nil {
			var msg string
			op, msg, err = previewer.PreviewRow(ctx, row, defaults)
			if err != nil {
				return ioengine.PreviewSummary{}, err
			}
			if msg != "" {
				summary.Invalid++
				if len(summary.Errors) < 200 {
					summary.Errors = append(summary.Errors, ioengine.RowError{Index: i + 1, Error: msg})
				}
				continue
			}
			switch op {
			case "create":
				summary.Creates++
			case "update":
				summary.Updates++
			}
		}
		summary.Valid++
		if len(previews) < 50 {
			previews = append(previews, ioengine.RowPreview{Index: i + 1, Data: row, Op: op})
		}
	}
	summary.Rows = previews
	return summary, nil
}

// jobContext scopes ctx to the organization of a tenant import job (CF-84):
// tenant adapters read the organization from orgctx, also in the worker.
func jobContext(ctx context.Context, job db.ImportJob) context.Context {
	if job.OrganizationID.Valid {
		return orgctx.WithOrganizationID(ctx, job.OrganizationID.Int64)
	}
	return ctx
}

func (s *Service) loadMappedRows(ctx context.Context, job db.ImportJob) ([]map[string]any, map[string]string, map[string]any, error) {
	if !job.FileKey.Valid || job.FileKey.String == "" {
		ext := string(job.Format)
		key := storage.ImportSourceObjectKey(job.Uuid.String(), ext)
		job.FileKey = pgtype.Text{String: key, Valid: true}
	}
	rc, _, err := s.storage.Download(ctx, job.FileKey.String)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, nil, err
	}
	headers, rawRows, err := ioengine.ParseUpload(ioengine.ImportFormat(job.Format), bytes.NewReader(data))
	if err != nil {
		return nil, nil, nil, err
	}
	var mapping map[string]string
	_ = json.Unmarshal(job.MappingJson, &mapping)
	if len(mapping) == 0 {
		adapter, _ := s.registry.Get(job.Resource)
		mapping = ioengine.SuggestMapping(headers, adapter.ImportSchema(), i18n.Normalize(job.Locale))
	}
	var defaults map[string]string
	_ = json.Unmarshal(job.DefaultsJson, &defaults)
	defAny := map[string]any{}
	for k, v := range defaults {
		defAny[k] = v
	}
	rows := ioengine.ApplyMapping(rawRows, mapping, defaults)
	return rows, mapping, defAny, nil
}

func (s *Service) getOwned(ctx context.Context, jobUUID uuid.UUID, actorID int64) (db.ImportJob, error) {
	row, err := s.q.GetImportJobByUUID(ctx, jobUUID)
	if err != nil {
		return db.ImportJob{}, ErrNotFound
	}
	if row.ActorID != actorID {
		return db.ImportJob{}, ErrForbidden
	}
	// Platform routes see platform jobs only; tenant routes only the jobs
	// of their own organization.
	orgID, scoped := orgctx.OrganizationID(ctx)
	if row.OrganizationID.Valid != scoped || (scoped && row.OrganizationID.Int64 != orgID) {
		return db.ImportJob{}, ErrNotFound
	}
	return row, nil
}

func (s *Service) failImport(ctx context.Context, jobID int64, msg string) error {
	_, _ = s.q.MarkImportJobFailed(ctx, db.MarkImportJobFailedParams{ID: jobID, Error: pgtype.Text{String: msg, Valid: true}})
	return errors.New(msg)
}

func mapImportJob(row db.ImportJob) ImportJobView {
	var mapping map[string]string
	_ = json.Unmarshal(row.MappingJson, &mapping)
	var defaults map[string]string
	_ = json.Unmarshal(row.DefaultsJson, &defaults)
	var preview ioengine.PreviewSummary
	_ = json.Unmarshal(row.PreviewJson, &preview)
	var errMsg *string
	if row.Error.Valid {
		errMsg = &row.Error.String
	}
	var rb *time.Time
	if row.RollbackUntil.Valid {
		t := row.RollbackUntil.Time
		rb = &t
	}
	var ap *time.Time
	if row.AppliedAt.Valid {
		t := row.AppliedAt.Time
		ap = &t
	}
	return ImportJobView{
		UUID: row.Uuid, Resource: row.Resource, Format: row.Format, Status: row.Status,
		Mapping: mapping, Defaults: defaults, PreviewSummary: preview, Error: errMsg,
		RollbackUntil: rb, AppliedAt: ap, CreatedAt: row.CreatedAt.Time,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
