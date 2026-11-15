package ai

import "context"

// The single embedding model (CF-214). Every stored vector has exactly
// EmbeddingDimensions values (the vector(N) columns are fixed by migration);
// AI_EMBEDDING_MODEL may name another model of the same dimension, which
// triggers background re-embedding of every stored vector.
const (
	DefaultEmbeddingModel = "text-embedding-3-small"
	EmbeddingDimensions   = 1536
)

// EmbeddingModel resolves the configured model name ("" → default).
func EmbeddingModel(configured string) string {
	if configured == "" {
		return DefaultEmbeddingModel
	}
	return configured
}

// ModelEmbedder embeds with a named model instead of the configured one. Search
// uses it while an organization's stored vectors are still from the previous
// model, so query and stored vectors always come from the same model.
type ModelEmbedder interface {
	Embedder
	EmbedModel(ctx context.Context, model string, input []string) (Embeddings, error)
}
