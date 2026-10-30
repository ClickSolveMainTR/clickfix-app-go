package inbox

import (
	"context"

	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine/adapters"
	"github.com/google/uuid"
)

// inboxMatchLimit bounds the conversations one inbox search takes from the
// index; the database applies the inbox filters and paging on top.
const inboxMatchLimit = 100

// searchHook enqueues index upserts for the records an inbox change touched
// (CF-45). EnqueueUpsert is fail-soft.
type searchHook struct{ ix *searchengine.Indexer }

func (h searchHook) ConversationChanged(ctx context.Context, id uuid.UUID) {
	h.ix.EnqueueUpsert(ctx, searchadapters.SpecConversations, id.String())
}

func (h searchHook) MessageChanged(ctx context.Context, id uuid.UUID) {
	h.ix.EnqueueUpsert(ctx, searchadapters.SpecMessages, id.String())
}

func (h searchHook) ContactChanged(ctx context.Context, id uuid.UUID) {
	h.ix.EnqueueUpsert(ctx, searchadapters.SpecContacts, id.String())
}

// searchFinder matches inbox search text in the organization's conversation
// and message indexes (private notes included: every agent reads them).
type searchFinder struct{ c *searchengine.Client }

func (f searchFinder) MatchConversations(ctx context.Context, orgID int64, q string) ([]convusecase.SearchMatch, error) {
	hits, err := f.c.Query(ctx, searchengine.Query{
		Specs:   []string{searchadapters.SpecMessages, searchadapters.SpecConversations},
		Q:       q,
		Limit:   inboxMatchLimit,
		Filter:  searchengine.OrgFilter(orgID),
		Snippet: true,
	})
	if err != nil {
		return nil, err
	}
	return MatchesFromHits(hits), nil
}

// MatchesFromHits maps index hits to conversation matches: every hit names
// its conversation (conversation_id); message hits carry the matched text.
func MatchesFromHits(hits []searchengine.Hit) []convusecase.SearchMatch {
	out := make([]convusecase.SearchMatch, 0, len(hits))
	for _, h := range hits {
		id, err := uuid.Parse(h.ConversationID)
		if err != nil {
			continue
		}
		m := convusecase.SearchMatch{ConversationUUID: id}
		if h.Spec == searchadapters.SpecMessages {
			m.Snippet = h.Snippet
			if m.Snippet == "" {
				m.Snippet = h.Subtitle
			}
			m.Private = h.Private
		}
		out = append(out, m)
	}
	return out
}
