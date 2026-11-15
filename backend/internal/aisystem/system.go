// Package aisystem composes the AI core and knowledge base in both API and worker.
package aisystem

import (
	"os"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	airepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/repository"
	aiusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	memusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aimemory/usecase"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/entitlements"
	kbrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/repository"
	kbusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/vectorstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

type System struct {
	AI     *aiusecase.Service
	KB     *kbusecase.Service
	RAG    *kbusecase.RAG
	Speech *aiusecase.MeteredTranscriber
	// Memory is the CF-216 knowledge layer (units, answer cache, case memory).
	Memory *memusecase.Service
	// Reembed rebuilds stored vectors after an embedding model change (CF-214).
	Reembed *vectorstore.Reembedder
}

func price(key string) int64 {
	n, _ := strconv.ParseInt(os.Getenv(key), 10, 64)
	// Bounds prevent multiplication overflow for the request's token limits.
	if n < 1 || n > 1000000000 {
		return 0
	}
	return n
}

// New reads provider credentials only from process environment, never request data.
// No hard-coded live rates: operators must configure effective prices before execution.
func New(pool *pgxpool.Pool, billing *billingrepo.Repository, ent *entitlements.Service, search config.SearchConfig) *System {
	anthropic := ai.NewAnthropic(os.Getenv("ANTHROPIC_API_KEY"))
	// One embedding model for every vector (CF-214); empty = ai.DefaultEmbeddingModel.
	openai := ai.NewOpenAI(os.Getenv("OPENAI_API_KEY"), os.Getenv("OPENAI_BASE_URL"), ai.EmbeddingModel(strings.TrimSpace(os.Getenv("AI_EMBEDDING_MODEL"))))
	routes := map[string]ai.Route{}
	for _, task := range []string{"triage", "reply", "complex"} {
		prefix := "AI_" + strings.ToUpper(task)
		available := anthropic.Key != ""
		route := ai.Route{Provider: anthropic, ProviderName: ai.ProviderAnthropic, Model: ai.ModelForTask(task), InputPrice: price(prefix + "_INPUT_PRICE_MICROS"), OutputPrice: price(prefix + "_OUTPUT_PRICE_MICROS")}
		route.CacheReadPrice = price(prefix + "_CACHE_READ_PRICE_MICROS")
		route.CacheCreatePrice = price(prefix + "_CACHE_CREATE_PRICE_MICROS")
		if os.Getenv(prefix+"_PROVIDER") == ai.ProviderOpenAI {
			available = openai.Key != ""
			route.Provider = openai
			route.ProviderName = ai.ProviderOpenAI
			route.Model = os.Getenv(prefix + "_MODEL")
		}
		fallbackModel := os.Getenv(prefix + "_FALLBACK_MODEL")
		fallbackIn := price(prefix + "_FALLBACK_INPUT_PRICE_MICROS")
		fallbackOut := price(prefix + "_FALLBACK_OUTPUT_PRICE_MICROS")
		if route.InputPrice > 0 && route.OutputPrice > 0 && route.ProviderName == ai.ProviderAnthropic && fallbackModel != "" && openai.Key != "" && fallbackIn > 0 && fallbackOut > 0 {
			route.Provider = &ai.Failover{Primary: anthropic, Secondary: openai, SecondaryModel: fallbackModel}
			route.InputPrice = max(route.InputPrice, fallbackIn)
			route.OutputPrice = max(route.OutputPrice, fallbackOut)
			// Fallback rates are effective upper bounds for either provider.
			route.CacheReadPrice = 0
			route.CacheCreatePrice = 0
		}
		if route.Model != "" && available {
			routes[task] = route
		}
	}
	aiStore := airepo.New(billing, ent)
	kb := kbusecase.New(kbrepo.New(pool))
	rag := &kbusecase.RAG{KB: kb}
	reembed := &vectorstore.Reembedder{Pool: pool, Sources: []vectorstore.Source{kbusecase.ChunkSource}, Skip: func(err error) bool { return kbusecase.SkipReason(err) != "" }}
	if openai.Key != "" && openai.Model() != "" {
		rag.Embedder = &aiusecase.MeteredEmbedder{Raw: openai, Repo: aiStore, InputPrice: price("AI_EMBEDDING_INPUT_PRICE_MICROS")}
		reembed.Embedder = rag.Embedder
	}
	if meili := kbusecase.NewMeili(search); meili != nil {
		rag.Lexical = meili
	}
	speech := &aiusecase.MeteredTranscriber{Raw: openai, Repo: aiStore, Model: os.Getenv("AI_TRANSCRIBE_MODEL"), PricePerMinute: price("AI_TRANSCRIBE_PRICE_MICROS")}
	if openai.Key == "" {
		speech.Raw = nil
	}
	core := &aiusecase.Service{Repo: aiStore, Routes: routes, RAG: rag}
	return &System{Reembed: reembed, Speech: speech, KB: kb, RAG: rag, AI: core, Memory: memusecase.New(pool, core, rag.Embedder)}
}
