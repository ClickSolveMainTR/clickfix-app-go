// Package usecase implements the engineering bridge over the inbox and sqlc repository.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalid     = errors.New("engineering: invalid request")
	ErrNotFound    = errors.New("engineering: not found")
	ErrConflict    = errors.New("engineering: revision conflict")
	ErrUnavailable = errors.New("engineering: integration unavailable")
)

type Service struct {
	Store        *repository.Store
	Inbox        *conv.Service
	Box          *crypto.SecretBox
	Adapters     map[string]Adapter
	SentryClient *http.Client
	Summarize    func(context.Context, int64, int64, string) (string, error)
}

func New(store *repository.Store, inbox *conv.Service, box *crypto.SecretBox) *Service {
	return &Service{Store: store, Inbox: inbox, Box: box, Adapters: DefaultAdapters(), SentryClient: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func nullable(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: id > 0} }
func invalid(message string) error  { return fmt.Errorf("%w: %s", ErrInvalid, message) }
func missing(err error) error {
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, conv.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
func validate(in model.CreateInput) error {
	if strings.TrimSpace(in.Title) == "" || len([]rune(in.Title)) > 240 || len(in.Description) > 50000 || len(in.Cycle) > 120 || len(in.Labels) > 30 {
		return invalid("title, description, cycle or labels exceed limits")
	}
	if !slices.Contains([]string{"backlog", "todo", "in_progress", "done", "canceled"}, in.Status) || !slices.Contains([]string{"low", "normal", "high", "urgent"}, in.Priority) {
		return invalid("unknown status or priority")
	}
	for _, label := range in.Labels {
		if strings.TrimSpace(label) == "" || len(label) > 80 {
			return invalid("invalid label")
		}
	}
	if len(in.Context) > 64000 || !json.Valid(in.Context) || len(in.Context) == 0 || in.Context[0] != '{' {
		return invalid("context must be an object up to 64KB")
	}
	return nil
}
func (s *Service) dto(ctx context.Context, q *db.Queries, r db.EngineeringIssue) (model.Issue, error) {
	refs, err := q.EngineeringIssueRefs(ctx, db.EngineeringIssueRefsParams{OrganizationID: r.OrganizationID, ID: r.ID})
	if err != nil {
		return model.Issue{}, err
	}
	ptr := func(v pgtype.UUID) *uuid.UUID {
		if !v.Valid {
			return nil
		}
		id := uuid.UUID(v.Bytes)
		return &id
	}
	var resolved *time.Time
	if r.ResolvedAt.Valid {
		resolved = &r.ResolvedAt.Time
	}
	labels := r.Labels
	if labels == nil {
		labels = []string{}
	}
	return model.Issue{UUID: r.Uuid, Title: r.Title, Description: r.Description, Status: r.Status, Priority: r.Priority, Labels: labels, Cycle: r.Cycle, Project: ptr(refs.ProjectUuid), Assignee: ptr(refs.AssigneeUuid), Release: ptr(refs.ReleaseUuid), Context: r.Context, Revision: r.Revision, ResolvedAt: resolved, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}, nil
}
func (s *Service) Get(ctx context.Context, org int64, id uuid.UUID) (model.Issue, error) {
	r, err := s.Store.Q.EngineeringIssue(ctx, db.EngineeringIssueParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return model.Issue{}, missing(err)
	}
	return s.dto(ctx, s.Store.Q, r)
}
func (s *Service) List(ctx context.Context, org int64, query apiquery.Query, status, priority string) (apiquery.Page[model.Issue], error) {
	for _, sort := range query.Sort {
		if sort.Field != "updated_at" || !sort.Desc {
			return apiquery.Page[model.Issue]{}, invalid("only -updated_at sort is supported")
		}
	}
	if status != "" && !slices.Contains([]string{"backlog", "todo", "in_progress", "done", "canceled"}, status) {
		return apiquery.Page[model.Issue]{}, invalid("unknown status")
	}
	if priority != "" && !slices.Contains([]string{"low", "normal", "high", "urgent"}, priority) {
		return apiquery.Page[model.Issue]{}, invalid("unknown priority")
	}
	rows, err := s.Store.Q.EngineeringListIssues(ctx, db.EngineeringListIssuesParams{OrganizationID: org, Status: status, Priority: priority, Q: query.Q, RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return apiquery.Page[model.Issue]{}, err
	}
	total, err := s.Store.Q.EngineeringCountIssues(ctx, db.EngineeringCountIssuesParams{OrganizationID: org, Status: status, Priority: priority, Q: query.Q})
	if err != nil {
		return apiquery.Page[model.Issue]{}, err
	}
	items := make([]model.Issue, 0, len(rows))
	for _, r := range rows {
		v, e := s.dto(ctx, s.Store.Q, r)
		if e != nil {
			return apiquery.Page[model.Issue]{}, e
		}
		items = append(items, v)
	}
	return apiquery.NewPage(items, total, query.Limit, query.Offset), nil
}
func references(ctx context.Context, q *db.Queries, org int64, project, assignee *uuid.UUID) (pgtype.Int8, pgtype.Int8, error) {
	var p, a pgtype.Int8
	if project != nil {
		v, err := q.EngineeringProject(ctx, db.EngineeringProjectParams{OrganizationID: org, Uuid: *project})
		if err != nil {
			return p, a, missing(err)
		}
		p = nullable(v.ID)
	}
	if assignee != nil {
		v, err := q.EngineeringAssignee(ctx, db.EngineeringAssigneeParams{OrganizationID: org, Uuid: *assignee})
		if err != nil {
			return p, a, missing(err)
		}
		a = nullable(v)
	}
	return p, a, nil
}
func emit(ctx context.Context, q *db.Queries, r db.EngineeringIssue, actor int64, action string) error {
	ev := events.New("engineering."+action).WithOrganization(r.OrganizationID).WithEntity("engineering_issues", &r.ID, &r.Uuid).WithPayload(map[string]any{"issue_uuid": r.Uuid, "status": r.Status, "revision": r.Revision})
	if actor > 0 {
		ev = ev.WithActor(actor)
	}
	return outbox.EnqueueQueries(ctx, q, ev)
}
func (s *Service) Create(ctx context.Context, org, actor int64, in model.CreateInput) (model.Issue, error) {
	if in.Status == "" {
		in.Status = "backlog"
	}
	if in.Priority == "" {
		in.Priority = "normal"
	}
	if len(in.Context) == 0 {
		in.Context = json.RawMessage(`{}`)
	}
	if in.Labels == nil {
		in.Labels = []string{}
	}
	var conversation db.Conversation
	if in.Conversation != nil {
		if s.Inbox == nil {
			return model.Issue{}, ErrUnavailable
		}
		d, err := s.Inbox.Get(ctx, org, actor, *in.Conversation)
		if err != nil {
			return model.Issue{}, missing(err)
		}
		conversation, err = s.Store.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: d.UUID})
		if err != nil {
			return model.Issue{}, missing(err)
		}
		if in.Title == "" {
			in.Title = d.Subject
			if strings.TrimSpace(in.Title) == "" {
				r := []rune(conversation.LastMessagePreview)
				if len(r) > 240 {
					r = r[:240]
				}
				in.Title = string(r)
			}
		}
		if in.Description == "" {
			in.Description = conversation.LastMessagePreview
		}
		// Snapshot widget diagnostics and API-owned attachment references from
		// the visible conversation. Bound each record to keep the snapshot small.
		messages, e := s.Inbox.Messages(ctx, org, *in.Conversation, uuid.Nil, 30)
		if e != nil {
			return model.Issue{}, e
		}
		diagnostics := []map[string]any{}
		diagnosticBytes := 0
		diagnosticsTruncated := false
		for _, message := range messages.Items {
			attachments := []map[string]any{}
			for _, file := range message.Attachments {
				attachments = append(attachments, map[string]any{"uuid": file.UUID, "filename": file.Filename, "content_type": file.ContentType})
			}
			record := map[string]any{"message_uuid": message.UUID, "attributes": message.Attributes, "attachments": attachments}
			raw, e := json.Marshal(record)
			if e != nil {
				return model.Issue{}, e
			}
			if diagnosticBytes+len(raw) <= 48000 {
				diagnostics = append(diagnostics, record)
				diagnosticBytes += len(raw)
			} else {
				diagnosticsTruncated = true
			}
		}
		snapshot := map[string]any{"conversation_uuid": *in.Conversation, "custom_fields": json.RawMessage(conversation.CustomFields), "diagnostics": diagnostics, "diagnostics_truncated": diagnosticsTruncated, "provided_context": in.Context}
		in.Context, e = json.Marshal(snapshot)
		if e != nil {
			return model.Issue{}, e
		}

		if in.AISummary {
			if s.Summarize == nil {
				return model.Issue{}, ErrUnavailable
			}
			var input strings.Builder
			for _, message := range messages.Items {
				if input.Len()+len(message.Content) > 22000 {
					break
				}
				input.WriteString(message.Direction + ": " + message.Content + "\n")
			}
			summary, e := s.Summarize(ctx, org, actor, input.String())
			if e != nil {
				return model.Issue{}, fmt.Errorf("%w: AI summary failed", ErrUnavailable)
			}
			in.Description = summary
		}
	}
	if in.AISummary && in.Conversation == nil {
		return model.Issue{}, invalid("ai_summary requires conversation_uuid")
	}
	if err := validate(in); err != nil {
		return model.Issue{}, err
	}
	var row db.EngineeringIssue
	err := s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		project, assignee, err := references(ctx, q, org, in.Project, in.Assignee)
		if err != nil {
			return err
		}
		row, err = q.EngineeringCreateIssue(ctx, db.EngineeringCreateIssueParams{OrganizationID: org, Title: strings.TrimSpace(in.Title), Description: in.Description, Status: in.Status, Priority: in.Priority, Labels: in.Labels, Cycle: in.Cycle, ProjectID: project, AssigneeUserID: assignee, CreatedBy: nullable(actor), Context: in.Context})
		if err != nil {
			return err
		}
		if conversation.ID > 0 {
			if _, err = q.EngineeringLink(ctx, db.EngineeringLinkParams{OrganizationID: org, IssueID: row.ID, ConversationID: conversation.ID, NotifyCustomer: true}); err != nil {
				return err
			}
		}
		if row.Status == "done" {
			if err = s.queueResolution(ctx, q, row, actor, resolutionGeneration(row), nil); err != nil {
				return err
			}
		}
		return emit(ctx, q, row, actor, "created")
	})
	if err != nil {
		return model.Issue{}, err
	}
	return s.dto(ctx, s.Store.Q, row)
}
func parseRef(value *string) (*uuid.UUID, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*value)
	if err != nil {
		return nil, invalid("reference must be a UUID or empty")
	}
	return &id, nil
}
func (s *Service) Patch(ctx context.Context, org, actor int64, id uuid.UUID, in model.PatchInput) (model.Issue, error) {
	var result db.EngineeringIssue
	err := s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		cur, err := q.EngineeringLockIssue(ctx, db.EngineeringLockIssueParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return missing(err)
		}
		if cur.Revision != in.Revision {
			return ErrConflict
		}
		dto, err := s.dto(ctx, q, cur)
		if err != nil {
			return err
		}
		if in.Title != nil {
			dto.Title = *in.Title
		}
		if in.Description != nil {
			dto.Description = *in.Description
		}
		if in.Status != nil {
			dto.Status = *in.Status
		}
		if in.Priority != nil {
			dto.Priority = *in.Priority
		}
		if in.Labels != nil {
			dto.Labels = *in.Labels
		}
		if in.Cycle != nil {
			dto.Cycle = *in.Cycle
		}
		if in.Project != nil {
			dto.Project, err = parseRef(in.Project)
			if err != nil {
				return err
			}
		}
		if in.Assignee != nil {
			dto.Assignee, err = parseRef(in.Assignee)
			if err != nil {
				return err
			}
		}
		if err = validate(model.CreateInput{Title: dto.Title, Description: dto.Description, Status: dto.Status, Priority: dto.Priority, Labels: dto.Labels, Cycle: dto.Cycle, Context: dto.Context}); err != nil {
			return err
		}
		project, assignee, err := references(ctx, q, org, dto.Project, dto.Assignee)
		if err != nil {
			return err
		}
		release := cur.ReleaseID
		if in.Release != nil {
			rid, e := parseRef(in.Release)
			if e != nil {
				return e
			}
			release = pgtype.Int8{}
			if rid != nil {
				r, e := q.EngineeringRelease(ctx, db.EngineeringReleaseParams{OrganizationID: org, Uuid: *rid})
				if e != nil {
					return missing(e)
				}
				release = nullable(r.ID)
			}
		}
		result, err = q.EngineeringUpdateIssue(ctx, db.EngineeringUpdateIssueParams{OrganizationID: org, ID: cur.ID, ExpectedRevision: cur.Revision, Title: dto.Title, Description: dto.Description, Status: dto.Status, Priority: dto.Priority, Labels: dto.Labels, Cycle: dto.Cycle, ProjectID: project, AssigneeUserID: assignee, ReleaseID: release})
		if err != nil {
			return missing(err)
		}
		if cur.Status != "done" && result.Status == "done" {
			if err = s.queueResolution(ctx, q, result, actor, resolutionGeneration(result), nil); err != nil {
				return err
			}
		}
		return emit(ctx, q, result, actor, "updated")
	})
	if err != nil {
		return model.Issue{}, err
	}
	return s.dto(ctx, s.Store.Q, result)
}
func (s *Service) Link(ctx context.Context, org, actor int64, id uuid.UUID, in model.LinkInput, remove bool) error {
	issue, err := s.Store.Q.EngineeringIssue(ctx, db.EngineeringIssueParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return missing(err)
	}
	if _, err = s.Inbox.Get(ctx, org, actor, in.Conversation); err != nil {
		return missing(err)
	}
	c, err := s.Store.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: in.Conversation})
	if err != nil {
		return missing(err)
	}
	return s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		issue, err = q.EngineeringLockIssue(ctx, db.EngineeringLockIssueParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return missing(err)
		}
		if remove {
			_, err = q.EngineeringUnlink(ctx, db.EngineeringUnlinkParams{OrganizationID: org, IssueID: issue.ID, ConversationID: c.ID})
		} else {
			_, err = q.EngineeringLink(ctx, db.EngineeringLinkParams{OrganizationID: org, IssueID: issue.ID, ConversationID: c.ID, NotifyCustomer: in.NotifyCustomer})
		}
		if err != nil {
			return err
		}
		if !remove && issue.Status == "done" {
			if err = s.queueResolution(ctx, q, issue, actor, resolutionGeneration(issue), nil); err != nil {
				return err
			}
		}
		return emit(ctx, q, issue, actor, "linked")
	})
}
func (s *Service) Links(ctx context.Context, org, actor int64, id uuid.UUID) ([]map[string]any, error) {
	issue, err := s.Store.Q.EngineeringIssue(ctx, db.EngineeringIssueParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, missing(err)
	}
	rows, err := s.Store.Q.EngineeringLinks(ctx, db.EngineeringLinksParams{OrganizationID: org, IssueID: issue.ID})
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		if _, err = s.Inbox.Get(ctx, org, actor, r.ConversationUuid); errors.Is(err, conv.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"conversation_uuid": r.ConversationUuid, "subject": r.Subject, "channel": r.ChannelType, "notify_customer": r.NotifyCustomer})
	}
	return out, nil
}

// Suggestions is a bounded lexical fallback. It spends no AI credits and never
// sends customer content to an external model without an organization policy.
func (s *Service) Suggestions(ctx context.Context, org, actor int64, conversation uuid.UUID) ([]model.Issue, error) {
	d, err := s.Inbox.Get(ctx, org, actor, conversation)
	if err != nil {
		return nil, missing(err)
	}
	words := strings.Fields(d.Subject)
	if len(words) == 0 {
		return []model.Issue{}, nil
	}
	term := words[0]
	for _, w := range words {
		if len(w) > len(term) {
			term = w
		}
	}
	result, err := s.List(ctx, org, apiquery.Query{Limit: 10, Q: term}, "", "")
	if err != nil {
		return nil, err
	}
	items := []model.Issue{}
	for _, i := range result.Items {
		if i.Status != "done" && i.Status != "canceled" {
			items = append(items, i)
		}
	}
	return items, nil
}
