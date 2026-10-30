package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

// MeteredEmbedder puts indexing and query embeddings through the same wallet, policy,
// concurrency, daily budgets, monthly entitlements and masked run log as generation.
type MeteredEmbedder struct {
	Raw        ai.Embedder
	Repo       *repository.Store
	InputPrice int64
}

func (e *MeteredEmbedder) Model() string { return e.Raw.Model() }
func (e *MeteredEmbedder) Embed(ctx context.Context, input []string) (ai.Embeddings, error) {
	org, ok := orgctx.OrganizationID(ctx)
	if !ok || e.InputPrice < 1 {
		return ai.Embeddings{}, ai.ErrNotConfigured
	}
	// Hard upper bound protects cost arithmetic and provider payloads for injected callers.
	if len(input) == 0 || len(input) > 256 {
		return ai.Embeddings{}, ai.ErrInvalidOutput
	}
	texts := make([]string, len(input))
	masker := ai.NewMasker()
	maxTokens := int64(1024)
	for i, s := range input {
		if len(s) > 24000 {
			return ai.Embeddings{}, ai.ErrInvalidOutput
		}
		if err := ai.Guard(s); err != nil {
			return ai.Embeddings{}, err
		}
		texts[i] = masker.Mask(s)
		maxTokens += int64(len(texts[i]))
	}
	route := ai.Route{InputPrice: e.InputPrice}
	id := uuid.New()
	reservedCredits := (maxTokens + 999) / 1000
	reservedCost := route.Cost(maxTokens, 0)
	_, _, err := e.Repo.Reserve(ctx, db.AIInsertRunParams{OrganizationID: org, Uuid: id, Task: "embedding", Provider: ai.ProviderOpenAI, Model: e.Model(), Status: "running", Input: strings.Join(texts, "\n\n"), RetrievedChunks: []byte(`[]`), ReservedTokens: maxTokens, ReservedCredits: reservedCredits, ReservedCostMicros: reservedCost})
	if err != nil {
		return ai.Embeddings{}, err
	}
	started := time.Now()
	out, err := e.Raw.Embed(ctx, texts)
	status := "completed"
	code := ""
	charged := (out.InputTokens + 999) / 1000
	cost := route.Cost(out.InputTokens, 0)
	remaining := int64(0)
	if err == nil && (out.InputTokens < 1 || out.InputTokens > maxTokens) {
		err = ai.ErrInvalidOutput
		out.InputTokens = 0
	}
	if err != nil {
		status = "unknown"
		code = "AI_EMBEDDING_OUTCOME_UNKNOWN"
		charged = reservedCredits
		remaining = reservedCost
		cost = 0
	}
	if ai.NotAccepted(err) {
		status = "failed"
		code = "AI_PROVIDER_REJECTED"
		charged = 0
		remaining = 0
		cost = 0
		out.InputTokens = 0
	}
	if out.InputTokens < 0 || out.InputTokens > maxTokens {
		out.InputTokens = 0
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	finishErr := e.Repo.Finish(finishCtx, db.AIFinishRunParams{OrganizationID: org, Uuid: id, Provider: ai.ProviderOpenAI, Model: e.Model(), Status: status, Output: fmt.Sprintf("%d embedding vectors", len(out.Vectors)), ToolCalls: []byte(`[]`), InputTokens: out.InputTokens, CostMicros: cost, ReservedCostMicros: remaining, ChargedCredits: charged, LatencyMs: time.Since(started).Milliseconds(), ErrorCode: code})
	if finishErr != nil {
		return ai.Embeddings{}, finishErr
	}
	return out, err
}
