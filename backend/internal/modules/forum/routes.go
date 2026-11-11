// Package forum registers the community forum alongside the public help center.
package forum

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"net/http"
	"strings"
)

func RegisterRoutes(rt supportops.Router, h *handler.Handler) {
	// Same-origin public API, rendered by help-center clients. No permissive CORS.
	base := "/v1/public/help/{organization_slug}/forum"
	rt.Mux.HandleFunc("GET "+base+"/sitemap", h.Public("sitemap"))
	rt.Mux.HandleFunc("GET "+base+"/categories", h.Public("categories"))
	rt.Mux.HandleFunc("GET "+base+"/topics", h.Public("topics"))
	rt.Mux.HandleFunc("GET "+base+"/topics/{topic_slug}", h.Public("topic"))
	rt.Mux.HandleFunc("POST "+base+"/topics", h.Public("topics"))
	for _, a := range []string{"replies", "votes", "subscribe"} {
		rt.Mux.HandleFunc("POST "+base+"/topics/{topic_slug}/"+a, h.Public(a))
	}
	rt.Mux.HandleFunc("GET "+base+"/notifications", h.Public("notifications"))
	rt.Mux.HandleFunc("POST "+base+"/notifications/{uuid}/read", h.Public("read"))
	tenant := "/v1/tenant/forum"
	rt.Tenant("POST "+tenant+"/categories", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		}](r)
		if err != nil {
			return nil, err
		}
		if !usecase.ValidSlug(in.Slug) || strings.TrimSpace(in.Name) == "" || len(in.Name) > 120 {
			return nil, supportops.ErrInvalid
		}
		return h.Service.Repo.Q.ForumCreateCategory(r.Context(), db.ForumCreateCategoryParams{OrganizationID: orgctx.Must(r.Context()).ID, Name: in.Name, Slug: in.Slug})
	})
	rt.Tenant("GET "+tenant+"/moderation", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := h.Service.Repo.Q.ForumModeration(r.Context(), db.ForumModerationParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := h.Service.Repo.Q.ForumModerationCount(r.Context(), org)
		return supportops.Page(rows, n, q), err
	})
	rt.Tenant("PATCH "+tenant+"/posts/{uuid}", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Status string `json:"status"`
		}](r)
		if err != nil {
			return nil, err
		}
		return h.Service.Moderate(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.Status)
	})
	rt.Tenant("GET "+tenant+"/removed", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		rows, err := h.Service.Repo.Q.ForumRemoved(r.Context(), db.ForumRemovedParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := h.Service.Repo.Q.ForumRemovedCount(r.Context(), org)
		return supportops.Page(rows, n, q), err
	})
	rt.Tenant("POST "+tenant+"/posts/{uuid}/unpublish", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		// The reason is optional, and so is the body itself.
		in, err := supportops.ReadOptional[struct {
			Reason string `json:"reason"`
		}](r)
		if err != nil {
			return nil, err
		}
		return h.Service.Unpublish(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.Reason)
	})
	rt.Tenant("POST "+tenant+"/posts/{uuid}/republish", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		return h.Service.Republish(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r))
	})
	rt.Tenant("POST "+tenant+"/posts/{uuid}/assist", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		return h.Service.Assist(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r))
	})
	rt.Tenant("POST "+tenant+"/topics/{uuid}/accept", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Reply uuid.UUID `json:"reply_uuid"`
		}](r)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"accepted": true}, h.Service.Accept(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.Reply)
	})
	rt.Tenant("POST "+tenant+"/from-conversation", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[model.ConversionInput](r)
		if err != nil {
			return nil, err
		}
		return h.Service.FromConversation(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in)
	})
	rt.Tenant("POST "+tenant+"/topics/{uuid}/conversation", "tenant.conversations.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[model.ConversionInput](r)
		if err != nil {
			return nil, err
		}
		return h.Service.ToConversation(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.Inbox)
	})
}
