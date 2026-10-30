package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

type fakeAccess struct {
	visible map[uuid.UUID]bool
	err     error
	asked   db.FilterVisibleConversationUUIDsParams
}

func (f *fakeAccess) ListTeamIDsForUser(context.Context, db.ListTeamIDsForUserParams) ([]int64, error) {
	return []int64{7}, nil
}

func (f *fakeAccess) ConversationVisibleTo(context.Context, db.ConversationVisibleToParams) (bool, error) {
	return false, nil
}

func (f *fakeAccess) FilterVisibleConversationUUIDs(_ context.Context, arg db.FilterVisibleConversationUUIDsParams) ([]uuid.UUID, error) {
	f.asked = arg
	if f.err != nil {
		return nil, f.err
	}
	out := []uuid.UUID{}
	for _, id := range arg.Uuids {
		if f.visible[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func TestVisibleHitsDropsHiddenConversations(t *testing.T) {
	mine, hidden := uuid.New(), uuid.New()
	fa := &fakeAccess{visible: map[uuid.UUID]bool{mine: true}}
	svc := New(nil, registry(), slog.Default()).WithConversationAccess(fa)
	scope := orgctx.Scope{ID: 4, Permissions: []string{rbac.PermTenantConversationsRead}}
	hits := []searchengine.Hit{
		{Spec: "conversations", ID: mine.String(), ConversationID: mine.String()},
		{Spec: "messages", ID: uuid.NewString(), ConversationID: hidden.String(), Snippet: "secret"},
		{Spec: "messages", ID: uuid.NewString(), ConversationID: mine.String()},
		{Spec: "contacts", ID: uuid.NewString()},
		{Spec: "conversations", ID: hidden.String()},
	}
	got := svc.visibleHits(context.Background(), scope, 9, hits)
	if len(got) != 3 {
		t.Fatalf("kept %d hits, want 3: %+v", len(got), got)
	}
	for _, h := range got {
		if h.ConversationID == hidden.String() || h.ID == hidden.String() {
			t.Fatalf("hidden conversation leaked: %+v", h)
		}
	}
	if fa.asked.OrganizationID != 4 || fa.asked.ViewerID != 9 || fa.asked.SeeAll || len(fa.asked.ViewerTeamIds) != 1 {
		t.Fatalf("visibility query args = %+v", fa.asked)
	}
}

func TestVisibleHitsFailsClosed(t *testing.T) {
	scope := orgctx.Scope{ID: 4, Permissions: []string{rbac.PermTenantConversationsRead}}
	hits := func() []searchengine.Hit {
		return []searchengine.Hit{
			{Spec: "messages", ID: uuid.NewString(), ConversationID: uuid.NewString()},
			{Spec: "contacts", ID: uuid.NewString()},
		}
	}
	for name, svc := range map[string]*Service{
		"no access queries": New(nil, registry(), slog.Default()),
		"query error":       New(nil, registry(), slog.Default()).WithConversationAccess(&fakeAccess{err: errors.New("db down")}),
	} {
		got := svc.visibleHits(context.Background(), scope, 9, hits())
		if len(got) != 1 || got[0].Spec != "contacts" {
			t.Fatalf("%s: got %+v, want only the contact hit", name, got)
		}
	}
}
