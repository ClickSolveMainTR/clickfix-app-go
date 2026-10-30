// Package handler provides cacheable public reads and verified visitor mutations.
package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forum/usecase"
	helphandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/helpcenter/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	widgetuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Limiter interface {
	Allow(context.Context, string, string, int, time.Duration) (bool, time.Duration)
}
type Handler struct {
	Service *usecase.Service
	Widget  *widgetuc.Service
	Limiter Limiter
}

func (h *Handler) Public(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 180
		if r.Method != "GET" || action == "notifications" {
			limit = 20
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=30")
		}
		if h.Limiter != nil {
			ok, wait := h.Limiter.Allow(r.Context(), "forum_"+action, r.PathValue("organization_slug")+":"+helphandler.ClientIP(r), limit, time.Minute)
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
				response.TooManyRequests(w, r, "Please slow down")
				return
			}
		}
		org, err := h.Service.Repo.Q.HelpOrganization(r.Context(), r.PathValue("organization_slug"))
		if err != nil {
			w.Header().Set("Cache-Control", "no-store")
			supportops.Fail(w, r, err)
			return
		}
		var out any
		if r.Method == "GET" && action != "notifications" {
			out, err = h.read(r, org.ID, action)
		} else {
			var contact int64
			contact, err = h.contact(r, org.ID)
			if err == nil {
				out, err = h.write(r, org.ID, contact, action)
			}
		}
		if err != nil {
			w.Header().Set("Cache-Control", "no-store")
			supportops.Fail(w, r, err)
			return
		}
		response.JSON(w, r, 200, out)
	}
}
func (h *Handler) contact(r *http.Request, org int64) (int64, error) {
	if h.Widget == nil {
		return 0, supportops.ErrDenied
	}
	id, err := uuid.Parse(r.Header.Get("X-Widget-ID"))
	if err != nil {
		return 0, supportops.ErrDenied
	}
	wd, err := h.Widget.Open(r.Context(), id, r.Header.Get("Origin"))
	if err != nil || wd.Instance.OrganizationID != org {
		return 0, supportops.ErrDenied
	}
	claims, err := h.Widget.Authenticate(wd, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if err != nil || !claims.Verified() {
		return 0, supportops.ErrDenied
	}
	return h.Service.Repo.Q.FeedbackContact(r.Context(), db.FeedbackContactParams{OrganizationID: org, ChannelInstanceID: pgtype.Int8{Int64: wd.Instance.ID, Valid: true}, ExternalID: claims.ExternalID()})
}
func (h *Handler) read(r *http.Request, org int64, action string) (any, error) {
	q, err := supportops.List(r)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	switch action {
	case "sitemap":
		rows, err := h.Service.Repo.Q.ForumSitemap(ctx, db.ForumSitemapParams{OrganizationID: org, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := h.Service.Repo.Q.ForumTopicCount(ctx, db.ForumTopicCountParams{OrganizationID: org})
		return apiquery.NewPage(rows, n, q.Limit, q.Offset), err
	case "categories":
		return h.Service.Repo.Q.ForumCategories(ctx, org)
	case "topics":
		rows, err := h.Service.Repo.Q.ForumTopics(ctx, db.ForumTopicsParams{OrganizationID: org, Category: r.URL.Query().Get("category"), Q: q.Q, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := h.Service.Repo.Q.ForumTopicCount(ctx, db.ForumTopicCountParams{OrganizationID: org, Category: r.URL.Query().Get("category"), Q: q.Q})
		return apiquery.NewPage(rows, n, q.Limit, q.Offset), err
	default:
		t, err := h.Service.Repo.Q.ForumTopicBySlug(ctx, db.ForumTopicBySlugParams{OrganizationID: org, Slug: r.PathValue("topic_slug")})
		if err != nil {
			return nil, err
		}
		rows, err := h.Service.Repo.Q.ForumReplies(ctx, db.ForumRepliesParams{OrganizationID: org, TopicID: pgtype.Int8{Int64: t.ID, Valid: true}, Limit: q.Limit, Offset: q.Offset})
		if err != nil {
			return nil, err
		}
		n, err := h.Service.Repo.Q.ForumReplyCount(ctx, db.ForumReplyCountParams{OrganizationID: org, TopicID: pgtype.Int8{Int64: t.ID, Valid: true}})
		if err != nil {
			return nil, err
		}
		var accepted any
		if t.AcceptedReplyID.Valid {
			a, err := h.Service.Repo.Q.ForumPublicAnswer(ctx, db.ForumPublicAnswerParams{OrganizationID: org, ID: t.AcceptedReplyID.Int64})
			if err == nil {
				accepted = a
			} else if err != pgx.ErrNoRows {
				return nil, err
			}
		}
		return map[string]any{"uuid": t.Uuid, "slug": t.Slug, "title": t.Title, "body": t.Body, "created_at": t.CreatedAt, "updated_at": t.UpdatedAt, "accepted_answer": accepted, "replies": apiquery.NewPage(rows, n, q.Limit, q.Offset)}, nil
	}
}
func (h *Handler) write(r *http.Request, org, contact int64, action string) (any, error) {
	ctx := r.Context()
	q, err := supportops.List(r)
	if err != nil {
		return nil, err
	}
	if action == "notifications" {
		rows, err := h.Service.Repo.Q.ForumNotifications(ctx, db.ForumNotificationsParams{OrganizationID: org, ContactID: contact, Limit: q.Limit, Offset: q.Offset})
		return map[string]any{"items": rows, "limit": q.Limit, "offset": q.Offset}, err
	}
	if action == "read" {
		n, err := h.Service.Repo.Q.ForumReadNotification(ctx, db.ForumReadNotificationParams{OrganizationID: org, ContactID: contact, Uuid: supportops.ID(r)})
		if err == nil && n == 0 {
			err = pgx.ErrNoRows
		}
		return map[string]bool{"read": true}, err
	}
	if action == "topics" {
		in, err := supportops.Read[model.PostInput](r)
		if err != nil {
			return nil, err
		}
		p, err := h.Service.Create(ctx, org, contact, nil, in)
		return submitted(p), err
	}
	t, err := h.Service.Repo.Q.ForumTopicBySlug(ctx, db.ForumTopicBySlugParams{OrganizationID: org, Slug: r.PathValue("topic_slug")})
	if err != nil {
		return nil, err
	}
	if action == "replies" {
		in, err := supportops.Read[model.PostInput](r)
		if err != nil {
			return nil, err
		}
		p, err := h.Service.Create(ctx, org, contact, &t, in)
		return submitted(p), err
	}
	in, err := supportops.Read[struct {
		Remove bool      `json:"remove"`
		Reply  uuid.UUID `json:"reply_uuid"`
	}](r)
	if err != nil {
		return nil, err
	}
	if action == "subscribe" {
		if in.Remove {
			err = h.Service.Repo.Q.ForumUnsubscribe(ctx, db.ForumUnsubscribeParams{OrganizationID: org, TopicID: t.ID, ContactID: contact})
		} else {
			err = h.Service.Repo.Q.ForumSubscribe(ctx, db.ForumSubscribeParams{OrganizationID: org, TopicID: t.ID, ContactID: contact})
		}
	} else {
		post := t
		if in.Reply != uuid.Nil {
			post, err = h.Service.Repo.Q.ForumPost(ctx, db.ForumPostParams{OrganizationID: org, Uuid: in.Reply})
			if err != nil {
				return nil, err
			}
			if !post.TopicID.Valid || post.TopicID.Int64 != t.ID || post.Status != "published" {
				return nil, pgx.ErrNoRows
			}
		}
		if in.Remove {
			err = h.Service.Repo.Q.ForumUnvote(ctx, db.ForumUnvoteParams{OrganizationID: org, PostID: post.ID, ContactID: contact})
		} else {
			err = h.Service.Repo.Q.ForumVote(ctx, db.ForumVoteParams{OrganizationID: org, PostID: post.ID, ContactID: contact})
		}
	}
	return map[string]bool{"active": !in.Remove}, err
}
func submitted(p db.ForumPost) any {
	return map[string]any{"uuid": p.Uuid, "slug": p.Slug, "status": p.Status}
}
