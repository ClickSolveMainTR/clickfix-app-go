package searchengine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/meilisearch/meilisearch-go"
)

// Client wraps Meilisearch index operations.
type Client struct {
	cfg    config.SearchConfig
	client meilisearch.ServiceManager
	log    *slog.Logger
}

// NewClient builds a Meilisearch client. Returns nil when search is disabled.
func NewClient(cfg config.SearchConfig, log *slog.Logger) *Client {
	if !cfg.Enabled || cfg.Driver != "meilisearch" {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		cfg:    cfg,
		client: meilisearch.New(cfg.MeiliHost, meilisearch.WithAPIKey(cfg.MeiliKey)),
		log:    log,
	}
}

// Enabled reports whether search indexing is active.
func (c *Client) Enabled() bool {
	return c != nil && c.client != nil
}

// Ping checks Meilisearch health.
func (c *Client) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	_, err := c.client.HealthWithContext(ctx)
	if err != nil {
		return fmt.Errorf("searchengine: meili health: %w", err)
	}
	return nil
}

func (c *Client) indexUID(spec string) string {
	return fmt.Sprintf("%s_%s", c.cfg.IndexPrefix, spec)
}

// EnsureIndex creates or updates index settings for a spec.
func (c *Client) EnsureIndex(ctx context.Context, spec Spec) error {
	if !c.Enabled() {
		return nil
	}
	uid := c.indexUID(spec.ID)
	task, err := c.client.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{Uid: uid, PrimaryKey: "id"})
	if err != nil {
		return fmt.Errorf("searchengine: create index %q: %w", uid, err)
	}
	if _, err := c.client.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("searchengine: wait create index %q: %w", uid, err)
	}

	searchable := spec.Searchable
	if len(searchable) == 0 {
		searchable = []string{"title", "subtitle", "keywords"}
	}
	settings := &meilisearch.Settings{
		SearchableAttributes: searchable,
		DisplayedAttributes:  displayedAttributes,
		FilterableAttributes: filterableAttributes(spec),
	}
	task, err = c.client.Index(uid).UpdateSettingsWithContext(ctx, settings)
	if err != nil {
		return fmt.Errorf("searchengine: update settings %q: %w", uid, err)
	}
	if _, err := c.client.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("searchengine: wait settings %q: %w", uid, err)
	}
	return nil
}

// UpsertDocuments adds or replaces documents in a spec index.
func (c *Client) UpsertDocuments(ctx context.Context, spec string, docs []Document) error {
	if !c.Enabled() || len(docs) == 0 {
		return nil
	}
	uid := c.indexUID(spec)
	task, err := c.client.Index(uid).AddDocumentsWithContext(ctx, docs)
	if err != nil {
		return fmt.Errorf("searchengine: upsert %q: %w", uid, err)
	}
	if _, err := c.client.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("searchengine: wait upsert %q: %w", uid, err)
	}
	return nil
}

// DeleteDocument removes one document from a spec index.
func (c *Client) DeleteDocument(ctx context.Context, spec, id string) error {
	if !c.Enabled() || strings.TrimSpace(id) == "" {
		return nil
	}
	uid := c.indexUID(spec)
	task, err := c.client.Index(uid).DeleteDocumentWithContext(ctx, id)
	if err != nil {
		return fmt.Errorf("searchengine: delete %q/%s: %w", uid, id, err)
	}
	if _, err := c.client.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("searchengine: wait delete %q/%s: %w", uid, id, err)
	}
	return nil
}

// DeleteDocuments removes a bounded batch and waits for the durable index task.
func (c *Client) DeleteDocuments(ctx context.Context, spec string, ids []string) error {
	if !c.Enabled() || len(ids) == 0 {
		return nil
	}
	uid := c.indexUID(spec)
	task, err := c.client.Index(uid).DeleteDocumentsWithContext(ctx, ids)
	if err != nil {
		return fmt.Errorf("searchengine: batch delete %q: %w", uid, err)
	}
	if _, err = c.client.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("searchengine: wait batch delete %q: %w", uid, err)
	}
	return nil
}

// Stats returns document count for a spec index (0 when missing).
func (c *Client) Stats(ctx context.Context, spec string) (int64, error) {
	if !c.Enabled() {
		return 0, nil
	}
	stats, err := c.client.Index(c.indexUID(spec)).GetStatsWithContext(ctx)
	if err != nil {
		return 0, err
	}
	return stats.NumberOfDocuments, nil
}
