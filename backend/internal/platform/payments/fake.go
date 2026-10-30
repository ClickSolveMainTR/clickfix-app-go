package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
)

// Fake is an in-process development gateway adapted from Takwim testpay and
// Rentopia iyzico-fake. Its callbacks require HMAC verification like real adapters.
type Fake struct {
	BaseURL string
	mu      sync.Mutex
	charges map[string]ChargeRequest
	refunds map[string]RefundRequest
	removed map[string]bool
}

func NewFake(baseURL string) *Fake {
	return &Fake{BaseURL: baseURL, charges: map[string]ChargeRequest{}, refunds: map[string]RefundRequest{}, removed: map[string]bool{}}
}
func (*Fake) Code() string { return "fake" }
func (p *Fake) CreateCheckout(_ context.Context, _ Account, in CheckoutRequest) (Checkout, error) {
	if in.OrderRef == "" || in.Amount <= 0 {
		return Checkout{}, &ProviderError{Code: "invalid_request"}
	}
	return Checkout{PageURL: p.BaseURL + "/pay/fake/" + url.PathEscape(in.OrderRef), Token: "fake-" + in.OrderRef}, nil
}
func (p *Fake) InitThreeDS(ctx context.Context, a Account, in CheckoutRequest) (Checkout, error) {
	return p.CreateCheckout(ctx, a, in)
}
func (p *Fake) TokenizeCard(_ context.Context, _ Account, in CheckoutRequest) (Checkout, error) {
	if in.OrderRef == "" {
		return Checkout{}, ErrMalformed
	}
	return Checkout{PageURL: p.BaseURL + "/pay/fake/cards/" + url.PathEscape(in.OrderRef), Token: "card-" + in.OrderRef}, nil
}
func (*Fake) ParseNotification(_ http.Header, body []byte) (Notification, error) {
	var n Notification
	if json.Unmarshal(body, &n) != nil || n.OrderRef == "" || n.EventKey == "" || n.Amount <= 0 {
		return n, ErrMalformed
	}
	if n.Status != StatusSucceeded && n.Status != StatusFailed {
		return n, ErrMalformed
	}
	return n, nil
}
func FakeSignature(secret string, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
func (*Fake) VerifyNotification(a Account, _ Notification, headers http.Header, body []byte) error {
	secret := a.Credentials["secret_key"]
	received, err := hex.DecodeString(headers.Get("X-Fake-Signature"))
	expected, _ := hex.DecodeString(FakeSignature(secret, body))
	if secret == "" || err != nil || !hmac.Equal(received, expected) {
		return ErrInvalidSignature
	}
	return nil
}
func (*Fake) Acknowledge(w http.ResponseWriter) { _, _ = w.Write([]byte("OK")) }
func (p *Fake) ChargeSavedCard(_ context.Context, _ Account, in ChargeRequest) (ChargeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if in.OrderRef == "" || in.Amount <= 0 || in.Card.Token == "" || p.removed[in.Card.Token] {
		return ChargeResult{}, &ProviderError{Code: "invalid_card"}
	}
	if old, ok := p.charges[in.OrderRef]; ok && (old.Amount != in.Amount || old.Currency != in.Currency || old.Card.Token != in.Card.Token) {
		return ChargeResult{}, &ProviderError{Code: "idempotency_conflict"}
	}
	p.charges[in.OrderRef] = in
	return ChargeResult{ProviderPaymentID: "fake-" + in.OrderRef, PaidAmount: in.Amount, Test: true}, nil
}
func (p *Fake) CheckCharge(_ context.Context, _ Account, in ChargeRequest) (Notification, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	old, ok := p.charges[in.OrderRef]
	if !ok {
		return Notification{OrderRef: in.OrderRef, Status: StatusFailed}, nil
	}
	return Notification{OrderRef: in.OrderRef, Status: StatusSucceeded, Amount: old.Amount, PaidAmount: old.Amount, Currency: old.Currency, ProviderPaymentID: "fake-" + in.OrderRef, Test: true}, nil
}
func (p *Fake) RemoveSavedCard(_ context.Context, _ Account, card SavedCard) error {
	if card.Token == "" {
		return ErrMalformed
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.removed[card.Token] = true
	return nil
}
func (p *Fake) Refund(_ context.Context, _ Account, in RefundRequest) (RefundResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	original, ok := p.charges[in.OrderRef]
	if !ok || in.RefundRef == "" || in.Amount <= 0 || original.Currency != in.Currency {
		return RefundResult{}, &ProviderError{Code: "invalid_refund"}
	}
	if old, exists := p.refunds[in.RefundRef]; exists {
		if old != in {
			return RefundResult{}, &ProviderError{Code: "idempotency_conflict"}
		}
		return RefundResult{ProviderRefundID: "refund-" + in.RefundRef, Test: true}, nil
	}
	remaining := original.Amount
	for _, r := range p.refunds {
		if r.OrderRef == in.OrderRef {
			remaining -= r.Amount
		}
	}
	if in.Amount > remaining {
		return RefundResult{}, &ProviderError{Code: "refund_exceeds_payment"}
	}
	p.refunds[in.RefundRef] = in
	return RefundResult{ProviderRefundID: "refund-" + in.RefundRef, Test: true}, nil
}
func (p *Fake) CheckRefund(_ context.Context, _ Account, in RefundRequest) (RefundCheck, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.refunds[in.RefundRef]; ok {
		return RefundCheck{Status: RefundFound, ProviderRefundID: fmt.Sprint("refund-", in.RefundRef)}, nil
	}
	return RefundCheck{Status: RefundNotDone}, nil
}

var _ Provider = (*Fake)(nil)
var _ CardCharger = (*Fake)(nil)
var _ ThreeDSInitializer = (*Fake)(nil)
var _ CardTokenizer = (*Fake)(nil)
