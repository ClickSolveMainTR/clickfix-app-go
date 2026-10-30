package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/google/uuid"
	"github.com/meilisearch/meilisearch-go"
)

// Meili uses its lexical ranking alongside exact cosine search in Postgres, fused
// by reciprocal rank. Meilisearch uses its native ranking rules, not literal BM25.
type Meili struct {
	client meilisearch.ServiceManager
	index  string
	mu     sync.Mutex
	ready  bool
}

func NewMeili(cfg config.SearchConfig) *Meili {
	if !cfg.Enabled || cfg.Driver != "meilisearch" {
		return nil
	}
	return &Meili{client: meilisearch.New(cfg.MeiliHost, meilisearch.WithAPIKey(cfg.MeiliKey)), index: cfg.IndexPrefix + "_knowledge_chunks"}
}

func (m *Meili) wait(ctx context.Context, t *meilisearch.TaskInfo, err error) error {
	if err != nil {
		return err
	}
	task, err := m.client.WaitForTaskWithContext(ctx, t.TaskUID, 0)
	if err != nil {
		return err
	}
	if task.Status != meilisearch.TaskStatusSucceeded {
		return fmt.Errorf("knowledge: meilisearch task failed")
	}
	return nil
}

func (m *Meili) ensure(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ready {
		return nil
	}
	// UpdateSettings creates an absent index and is idempotent across API/worker processes.
	t, err := m.client.Index(m.index).UpdateSettingsWithContext(ctx, &meilisearch.Settings{SearchableAttributes: []string{"content"}, FilterableAttributes: []string{"organization_id", "article_uuid", "locale", "visibility"}, DisplayedAttributes: []string{"id"}})
	if err = m.wait(ctx, t, err); err != nil {
		return err
	}
	m.ready = true
	return nil
}

func (m *Meili) Replace(ctx context.Context, org int64, id uuid.UUID, docs []Document) error {
	if err := m.ensure(ctx); err != nil {
		return err
	}
	filter := fmt.Sprintf("organization_id = %d AND article_uuid = %q", org, id.String())
	t, err := m.client.Index(m.index).DeleteDocumentsByFilterWithContext(ctx, filter)
	if err = m.wait(ctx, t, err); err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}
	t, err = m.client.Index(m.index).AddDocumentsWithContext(ctx, docs, "id")
	return m.wait(ctx, t, err)
}

func (m *Meili) Search(ctx context.Context, org int64, q, locale string, publicOnly bool) ([]uuid.UUID, error) {
	if locale != "tr" && locale != "en" {
		return nil, fmt.Errorf("knowledge: unsupported locale")
	}
	if err := m.ensure(ctx); err != nil {
		return nil, err
	}
	filter := fmt.Sprintf("organization_id = %d AND locale = %q", org, locale)
	if publicOnly {
		filter += " AND visibility = public"
	}
	result, err := m.client.Index(m.index).SearchWithContext(ctx, strings.TrimSpace(q), &meilisearch.SearchRequest{Limit: 40, Filter: filter})
	if err != nil {
		return nil, err
	}
	ids := []uuid.UUID{}
	for _, hit := range result.Hits {
		row, ok := hit.(map[string]any)
		if !ok {
			continue
		}
		raw, _ := row["id"].(string)
		id, err := uuid.Parse(raw)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
