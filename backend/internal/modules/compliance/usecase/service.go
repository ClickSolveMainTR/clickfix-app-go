// Package usecase implements organization-scoped data subject and retention operations.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/reqid"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const TaskProcess = "app:compliance:process"
const TaskSweep = "app:compliance:sweep"
const Queue = "maintenance"

type Enqueuer interface {
	EnqueueContext(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error)
}
type SearchDeleter interface {
	DeleteDocument(context.Context, string, string) error
}

type Service struct {
	Search   SearchDeleter
	Repo     *repository.Postgres
	Store    storage.Driver
	Jobs     Enqueuer
	Activity *activity.Recorder
}
type Payload struct {
	OrganizationID int64     `json:"organization_id"`
	UUID           uuid.UUID `json:"uuid"`
}
type Policy struct {
	ConversationDays int32  `json:"conversation_days"`
	AttachmentDays   int32  `json:"attachment_days"`
	AILogDays        int32  `json:"ai_log_days"`
	Action           string `json:"action"`
}
type Request struct {
	UUID        uuid.UUID  `json:"uuid"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	DownloadURL string     `json:"download_url,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

func View(row db.DataSubjectRequest) Request {
	v := Request{UUID: row.Uuid, Kind: row.Kind, Status: row.Status}
	if row.ExpiresAt.Valid {
		v.ExpiresAt = &row.ExpiresAt.Time
	}
	if row.ObjectKey != "" && row.Status == "completed" && row.ExpiresAt.Time.After(time.Now()) {
		v.DownloadURL = "/api/v1/tenant/compliance/requests/" + row.Uuid.String() + "/download"
	}
	return v
}

// RequestItem is a data subject request with its subject and requester, for
// the organization's request log.
type RequestItem struct {
	Request
	CreatedAt   time.Time      `json:"created_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Contact     RequestContact `json:"contact"`
	RequestedBy *RequestActor  `json:"requested_by"`
}
type RequestContact struct {
	UUID   uuid.UUID `json:"uuid"`
	Name   string    `json:"name"`
	Email  string    `json:"email"`
	Erased bool      `json:"erased"`
}
type RequestActor struct {
	Name    string `json:"name"`
	Surname string `json:"surname"`
}

func (s *Service) ListRequests(ctx context.Context, org int64, limit, offset int32) ([]RequestItem, int64, error) {
	rows, err := s.Repo.Q.ComplianceListRequests(ctx, db.ComplianceListRequestsParams{OrganizationID: org, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.Repo.Q.ComplianceRequestCount(ctx, org)
	if err != nil {
		return nil, 0, err
	}
	out := make([]RequestItem, 0, len(rows))
	for _, r := range rows {
		v := RequestItem{
			Request:   View(db.DataSubjectRequest{Uuid: r.Uuid, Kind: r.Kind, Status: r.Status, ObjectKey: r.ObjectKey, ExpiresAt: r.ExpiresAt}),
			CreatedAt: r.CreatedAt.Time,
			Contact:   RequestContact{UUID: r.ContactUuid, Name: r.ContactName, Email: r.ContactEmail.String, Erased: r.ContactErasedAt.Valid},
		}
		if r.CompletedAt.Valid {
			v.CompletedAt = &r.CompletedAt.Time
		}
		if r.RequestedBy.Valid {
			v.RequestedBy = &RequestActor{Name: r.RequesterName, Surname: r.RequesterSurname}
		}
		out = append(out, v)
	}
	return out, total, nil
}
func (s *Service) GetPolicy(ctx context.Context, org int64) (Policy, error) {
	v, err := s.Repo.Q.CompliancePolicy(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{Action: "anonymize"}, nil
	}
	return Policy{v.ConversationDays, v.AttachmentDays, v.AiLogDays, v.Action}, err
}

var ErrInvalid = errors.New("compliance: invalid input")

func (s *Service) SavePolicy(ctx context.Context, org int64, in Policy) (Policy, error) {
	if in.ConversationDays < 0 || in.ConversationDays > 36500 || in.AttachmentDays < 0 || in.AttachmentDays > 36500 || in.AILogDays < 0 || in.AILogDays > 36500 || (in.Action != "delete" && in.Action != "anonymize") {
		return Policy{}, ErrInvalid
	}
	_, err := s.Repo.Q.ComplianceSavePolicy(ctx, db.ComplianceSavePolicyParams{OrganizationID: org, ConversationDays: in.ConversationDays, AttachmentDays: in.AttachmentDays, AiLogDays: in.AILogDays, Action: in.Action})
	return in, err
}
func (s *Service) Create(ctx context.Context, org, actor int64, contact uuid.UUID, kind string) (Request, error) {
	if kind != "export" && kind != "erase" {
		return Request{}, ErrInvalid
	}
	var row db.DataSubjectRequest
	err := s.Repo.Transaction(ctx, func(q *db.Queries) error {
		c, e := q.ComplianceContact(ctx, db.ComplianceContactParams{OrganizationID: org, Uuid: contact})
		if e != nil {
			return e
		}
		row, e = q.ComplianceRequest(ctx, db.ComplianceRequestParams{OrganizationID: org, ContactID: c.ID, RequestedBy: pgtype.Int8{Int64: actor, Valid: actor > 0}, Kind: kind, RequestID: reqid.From(ctx)})
		return e
	})
	if err != nil {
		return Request{}, err
	}
	// Durable row survives a Redis outage; the daily sweep recovers pending requests.
	if s.Jobs != nil {
		body, _ := json.Marshal(Payload{org, row.Uuid})
		_, _ = s.Jobs.EnqueueContext(ctx, asynq.NewTask(TaskProcess, body), asynq.Queue(Queue), asynq.MaxRetry(8), asynq.Timeout(30*time.Minute))
	}
	s.Activity.Record(ctx, &actor, "compliance.requested", "compliance", &row.Uuid, map[string]any{"kind": kind}, nil)
	return View(row), nil
}
func (s *Service) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var in Payload
	if json.Unmarshal(t.Payload(), &in) != nil || in.OrganizationID <= 0 || in.UUID == uuid.Nil {
		return fmt.Errorf("invalid compliance task: %w", asynq.SkipRetry)
	}
	return s.Process(ctx, in.OrganizationID, in.UUID)
}
func (s *Service) Process(ctx context.Context, org int64, id uuid.UUID) error {
	ctx = orgctx.WithOrganizationID(ctx, org)
	return s.Repo.Transaction(ctx, func(q *db.Queries) error {
		r, err := q.ComplianceLockRequest(ctx, db.ComplianceLockRequestParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if r.Status == "completed" {
			return nil
		}
		// Lock the contact to serialize export and erasure of the same subject.
		c, err := q.ComplianceSubjectByID(ctx, db.ComplianceSubjectByIDParams{OrganizationID: org, ID: r.ContactID})
		if err != nil {
			return err
		}
		ctx = reqid.With(ctx, r.RequestID)
		key := ""
		if r.Kind == "export" {
			key, err = s.export(ctx, q, r, c)
		} else {
			err = s.erase(ctx, q, org, c)
		}
		if err != nil {
			return err
		}
		if err = q.ComplianceCompleteRequest(ctx, db.ComplianceCompleteRequestParams{OrganizationID: org, Uuid: id, ObjectKey: key}); err != nil {
			return err
		}
		s.Activity.Record(ctx, nil, "compliance.completed", "compliance", &r.Uuid, map[string]any{"kind": r.Kind}, nil)
		return nil
	})
}
func (s *Service) export(ctx context.Context, q *db.Queries, r db.DataSubjectRequest, c db.Contact) (string, error) {
	f, err := os.CreateTemp("", "clickfix-subject-*.zip")
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	z := ioengine.NewArchive(f, 1<<30)
	convs, err := q.ComplianceConversations(ctx, db.ComplianceConversationsParams{OrganizationID: r.OrganizationID, ContactID: c.ID})
	if err != nil {
		return "", err
	}
	for offset := int32(0); ; offset += 200 {
		msgs, e := q.ComplianceMessages(ctx, db.ComplianceMessagesParams{OrganizationID: r.OrganizationID, ContactID: c.ID, Limit: 200, Offset: offset})
		if e != nil {
			return "", e
		}
		for _, m := range msgs {
			if e = z.JSON("messages/"+m.Uuid.String()+".json", m); e != nil {
				return "", e
			}
		}
		if len(msgs) < 200 {
			break
		}
	}
	ai, err := q.ComplianceAIRuns(ctx, db.ComplianceAIRunsParams{OrganizationID: r.OrganizationID, ContactID: c.ID})
	if err != nil {
		return "", err
	}
	consents, err := q.ComplianceAllConsents(ctx, db.ComplianceAllConsentsParams{OrganizationID: r.OrganizationID, ContactID: c.ID})
	if err != nil {
		return "", err
	}
	notes, err := q.ListContactNotesForExport(ctx, db.ListContactNotesForExportParams{OrganizationID: r.OrganizationID, ContactID: c.ID})
	if err != nil {
		return "", err
	}
	for _, entry := range []struct {
		name string
		data any
	}{{"contact.json", c}, {"conversations.json", convs}, {"ai_runs.json", ai}, {"consents.json", consents}, {"notes.json", notes}} {
		if err = z.JSON(entry.name, entry.data); err != nil {
			return "", err
		}
	}
	files, err := q.ComplianceAttachments(ctx, db.ComplianceAttachmentsParams{OrganizationID: r.OrganizationID, ContactID: c.ID})
	if err != nil {
		return "", err
	}
	manifest := make([]map[string]any, 0, len(files))
	for _, a := range files {
		if a.ObjectKey == "" {
			continue
		}
		body, _, e := s.Store.Download(ctx, a.ObjectKey)
		if e != nil {
			return "", e
		}
		e = z.Add("attachments/"+a.Uuid.String(), body)
		_ = body.Close()
		if e != nil {
			return "", e
		}
		manifest = append(manifest, map[string]any{"uuid": a.Uuid, "filename": a.Filename, "content_type": a.ContentType, "sha256": a.Sha256})
	}
	if err = z.JSON("attachments.json", manifest); err != nil {
		return "", err
	}
	if err = z.Close(); err != nil {
		return "", err
	}
	size, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	key := storage.ExportObjectKey(r.Uuid.String(), "zip")
	if err = s.Store.Upload(ctx, storage.File{Body: f, Size: size, ContentType: "application/zip", Filename: "data-subject.zip"}, key); err != nil {
		return "", err
	}
	return key, nil
}
func (s *Service) purgeAttachment(ctx context.Context, q *db.Queries, a db.Attachment) error {
	// Delete bytes before clearing metadata: on failure the transaction rolls back
	// and retries retain the keys. Driver Delete is idempotent, including missing keys.
	for _, key := range []string{a.ObjectKey, a.ThumbKey.String} {
		if key != "" {
			if err := storage.PurgeObject(ctx, s.Store, key); err != nil {
				return err
			}
		}
	}
	return q.CompliancePurgeAttachment(ctx, db.CompliancePurgeAttachmentParams{OrganizationID: a.OrganizationID, ID: a.ID})
}
func (s *Service) scrubConversation(ctx context.Context, q *db.Queries, org, id int64, erase bool) error {
	chunks, err := q.ConversationReplayChunks(ctx, db.ConversationReplayChunksParams{OrganizationID: org, ConversationID: id})
	if err != nil {
		return err
	}
	for _, ch := range chunks {
		if err := storage.PurgeObject(ctx, s.Store, ch.ObjectKey); err != nil {
			return err
		}
		if err := q.DeleteWidgetReplayChunk(ctx, db.DeleteWidgetReplayChunkParams{OrganizationID: org, Uuid: ch.Uuid}); err != nil {
			return err
		}
	}
	if err := q.EraseConversationReplaySessions(ctx, db.EraseConversationReplaySessionsParams{OrganizationID: org, ConversationID: id}); err != nil {
		return err
	}

	if s.Search != nil {
		cid, e := q.ComplianceConversationPublicID(ctx, db.ComplianceConversationPublicIDParams{OrganizationID: org, ID: id})
		if e != nil {
			return e
		}
		ids, e := q.ComplianceConversationMessageIDs(ctx, db.ComplianceConversationMessageIDsParams{OrganizationID: org, ConversationID: id})
		if e != nil {
			return e
		}
		for _, mid := range ids {
			if e = s.Search.DeleteDocument(ctx, "messages", mid.String()); e != nil {
				return e
			}
		}
		if e = s.Search.DeleteDocument(ctx, "conversations", cid.String()); e != nil {
			return e
		}
	}
	anonymous, e := q.ComplianceAnonymousContact(ctx, org)
	if e != nil {
		return e
	}

	if err := q.ComplianceScrubAI(ctx, db.ComplianceScrubAIParams{OrganizationID: org, ConversationID: pgtype.Int8{Int64: id, Valid: true}}); err != nil {
		return err
	}
	if err := q.ComplianceScrubJobs(ctx, db.ComplianceScrubJobsParams{OrganizationID: org, ConversationID: id}); err != nil {
		return err
	}
	if err := q.ComplianceScrubMessages(ctx, db.ComplianceScrubMessagesParams{OrganizationID: org, ConversationID: id}); err != nil {
		return err
	}
	if err := q.ComplianceScrubDerived(ctx, db.ComplianceScrubDerivedParams{OrganizationID: org, ConversationID: id}); err != nil {
		return err
	}
	return q.ComplianceScrubConversation(ctx, db.ComplianceScrubConversationParams{OrganizationID: org, ID: id, Erase: erase, AnonymousContactID: anonymous})
}
func (s *Service) erase(ctx context.Context, q *db.Queries, org int64, c db.Contact) error {
	if s.Search != nil {
		if e := s.Search.DeleteDocument(ctx, "contacts", c.Uuid.String()); e != nil {
			return e
		}
	}
	exports, e := q.ComplianceSubjectExports(ctx, db.ComplianceSubjectExportsParams{OrganizationID: org, ContactID: c.ID})
	if e != nil {
		return e
	}
	for _, r := range exports {
		if e = storage.PurgeObject(ctx, s.Store, r.ObjectKey); e != nil {
			return e
		}
		if e = q.ComplianceClearExport(ctx, db.ComplianceClearExportParams{OrganizationID: org, Uuid: r.Uuid}); e != nil {
			return e
		}
	}

	convs, err := q.ComplianceConversations(ctx, db.ComplianceConversationsParams{OrganizationID: org, ContactID: c.ID})
	if err != nil {
		return err
	}
	files, err := q.ComplianceAttachments(ctx, db.ComplianceAttachmentsParams{OrganizationID: org, ContactID: c.ID})
	if err != nil {
		return err
	}
	for _, a := range files {
		if err = s.purgeAttachment(ctx, q, a); err != nil {
			return err
		}
	}
	for _, v := range convs {
		if err = s.scrubConversation(ctx, q, org, v.ID, true); err != nil {
			return err
		}
	}
	if err = q.ComplianceEraseIdentities(ctx, db.ComplianceEraseIdentitiesParams{OrganizationID: org, ContactID: c.ID}); err != nil {
		return err
	}
	// CF-221: agent notes and cached AI summaries are free text about the person.
	if _, err = q.DeleteContactNotes(ctx, db.DeleteContactNotesParams{OrganizationID: org, ContactID: c.ID}); err != nil {
		return err
	}
	if _, err = q.DeleteContactAISummaries(ctx, db.DeleteContactAISummariesParams{OrganizationID: org, ContactID: c.ID}); err != nil {
		return err
	}
	return q.ComplianceEraseContact(ctx, db.ComplianceEraseContactParams{OrganizationID: org, ID: c.ID})
}
func (s *Service) Sweep(ctx context.Context, _ *asynq.Task) error {
	metaErr := s.sweepMeta(ctx)
	pending, err := s.Repo.Q.CompliancePendingRequests(ctx)
	if err != nil {
		return err
	}
	var errs []error
	if metaErr != nil {
		errs = append(errs, metaErr)
	}
	for _, r := range pending {
		if e := s.Process(ctx, r.OrganizationID, r.Uuid); e != nil {
			errs = append(errs, e)
		}
	}
	expired, err := s.Repo.Q.ComplianceExpiredExports(ctx)
	if err != nil {
		return err
	}
	for _, r := range expired {
		if e := storage.PurgeObject(ctx, s.Store, r.ObjectKey); e != nil {
			errs = append(errs, e)
			continue
		}
		if e := s.Repo.Q.ComplianceClearExport(ctx, db.ComplianceClearExportParams{OrganizationID: r.OrganizationID, Uuid: r.Uuid}); e != nil {
			errs = append(errs, e)
		}
	}
	policies, err := s.Repo.Q.CompliancePolicies(ctx)
	if err != nil {
		return err
	}
	for _, p := range policies {
		if e := s.retain(orgctx.WithOrganizationID(ctx, p.OrganizationID), p); e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}
func (s *Service) retain(ctx context.Context, p db.RetentionPolicy) error {
	return s.Repo.Transaction(ctx, func(q *db.Queries) error {
		if p.ConversationDays > 0 {
			for {
				rows, err := q.ComplianceRetentionConversations(ctx, db.ComplianceRetentionConversationsParams{OrganizationID: p.OrganizationID, Days: p.ConversationDays})
				if err != nil {
					return err
				}
				for _, v := range rows {
					files, e := q.ComplianceConversationAttachments(ctx, db.ComplianceConversationAttachmentsParams{OrganizationID: p.OrganizationID, ID: v.ID})
					if e != nil {
						return e
					}
					for _, a := range files {
						if e = s.purgeAttachment(ctx, q, a); e != nil {
							return e
						}
					}
					if e = s.scrubConversation(ctx, q, p.OrganizationID, v.ID, p.Action == "delete"); e != nil {
						return e
					}
				}
				if len(rows) < 200 {
					break
				}
			}
		}
		if p.AttachmentDays > 0 {
			for {
				rows, err := q.ComplianceRetentionAttachments(ctx, db.ComplianceRetentionAttachmentsParams{OrganizationID: p.OrganizationID, Days: p.AttachmentDays})
				if err != nil {
					return err
				}
				for _, v := range rows {
					if err = s.purgeAttachment(ctx, q, v); err != nil {
						return err
					}
				}
				if len(rows) < 200 {
					break
				}
			}
		}
		if p.AiLogDays > 0 {
			if err := q.ComplianceRetentionAI(ctx, db.ComplianceRetentionAIParams{OrganizationID: p.OrganizationID, Days: p.AiLogDays}); err != nil {
				return err
			}
		}
		s.Activity.Record(ctx, nil, "compliance.retention_applied", "compliance", nil, map[string]any{"action": p.Action}, nil)
		return nil
	})
}
