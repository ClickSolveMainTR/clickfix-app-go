package ai

import "context"

// Lanes of AI calls (CF-228). Live calls serve a customer or an agent now;
// batch calls come from background jobs (history pipeline, re-embedding,
// evaluations) and may use only their share of the daily limits, so a long
// job cannot stop live AI for the rest of the day.
const (
	LaneLive  = "live"
	LaneBatch = "batch"
)

type laneKey struct{}

// WithBatch marks every AI call made with ctx as batch work.
func WithBatch(ctx context.Context) context.Context {
	return context.WithValue(ctx, laneKey{}, LaneBatch)
}

// LaneOf returns the lane of calls made with ctx (live unless marked).
func LaneOf(ctx context.Context) string {
	if v, _ := ctx.Value(laneKey{}).(string); v == LaneBatch {
		return LaneBatch
	}
	return LaneLive
}

// BatchAllowance is what batch jobs of one organization may still use today
// (CF-228). A nil limit is unlimited.
type BatchAllowance struct {
	Enabled bool
	// Daily batch share and what is left of it today (also bounded by the
	// organization's total daily limit).
	CallsPerDay, CallsLeft               *int64
	BudgetMicrosPerDay, BudgetMicrosLeft *int64
}

// BatchBudgeter is implemented by the metered embedder: the pre-upload
// estimate of a batch job reads the allowance and prices its embeddings.
type BatchBudgeter interface {
	BatchAllowance(ctx context.Context, org int64) (BatchAllowance, error)
	EmbeddingCostMicros(tokens int64) int64
}
