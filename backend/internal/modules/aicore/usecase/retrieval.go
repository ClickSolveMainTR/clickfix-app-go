package usecase

import (
	"context"
	"log/slog"
	"time"
	"unicode/utf8"

	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

type RetrievalResult struct {
	Citations []kbmodel.Citation
	Err       error
}

func (s *Service) retrieve(ctx context.Context, org int64, input, locale string, publicOnly bool) ([]kbmodel.Citation, error) {
	if s.RAG == nil {
		return nil, ai.ErrNotConfigured
	}
	if err := ai.Guard(input); err != nil {
		return nil, err
	}
	query := ai.NewMasker().Mask(input)
	if len(query) > 4000 {
		query = query[len(query)-4000:]
		for !utf8.ValidString(query) {
			query = query[1:]
		}
	}
	started := time.Now()
	citations, err := s.RAG.Search(ctx, org, query, locale, publicOnly)
	slog.InfoContext(ctx, "ai_stage", "stage", "retrieval", "organization_id", org, "duration_ms", time.Since(started).Milliseconds(), "failed", err != nil)
	return citations, err
}

// PrefetchReply uses the same guarded, public-only retriever and metered embedder
// as regular reply runs. The buffered, closed result is safe to cancel and join.
func (s *Service) PrefetchReply(ctx context.Context, org int64, input, locale string) <-chan RetrievalResult {
	if s.RAG == nil {
		return nil
	}
	policy, err := s.Repo.Policy(ctx, org)
	if err != nil || !policy.Enabled || policy.Concurrency < 2 {
		return nil
	}
	active, err := s.Repo.Billing.Q.AIActive(ctx, org)
	if err != nil || active+2 > int64(policy.Concurrency) {
		return nil
	}
	result := make(chan RetrievalResult, 1)
	go func() {
		defer close(result)
		citations, err := s.retrieve(ctx, org, input, locale, true)
		result <- RetrievalResult{Citations: citations, Err: err}
	}()
	return result
}
