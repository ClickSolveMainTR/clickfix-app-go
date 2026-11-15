package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/jackc/pgx/v5/pgtype"
)

// CallsPerSuggestion is the AI calls one suggestion costs: its draft and the
// embedding that compares it with rejected ideas and published articles.
const CallsPerSuggestion = 2

// Estimate is the expected AI use of an upload (CF-228). The panel shows it
// before the upload, so a run that needs more than today's batch share does
// not come as a surprise. Embedding calls are exact for the file; suggestion
// calls depend on what the clusters turn out to be and are counted per
// suggestion. A nil allowance is unlimited.
type Estimate struct {
	Records            int        `json:"records"`
	EmbeddingCalls     int        `json:"embedding_calls"`
	CallsPerSuggestion int        `json:"calls_per_suggestion"`
	EstimatedTokens    int64      `json:"estimated_tokens"`
	CostMicros         int64      `json:"cost_micros"`
	AIEnabled          bool       `json:"ai_enabled"`
	BatchCallsPerDay   *int64     `json:"batch_calls_per_day"`
	BatchCallsLeft     *int64     `json:"batch_calls_left"`
	BatchBudgetPerDay  *int64     `json:"batch_budget_micros_per_day"`
	BatchBudgetLeft    *int64     `json:"batch_budget_micros_left"`
	ExceedsToday       bool       `json:"exceeds_today"`
	DaysNeeded         int        `json:"days_needed"`
	LimitResetsAt      time.Time  `json:"limit_resets_at"`
	RawRetentionHours  int32      `json:"raw_retention_hours"`
	ExceedsRetention   bool       `json:"exceeds_retention"`
	FinishesBy         *time.Time `json:"finishes_by"`
}

// embedChunks splits texts the way embedAll sends them and returns the
// size of each call.
func embedChunks(sizes []int, maxInputs, maxBytes int) []int {
	var out []int
	for start := 0; start < len(sizes); {
		end, bytes := start, 0
		for end < len(sizes) && end-start < maxInputs && (end == start || bytes+sizes[end] <= maxBytes) {
			bytes += sizes[end]
			end++
		}
		out = append(out, end-start)
		start = end
	}
	return out
}

// Estimate parses an export like Upload does, without storing it, and
// estimates its AI calls and cost against today's batch allowance.
func (s *Service) Estimate(ctx context.Context, org int64, name string, data []byte) (Estimate, error) {
	b, err := ParseBundle(name, data)
	if err != nil {
		return Estimate{}, err
	}
	settings, err := s.Settings(ctx, org)
	if err != nil {
		return Estimate{}, err
	}
	now := s.now()
	out := Estimate{CallsPerSuggestion: CallsPerSuggestion, AIEnabled: true, LimitResetsAt: NextLimitReset(now), RawRetentionHours: settings.RawRetentionHours}
	records := BundleRecords(b, pgtype.UUID{})
	out.Records = len(records)
	size := max(1, min(s.BatchSize, model.MaxBatchRecords))
	var bytes int64
	for start := 0; start < len(records); start += size {
		var sizes []int
		for _, r := range records[start:min(start+size, len(records))] {
			c, ok := BuildCase(org, r)
			if !ok {
				continue
			}
			sizes = append(sizes, len(c.EmbedText()), len(c.Resolution))
			bytes += int64(len(c.EmbedText()) + len(c.Resolution))
		}
		out.EmbeddingCalls += len(embedChunks(sizes, model.MaxEmbedInputs, model.MaxEmbedBytes))
	}
	// About four bytes per token; bytes are what the reservation holds.
	out.EstimatedTokens = (bytes + 3) / 4
	budgeter, ok := s.Embedder.(ai.BatchBudgeter)
	if !ok {
		return out, nil
	}
	out.CostMicros = budgeter.EmbeddingCostMicros(out.EstimatedTokens)
	allow, err := budgeter.BatchAllowance(ctx, org)
	if err != nil {
		return Estimate{}, err
	}
	out.AIEnabled = allow.Enabled
	out.BatchCallsPerDay, out.BatchCallsLeft = allow.CallsPerDay, allow.CallsLeft
	out.BatchBudgetPerDay, out.BatchBudgetLeft = allow.BudgetMicrosPerDay, allow.BudgetMicrosLeft
	out.DaysNeeded = max(days(int64(out.EmbeddingCalls), allow.CallsPerDay, allow.CallsLeft), days(out.CostMicros, allow.BudgetMicrosPerDay, allow.BudgetMicrosLeft))
	out.ExceedsToday = out.DaysNeeded != 1
	if out.DaysNeeded > 1 {
		// The last part runs on the last day; the raw file must outlive it.
		finish := out.LimitResetsAt.Add(time.Duration(out.DaysNeeded-1) * 24 * time.Hour)
		out.FinishesBy = &finish
		out.ExceedsRetention = finish.After(now.Add(time.Duration(settings.RawRetentionHours) * time.Hour))
	}
	return out, nil
}

// days is how many days (today included) need fits into: 1 when today's
// allowance covers it, 0 when it never fits (a share of zero).
func days(need int64, perDay, left *int64) int {
	if perDay == nil || left == nil || need <= *left {
		return 1
	}
	if *perDay < 1 {
		return 0
	}
	rest := need - *left
	return 1 + int((rest+*perDay-1) / *perDay)
}
