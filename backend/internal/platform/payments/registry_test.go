package payments_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/iyzico"
)

type selection payments.Selection

func (s selection) PaymentSelection(context.Context) (payments.Selection, error) {
	return payments.Selection(s), nil
}

// CF-154: an empty platform default must not fail every checkout when one
// provider is enabled; naming a disabled provider is its own error.
func TestRegistrySelect(t *testing.T) {
	ctx := context.Background()
	empty := payments.NewRegistry(selection{}, true)
	if _, err := empty.Select(ctx, "TRY", ""); !errors.Is(err, payments.ErrNotConfigured) {
		t.Fatalf("nothing enabled %v", err)
	}
	reg := payments.NewRegistry(selection{Default: "paytr"}, true)
	if err := reg.Register(iyzico.New("", nil)); err != nil {
		t.Fatal(err)
	}
	p, err := reg.Select(ctx, "TRY", "")
	if err != nil || p.Code() != "iyzico" {
		t.Fatalf("fallback %v %v", p, err)
	}
	if _, err = reg.Select(ctx, "TRY", "paytr"); !errors.Is(err, payments.ErrProviderDisabled) || !errors.Is(err, payments.ErrNotConfigured) {
		t.Fatalf("explicit disabled %v", err)
	}
	if _, err = reg.Select(ctx, "GBP", ""); !errors.Is(err, payments.ErrNotConfigured) {
		t.Fatalf("currency %v", err)
	}
	if got := reg.Codes(); len(got) != 1 || got[0] != "iyzico" {
		t.Fatalf("codes %v", got)
	}
}
