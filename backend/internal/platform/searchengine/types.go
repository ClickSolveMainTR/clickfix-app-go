package searchengine

import (
	"context"
	"errors"
)

// ErrNotFound: the entity no longer exists (or is no longer indexable). The
// indexer removes its document instead of upserting.
var ErrNotFound = errors.New("searchengine: document not found")

// Spec describes one searchable resource catalog entry.
type Spec struct {
	ID         string   `json:"id"`
	LabelKey   string   `json:"label_key"`
	Permission string   `json:"permission,omitempty"`
	Icon       string   `json:"icon,omitempty"`
	Searchable []string `json:"searchable_fields,omitempty"`
	Filterable []string `json:"filterable_fields,omitempty"`
	// Tenant specs index organization data: every document carries
	// organization_id, every query is filtered by the caller's organization
	// and Permission is a tenant permission. They are served by
	// /v1/tenant/search only, never by the platform /v1/search.
	Tenant bool `json:"tenant,omitempty"`
}

// Document is the Meilisearch index payload for one entity.
type Document struct {
	ID       string   `json:"id"`
	Spec     string   `json:"spec"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
	Href     string   `json:"href"`
	Icon     string   `json:"icon,omitempty"`
	// OrganizationID scopes tenant documents (required for Tenant specs).
	OrganizationID int64 `json:"organization_id,omitempty"`
	// Body is long searchable text (message content, already truncated);
	// hits return a cropped snippet of it.
	Body string `json:"body,omitempty"`
	// ConversationID links a message hit to its conversation uuid.
	ConversationID string `json:"conversation_id,omitempty"`
	// Private marks internal notes (visible to the organization's agents).
	Private bool `json:"private,omitempty"`
}

// Hit is a normalized search result returned to clients.
type Hit struct {
	Spec           string  `json:"spec"`
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Subtitle       string  `json:"subtitle,omitempty"`
	Href           string  `json:"href"`
	Icon           string  `json:"icon,omitempty"`
	Score          float64 `json:"score,omitempty"`
	Snippet        string  `json:"snippet,omitempty"`
	ConversationID string  `json:"conversation_id,omitempty"`
	Private        bool    `json:"private,omitempty"`
}

// Adapter connects searchengine to a list resource.
type Adapter interface {
	Spec() Spec
	ListAll(ctx context.Context) ([]Document, error)
	Document(ctx context.Context, id string) (Document, error)
}

// BatchLister is implemented by adapters whose tables are too large to load
// at once (messages). The reindex walks them batch by batch instead of
// calling ListAll.
type BatchLister interface {
	ListBatches(ctx context.Context, size int, fn func([]Document) error) error
}
