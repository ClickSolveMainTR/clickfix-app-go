package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/jackc/pgx/v5"
)

// CheckoutTTL bounds a hosted checkout page. iyzico Checkout Form tokens and
// PayTR iFrame tokens (timeout_limit) live 30 minutes; after this the payment
// is expired so the invoice can be paid again. A provider result that still
// arrives later settles the expired payment (any surplus goes to the wallet).
const CheckoutTTL = 45 * time.Minute

// MethodAvailability says whether one part of the paying flow is ready;
// Reason is the error code the API would answer otherwise.
type MethodAvailability struct {
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}

// CardAvailability is hosted card checkout. Provider and Mode name what a
// checkout would use (Mode "sandbox" means no real money moves).
type CardAvailability struct {
	MethodAvailability
	Provider string `json:"provider,omitempty"`
	Mode     string `json:"mode,omitempty"`
}

// PaymentAvailability lets the UI hide or disable purchases that would fail
// (CF-154). Ready: at least one method can pay an invoice of this organization.
type PaymentAvailability struct {
	Ready          bool               `json:"ready"`
	Card           CardAvailability   `json:"card"`
	BankTransfer   MethodAvailability `json:"bank_transfer"`
	Invoicing      MethodAvailability `json:"invoicing"`
	CreditPurchase MethodAvailability `json:"credit_purchase"`
}

func reason(err error) string {
	var coded *model.CodedError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &coded):
		return coded.Code
	case errors.Is(err, payments.ErrNotConfigured):
		return "PAYMENT_GATEWAY_NOT_CONFIGURED"
	default:
		return "UNAVAILABLE"
	}
}

// cardProvider resolves the provider and account a TRY checkout would use.
func (s *Service) cardProvider(ctx context.Context) (payments.Provider, payments.Account, error) {
	if s.gateway == nil {
		return nil, payments.Account{}, payments.ErrNotConfigured
	}
	p, err := s.gateway.Executor.Registry.Select(ctx, "TRY", "")
	if err != nil {
		return nil, payments.Account{}, err
	}
	a, err := s.gateway.Account(ctx, p.Code(), "")
	return p, a, err
}

func (s *Service) bankTransferReady(ctx context.Context) (bool, error) {
	rows, err := s.repo.Q.BillingBankAccounts(ctx, db.BillingBankAccountsParams{RowLimit: 1})
	return len(rows) > 0, err
}

// PaymentAvailability is read on every request: env and platform settings
// changes apply without a restart of the frontend.
func (s *Service) PaymentAvailability(ctx context.Context, org int64) (PaymentAvailability, error) {
	var out PaymentAvailability
	p, a, err := s.cardProvider(ctx)
	if err == nil {
		out.Card = CardAvailability{MethodAvailability: MethodAvailability{Ready: true}, Provider: p.Code(), Mode: strings.ToLower(a.Mode)}
	} else {
		out.Card.Reason = reason(err)
	}
	bank, err := s.bankTransferReady(ctx)
	if err != nil {
		return out, err
	}
	out.BankTransfer.Ready = bank
	if !bank {
		out.BankTransfer.Reason = "PAYMENT_METHOD_UNAVAILABLE"
	}
	out.Ready = out.Card.Ready || bank
	sub, err := s.repo.Q.BillingGetSubscription(ctx, org)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		out.Invoicing.Reason = model.ErrAccountMissing.Code
	case err != nil:
		return out, err
	default:
		if _, err = s.invoiceSeller(ctx, s.repo, sub); err != nil && !errors.Is(err, model.ErrSellerNotConfigured) {
			return out, err
		}
		out.Invoicing = MethodAvailability{Ready: err == nil, Reason: reason(err)}
	}
	out.CreditPurchase = out.Invoicing
	if out.Invoicing.Ready && !out.Ready {
		out.CreditPurchase = MethodAvailability{Reason: model.ErrNoPaymentMethod.Code}
	}
	return out, nil
}

// invoicePayable: card checkout or bank transfer can settle a new invoice.
func (s *Service) invoicePayable(ctx context.Context) (bool, error) {
	if _, _, err := s.cardProvider(ctx); err == nil {
		return true, nil
	} else if !errors.Is(err, payments.ErrNotConfigured) {
		return false, err
	}
	return s.bankTransferReady(ctx)
}

// SellerConfigured: BILLING_SELLER_JSON is set (invoices of organizations
// without an earlier invoice need it).
func (s *Service) SellerConfigured() bool { return s.seller.LegalName != "" }

// PaymentProviders lists the enabled card providers (startup diagnostics).
func (s *Service) PaymentProviders() []string {
	if s.gateway == nil {
		return []string{}
	}
	return s.gateway.Executor.Registry.Codes()
}
