package searchengine

import (
	"context"
	"fmt"
	"strings"

	"github.com/meilisearch/meilisearch-go"
)

// Query limits.
const (
	// DefaultLimit is the hit count when a query names none.
	DefaultLimit = 20
	// MaxPaletteLimit caps client supplied limits (/v1/search, /v1/tenant/search).
	MaxPaletteLimit = 50
	// MaxQueryLimit caps server side queries (inbox conversation matching).
	MaxQueryLimit = 200
	// snippetWords is the crop length of Body snippets.
	snippetWords = 24
)

// Query is one search over one or more spec indexes.
type Query struct {
	Specs []string
	Q     string
	Limit int
	// Filter is a Meilisearch filter expression applied to every index
	// (OrgFilter for tenant specs).
	Filter string
	// Snippet crops Body around the match into Hit.Snippet.
	Snippet bool
}

// OrgFilter is the mandatory filter of tenant queries.
func OrgFilter(orgID int64) string {
	return fmt.Sprintf("organization_id = %d", orgID)
}

// Search queries one or more spec indexes (platform palette; no filter).
func (c *Client) Search(ctx context.Context, specs []string, q string, limit int) ([]Hit, error) {
	if limit > MaxPaletteLimit {
		limit = MaxPaletteLimit
	}
	return c.Query(ctx, Query{Specs: specs, Q: q, Limit: limit})
}

// Query runs q. Hits of several indexes are interleaved (round robin) so one
// large index does not crowd the others out of the limit.
func (c *Client) Query(ctx context.Context, q Query) ([]Hit, error) {
	if !c.Enabled() {
		return nil, nil
	}
	q.Q = strings.TrimSpace(q.Q)
	if q.Q == "" || len(q.Specs) == 0 {
		return nil, nil
	}
	if q.Limit <= 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit > MaxQueryLimit {
		q.Limit = MaxQueryLimit
	}

	queries := make([]*meilisearch.SearchRequest, 0, len(q.Specs))
	for _, spec := range q.Specs {
		queries = append(queries, c.request(spec, q))
	}
	if len(queries) == 1 {
		req := queries[0]
		resp, err := c.client.Index(req.IndexUID).SearchWithContext(ctx, q.Q, req)
		if err != nil {
			return nil, fmt.Errorf("searchengine: search %q: %w", q.Specs[0], err)
		}
		return mapHits(q.Specs[0], resp.Hits), nil
	}
	resp, err := c.client.MultiSearchWithContext(ctx, &meilisearch.MultiSearchRequest{Queries: queries})
	if err != nil {
		return nil, fmt.Errorf("searchengine: multi search: %w", err)
	}
	groups := make([][]Hit, 0, len(resp.Results))
	for i, result := range resp.Results {
		if i >= len(q.Specs) {
			break
		}
		groups = append(groups, mapHits(q.Specs[i], result.Hits))
	}
	return interleave(groups, q.Limit), nil
}

func (c *Client) request(spec string, q Query) *meilisearch.SearchRequest {
	req := &meilisearch.SearchRequest{
		IndexUID: c.indexUID(spec),
		Query:    q.Q,
		Limit:    int64(q.Limit),
	}
	if q.Filter != "" {
		req.Filter = q.Filter
	}
	if q.Snippet {
		req.AttributesToCrop = []string{"body"}
		req.CropLength = snippetWords
	}
	return req
}

// interleave merges per-index hit lists round robin up to limit.
func interleave(groups [][]Hit, limit int) []Hit {
	out := make([]Hit, 0, limit)
	for i := 0; len(out) < limit; i++ {
		added := false
		for _, g := range groups {
			if i < len(g) {
				out = append(out, g[i])
				added = true
				if len(out) >= limit {
					return out
				}
			}
		}
		if !added {
			break
		}
	}
	return out
}

func mapHits(spec string, hits []interface{}) []Hit {
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, mapHit(spec, h))
	}
	return out
}

func mapHit(spec string, hit interface{}) Hit {
	row, ok := hit.(map[string]interface{})
	if !ok {
		return Hit{Spec: spec}
	}
	out := Hit{
		Spec:           spec,
		ID:             str(row, "id"),
		Title:          str(row, "title"),
		Subtitle:       str(row, "subtitle"),
		Href:           str(row, "href"),
		Icon:           str(row, "icon"),
		ConversationID: str(row, "conversation_id"),
	}
	if v, ok := row["private"].(bool); ok {
		out.Private = v
	}
	if v, ok := row["_rankingScore"].(float64); ok {
		out.Score = v
	}
	if f, ok := row["_formatted"].(map[string]interface{}); ok {
		out.Snippet = str(f, "body")
	}
	return out
}

func str(row map[string]interface{}, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// displayedAttributes are returned with hits; organization_id is filter only.
var displayedAttributes = []string{"id", "spec", "title", "subtitle", "keywords", "href", "icon", "body", "conversation_id", "private"}

// filterableAttributes: spec, the adapter's own and organization_id for
// tenant specs.
func filterableAttributes(spec Spec) []string {
	out := append([]string{"spec"}, spec.Filterable...)
	if spec.Tenant {
		out = append(out, "organization_id")
	}
	return out
}

// DeleteAll empties a spec index (fresh rebuild).
func (c *Client) DeleteAll(ctx context.Context, spec string) error {
	if !c.Enabled() {
		return nil
	}
	uid := c.indexUID(spec)
	task, err := c.client.Index(uid).DeleteAllDocumentsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("searchengine: delete all %q: %w", uid, err)
	}
	if _, err := c.client.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("searchengine: wait delete all %q: %w", uid, err)
	}
	return nil
}
