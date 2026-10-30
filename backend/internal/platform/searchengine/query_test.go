package searchengine

import (
	"context"
	"slices"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
)

func TestOrgFilter(t *testing.T) {
	if got := OrgFilter(42); got != "organization_id = 42" {
		t.Fatalf("OrgFilter = %q", got)
	}
}

func TestRequestCarriesFilterAndCrop(t *testing.T) {
	c := &Client{cfg: config.SearchConfig{IndexPrefix: "cf"}}
	req := c.request("messages", Query{Q: "fatura", Limit: 10, Filter: OrgFilter(7), Snippet: true})
	if req.IndexUID != "cf_messages" || req.Filter != "organization_id = 7" || req.Limit != 10 {
		t.Fatalf("request = %+v", req)
	}
	if !slices.Equal(req.AttributesToCrop, []string{"body"}) || req.CropLength != snippetWords {
		t.Fatalf("crop = %v / %d", req.AttributesToCrop, req.CropLength)
	}
	plain := c.request("users", Query{Q: "x", Limit: 5})
	if plain.Filter != nil || len(plain.AttributesToCrop) != 0 {
		t.Fatalf("platform request must not filter or crop: %+v", plain)
	}
}

func TestFilterableAttributesTenant(t *testing.T) {
	if got := filterableAttributes(Spec{ID: "users"}); slices.Contains(got, "organization_id") {
		t.Fatalf("platform spec filterable = %v", got)
	}
	got := filterableAttributes(Spec{ID: "messages", Tenant: true, Filterable: []string{"private"}})
	if !slices.Equal(got, []string{"spec", "private", "organization_id"}) {
		t.Fatalf("tenant spec filterable = %v", got)
	}
	if slices.Contains(displayedAttributes, "organization_id") {
		t.Fatal("organization_id must not be returned with hits")
	}
}

func TestInterleave(t *testing.T) {
	a := []Hit{{ID: "a1"}, {ID: "a2"}, {ID: "a3"}}
	b := []Hit{{ID: "b1"}}
	got := interleave([][]Hit{a, b}, 3)
	ids := []string{}
	for _, h := range got {
		ids = append(ids, h.ID)
	}
	if !slices.Equal(ids, []string{"a1", "b1", "a2"}) {
		t.Fatalf("interleave = %v", ids)
	}
	if n := len(interleave([][]Hit{a, b}, 10)); n != 4 {
		t.Fatalf("interleave all = %d", n)
	}
}

func TestMapHitSnippet(t *testing.T) {
	h := mapHit("messages", map[string]interface{}{
		"id": "m1", "title": "#7 · Ayşe", "subtitle": "Faturam…", "href": "/inbox?c=x",
		"conversation_id": "c1", "private": true, "_rankingScore": 0.9,
		"_formatted": map[string]interface{}{"body": "…faturam hâlâ gelmedi…"},
	})
	if h.ID != "m1" || h.ConversationID != "c1" || !h.Private || h.Snippet != "…faturam hâlâ gelmedi…" || h.Score != 0.9 {
		t.Fatalf("hit = %+v", h)
	}
	if got := mapHit("users", "bad"); got.Spec != "users" || got.ID != "" {
		t.Fatalf("bad hit = %+v", got)
	}
}

func TestDisabledClientIsNoop(t *testing.T) {
	var c *Client
	hits, err := c.Query(context.Background(), Query{Specs: []string{"messages"}, Q: "x"})
	if err != nil || hits != nil {
		t.Fatalf("disabled query = %v, %v", hits, err)
	}
	var ix *Indexer
	ix.EnqueueUpsert(context.Background(), "messages", "id") // must not panic
	if counts, err := ix.Rebuild(context.Background(), "", false); err != nil || len(counts) != 0 {
		t.Fatalf("disabled rebuild = %v, %v", counts, err)
	}
}

type fakeAdapter struct{ spec Spec }

func (f fakeAdapter) Spec() Spec                                  { return f.spec }
func (f fakeAdapter) ListAll(context.Context) ([]Document, error) { return nil, nil }
func (f fakeAdapter) Document(context.Context, string) (Document, error) {
	return Document{}, ErrNotFound
}

func TestRegistryKeepsOrder(t *testing.T) {
	reg := NewRegistry(fakeAdapter{Spec{ID: "users"}}, nil, fakeAdapter{Spec{ID: "conversations"}}, fakeAdapter{Spec{ID: "messages"}})
	if got := reg.SpecIDs(); !slices.Equal(got, []string{"users", "conversations", "messages"}) {
		t.Fatalf("order = %v", got)
	}
	if _, err := reg.Get("nope"); err == nil {
		t.Fatal("unknown spec must fail")
	}
}
