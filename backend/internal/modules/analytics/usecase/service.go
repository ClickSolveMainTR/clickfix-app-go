package usecase

import (
	"context"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/model"
	"time"
)

var ErrInvalid = errors.New("analytics: invalid filter")

type Repository interface {
	Sync(context.Context, int64, int64) error
	Backfill(context.Context, int64) error
	Dashboard(context.Context, int64, model.Filter) (map[string]any, error)
}
type Service struct{ repo Repository }

func New(r Repository) *Service                                  { return &Service{r} }
func (s *Service) Sync(ctx context.Context, org, id int64) error { return s.repo.Sync(ctx, org, id) }
func (s *Service) Backfill(ctx context.Context, org int64) error { return s.repo.Backfill(ctx, org) }
func (s *Service) Dashboard(ctx context.Context, org int64, f model.Filter) (map[string]any, error) {
	if org <= 0 || f.From.IsZero() || !f.To.After(f.From) || f.To.Sub(f.From) > 366*24*time.Hour {
		return nil, ErrInvalid
	}
	if f.Bucket == "" {
		f.Bucket = "day"
	}
	if f.Bucket != "hour" && f.Bucket != "day" && f.Bucket != "week" && f.Bucket != "month" {
		return nil, ErrInvalid
	}
	if f.Timezone != "" {
		if _, err := time.LoadLocation(f.Timezone); err != nil {
			return nil, ErrInvalid
		}
	}
	return s.repo.Dashboard(ctx, org, f)
}
