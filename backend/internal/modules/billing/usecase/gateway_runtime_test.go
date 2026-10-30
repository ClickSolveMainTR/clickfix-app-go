package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
)

func TestProductionRegistersConfiguredGatewaysAndBindsMerchant(t *testing.T) {
	for k, v := range map[string]string{
		"PAYMENTS_PAYTR_ENABLED": "true", "PAYMENTS_PAYTR_MERCHANT_ID": "123456", "PAYMENTS_PAYTR_MERCHANT_KEY": "fixture-key", "PAYMENTS_PAYTR_MERCHANT_SALT": "fixture-salt", "PAYMENTS_PAYTR_MODE": "sandbox", "PAYMENTS_PAYTR_ACCOUNT_UUID": "",
		"PAYMENTS_IYZICO_ENABLED": "true", "PAYMENTS_IYZICO_API_KEY": "fixture-api", "PAYMENTS_IYZICO_SECRET_KEY": "fixture-secret", "PAYMENTS_IYZICO_MODE": "sandbox", "PAYMENTS_IYZICO_ACCOUNT_UUID": "",
	} {
		t.Setenv(k, v)
	}
	s := New(nil)
	if err := s.DevelopmentGateways(nil, "production", "01234567890123456789012345678901", "https://example.test"); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"paytr", "iyzico"} {
		if _, err := s.gateway.Executor.Registry.Get(code); err != nil {
			t.Fatal(err)
		}
		a, err := s.gateway.Account(context.Background(), code, "")
		if err != nil || a.UUID == "" {
			t.Fatalf("account %+v %v", a, err)
		}
		if _, err = s.gateway.Account(context.Background(), code, "different-merchant"); !errors.Is(err, payments.ErrNotConfigured) {
			t.Fatalf("merchant %v", err)
		}
	}
	if _, err := s.gateway.Executor.Registry.Get("fake"); !errors.Is(err, payments.ErrNotConfigured) {
		t.Fatalf("production fake %v", err)
	}
}
