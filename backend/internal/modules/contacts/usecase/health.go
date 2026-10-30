package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

// HealthSummary is a company's current health score (CF-87); the details
// live under /v1/tenant/companies/{uuid}/health.
type HealthSummary struct {
	Score          *int       `json:"score"`
	BelowThreshold bool       `json:"below_threshold"`
	ComputedAt     *time.Time `json:"computed_at"`
}

// attachHealth fills Health on a page of companies with one query.
func (s *Service) attachHealth(ctx context.Context, orgID int64, ids []int64, out []Company) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.q.ListCompanyHealthByCompanyIDs(ctx, db.ListCompanyHealthByCompanyIDsParams{OrganizationID: orgID, CompanyIds: ids})
	if err != nil {
		return err
	}
	byID := make(map[int64]HealthSummary, len(rows))
	for _, r := range rows {
		if !r.ComputedAt.Valid {
			continue // only a usage report so far
		}
		h := HealthSummary{BelowThreshold: r.BelowThreshold, ComputedAt: timePtr(r.ComputedAt)}
		if r.Score.Valid {
			v := int(r.Score.Int16)
			h.Score = &v
		}
		byID[r.CompanyID] = h
	}
	for i, id := range ids {
		if h, ok := byID[id]; ok {
			out[i].Health = &h
		}
	}
	return nil
}
