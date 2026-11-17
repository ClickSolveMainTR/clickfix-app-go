package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/vectorstore"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type scopeRecorder struct{ scopes []vectorstore.Scope }

func (r *scopeRecorder) SearchScoped(_ context.Context, scope vectorstore.Scope, _, _ string, _ bool) ([]kbmodel.Citation, error) {
	r.scopes = append(r.scopes, scope)
	return []kbmodel.Citation{}, nil
}

// CF-214: the brand persona reaches the system prompt (not the untrusted user
// turn) and retrieval runs in the conversation's brand scope; the run's own
// organization always wins over the scope's.
func TestRunAgentBrandPersonaAndScope(t *testing.T) {
	svc, fake, org, actor := aiFixture(t)
	rec := &scopeRecorder{}
	svc.RAG = rec
	brand := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	scope := vectorstore.Conversation(org+1000, brand, pgtype.Int8{})
	persona := "Adımız Acme Destek; samimi ve kısa yazarız."
	_, err := svc.RunAgent(context.Background(), org, actor, model.RunInput{Task: "reply", Input: "Siparişim nerede?", Retrieve: true}, AgentOptions{Scope: &scope, BrandPersona: persona})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.instructions, "Brand persona") || !strings.Contains(fake.instructions, persona) {
		t.Fatalf("persona missing from system prompt: %q", fake.instructions)
	}
	if strings.Contains(fake.input, persona) {
		t.Fatal("persona must not be sent as the customer turn")
	}
	if len(rec.scopes) != 1 {
		t.Fatalf("retrieval calls %d", len(rec.scopes))
	}
	got := rec.scopes[0]
	if got.OrganizationID != org || got.BrandID != brand || got.AllBrands {
		t.Fatalf("scope %+v", got)
	}

	// Without options the run searches the whole organization and has no persona.
	rec.scopes = nil
	if _, err = svc.RunAgent(context.Background(), org, actor, model.RunInput{Task: "reply", Input: "Merhaba", Retrieve: true}, AgentOptions{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fake.instructions, "Brand persona") {
		t.Fatal("persona leaked into an unbranded run")
	}
	if len(rec.scopes) != 1 || !rec.scopes[0].AllBrands || rec.scopes[0].OrganizationID != org {
		t.Fatalf("default scope %+v", rec.scopes)
	}

	if _, err = svc.RunAgent(context.Background(), org, actor, model.RunInput{Task: "reply", Input: "x"}, AgentOptions{BrandPersona: strings.Repeat("a", 4001)}); err != model.ErrInvalid {
		t.Fatalf("oversized persona %v", err)
	}
}
