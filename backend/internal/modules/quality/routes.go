package quality

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func RegisterRoutes(rt supportops.Router, s *Service) {
	rt.Tenant("PUT /v1/tenant/quality/scorecards/{uuid}", "tenant.quality.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[CardInput](r)
		if err != nil {
			return nil, err
		}
		out, err := s.UpdateCard(r.Context(), orgctx.Must(r.Context()).ID, supportops.ID(r), in)
		return CardView(out), err
	})
	rt.Tenant("GET /v1/tenant/quality/scorecards/meta", "tenant.quality.read", func(r *http.Request) (any, error) { return supportops.Meta("tenant.quality.scorecards"), nil })

	rt.Tenant("GET /v1/tenant/quality/reviews/meta", "tenant.quality.read", func(r *http.Request) (any, error) { return supportops.Meta("tenant.quality.reviews"), nil })

	rt.Tenant("GET /v1/tenant/quality/scorecards", "tenant.quality.read", func(r *http.Request) (any, error) {
		rows, err := s.Repo.Q.QAListScorecards(r.Context(), orgctx.Must(r.Context()).ID)
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			out = append(out, CardView(row))
		}
		return out, err
	})
	rt.Tenant("POST /v1/tenant/quality/scorecards", "tenant.quality.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[CardInput](r)
		if err != nil {
			return nil, err
		}
		out, err := s.Card(r.Context(), orgctx.Must(r.Context()).ID, in)
		return CardView(out), err
	})
	rt.Tenant("GET /v1/tenant/quality/reviews", "tenant.quality.read", func(r *http.Request) (any, error) {
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		org := orgctx.Must(r.Context()).ID
		pending := r.URL.Query().Get("pending_only") == "true"
		rows, err := s.Repo.Q.QAListReviews(r.Context(), db.QAListReviewsParams{OrganizationID: org, PendingOnly: pending, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		total, err := s.Repo.Q.QACountReviews(r.Context(), db.QACountReviewsParams{OrganizationID: org, PendingOnly: pending})
		return supportops.Page(reviewListView(rows), total, q), err
	})
	rt.Tenant("POST /v1/tenant/quality/reviews", "tenant.quality.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[ReviewInput](r)
		if err != nil {
			return nil, err
		}
		out, err := s.Review(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in, false)
		view := ReviewView(out)
		view["conversation_uuid"] = in.Conversation
		return view, err
	})
	rt.Tenant("POST /v1/tenant/quality/auto-score", "tenant.quality.manage", func(r *http.Request) (any, error) {
		in, err := supportops.Read[ReviewInput](r)
		if err != nil {
			return nil, err
		}
		out, err := s.Review(r.Context(), orgctx.Must(r.Context()).ID, supportops.Actor(r.Context()), in, true)
		view := ReviewView(out)
		view["conversation_uuid"] = in.Conversation
		return view, err
	})
	rt.Tenant("POST /v1/tenant/quality/reviews/{uuid}/acknowledge", "tenant.quality.manage", func(r *http.Request) (any, error) {
		actor := supportops.Actor(r.Context())
		n, err := s.Repo.Q.QAAcknowledge(r.Context(), db.QAAcknowledgeParams{OrganizationID: orgctx.Must(r.Context()).ID, Uuid: supportops.ID(r), ReviewerID: repository.Int(&actor)})
		if err == nil && n == 0 {
			err = pgx.ErrNoRows
		}
		return map[string]bool{"acknowledged": true}, err
	})
	rt.Tenant("GET /v1/tenant/quality/reports", "tenant.quality.read", func(r *http.Request) (any, error) {
		a, b, err := supportops.Window(r)
		if err != nil {
			return nil, err
		}
		return s.Repo.Q.QAReport(r.Context(), db.QAReportParams{OrganizationID: orgctx.Must(r.Context()).ID, CreatedAt: repository.TS(a), CreatedAt_2: repository.TS(b)})
	})
}
