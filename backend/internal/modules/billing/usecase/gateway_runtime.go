package usecase

import (
	"context"
	"os"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/paytr"
	"github.com/google/uuid"
)

// DevelopmentGateways is retained for startup compatibility; it now wires live
// adapters too. Credentials come exclusively from environment, never API input.
func (s *Service) DevelopmentGateways(repo *repository.Repository, env, key, baseURL string) error {
	s.paymentFrontend = strings.TrimRight(baseURL, "/")
	production := strings.EqualFold(env, "production") || strings.EqualFold(env, "prod")
	reg := payments.NewRegistry(s, production)
	accounts := map[string]payments.Account{}
	for _, provider := range []string{"paytr", "iyzico"} {
		prefix := "PAYMENTS_" + strings.ToUpper(provider) + "_"
		if os.Getenv(prefix+"ENABLED") != "true" {
			continue
		}
		mode := os.Getenv(prefix + "MODE")
		if mode == "" {
			mode = "sandbox"
		}
		if mode != "live" && mode != "sandbox" {
			return payments.ErrNotConfigured
		}
		c := map[string]string{"success_url": baseURL + "/billing", "fail_url": baseURL + "/billing"}
		fields := []string{"merchant_id", "merchant_key", "merchant_salt"}
		if provider == "iyzico" {
			fields = []string{"api_key", "secret_key"}
		}
		for _, field := range fields {
			c[field] = os.Getenv(prefix + strings.ToUpper(field))
			if c[field] == "" {
				return payments.ErrNotConfigured
			}
		}
		// An account id identifies the merchant and mode across restarts. Rotation of
		// its secrets keeps the id; changing merchant requires a new explicit UUID.
		accountID := os.Getenv(prefix + "ACCOUNT_UUID")
		if accountID == "" {
			accountID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(provider+":"+mode+":"+c[fields[0]])).String()
		}
		if _, err := uuid.Parse(accountID); err != nil {
			return err
		}
		accounts[provider] = payments.Account{UUID: accountID, Mode: mode, Credentials: c}
		var adapter payments.Provider
		if provider == "paytr" {
			adapter = paytr.New("", nil)
		} else {
			adapter = iyzico.New("", nil)
		}
		if err := reg.Register(adapter); err != nil {
			return err
		}
	}
	if !production {
		if err := reg.Register(payments.NewFake(baseURL)); err != nil {
			return err
		}
		accounts["fake"] = payments.Account{UUID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("clickfix:fake")).String(), Mode: "sandbox", Credentials: map[string]string{"secret_key": key}}
	}
	if a, ok := accounts["iyzico"]; ok && os.Getenv("PAYMENTS_IYZICO_STATEMENTS_ENABLED") != "false" {
		s.SetStatementSource(&IyzicoStatementSource{Provider: iyzico.New("", nil), Account: a})
	}
	if len(accounts) == 0 {
		return nil
	}
	vault, err := payments.NewCardVault(key)
	if err != nil {
		return err
	}
	return s.ConfigurePayments(&TrialGateway{Executor: NewGatewayExecutor(repo, reg), Vault: vault, Account: func(_ context.Context, provider, merchant string) (payments.Account, error) {
		a, ok := accounts[provider]
		if !ok || (merchant != "" && merchant != a.UUID) {
			return payments.Account{}, payments.ErrNotConfigured
		}
		return a, nil
	}}, key, baseURL)
}
