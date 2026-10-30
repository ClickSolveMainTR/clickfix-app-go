package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	billing "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestReserveFreshPlanGrantAndExhaustion(t *testing.T) {
	s, _, org, actor := aiFixture(t)
	ctx := context.Background()
	in := db.AIInsertRunParams{OrganizationID: org, Uuid: uuid.New(), ActorID: pgtype.Int8{Int64: actor, Valid: true}, Task: "reply", Provider: "anthropic", Model: "test", Status: "running", Input: "test", RetrievedChunks: []byte("[]"), ReservedCredits: 1000, ReservedTokens: 100}
	if _, _, e := s.Repo.Reserve(ctx, in); e != nil {
		t.Fatal(e)
	}
	if e := s.Repo.Finish(ctx, db.AIFinishRunParams{OrganizationID: org, Uuid: in.Uuid, Status: "completed", ChargedCredits: 1000, InputTokens: 100, ToolCalls: []byte("[]")}); e != nil {
		t.Fatal(e)
	}
	in.Uuid = uuid.New()
	in.ReservedCredits = 1
	if _, _, e := s.Repo.Reserve(ctx, in); !errors.Is(e, model.ErrCredits) {
		t.Fatalf("expected AI_CREDITS_EXHAUSTED, got %v", e)
	}
	// Purchased credits remain usable after the included allowance is consumed.
	if _, e := billing.New(s.Repo.Billing).WalletCommand(ctx, org, billing.WalletInput{Unit: "ai_credits", Delta: 50, Reason: "Purchased credits", IdempotencyKey: "purchase:test"}); e != nil {
		t.Fatal(e)
	}
	in.Uuid = uuid.New()
	in.ReservedCredits = 50
	if _, _, e := s.Repo.Reserve(ctx, in); e != nil {
		t.Fatalf("purchased credits after plan allowance: %v", e)
	}
}
func TestUnlimitedPlanSkipsWalletAndFinishCannotMintCredits(t *testing.T) {
	s, _, org, actor := aiFixture(t, "unlimited")
	ctx := context.Background()
	in := db.AIInsertRunParams{OrganizationID: org, Uuid: uuid.New(), ActorID: pgtype.Int8{Int64: actor, Valid: true}, Task: "reply", Provider: "anthropic", Model: "test", Status: "running", Input: "test", RetrievedChunks: []byte("[]"), ReservedCredits: 1000000, ReservedTokens: 100}
	if _, _, e := s.Repo.Reserve(ctx, in); e != nil {
		t.Fatal(e)
	}
	if e := s.Repo.Finish(ctx, db.AIFinishRunParams{OrganizationID: org, Uuid: in.Uuid, Status: "completed", ChargedCredits: 1, InputTokens: 1, ToolCalls: []byte("[]")}); e != nil {
		t.Fatal(e)
	}
	balance, e := s.Repo.Billing.Q.AICreditBalance(ctx, org)
	if e != nil || balance != 0 {
		t.Fatal(balance, e)
	}
}
func TestMissingTokenEntitlementExplicit(t *testing.T) {
	s, _, org, actor := aiFixture(t, "missing-tokens")
	ctx := context.Background()
	_, _, e := s.Repo.Reserve(ctx, db.AIInsertRunParams{OrganizationID: org, Uuid: uuid.New(), ActorID: pgtype.Int8{Int64: actor, Valid: true}, ReservedCredits: 1, ReservedTokens: 1})
	var denied *entitlement.Error
	if !errors.As(e, &denied) || denied.Code != entitlement.CodeFeatureRequired || denied.Feature != "ai.tokens" {
		t.Fatalf("missing tokens: %v", e)
	}
}
