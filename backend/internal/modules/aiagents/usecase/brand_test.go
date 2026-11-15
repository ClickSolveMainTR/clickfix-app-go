package usecase

import (
	"context"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// CF-214: similar conversations come from the conversation's brand first,
// then brandless ones; another brand never matches. The brand's ai_persona is
// what the customer AI receives.
func TestSimilarConversationsBrandFilterAndPersona(t *testing.T) {
	engine, _, org, _ := agentCoreFixture(t)
	svc := New(&repository.Store{Repository: engine.Repo.Billing}, engine, nil)
	ctx := context.Background()
	c, _ := conversationFixture(t, engine, org)
	pool := engine.Repo.Billing.Pool
	brand := func(name, persona string) pgtype.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx, "INSERT INTO brands(organization_id,name,ai_persona) VALUES($1,$2,$3) RETURNING uuid", org, name, persona).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return pgtype.UUID{Bytes: id, Valid: true}
	}
	brandA, brandB := brand("A", "Biz A markasıyız."), brand("B", "Biz B markasıyız.")
	insert := func(number int, b pgtype.UUID) int64 {
		var id int64
		if err := pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,contact_id,channel_type,source,subject,brand_id) VALUES($1,$2,$3,'web','inbound','Refund delayed',$4) RETURNING id", org, number, c.ContactID, b).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	sameBrand, generic, otherBrand := insert(11, brandA), insert(12, pgtype.UUID{}), insert(13, brandB)
	c.Subject = "Refund"
	c.BrandID = brandA
	similar, _, err := svc.enrichTriage(ctx, org, c, "Refund")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[uuid.UUID]bool{}
	for _, m := range similar {
		ids[m.UUID] = true
	}
	var sameUUID, genericUUID, otherUUID uuid.UUID
	for id, target := range map[int64]*uuid.UUID{sameBrand: &sameUUID, generic: &genericUUID, otherBrand: &otherUUID} {
		if err = pool.QueryRow(ctx, "SELECT uuid FROM conversations WHERE id=$1", id).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if len(similar) != 2 || similar[0].UUID != sameUUID || !ids[genericUUID] || ids[otherUUID] {
		t.Fatalf("brand A similar: %+v", similar)
	}
	c.BrandID = pgtype.UUID{}
	if similar, _, err = svc.enrichTriage(ctx, org, c, "Refund"); err != nil || len(similar) != 1 || similar[0].UUID != genericUUID {
		t.Fatalf("brandless similar: %+v %v", similar, err)
	}

	c.BrandID = brandA
	if got := svc.brandPersona(ctx, org, c); got != "Biz A markasıyız." {
		t.Fatalf("persona %q", got)
	}
	c.BrandID = pgtype.UUID{}
	if got := svc.brandPersona(ctx, org, c); got != "" {
		t.Fatalf("brandless persona %q", got)
	}
	// A brand UUID of another organization yields no persona.
	c.BrandID = pgtype.UUID{Bytes: uuid.New(), Valid: true}
	if got := svc.brandPersona(ctx, org+100000, c); got != "" {
		t.Fatalf("foreign persona %q", got)
	}
	c.BrandID = brandB
	if scope := conversationScope(org, c); scope.BrandID != brandB || scope.AllBrands || scope.OrganizationID != org {
		t.Fatalf("scope %+v", scope)
	}
}
