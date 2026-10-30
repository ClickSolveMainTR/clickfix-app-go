package inbox_test

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/inbox"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine/adapters"
	"github.com/google/uuid"
)

type fakeIndexer struct {
	mu      sync.Mutex
	touched map[string][]uuid.UUID
}

func (f *fakeIndexer) add(kind string, id uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched[kind] = append(f.touched[kind], id)
}
func (f *fakeIndexer) ConversationChanged(_ context.Context, id uuid.UUID) { f.add("conversation", id) }
func (f *fakeIndexer) MessageChanged(_ context.Context, id uuid.UUID)      { f.add("message", id) }
func (f *fakeIndexer) ContactChanged(_ context.Context, id uuid.UUID)      { f.add("contact", id) }

type fakeFinder struct{ matches []convusecase.SearchMatch }

func (f fakeFinder) MatchConversations(context.Context, int64, string) ([]convusecase.SearchMatch, error) {
	return f.matches, nil
}

func TestSearchHooksAndIndexMatches(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	ix := &fakeIndexer{touched: map[string][]uuid.UUID{}}
	finder := &fakeFinder{}
	e.ib.Conversations.WithSearch(ix, finder)

	if code := e.post(t, `{"messages":[{"id":"s-1","from":{"id":"v-s","name":"Deniz Ak","email":"deniz@example.test"},"text":"Kargo numaram KRG-778 hâlâ gelmedi"}]}`, ""); code != http.StatusOK {
		t.Fatalf("webhook = %d", code)
	}
	var convUUID, msgUUID, contactUUID uuid.UUID
	if err := e.pool.QueryRow(ctx, `SELECT c.uuid, m.uuid, ct.uuid FROM conversations c
		JOIN messages m ON m.conversation_id = c.id JOIN contacts ct ON ct.id = c.contact_id
		WHERE c.organization_id = $1`, e.org).Scan(&convUUID, &msgUUID, &contactUUID); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ix.touched["conversation"], convUUID) || !slices.Contains(ix.touched["message"], msgUUID) || !slices.Contains(ix.touched["contact"], contactUUID) {
		t.Fatalf("inbound change not indexed: %v", ix.touched)
	}

	note, err := e.ib.Conversations.Post(ctx, convusecase.PostInput{
		OrganizationID: e.org, ConversationUUID: convUUID, AuthorID: e.agent, Content: "İade onayı muhasebeden bekleniyor", Private: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ix.touched["message"], note.Message.UUID) {
		t.Fatalf("note not indexed: %v", ix.touched["message"])
	}

	// The adapters read the same rows back with the organization attached.
	q := db.New(e.pool)
	noteDoc, err := searchadapters.NewMessages(q).Document(ctx, note.Message.UUID.String())
	if err != nil {
		t.Fatal(err)
	}
	if !noteDoc.Private || noteDoc.OrganizationID != e.org || noteDoc.ConversationID != convUUID.String() || noteDoc.Body != "İade onayı muhasebeden bekleniyor" {
		t.Fatalf("note doc = %+v", noteDoc)
	}
	convDoc, err := searchadapters.NewConversations(q).Document(ctx, convUUID.String())
	if err != nil || convDoc.OrganizationID != e.org || !slices.Contains(convDoc.Keywords, "deniz@example.test") {
		t.Fatalf("conversation doc = %+v, %v", convDoc, err)
	}
	contactDoc, err := searchadapters.NewContacts(q).Document(ctx, contactUUID.String())
	if err != nil || contactDoc.OrganizationID != e.org || contactDoc.Title != "Deniz Ak" {
		t.Fatalf("contact doc = %+v, %v", contactDoc, err)
	}

	// Timeline events are not indexed: their lookup reports not found.
	status := "pending"
	if _, err := e.ib.Conversations.Update(ctx, convusecase.UpdateInput{OrganizationID: e.org, ConversationUUID: convUUID, Actor: convusecase.Actor{ID: e.agent}, Status: &status}); err != nil {
		t.Fatal(err)
	}
	var eventUUID uuid.UUID
	if err := e.pool.QueryRow(ctx, "SELECT uuid FROM messages WHERE organization_id = $1 AND kind = 'event' LIMIT 1", e.org).Scan(&eventUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := searchadapters.NewMessages(q).Document(ctx, eventUUID.String()); err == nil {
		t.Fatal("timeline event must not be indexed")
	}

	// The note text is not in subject/preview/contact: only the index finds it.
	list := func() ([]uuid.UUID, string) {
		items, _, err := e.ib.Conversations.List(ctx, e.org, e.agent, convusecase.ListFilter{Q: "muhasebeden", Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		out, snippet := []uuid.UUID{}, ""
		for _, c := range items {
			out = append(out, c.UUID)
			if c.SearchMatch != nil {
				snippet = c.SearchMatch.Snippet
			}
		}
		return out, snippet
	}
	if got, _ := list(); len(got) != 0 {
		t.Fatalf("database search found the note text: %v", got)
	}
	finder.matches = []convusecase.SearchMatch{
		{ConversationUUID: convUUID, Snippet: "…onayı muhasebeden bekleniyor", Private: true},
		{ConversationUUID: uuid.New()}, // another organization's id: the DB filter drops it
	}
	got, snippet := list()
	if !slices.Equal(got, []uuid.UUID{convUUID}) || snippet != "…onayı muhasebeden bekleniyor" {
		t.Fatalf("index match = %v snippet %q", got, snippet)
	}
	// Index matches are still filtered by the inbox filters.
	items, _, err := e.ib.Conversations.List(ctx, e.org, e.agent, convusecase.ListFilter{Q: "muhasebeden", Statuses: []string{"resolved"}, Limit: 20})
	if err != nil || len(items) != 0 {
		t.Fatalf("status filter ignored: %d, %v", len(items), err)
	}
}

func TestMatchesFromHits(t *testing.T) {
	conv := uuid.New()
	got := inbox.MatchesFromHits([]searchengine.Hit{
		{Spec: searchadapters.SpecMessages, ConversationID: conv.String(), Snippet: "…kargo…", Private: true},
		{Spec: searchadapters.SpecConversations, ConversationID: conv.String(), Subtitle: "ignored"},
		{Spec: searchadapters.SpecMessages, ConversationID: "bad"},
		{Spec: searchadapters.SpecMessages, ConversationID: conv.String(), Subtitle: "fallback"},
	})
	if len(got) != 3 {
		t.Fatalf("matches = %+v", got)
	}
	if got[0].Snippet != "…kargo…" || !got[0].Private || got[1].Snippet != "" || got[2].Snippet != "fallback" {
		t.Fatalf("matches = %+v", got)
	}
}
