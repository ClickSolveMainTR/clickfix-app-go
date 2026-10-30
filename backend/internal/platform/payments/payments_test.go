package payments_test

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/google/uuid"
)

func TestVaultBindsCardToOrganizationAndRow(t *testing.T) {
	vault, err := payments.NewCardVault(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	row := uuid.New()
	card := payments.SavedCard{OwnerKey: "owner", Token: "token"}
	owner, token, err := vault.Seal("org:1", row, card)
	if err != nil {
		t.Fatal(err)
	}
	o, c, err := vault.Open("org:1", row, owner, token)
	if err != nil || o != "owner" || c != "token" {
		t.Fatalf("roundtrip %s %s %v", o, c, err)
	}
	for _, tc := range []struct {
		name, scope  string
		row          uuid.UUID
		owner, token string
	}{
		{name: "other org", scope: "org:2", row: row, owner: owner, token: token},
		{name: "other row", scope: "org:1", row: uuid.New(), owner: owner, token: token},
		{name: "field substitution", scope: "org:1", row: row, owner: token, token: owner},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := vault.Open(tc.scope, tc.row, tc.owner, tc.token); err == nil {
				t.Fatal("ciphertext substitution accepted")
			}
		})
	}
	if vault.TokenHash("org:1", "fake", "token") == vault.TokenHash("org:2", "fake", "token") {
		t.Fatal("cross-scope index collision")
	}
}
func TestFakePartialRefundLimitsAndIdempotency(t *testing.T) {
	fake := payments.NewFake("https://example.test")
	ctx := context.Background()
	account := payments.Account{}
	if _, err := fake.ChargeSavedCard(ctx, account, payments.ChargeRequest{OrderRef: "charge", Amount: 100, Currency: "TRY", Card: payments.SavedCard{Token: "card"}}); err != nil {
		t.Fatal(err)
	}
	in := payments.RefundRequest{OrderRef: "charge", RefundRef: "partial", Amount: 60, Currency: "TRY"}
	for range 2 {
		if _, err := fake.Refund(ctx, account, in); err != nil {
			t.Fatal(err)
		}
	}
	in.RefundRef = "excess"
	in.Amount = 41
	if _, err := fake.Refund(ctx, account, in); err == nil {
		t.Fatal("excess refund accepted")
	}
	in.Amount = 40
	if _, err := fake.Refund(ctx, account, in); err != nil {
		t.Fatal(err)
	}
	reg := payments.NewRegistry(nil, true)
	if err := reg.Register(fake); err == nil {
		t.Fatal("fake registered in production")
	}
}
