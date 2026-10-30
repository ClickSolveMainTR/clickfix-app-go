package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
)

// AdminFilter narrows the platform billing lists (CF-39).
type AdminFilter struct {
	Status   string
	LiveOnly bool
	Search   string
	Limit    int32
	Offset   int32
}

var (
	adminSubscriptionStatuses = map[string]bool{"": true, "trialing": true, "active": true, "past_due": true, "grace": true, "paused": true, "canceled": true, "expired": true, "suspended": true}
	adminPaymentStatuses      = map[string]bool{"": true, "pending": true, "unknown": true, "succeeded": true, "failed": true}
	adminTransferStatuses     = map[string]bool{"": true, "reported": true, "approved": true, "rejected": true}
)

func (f AdminFilter) paging() (int32, int32) { return paymentPaging([]int32{f.Limit, f.Offset}) }

// AdminSubscriptionStats is the live-subscription breakdown the platform
// billing header shows. MRR is the monthly-normalized list price per currency
// (yearly prices divided by 12, rounded down) of paying subscriptions; trials,
// seats and add-ons are excluded.
type AdminSubscriptionStats struct {
	ByStatus map[string]int64 `json:"by_status"`
	MRRMinor map[string]int64 `json:"mrr_minor"`
	Live     int64            `json:"live"`
}

func (s *Service) AdminSubscriptions(ctx context.Context, f AdminFilter) ([]db.BillingAdminSubscriptionsRow, int64, error) {
	f.Search = strings.TrimSpace(f.Search)
	if !adminSubscriptionStatuses[f.Status] || len(f.Search) > 200 {
		return nil, 0, model.ErrInvalid
	}
	limit, offset := f.paging()
	rows, err := s.repo.Q.BillingAdminSubscriptions(ctx, db.BillingAdminSubscriptionsParams{Status: f.Status, LiveOnly: f.LiveOnly, Search: f.Search, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.Q.BillingAdminSubscriptionCount(ctx, db.BillingAdminSubscriptionCountParams{Status: f.Status, LiveOnly: f.LiveOnly, Search: f.Search})
	return rows, total, err
}

func (s *Service) AdminSubscriptionStats(ctx context.Context) (AdminSubscriptionStats, error) {
	out := AdminSubscriptionStats{ByStatus: map[string]int64{}, MRRMinor: map[string]int64{}}
	rows, err := s.repo.Q.BillingAdminSubscriptionStats(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		out.ByStatus[r.Status] += r.Subscriptions
		out.Live += r.Subscriptions
		// Trials have not paid yet; they do not count towards recurring revenue.
		if r.Status == "trialing" {
			continue
		}
		amount := r.AmountMinor
		if r.PriceInterval == "yearly" {
			amount /= 12
		}
		out.MRRMinor[r.PriceCurrency] += amount
	}
	return out, nil
}

func (s *Service) AdminPayments(ctx context.Context, f AdminFilter) ([]db.BillingAdminPaymentsRow, int64, error) {
	if !adminPaymentStatuses[f.Status] {
		return nil, 0, model.ErrInvalid
	}
	limit, offset := f.paging()
	rows, err := s.repo.Q.BillingAdminPayments(ctx, db.BillingAdminPaymentsParams{Status: f.Status, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.Q.BillingAdminPaymentCount(ctx, db.BillingAdminPaymentCountParams{Status: f.Status})
	return rows, total, err
}

func (s *Service) AdminTransfers(ctx context.Context, f AdminFilter) ([]db.BillingAdminTransfersRow, int64, error) {
	if !adminTransferStatuses[f.Status] {
		return nil, 0, model.ErrInvalid
	}
	limit, offset := f.paging()
	rows, err := s.repo.Q.BillingAdminTransfers(ctx, db.BillingAdminTransfersParams{Status: f.Status, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.Q.BillingAdminTransferCount(ctx, f.Status)
	return rows, total, err
}
