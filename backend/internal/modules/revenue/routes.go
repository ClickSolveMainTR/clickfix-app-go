// Package revenue exposes platform-only, currency-separated subscription revenue reports.
package revenue

import (
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"net/http"
)

func RegisterRoutes(rt supportops.Router, repo *repository.Repository) {
	rt.Platform("GET /v1/platform/revenue", "platform.revenue.read", func(r *http.Request) (any, error) {
		// CF-113 is explicitly super-admin only, even if a custom role accidentally gains the permission.
		if !authctx.MustPrincipal(r.Context()).IsSuperAdmin {
			return nil, supportops.ErrDenied
		}
		coverage, err := repo.Q.RevenueCoverage(r.Context())
		if err != nil {
			return nil, err
		}
		if r.URL.Query().Get("since") == "" {
			return map[string]any{"coverage_start": coverage}, nil
		}
		a, b, err := supportops.Window(r)
		if err != nil {
			return nil, err
		}
		if a.Before(coverage.Time) {
			return nil, supportops.ErrInvalid
		}
		raw, err := repo.Q.RevenueReport(r.Context(), db.RevenueReportParams{Since: repository.TS(a), Until: repository.TS(b)})
		return map[string]any{"coverage_start": coverage, "since": a, "until": b, "report": json.RawMessage(raw)}, err
	})
}
