package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"time"
	"unicode/utf8"
)

type Sender interface {
	Post(context.Context, conv.PostInput) (conv.Posted, error)
}
type Service struct {
	repo            *repository.Repository
	sender          Sender
	secret, baseURL string
}

func New(r *repository.Repository, s Sender, secret, baseURL string) *Service {
	return &Service{r, s, secret, strings.TrimRight(baseURL, "/")}
}
func (s *Service) Templates(ctx context.Context, org int64) (any, error) {
	return s.repo.Q.SurveyTemplates(ctx, org)
}
func (s *Service) SaveTemplate(ctx context.Context, org int64, id uuid.UUID, in model.TemplateInput) (any, error) {
	if utf8.RuneCountInString(strings.TrimSpace(in.Name)) < 1 || utf8.RuneCountInString(in.Name) > 120 || utf8.RuneCountInString(strings.TrimSpace(in.Question)) < 1 || utf8.RuneCountInString(in.Question) > 1000 {
		return nil, model.ErrInvalid
	}
	if id != uuid.Nil {
		return s.repo.Q.SurveyUpdateTemplate(ctx, db.SurveyUpdateTemplateParams{OrganizationID: org, Uuid: id, Name: in.Name, Question: in.Question, OnResolve: in.OnResolve, Enabled: in.Enabled})
	}
	if in.Kind != "csat" && in.Kind != "nps" && in.Kind != "ces" {
		return nil, model.ErrInvalid
	}
	return s.repo.Q.SurveyCreateTemplate(ctx, db.SurveyCreateTemplateParams{OrganizationID: org, Name: in.Name, Kind: in.Kind, Question: in.Question, OnResolve: in.OnResolve, Enabled: in.Enabled})
}
func (s *Service) Invite(ctx context.Context, org int64, in model.InviteInput) (uuid.UUID, error) {
	if in.Template == uuid.Nil || in.Conversation == uuid.Nil || len(in.PeriodKey) > 64 {
		return uuid.Nil, model.ErrInvalid
	}
	var row db.SurveyInvitation
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		var err error
		row, err = q.SurveyInvite(ctx, db.SurveyInviteParams{OrganizationID: org, TemplateUuid: in.Template, ConversationUuid: in.Conversation, PeriodKey: in.PeriodKey})
		if err != nil {
			return err
		}
		return outbox.EnqueueQueries(ctx, q, events.New("surveys.invited").WithOrganization(org).WithEntity("conversation", &row.ConversationID, &in.Conversation).WithPayload(map[string]any{"invitation_uuid": row.Uuid.String()}).WithIdempotencyKey(row.Uuid.String()))
	})
	return row.Uuid, err
}
func (s *Service) Resolve(ctx context.Context, org, id int64) error {
	c, err := s.repo.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: org, ID: id})
	if err != nil {
		return err
	}
	if c.Status != "resolved" && c.Status != "closed" {
		return nil
	}
	templates, err := s.repo.Q.SurveyResolveTemplates(ctx, org)
	if err != nil {
		return err
	}
	for _, t := range templates {
		if _, err = s.Invite(ctx, org, model.InviteInput{Template: t, Conversation: c.Uuid}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Deliver(ctx context.Context, org int64, id uuid.UUID) error {
	var i db.SurveyInvitation
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		var err error
		i, err = q.SurveyInvitation(ctx, db.SurveyInvitationParams{OrganizationID: org, Uuid: id})
		return err
	})
	if err != nil {
		return err
	}
	if i.AnsweredAt.Valid || !i.ExpiresAt.Time.After(time.Now()) {
		return nil
	}
	c, err := s.repo.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: org, ID: i.ConversationID})
	if err != nil {
		return err
	}
	if i.Kind == "csat" {
		if c.Status != "resolved" && c.Status != "closed" {
			return nil
		}
		if _, err := s.repo.Q.GetConversationRating(ctx, db.GetConversationRatingParams{OrganizationID: org, ConversationID: c.ID}); err == nil {
			return nil
		} else if !isNoRows(err) {
			return err
		}
	}
	if s.sender == nil {
		return model.ErrInvalid
	}
	_, err = s.sender.Post(ctx, conv.PostInput{OrganizationID: org, ConversationUUID: c.Uuid, Content: i.Question + "\n" + s.baseURL + "/survey/" + s.token(org, i.Uuid, i.ExpiresAt.Time), ClientID: uuid.NewSHA1(i.Uuid, []byte("survey-delivery"))})
	return err
}
func (s *Service) Public(ctx context.Context, token string) (any, error) {
	c, err := s.verify(token)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		i, err := q.SurveyInvitation(ctx, db.SurveyInvitationParams{OrganizationID: c.Org, Uuid: c.ID})
		if err != nil {
			return err
		}
		if !i.ExpiresAt.Time.After(time.Now()) {
			return model.ErrExpired
		}
		out = map[string]any{"kind": i.Kind, "question": i.Question, "answered": i.AnsweredAt.Valid, "expires_at": i.ExpiresAt.Time}
		if i.Kind == "csat" {
			rating, err := q.GetConversationRating(ctx, db.GetConversationRatingParams{OrganizationID: c.Org, ConversationID: i.ConversationID})
			if err == nil {
				out["answered"] = true
				out["score"] = rating.Score
			} else if !isNoRows(err) {
				return err
			}
		}
		return nil
	})
	return out, err
}
func (s *Service) Answer(ctx context.Context, token string, in model.Answer) (model.Answer, error) {
	c, err := s.verify(token)
	if err != nil {
		return model.Answer{}, err
	}
	if utf8.RuneCountInString(in.Comment) > 2000 {
		return model.Answer{}, model.ErrInvalid
	}
	var out model.Answer
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		i, err := q.SurveyInvitation(ctx, db.SurveyInvitationParams{OrganizationID: c.Org, Uuid: c.ID})
		if err != nil {
			return err
		}
		if !i.ExpiresAt.Time.After(time.Now()) {
			return model.ErrExpired
		}
		if (i.Kind == "csat" && (in.Score < 1 || in.Score > 5)) || (i.Kind == "nps" && (in.Score < 0 || in.Score > 10)) || (i.Kind == "ces" && (in.Score < 1 || in.Score > 7)) {
			return model.ErrInvalid
		}
		convRow, err := q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: c.Org, ID: i.ConversationID})
		if err != nil {
			return err
		}
		if i.Kind == "csat" {
			if convRow.Status != "resolved" && convRow.Status != "closed" {
				return model.ErrInvalid
			}
			_, err = q.InsertConversationRating(ctx, db.InsertConversationRatingParams{OrganizationID: c.Org, ConversationID: i.ConversationID, ContactID: pgtype.Int8{Int64: convRow.ContactID, Valid: true}, ChannelInstanceID: convRow.ChannelInstanceID, Score: in.Score, Comment: in.Comment})
			if err != nil && !isNoRows(err) {
				return err
			}
			rating, err := q.GetConversationRating(ctx, db.GetConversationRatingParams{OrganizationID: c.Org, ConversationID: i.ConversationID})
			if err != nil {
				return err
			}
			out = model.Answer{Score: rating.Score, Comment: rating.Comment}
		} else {
			a, err := q.SurveyAnswer(ctx, db.SurveyAnswerParams{OrganizationID: c.Org, InvitationID: i.ID, Kind: i.Kind, Score: in.Score, Comment: in.Comment})
			if err != nil {
				return err
			}
			out = model.Answer{Score: a.Score, Comment: a.Comment}
		}
		if err = q.SurveyMarkAnswered(ctx, db.SurveyMarkAnsweredParams{OrganizationID: c.Org, ID: i.ID}); err != nil {
			return err
		}
		return outbox.EnqueueQueries(ctx, q, events.New("surveys.responded").WithOrganization(c.Org).WithEntity("conversation", &convRow.ID, &convRow.Uuid).WithPayload(map[string]any{"kind": i.Kind, "score": out.Score}).WithIdempotencyKey(i.Uuid.String()))
	})
	return out, err
}
func (s *Service) Results(ctx context.Context, org int64, from, to time.Time) (json.RawMessage, error) {
	if from.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return nil, model.ErrInvalid
	}
	raw, err := s.repo.Q.SurveyResults(ctx, db.SurveyResultsParams{OrganizationID: org, FromAt: pgtype.Timestamptz{Time: from, Valid: true}, ToAt: pgtype.Timestamptz{Time: to, Valid: true}})
	return json.RawMessage(raw), err
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func (s *Service) WithSender(sender Sender) *Service { s.sender = sender; return s }
