// Package feedback implements customer requests and published roadmap/changelog content.
package feedback

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	aiusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Service struct {
	Repo          *repository.Repository
	AI            supportops.AI
	PublicBaseURL string
}
type RequestInput struct {
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	Public       bool       `json:"is_public"`
	Conversation *uuid.UUID `json:"conversation_uuid"`
}

func (s *Service) Create(ctx context.Context, org, actor int64, in RequestInput) (db.FeatureRequest, error) {
	var out db.FeatureRequest
	in.Title = strings.TrimSpace(in.Title)
	if len(in.Title) == 0 || len(in.Title) > 200 || len(in.Body) > 16000 {
		return out, supportops.ErrInvalid
	}
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		var conv db.Conversation
		var err error
		if in.Conversation != nil {
			conv, err = supportops.Conversation(ctx, r.Q, org, actor, *in.Conversation)
			if err != nil {
				return err
			}
		}
		out, err = r.Q.FeedbackCreate(ctx, db.FeedbackCreateParams{OrganizationID: org, Title: in.Title, Body: in.Body, IsPublic: in.Public, CreatedBy: repository.Int(&actor)})
		if err != nil {
			return err
		}
		if in.Conversation != nil {
			if err = r.Q.FeedbackSource(ctx, db.FeedbackSourceParams{OrganizationID: org, RequestID: out.ID, ConversationID: conv.ID}); err != nil {
				return err
			}
			if err = r.Q.FeedbackVote(ctx, db.FeedbackVoteParams{OrganizationID: org, RequestID: out.ID, ContactID: conv.ContactID}); err != nil {
				return err
			}
		}
		return r.Emit(ctx, events.New("feedback.created").WithOrganization(org).WithActor(actor).WithEntity("feature_request", &out.ID, &out.Uuid))
	})
	return out, err
}
func (s *Service) Link(ctx context.Context, org, actor int64, id, conversation uuid.UUID) error {
	return s.Repo.Within(ctx, func(r *repository.Repository) error {
		f, err := r.Q.FeedbackLock(ctx, db.FeedbackLockParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if f.Status == "merged" {
			return supportops.ErrConflict
		}
		c, err := supportops.Conversation(ctx, r.Q, org, actor, conversation)
		if err != nil {
			return err
		}
		if err = r.Q.FeedbackSource(ctx, db.FeedbackSourceParams{OrganizationID: org, RequestID: f.ID, ConversationID: c.ID}); err != nil {
			return err
		}
		return r.Q.FeedbackVote(ctx, db.FeedbackVoteParams{OrganizationID: org, RequestID: f.ID, ContactID: c.ContactID})
	})
}
func (s *Service) Status(ctx context.Context, org, actor int64, id uuid.UUID, status string, public bool) (db.FeatureRequest, error) {
	var out db.FeatureRequest
	if status != "reviewing" && status != "planned" && status != "in_progress" && status != "released" {
		return out, supportops.ErrInvalid
	}
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		f, err := r.Q.FeedbackLock(ctx, db.FeedbackLockParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		if f.Status == "merged" {
			return supportops.ErrConflict
		}
		out, err = r.Q.FeedbackStatus(ctx, db.FeedbackStatusParams{OrganizationID: org, ID: f.ID, Status: status, IsPublic: public})
		if err != nil {
			return err
		}
		if status == f.Status {
			return nil
		}
		return r.Emit(ctx, events.New("feedback.status_changed").WithOrganization(org).WithActor(actor).WithEntity("feature_request", &out.ID, &out.Uuid).WithPayload(map[string]any{"title": out.Title, "status": status, "public": public}))
	})
	return out, err
}

// Merge locks in id order, copies relationships with unique-key deduplication, then retires the source.
func (s *Service) Merge(ctx context.Context, org, actor int64, source, target uuid.UUID) error {
	if source == target {
		return supportops.ErrInvalid
	}
	return s.Repo.Within(ctx, func(r *repository.Repository) error {
		a, b := source, target
		if strings.Compare(a.String(), b.String()) > 0 {
			a, b = b, a
		}
		x, err := r.Q.FeedbackLock(ctx, db.FeedbackLockParams{OrganizationID: org, Uuid: a})
		if err != nil {
			return err
		}
		y, err := r.Q.FeedbackLock(ctx, db.FeedbackLockParams{OrganizationID: org, Uuid: b})
		if err != nil {
			return err
		}
		f, t := x, y
		if f.Uuid != source {
			f, t = t, f
		}
		if f.Status == "merged" || t.Status == "merged" {
			return supportops.ErrConflict
		}
		if err = r.Q.FeedbackMergeVotes(ctx, db.FeedbackMergeVotesParams{OrganizationID: org, RequestID: f.ID, RequestID_2: t.ID}); err != nil {
			return err
		}
		if err = r.Q.FeedbackMergeSources(ctx, db.FeedbackMergeSourcesParams{OrganizationID: org, RequestID: f.ID, RequestID_2: t.ID}); err != nil {
			return err
		}
		if err = r.Q.FeedbackMergeComments(ctx, db.FeedbackMergeCommentsParams{OrganizationID: org, RequestID: f.ID, RequestID_2: t.ID}); err != nil {
			return err
		}
		if err = r.Q.FeedbackDeleteVotes(ctx, db.FeedbackDeleteVotesParams{OrganizationID: org, RequestID: f.ID}); err != nil {
			return err
		}
		if err = r.Q.FeedbackDeleteSources(ctx, db.FeedbackDeleteSourcesParams{OrganizationID: org, RequestID: f.ID}); err != nil {
			return err
		}
		if err = r.Q.FeedbackMerge(ctx, db.FeedbackMergeParams{OrganizationID: org, ID: f.ID, MergedIntoID: repository.Int(&t.ID)}); err != nil {
			return err
		}
		return r.Emit(ctx, events.New("feedback.merged").WithOrganization(org).WithActor(actor).WithEntity("feature_request", &t.ID, &t.Uuid))
	})
}

type Suggestion struct {
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Existing  *uuid.UUID `json:"existing_uuid"`
	IsRequest bool       `json:"is_request"`
}

// suggestionSchema makes the provider return the suggestion itself (no prose).
var suggestionSchema = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"is_request", "title", "body", "existing_uuid"}, "properties": map[string]any{
	"is_request": map[string]any{"type": "boolean"}, "title": map[string]any{"type": "string", "maxLength": 200}, "body": map[string]any{"type": "string", "maxLength": 16000},
	"existing_uuid": map[string]any{"anyOf": []any{map[string]any{"type": "string", "format": "uuid"}, map[string]any{"type": "null"}}},
}}

func (s *Service) Extract(ctx context.Context, org, actor int64, conversation uuid.UUID) (Suggestion, error) {
	var out Suggestion
	if s.AI == nil {
		return out, supportops.ErrConflict
	}
	_, text, err := supportops.Transcript(ctx, s.Repo.Q, org, actor, conversation)
	if err != nil {
		return out, err
	}
	existing, err := s.Repo.Q.FeedbackList(ctx, db.FeedbackListParams{OrganizationID: org, RowLimit: 20})
	if err != nil {
		return out, err
	}
	candidates := []map[string]any{}
	for _, e := range existing {
		candidates = append(candidates, map[string]any{"uuid": e.Uuid, "title": e.Title})
	}
	b, _ := json.Marshal(candidates)
	result, err := s.AI.RunAgent(ctx, org, actor, aimodel.RunInput{Task: "triage", Input: text + "\nExisting requests:\n" + string(b), MaxTokens: 1024}, aiusecase.AgentOptions{Instructions: "Identify a product feature request. Return JSON {is_request:boolean,title:string,body:string,existing_uuid:UUID|null}. Suggest only; do not perform actions. Do not include customer personal data in title or body.", Schema: suggestionSchema})
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(result.Text), &out) != nil || out.IsRequest && strings.TrimSpace(out.Title) == "" || len(out.Title) > 200 || len(out.Body) > 16000 {
		return out, supportops.ErrInvalid
	}
	if out.Existing != nil {
		f, err := s.Repo.Q.FeedbackGet(ctx, db.FeedbackGetParams{OrganizationID: org, Uuid: *out.Existing})
		if err != nil || f.Status == "merged" {
			return Suggestion{}, supportops.ErrInvalid
		}
	}
	return out, nil
}

type ChangelogInput struct {
	Title   string     `json:"title"`
	Body    string     `json:"body"`
	Tags    []string   `json:"tags"`
	Request *uuid.UUID `json:"request_uuid"`
}

func (s *Service) Changelog(ctx context.Context, org int64, in ChangelogInput) (db.ChangelogEntry, error) {
	var request pgtype.Int8
	if strings.TrimSpace(in.Title) == "" || len(in.Title) > 200 || len(in.Body) == 0 || len(in.Body) > 32000 || len(in.Tags) > 20 {
		return db.ChangelogEntry{}, supportops.ErrInvalid
	}
	for _, tag := range in.Tags {
		if len(tag) > 60 {
			return db.ChangelogEntry{}, supportops.ErrInvalid
		}
	}
	if in.Request != nil {
		f, err := s.Repo.Q.FeedbackGet(ctx, db.FeedbackGetParams{OrganizationID: org, Uuid: *in.Request})
		if err != nil {
			return db.ChangelogEntry{}, err
		}
		request = repository.Int(&f.ID)
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	return s.Repo.Q.ChangelogCreate(ctx, db.ChangelogCreateParams{OrganizationID: org, Title: in.Title, Body: in.Body, Tags: in.Tags, RequestID: request})
}
func (s *Service) Publish(ctx context.Context, org, actor int64, id uuid.UUID) (db.ChangelogEntry, error) {
	var out db.ChangelogEntry
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		var err error
		out, err = r.Q.ChangelogPublish(ctx, db.ChangelogPublishParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return err
		}
		return r.Emit(ctx, events.New("feedback.changelog_published").WithOrganization(org).WithActor(actor).WithEntity("changelog", &out.ID, &out.Uuid).WithPayload(map[string]any{"request_id": out.RequestID.Int64, "title": out.Title}))
	})
	return out, err
}

// RoadmapInput is editorial publication of an engineering issue. It never pulls private provider content automatically.
type RoadmapInput struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Status    string `json:"status"`
	SourceURL string `json:"source_issue_url"`
	Public    bool   `json:"is_public"`
}

func (s *Service) Roadmap(ctx context.Context, org int64, id uuid.UUID, in RoadmapInput) (db.RoadmapEntry, error) {
	u, err := url.Parse(in.SourceURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(in.SourceURL) > 2000 || strings.TrimSpace(in.Title) == "" || len(in.Title) > 200 || len(in.Body) > 16000 || in.Status != "planned" && in.Status != "in_progress" && in.Status != "released" {
		return db.RoadmapEntry{}, supportops.ErrInvalid
	}
	if id == uuid.Nil {
		return s.Repo.Q.RoadmapCreate(ctx, db.RoadmapCreateParams{OrganizationID: org, Title: in.Title, Body: in.Body, Status: in.Status, SourceIssueUrl: in.SourceURL, IsPublic: in.Public})
	}
	return s.Repo.Q.RoadmapUpdate(ctx, db.RoadmapUpdateParams{OrganizationID: org, Uuid: id, Title: in.Title, Body: in.Body, Status: in.Status, SourceIssueUrl: in.SourceURL, IsPublic: in.Public})
}
