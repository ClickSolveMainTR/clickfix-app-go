package teamchat

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"net/http"
)

func RegisterRoutes(rt supportops.Router, s *Service) {
	rt.Tenant("GET /v1/tenant/team-chat/meta", "tenant.team_chat.use", func(r *http.Request) (any, error) { return supportops.Meta("tenant.team_chat"), nil })

	rt.Tenant("GET /v1/tenant/team-chat", "tenant.team_chat.use", func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org, actor := orgctx.Must(r.Context()).ID, supportops.Actor(r.Context())
		rows, err := s.Repo.Q.ChatListThreads(r.Context(), db.ChatListThreadsParams{OrganizationID: org, UserID: actor, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			return nil, err
		}
		total, err := s.Repo.Q.ChatCountThreads(r.Context(), db.ChatCountThreadsParams{OrganizationID: org, UserID: actor})
		return supportops.Page(rows, total, q), err
	})
	rt.Tenant("POST /v1/tenant/team-chat", "tenant.team_chat.use", func(r *http.Request) (any, error) {
		in, err := supportops.Read[ThreadInput](r)
		if err != nil {
			return nil, err
		}
		return s.Create(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in)
	})
	rt.Tenant("GET /v1/tenant/team-chat/{uuid}/messages", "tenant.team_chat.use", func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		t, err := s.Thread(r.Context(), org, supportops.Actor(r.Context()), supportops.ID(r))
		if err != nil {
			return nil, err
		}
		return s.Repo.Q.ChatMessages(r.Context(), db.ChatMessagesParams{OrganizationID: org, ThreadID: t.ID, Limit: q.Limit, Offset: q.Offset})
	})
	rt.Tenant("POST /v1/tenant/team-chat/{uuid}/messages", "tenant.team_chat.use", func(r *http.Request) (any, error) {
		in, err := supportops.Read[PostInput](r)
		if err != nil {
			return nil, err
		}
		return s.Post(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in, false)
	})
	rt.Tenant("POST /v1/tenant/team-chat/{uuid}/ask-ai", "tenant.team_chat.use", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Question string `json:"question"`
		}](r)
		if err != nil {
			return nil, err
		}
		return s.Ask(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.Question)
	})
	rt.Tenant("POST /v1/tenant/team-chat/{uuid}/members", "tenant.team_chat.use", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			User uuid.UUID `json:"user_uuid"`
		}](r)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"added": true}, s.AddMember(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.User)
	})
	rt.Tenant("POST /v1/tenant/team-chat/{uuid}/read", "tenant.team_chat.use", func(r *http.Request) (any, error) {
		org := orgctx.Must(r.Context()).ID
		actor := supportops.Actor(r.Context())
		t, err := s.Thread(r.Context(), org, actor, supportops.ID(r))
		if err != nil {
			return nil, err
		}
		return map[string]bool{"read": true}, s.Repo.Q.ChatRead(r.Context(), db.ChatReadParams{OrganizationID: org, ThreadID: t.ID, UserID: actor})
	})
	rt.Tenant("POST /v1/tenant/team-chat/{uuid}/close", "tenant.team_chat.use", func(r *http.Request) (any, error) {
		org := orgctx.Must(r.Context()).ID
		actor := supportops.Actor(r.Context())
		t, err := s.Thread(r.Context(), org, actor, supportops.ID(r))
		if err != nil {
			return nil, err
		}
		if t.CreatedBy != actor {
			return nil, supportops.ErrDenied
		}
		return map[string]bool{"closed": true}, s.Repo.Q.ChatClose(r.Context(), db.ChatCloseParams{OrganizationID: org, ID: t.ID})
	})
}
