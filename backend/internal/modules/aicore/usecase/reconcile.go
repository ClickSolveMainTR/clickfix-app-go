package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

// Reconcile frees concurrency slots after a crashed process without refunding a
// potentially billable request. Unknown reservations remain visible for operator review.
func (s *Service) Reconcile(ctx context.Context) error {
	runs, err := s.Repo.Stale(ctx)
	if err != nil {
		return err
	}
	errs := []error{}
	for _, run := range runs {
		err = s.Repo.Finish(ctx, db.AIFinishRunParams{OrganizationID: run.OrganizationID, Uuid: run.Uuid, Model: run.Model, Provider: run.Provider, Status: "unknown", ToolCalls: []byte(`[]`), ChargedCredits: run.ReservedCredits, ReservedCostMicros: run.ReservedCostMicros, LatencyMs: time.Since(run.CreatedAt.Time).Milliseconds(), ErrorCode: "AI_PROCESS_INTERRUPTED"})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
