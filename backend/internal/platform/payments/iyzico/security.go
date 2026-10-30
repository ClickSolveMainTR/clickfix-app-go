package iyzico

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
)

const SignatureHeader = "X-IYZ-SIGNATURE-V3"

// WebhookSignature ports Rentopia's V3 signature contract.
func WebhookSignature(secret string, h payments.Webhook) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(secret + h.EventType + h.PaymentID + h.Token + h.ConversationID + h.Status))
	return hex.EncodeToString(mac.Sum(nil))
}
func (p *Provider) verifyWebhook(a payments.Account, header http.Header, raw []byte) error {
	var w struct {
		EventType      string `json:"iyziEventType"`
		PaymentID      any    `json:"paymentId"`
		IyziPaymentID  any    `json:"iyziPaymentId"`
		Token          string `json:"token"`
		ConversationID string `json:"paymentConversationId"`
		Status         string `json:"status"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&w) != nil {
		return payments.ErrMalformed
	}
	id := jsText(w.PaymentID)
	if w.Token != "" {
		id = jsText(w.IyziPaymentID)
	}
	secret := a.Credentials["secret_key"]
	if secret == "" || header.Get(SignatureHeader) == "" || id == "" || w.EventType == "" || w.ConversationID == "" || w.Status == "" {
		return payments.ErrInvalidSignature
	}
	expected := WebhookSignature(secret, payments.Webhook{EventType: w.EventType, PaymentID: id, Token: w.Token, ConversationID: w.ConversationID, Status: w.Status})
	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(header.Get(SignatureHeader)))) {
		return payments.ErrInvalidSignature
	}
	return nil
}
func Sign(secret string, values ...string) string { return ResponseSignature(secret, values...) }
func FormatPrice(m money.Money) string {
	s := strings.TrimRight(m.String(), "0")
	if strings.HasSuffix(s, ".") {
		s += "0"
	}
	return s
}
func (p *Provider) InitThreeDS(ctx context.Context, a payments.Account, in payments.CheckoutRequest) (payments.Checkout, error) {
	return p.CreateCheckout(ctx, a, in)
}

// numberForms ports Rentopia's normalization of signed decimal amounts.
func numberForms(v any) []string {
	raw := jsText(v)
	if n, ok := v.(json.Number); ok {
		raw = n.String()
	}
	out := []string{raw}
	m, err := money.Parse(raw)
	if err != nil {
		return out
	}
	return append(out, FormatPrice(m), m.String(), strings.TrimSuffix(FormatPrice(m), ".0"))
}
func validPaymentSignature(secret string, r paymentResult, checkout bool) bool {
	for _, paid := range numberForms(r.PaidPrice) {
		for _, price := range numberForms(r.Price) {
			values := []string{jsText(r.PaymentID), r.Currency, r.BasketID, r.ConversationID, paid, price}
			if checkout {
				values = append([]string{r.PaymentStatus}, values...)
				values = append(values, r.Token)
			}
			if hmac.Equal([]byte(r.Signature), []byte(ResponseSignature(secret, values...))) {
				return true
			}
		}
	}
	return false
}

func (p *Provider) ListSavedCards(ctx context.Context, a payments.Account, owner string) ([]payments.SavedCard, error) {
	var out struct {
		apiResult
		Cards []struct {
			Token string `json:"cardToken"`
			Last4 string `json:"lastFourDigits"`
			Brand string `json:"cardAssociation"`
		} `json:"cardDetails"`
	}
	if err := p.call(ctx, a, "/cardstorage/cards", map[string]any{"locale": "tr", "cardUserKey": owner}, &out); err != nil {
		return nil, err
	}
	if out.Status != "success" {
		return nil, out.refusal()
	}
	cards := make([]payments.SavedCard, 0, len(out.Cards))
	for _, c := range out.Cards {
		cards = append(cards, payments.SavedCard{OwnerKey: owner, Token: c.Token, Last4: c.Last4, Brand: c.Brand})
	}
	return cards, nil
}

func (p *Provider) CancelPayment(ctx context.Context, a payments.Account, in payments.RefundRequest) (payments.RefundResult, error) {
	if in.ProviderPaymentID == "" {
		return payments.RefundResult{}, &payments.ProviderError{Code: "invalid_request"}
	}
	var out apiResult
	if err := p.call(ctx, a, "/payment/cancel", map[string]any{"locale": "tr", "conversationId": in.RefundRef, "paymentId": in.ProviderPaymentID, "ip": "127.0.0.1"}, &out); err != nil {
		return payments.RefundResult{}, err
	}
	if out.Status != "success" {
		return payments.RefundResult{}, out.refusal()
	}
	return payments.RefundResult{ProviderRefundID: in.RefundRef, Test: a.Mode != "live"}, nil
}
