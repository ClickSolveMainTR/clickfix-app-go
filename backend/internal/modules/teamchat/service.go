// Package teamchat ports the Laravel explicit-participant internal chat model.
package teamchat

import (
	"context"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	aiusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"sort"
	"strings"
)

type Service struct {
	Repo *repository.Repository
	AI   supportops.AI
}
type ThreadInput struct {
	Kind         string      `json:"kind"`
	Title        string      `json:"title"`
	Members      []uuid.UUID `json:"members"`
	Conversation *uuid.UUID  `json:"conversation_uuid"`
}

func (s *Service) Create(ctx context.Context, org, actor int64, in ThreadInput) (db.InternalThread, error) {
	var out db.InternalThread
	if (in.Kind != "direct" && in.Kind != "group" && in.Kind != "swarm") || len(in.Members) < 1 || len(in.Members) > 50 || len(in.Title) > 200 || in.Kind != "direct" && strings.TrimSpace(in.Title) == "" || in.Kind == "swarm" && in.Conversation == nil {
		return out, supportops.ErrInvalid
	}
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		users := map[int64]bool{actor: true}
		for _, id := range in.Members {
			u, err := r.Q.ScheduleMember(ctx, db.ScheduleMemberParams{OrganizationID: org, Uuid: id})
			if err != nil {
				return err
			}
			users[u] = true
		}
		if len(users) < 2 || in.Kind == "direct" && len(users) != 2 {
			return supportops.ErrInvalid
		}
		var conv pgtype.Int8
		if in.Conversation != nil {
			c, err := supportops.Conversation(ctx, r.Q, org, actor, *in.Conversation)
			if err != nil {
				return err
			}
			conv = repository.Int(&c.ID)
		}
		var key pgtype.Text
		if in.Kind == "direct" {
			ids := []int64{}
			for id := range users {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			key = pgtype.Text{String: fmt.Sprintf("%d:%d", ids[0], ids[1]), Valid: true}
			in.Title = ""
			if in.Conversation != nil {
				return supportops.ErrInvalid
			}
		}
		var err error
		out, err = r.Q.ChatCreateThread(ctx, db.ChatCreateThreadParams{OrganizationID: org, Kind: in.Kind, Title: in.Title, DirectKey: key, ConversationID: conv, CreatedBy: actor})
		if err != nil {
			return err
		}
		for id := range users {
			if conv.Valid {
				if _, err = r.Q.AddConversationParticipant(ctx, db.AddConversationParticipantParams{OrganizationID: org, ConversationID: conv.Int64, UserID: id, Role: "collaborator", AddedBy: repository.Int(&actor)}); err != nil {
					return err
				}
			}
			if err = r.Q.ChatAddMember(ctx, db.ChatAddMemberParams{OrganizationID: org, ThreadID: out.ID, UserID: id}); err != nil {
				return err
			}
		}
		return r.Emit(ctx, events.New("teamchat.changed").WithOrganization(org).WithActor(actor).WithEntity("internal_thread", &out.ID, &out.Uuid))
	})
	return out, err
}
func (s *Service) Thread(ctx context.Context, org, actor int64, id uuid.UUID) (db.InternalThread, error) {
	return thread(ctx, s.Repo, org, actor, id)
}
func thread(ctx context.Context, r *repository.Repository, org, actor int64, id uuid.UUID) (db.InternalThread, error) {
	t, err := r.Q.ChatGetThread(ctx, db.ChatGetThreadParams{OrganizationID: org, Uuid: id, UserID: actor})
	if err != nil {
		return t, err
	}
	if t.ConversationID.Valid {
		c, err := r.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: org, ID: t.ConversationID.Int64})
		if err != nil {
			return t, err
		}
		if _, err = supportops.Conversation(ctx, r.Q, org, actor, c.Uuid); err != nil {
			return t, err
		}
	}
	return t, nil
}

type PostInput struct {
	Body     string      `json:"body"`
	Client   uuid.UUID   `json:"client_id"`
	Mentions []uuid.UUID `json:"mentions"`
}

func (s *Service) Post(ctx context.Context, org, actor int64, id uuid.UUID, in PostInput, isAI bool) (db.InternalMessage, error) {
	var out db.InternalMessage
	if strings.TrimSpace(in.Body) == "" || len(in.Body) > 8000 || in.Client == uuid.Nil || len(in.Mentions) > 50 {
		return out, supportops.ErrInvalid
	}
	err := s.Repo.Within(ctx, func(r *repository.Repository) error {
		t, err := thread(ctx, r, org, actor, id)
		if err != nil {
			return err
		}
		if t.ClosedAt.Valid {
			return supportops.ErrConflict
		}
		members, err := r.Q.ChatThreadMembers(ctx, db.ChatThreadMembersParams{OrganizationID: org, ThreadID: t.ID})
		if err != nil {
			return err
		}
		allowed := map[uuid.UUID]bool{}
		for _, m := range members {
			allowed[m.Uuid] = true
		}
		for _, m := range in.Mentions {
			if !allowed[m] {
				return supportops.ErrInvalid
			}
		}
		out, err = r.Q.ChatPost(ctx, db.ChatPostParams{OrganizationID: org, ThreadID: t.ID, SenderID: actor, Body: in.Body, ClientID: in.Client, IsAi: isAI})
		if err != nil {
			return err
		}
		if out.SenderID != actor || out.Body != in.Body || out.IsAi != isAI {
			return supportops.ErrConflict
		}
		if err = r.Q.ChatTouch(ctx, db.ChatTouchParams{OrganizationID: org, ID: t.ID}); err != nil {
			return err
		}
		return r.Emit(ctx, events.New("teamchat.message_created").WithOrganization(org).WithActor(actor).WithEntity("internal_thread", &t.ID, &t.Uuid).WithIdempotencyKey(out.Uuid.String()).WithPayload(map[string]any{"message_uuid": out.Uuid, "mentions": in.Mentions}))
	})
	return out, err
}
func (s *Service) Ask(ctx context.Context, org, actor int64, id uuid.UUID, question string) (db.InternalMessage, error) {
	var out db.InternalMessage
	if s.AI == nil || strings.TrimSpace(question) == "" || len(question) > 4000 {
		return out, supportops.ErrInvalid
	}
	t, err := s.Thread(ctx, org, actor, id)
	if err != nil {
		return out, err
	}
	if t.ClosedAt.Valid {
		return out, supportops.ErrConflict
	}
	text := question
	if t.ConversationID.Valid {
		c, err := s.Repo.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: org, ID: t.ConversationID.Int64})
		if err != nil {
			return out, err
		}
		_, transcript, err := supportops.Transcript(ctx, s.Repo.Q, org, actor, c.Uuid)
		if err != nil {
			return out, err
		}
		text += "\nConversation:\n" + transcript
	}
	result, err := s.AI.RunAgent(ctx, org, actor, aimodel.RunInput{Task: "reply", Input: text, Retrieve: true, MaxTokens: 2048}, aiusecase.AgentOptions{Instructions: "Answer the internal team's question. Do not take actions. Cite retrieved knowledge when available."})
	if err != nil {
		return out, err
	}
	return s.Post(ctx, org, actor, id, PostInput{Body: result.Text, Client: result.UUID}, true)
}

// AddMember allows the owner to invite experts into a group or swarm.
func (s *Service) AddMember(ctx context.Context, org, actor int64, id, user uuid.UUID) error {
	return s.Repo.Within(ctx, func(r *repository.Repository) error {
		t, err := thread(ctx, r, org, actor, id)
		if err != nil {
			return err
		}
		if t.Kind == "direct" || t.CreatedBy != actor || t.ClosedAt.Valid {
			return supportops.ErrDenied
		}
		u, err := r.Q.ScheduleMember(ctx, db.ScheduleMemberParams{OrganizationID: org, Uuid: user})
		if err != nil {
			return err
		}
		members, err := r.Q.ChatThreadMembers(ctx, db.ChatThreadMembersParams{OrganizationID: org, ThreadID: t.ID})
		if err != nil {
			return err
		}
		if len(members) >= 50 {
			return supportops.ErrConflict
		}
		if t.ConversationID.Valid {
			if _, err = r.Q.AddConversationParticipant(ctx, db.AddConversationParticipantParams{OrganizationID: org, ConversationID: t.ConversationID.Int64, UserID: u, Role: "collaborator", AddedBy: repository.Int(&actor)}); err != nil {
				return err
			}
		}
		if err = r.Q.ChatAddMember(ctx, db.ChatAddMemberParams{OrganizationID: org, ThreadID: t.ID, UserID: u}); err != nil {
			return err
		}
		return r.Emit(ctx, events.New("teamchat.changed").WithOrganization(org).WithEntity("internal_thread", &t.ID, &t.Uuid))
	})
}
