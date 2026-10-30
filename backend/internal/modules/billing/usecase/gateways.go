package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/jackc/pgx/v5"
)

const gatewayPersistTimeout = 5 * time.Second

func (s *Service) PaymentSelection(ctx context.Context) (payments.Selection, error) {
	cfg, err := s.repo.Q.BillingGatewaySettings(ctx)
	out := payments.Selection{Default: cfg.DefaultProvider, Currencies: map[string]string{}}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(cfg.CurrencyProviders, &out.Currencies)
	return out, err
}

// GatewayExecutor persists an attempt before contacting a provider. A crash,
// timeout or ambiguous result stays pending/unknown and is never charged again.
// Callers must reconcile using ChargeChecker/RefundChecker before proceeding.
type attemptInput struct {
	Account string
	Request any
}

type GatewayExecutor struct {
	repo     *repository.Repository
	Registry *payments.Registry
}

func NewGatewayExecutor(r *repository.Repository, reg *payments.Registry) *GatewayExecutor {
	return &GatewayExecutor{repo: r, Registry: reg}
}
func (g *GatewayExecutor) execute(ctx context.Context, org int64, key, operation, provider string, input any, call func() (any, error), out any) error {
	if org <= 0 || strings.TrimSpace(key) == "" || len(key) > 200 {
		return model.ErrInvalid
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(hash[:])
	var attempt db.BillingPaymentAttempt
	existing := false
	err = g.repo.Within(ctx, func(r *repository.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		row, e := r.Q.BillingAttempt(ctx, db.BillingAttemptParams{OrganizationID: org, IdempotencyKey: key})
		if e == nil {
			if row.RequestHash != fingerprint || row.Provider != provider || row.Operation != operation {
				return model.ErrConflict
			}
			attempt = row
			existing = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		attempt, e = r.Q.BillingCreateAttempt(ctx, db.BillingCreateAttemptParams{OrganizationID: org, IdempotencyKey: key, Provider: provider, Operation: operation, RequestHash: fingerprint})
		return e
	})
	if err != nil {
		return err
	}
	if existing {
		if attempt.Status == "succeeded" {
			return json.Unmarshal(attempt.Result, out)
		}
		if attempt.Status == "failed" {
			return &payments.ProviderError{Code: attempt.FailureCode}
		}
		return model.ErrConflict
	}
	result, callErr := call()
	status, code := "succeeded", ""
	var body []byte
	if callErr != nil {
		status = "unknown"
		var refusal *payments.ProviderError
		if errors.As(callErr, &refusal) && !refusal.Temporary {
			status = "failed"
			code = refusal.Code
		}
	} else {
		body, err = json.Marshal(result)
		if err != nil {
			return err
		}
	}
	// If the caller timed out, preserve the unknown attempt using a fresh bounded context.
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), gatewayPersistTimeout)
	defer cancel()
	if err = g.repo.Q.BillingFinishAttempt(persist, db.BillingFinishAttemptParams{OrganizationID: org, ID: attempt.ID, Status: status, Result: body, FailureCode: code}); err != nil {
		return err
	}
	if callErr != nil {
		return callErr
	}
	return json.Unmarshal(body, out)
}
func (g *GatewayExecutor) Checkout(ctx context.Context, org int64, key string, p payments.Provider, a payments.Account, in payments.CheckoutRequest, threeDS bool) (payments.Checkout, error) {
	var out payments.Checkout
	if in.Amount <= 0 {
		return out, model.ErrInvalid
	}
	in.OrderRef = OrderReference(org, key)
	operation := "checkout"
	if threeDS {
		operation = "three_ds"
	}
	err := g.execute(ctx, org, key, operation, p.Code(), attemptInput{Account: a.UUID, Request: in}, func() (any, error) {
		if threeDS {
			adapter, ok := p.(payments.ThreeDSInitializer)
			if !ok {
				return nil, &payments.ProviderError{Code: "unsupported"}
			}
			return adapter.InitThreeDS(ctx, a, in)
		}
		return p.CreateCheckout(ctx, a, in)
	}, &out)
	return out, err
}
func (g *GatewayExecutor) Charge(ctx context.Context, org int64, key string, p payments.Provider, a payments.Account, in payments.ChargeRequest) (payments.ChargeResult, error) {
	var out payments.ChargeResult
	adapter, ok := payments.CardChargerOf(p)
	if !ok {
		return out, payments.ErrUnsupported
	}
	if in.Amount <= 0 {
		return out, model.ErrInvalid
	}
	in.OrderRef = OrderReference(org, key)
	err := g.execute(ctx, org, key, "recurring", p.Code(), attemptInput{Account: a.UUID, Request: in}, func() (any, error) {
		result, err := adapter.ChargeSavedCard(ctx, a, in)
		if err == nil && result.PaidAmount < in.Amount {
			return nil, &payments.ProviderError{Code: "amount_mismatch", Temporary: true}
		}
		return result, err
	}, &out)
	return out, err
}
func (g *GatewayExecutor) Refund(ctx context.Context, org int64, key string, p payments.Provider, a payments.Account, in payments.RefundRequest) (payments.RefundResult, error) {
	var out payments.RefundResult
	adapter, ok := payments.RefunderOf(p)
	if !ok {
		return out, payments.ErrUnsupported
	}
	if in.Amount <= 0 {
		return out, model.ErrInvalid
	}
	in.RefundRef = OrderReference(org, key)
	err := g.execute(ctx, org, key, "refund", p.Code(), attemptInput{Account: a.UUID, Request: in}, func() (any, error) { return adapter.Refund(ctx, a, in) }, &out)
	return out, err
}

// OrderReference is the stable alphanumeric provider reference for a logical attempt.
// Persist it with the payment so later refunds/status lookups can name the order.
func OrderReference(org int64, key string) string {
	raw, _ := json.Marshal([]any{org, key})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:16])
}

func (g *GatewayExecutor) Tokenize(ctx context.Context, org int64, key string, p payments.Provider, a payments.Account, in payments.CheckoutRequest) (payments.Checkout, error) {
	var out payments.Checkout
	adapter, ok := p.(payments.CardTokenizer)
	if !ok {
		return out, payments.ErrUnsupported
	}
	in.OrderRef = OrderReference(org, key)
	err := g.execute(ctx, org, key, "tokenize", p.Code(), attemptInput{Account: a.UUID, Request: in}, func() (any, error) { return adapter.TokenizeCard(ctx, a, in) }, &out)
	return out, err
}
func (g *GatewayExecutor) RemoveCard(ctx context.Context, org int64, key string, p payments.Provider, a payments.Account, card payments.SavedCard) error {
	adapter, ok := payments.CardRemoverOf(p)
	if !ok {
		return payments.ErrUnsupported
	}
	var out struct{}
	return g.execute(ctx, org, key, "remove_card", p.Code(), attemptInput{Account: a.UUID, Request: card}, func() (any, error) { return struct{}{}, adapter.RemoveSavedCard(ctx, a, card) }, &out)
}

// VerifyCallback never trusts parsed data until signature verification succeeds.
// Settlement callers must additionally match order, merchant, currency and amount.
func (g *GatewayExecutor) VerifyCallback(p payments.Provider, a payments.Account, header http.Header, body []byte) (payments.Notification, error) {
	n, err := p.ParseNotification(header, body)
	if err != nil {
		return payments.Notification{}, err
	}
	if err = p.VerifyNotification(a, n, header, body); err != nil {
		return payments.Notification{}, err
	}
	return n, nil
}

// ReconcileCharge only queries the provider; it never submits a second charge.
func (g *GatewayExecutor) ReconcileCharge(ctx context.Context, org int64, key string, p payments.Provider, a payments.Account, in payments.ChargeRequest) (payments.ChargeResult, error) {
	var out payments.ChargeResult
	checker, ok := payments.ChargeCheckerOf(p)
	if !ok {
		return out, payments.ErrUnsupported
	}
	in.OrderRef = OrderReference(org, key)
	raw, err := json.Marshal(attemptInput{Account: a.UUID, Request: in})
	if err != nil {
		return out, err
	}
	hash := sha256.Sum256(raw)
	attempt, err := g.repo.Q.BillingAttempt(ctx, db.BillingAttemptParams{OrganizationID: org, IdempotencyKey: key})
	if err != nil {
		return out, err
	}
	if attempt.Provider != p.Code() || attempt.Operation != "recurring" || attempt.RequestHash != hex.EncodeToString(hash[:]) {
		return out, model.ErrConflict
	}
	if attempt.Status == "succeeded" {
		err = json.Unmarshal(attempt.Result, &out)
		return out, err
	}
	if attempt.Status == "failed" {
		return out, &payments.ProviderError{Code: attempt.FailureCode}
	}
	n, err := checker.CheckCharge(ctx, a, in)
	if err != nil {
		return out, err
	}
	if n.Status == payments.StatusFailed && n.OrderRef == in.OrderRef {
		err = g.repo.Q.BillingFinishAttempt(ctx, db.BillingFinishAttemptParams{OrganizationID: org, ID: attempt.ID, Status: "failed", FailureCode: n.FailureCode})
		if err != nil {
			return out, err
		}
		return out, &payments.ProviderError{Code: n.FailureCode}
	}
	if n.Status != payments.StatusSucceeded {
		return out, model.ErrConflict
	}
	if n.OrderRef != in.OrderRef || n.Amount != in.Amount || n.PaidAmount < in.Amount || n.Currency != in.Currency {
		return out, model.ErrConflict
	}
	out = payments.ChargeResult{ProviderPaymentID: n.ProviderPaymentID, PaidAmount: n.PaidAmount, Test: n.Test}
	body, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	err = g.repo.Q.BillingFinishAttempt(ctx, db.BillingFinishAttemptParams{OrganizationID: org, ID: attempt.ID, Status: "succeeded", Result: body})
	return out, err
}

func (g *GatewayExecutor) Cancel(ctx context.Context, org int64, key string, p payments.Provider, a payments.Account, in payments.RefundRequest) (payments.RefundResult, error) {
	var out payments.RefundResult
	adapter, ok := p.(payments.PaymentCanceller)
	if !ok {
		return out, payments.ErrUnsupported
	}
	in.RefundRef = OrderReference(org, key)
	err := g.execute(ctx, org, key, "refund", p.Code(), attemptInput{Account: a.UUID, Request: struct {
		Cancel  bool
		Request payments.RefundRequest
	}{true, in}}, func() (any, error) { return adapter.CancelPayment(ctx, a, in) }, &out)
	return out, err
}
