package feedback

import (
	"encoding/xml"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	widgethandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/handler"
	widgetusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"strings"
	"time"
)

func RegisterRoutes(rt supportops.Router, s *Service, widget *widgetusecase.Service) {
	publicHandle := func(pattern string, h http.HandlerFunc) { rt.Mux.Handle(pattern, widgethandler.CORS(h)) }
	for _, prefix := range []string{"feedback", "roadmap", "changelog"} {
		publicHandle("OPTIONS /v1/public/"+prefix+"/{organization}/{rest...}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	}

	rt.Tenant("GET /v1/tenant/feedback/meta", "tenant.feedback.read", func(r *http.Request) (any, error) { return supportops.Meta("tenant.feedback"), nil })

	rt.Tenant("GET /v1/tenant/changelog/meta", "tenant.feedback.read", func(r *http.Request) (any, error) { return supportops.Meta("tenant.changelog"), nil })

	rt.Tenant("GET /v1/tenant/roadmap/meta", "tenant.feedback.read", func(r *http.Request) (any, error) { return supportops.Meta("tenant.roadmap"), nil })
	rt.Tenant("GET /v1/tenant/roadmap", "tenant.feedback.read", func(r *http.Request) (any, error) { return roadmap(r, s, orgctx.Must(r.Context()).ID, false) })
	rt.Tenant("POST /v1/tenant/roadmap", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[RoadmapInput](r)
		if err != nil {
			return nil, err
		}
		return s.Roadmap(r.Context(), orgctx.Must(r.Context()).ID, uuid.Nil, in)
	})
	rt.Tenant("PUT /v1/tenant/roadmap/{uuid}", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[RoadmapInput](r)
		if err != nil {
			return nil, err
		}
		if supportops.ID(r) == uuid.Nil {
			return nil, supportops.ErrInvalid
		}
		return s.Roadmap(r.Context(), orgctx.Must(r.Context()).ID, supportops.ID(r), in)
	})
	rt.Tenant("GET /v1/tenant/feedback/settings", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		enabled, err := s.Repo.Q.FeedbackSettings(r.Context(), orgctx.Must(r.Context()).ID)
		return map[string]bool{"public_enabled": enabled}, err
	})
	rt.Tenant("PUT /v1/tenant/feedback/settings", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Enabled bool `json:"public_enabled"`
		}](r)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"public_enabled": in.Enabled}, s.Repo.Q.FeedbackSaveSettings(r.Context(), db.FeedbackSaveSettingsParams{OrganizationID: orgctx.Must(r.Context()).ID, PublicEnabled: in.Enabled})
	})
	rt.Tenant("GET /v1/tenant/feedback", "tenant.feedback.read", func(r *http.Request) (any, error) { return list(r, s, orgctx.Must(r.Context()).ID, false) })
	rt.Tenant("POST /v1/tenant/feedback", "tenant.feedback.write", func(r *http.Request) (any, error) {
		in, err := supportops.Read[RequestInput](r)
		if err != nil {
			return nil, err
		}
		return s.Create(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in)
	})
	rt.Tenant("POST /v1/tenant/feedback/extract", "tenant.feedback.write", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Conversation uuid.UUID `json:"conversation_uuid"`
		}](r)
		if err != nil {
			return nil, err
		}
		return s.Extract(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in.Conversation)
	})
	rt.Tenant("PATCH /v1/tenant/feedback/{uuid}", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Status string `json:"status"`
			Public bool   `json:"is_public"`
		}](r)
		if err != nil {
			return nil, err
		}
		return s.Status(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.Status, in.Public)
	})
	rt.Tenant("POST /v1/tenant/feedback/{uuid}/merge", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Target uuid.UUID `json:"target_uuid"`
		}](r)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"merged": true}, s.Merge(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.Target)
	})
	rt.Tenant("POST /v1/tenant/feedback/{uuid}/sources", "tenant.feedback.write", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Conversation uuid.UUID `json:"conversation_uuid"`
		}](r)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"linked": true}, s.Link(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r), in.Conversation)
	})
	rt.Tenant("GET /v1/tenant/feedback/{uuid}/revenue", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		org := orgctx.Must(r.Context()).ID
		f, err := s.Repo.Q.FeedbackGet(r.Context(), db.FeedbackGetParams{OrganizationID: org, Uuid: supportops.ID(r)})
		if err != nil {
			return nil, err
		}
		return s.Repo.Q.FeedbackRevenue(r.Context(), db.FeedbackRevenueParams{OrganizationID: org, RequestID: f.ID})
	})
	rt.Tenant("GET /v1/tenant/feedback/{uuid}/comments", "tenant.feedback.read", func(r *http.Request) (any, error) { return comments(r, s, orgctx.Must(r.Context()).ID, false) })
	rt.Tenant("PATCH /v1/tenant/feedback-comments/{uuid}", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Approved bool `json:"approved"`
		}](r)
		if err != nil {
			return nil, err
		}
		n, err := s.Repo.Q.FeedbackModerate(r.Context(), db.FeedbackModerateParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r), Approved: in.Approved})
		if err == nil && n == 0 {
			err = pgx.ErrNoRows
		}
		return map[string]bool{"approved": in.Approved}, err
	})
	rt.Tenant("GET /v1/tenant/changelog", "tenant.feedback.read", func(r *http.Request) (any, error) { return changelog(r, s, orgctx.Must(r.Context()).ID, false) })
	rt.Tenant("POST /v1/tenant/changelog", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[ChangelogInput](r)
		if err != nil {
			return nil, err
		}
		return s.Changelog(r.Context(), orgctx.Must(r.Context()).ID, in)
	})
	rt.Tenant("POST /v1/tenant/changelog/{uuid}/publish", "tenant.feedback.manage", func(r *http.Request) (any, error) {
		return s.Publish(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), supportops.ID(r))
	})
	publicOrg := func(r *http.Request) (int64, error) {
		id, _ := uuid.Parse(r.PathValue("organization"))
		o, err := s.Repo.Q.FeedbackPublicOrg(r.Context(), id)
		return o.ID, err
	}
	publicHandle("GET /v1/public/feedback/{organization}", rt.Handler(func(r *http.Request) (any, error) {
		org, err := publicOrg(r)
		if err != nil {
			return nil, err
		}
		return list(r, s, org, true)
	}, false))
	publicHandle("GET /v1/public/feedback/{organization}/{uuid}/comments", rt.Handler(func(r *http.Request) (any, error) {
		org, err := publicOrg(r)
		if err != nil {
			return nil, err
		}
		return comments(r, s, org, true)
	}, false))
	publicHandle("GET /v1/public/roadmap/{organization}", rt.Handler(func(r *http.Request) (any, error) {
		org, err := publicOrg(r)
		if err != nil {
			return nil, err
		}
		return roadmap(r, s, org, true)
	}, false))
	publicHandle("GET /v1/public/changelog/{organization}", rt.Handler(func(r *http.Request) (any, error) {
		org, err := publicOrg(r)
		if err != nil {
			return nil, err
		}
		return changelog(r, s, org, true)
	}, false))
	publicHandle("GET /v1/public/changelog/{organization}/rss", func(w http.ResponseWriter, r *http.Request) {
		org, err := publicOrg(r)
		if err != nil {
			supportops.Fail(w, r, err)
			return
		}
		rows, err := s.Repo.Q.ChangelogList(r.Context(), db.ChangelogListParams{OrganizationID: org, PublicOnly: true, RowLimit: 100})
		if err != nil {
			supportops.Fail(w, r, err)
			return
		}
		feed := rss{Version: "2.0", Channel: rssChannel{Title: "ClickFix changelog", Description: "Product updates", Link: strings.TrimRight(s.PublicBaseURL, "/") + "/api/v1/public/changelog/" + r.PathValue("organization")}}
		for _, e := range rows {
			feed.Channel.Items = append(feed.Channel.Items, rssItem{Title: e.Title, Description: e.Body, GUID: rssGUID{Value: e.Uuid.String()}, Date: e.PublishedAt.Time.Format(time.RFC1123Z)})
		}
		b, err := xml.Marshal(feed)
		if err != nil {
			supportops.Fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write(append([]byte(xml.Header), b...))
	})
	// Public mutations use verified widget identities; callers cannot supply a contact id or email.
	for _, action := range []string{"votes", "comments"} {
		action := action
		publicHandle("POST /v1/public/feedback/{organization}/{uuid}/"+action, rt.Handler(func(r *http.Request) (any, error) {
			org, err := publicOrg(r)
			if err != nil {
				return nil, err
			}
			if widget == nil {
				return nil, supportops.ErrDenied
			}
			in, err := supportops.Read[struct {
				Widget uuid.UUID `json:"widget_uuid"`
				Body   string    `json:"body"`
				Remove bool      `json:"remove"`
			}](r)
			if err != nil {
				return nil, err
			}
			opened, err := widget.Open(r.Context(), in.Widget, r.Header.Get("Origin"))
			if err != nil || opened.Instance.OrganizationID != org {
				return nil, supportops.ErrDenied
			}
			claims, err := widget.Authenticate(opened, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if err != nil || !claims.Verified() {
				return nil, supportops.ErrDenied
			}
			contact, err := s.Repo.Q.FeedbackContact(r.Context(), db.FeedbackContactParams{OrganizationID: org, ChannelInstanceID: pgtype.Int8{Int64: opened.Instance.ID, Valid: true}, ExternalID: claims.ExternalID()})
			if err != nil {
				return nil, err
			}
			f, err := s.Repo.Q.FeedbackGet(r.Context(), db.FeedbackGetParams{OrganizationID: org, Uuid: supportops.ID(r)})
			if err != nil {
				return nil, err
			}
			if !f.IsPublic || f.Status == "merged" {
				return nil, pgx.ErrNoRows
			}
			if action == "comments" {
				if strings.TrimSpace(in.Body) == "" || len(in.Body) > 4000 {
					return nil, supportops.ErrInvalid
				}
				id, err := s.Repo.Q.FeedbackComment(r.Context(), db.FeedbackCommentParams{OrganizationID: org, RequestID: f.ID, ContactID: contact, Body: in.Body})
				return map[string]any{"uuid": id, "approved": false}, err
			}
			if in.Remove {
				return map[string]bool{"voted": false}, s.Repo.Q.FeedbackUnvote(r.Context(), db.FeedbackUnvoteParams{OrganizationID: org, RequestID: f.ID, ContactID: contact})
			}
			return map[string]bool{"voted": true}, s.Repo.Q.FeedbackVote(r.Context(), db.FeedbackVoteParams{OrganizationID: org, RequestID: f.ID, ContactID: contact})
		}, false))
	}
}
func list(r *http.Request, s *Service, org int64, public bool) (any, error) {
	q := apiquery.Parse(r.URL.Query())
	sort := ""
	if len(q.Sort) > 0 {
		if len(q.Sort) != 1 || !q.Sort[0].Desc || q.Sort[0].Field != "votes" && q.Sort[0].Field != "created_at" && q.Sort[0].Field != "revenue" {
			return nil, supportops.ErrInvalid
		}
		sort = "-" + q.Sort[0].Field
		if public && sort == "-revenue" {
			return nil, supportops.ErrDenied
		}
	}
	if sort == "-revenue" {
		currency := r.URL.Query().Get("currency")
		if currency != "TRY" && currency != "USD" && currency != "EUR" {
			return nil, supportops.ErrInvalid
		}
	}
	rows, err := s.Repo.Q.FeedbackList(r.Context(), db.FeedbackListParams{OrganizationID: org, PublicOnly: public, Q: q.Q, Sort: sort, Currency: r.URL.Query().Get("currency"), RowLimit: q.Limit, RowOffset: q.Offset})
	if err != nil {
		return nil, err
	}
	total, err := s.Repo.Q.FeedbackCount(r.Context(), db.FeedbackCountParams{OrganizationID: org, PublicOnly: public, Q: q.Q})
	return supportops.Page(rows, total, q), err
}
func comments(r *http.Request, s *Service, org int64, public bool) (any, error) {
	q, err := supportops.List(r)
	if err != nil {
		return nil, err
	}
	f, err := s.Repo.Q.FeedbackGet(r.Context(), db.FeedbackGetParams{OrganizationID: org, Uuid: supportops.ID(r)})
	if err != nil {
		return nil, err
	}
	if public && !f.IsPublic {
		return nil, pgx.ErrNoRows
	}
	rows, err := s.Repo.Q.FeedbackComments(r.Context(), db.FeedbackCommentsParams{OrganizationID: org, RequestID: f.ID, PublicOnly: public, RowLimit: q.Limit, RowOffset: q.Offset})
	if err != nil {
		return nil, err
	}
	total, err := s.Repo.Q.FeedbackCommentCount(r.Context(), db.FeedbackCommentCountParams{OrganizationID: org, RequestID: f.ID, PublicOnly: public})
	return supportops.Page(rows, total, q), err
}
func changelog(r *http.Request, s *Service, org int64, public bool) (any, error) {
	q, err := supportops.List(r)
	if err != nil {
		return nil, err
	}
	rows, err := s.Repo.Q.ChangelogList(r.Context(), db.ChangelogListParams{OrganizationID: org, PublicOnly: public, RowLimit: q.Limit, RowOffset: q.Offset})
	if err != nil {
		return nil, err
	}
	total, err := s.Repo.Q.ChangelogCount(r.Context(), db.ChangelogCountParams{OrganizationID: org, PublicOnly: public})
	return supportops.Page(rows, total, q), err
}

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}
type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}
type rssItem struct {
	Title       string  `xml:"title"`
	Description string  `xml:"description"`
	GUID        rssGUID `xml:"guid"`
	Date        string  `xml:"pubDate"`
}

func roadmap(r *http.Request, s *Service, org int64, public bool) (any, error) {
	q, err := supportops.List(r)
	if err != nil {
		return nil, err
	}
	rows, err := s.Repo.Q.RoadmapList(r.Context(), db.RoadmapListParams{OrganizationID: org, PublicOnly: public, RowLimit: q.Limit, RowOffset: q.Offset})
	if err != nil {
		return nil, err
	}
	total, err := s.Repo.Q.RoadmapCount(r.Context(), db.RoadmapCountParams{OrganizationID: org, PublicOnly: public})
	return supportops.Page(rows, total, q), err
}

type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermalink bool   `xml:"isPermaLink,attr"`
}
