package paytr

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
)

func (p *Provider) post(ctx context.Context, path string, v url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+path, strings.NewReader(v.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.Client.Do(req)
	if err != nil {
		return &payments.ProviderError{Code: "transport", Temporary: true}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return &payments.ProviderError{Code: "http", Temporary: true}
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out) != nil {
		return &payments.ProviderError{Code: "malformed", Temporary: true}
	}
	return nil
}
func (p *Provider) InitThreeDS(ctx context.Context, a payments.Account, in payments.CheckoutRequest) (payments.Checkout, error) {
	return p.CreateCheckout(ctx, a, in)
}

// ChargeSavedCard ports comcode's direct API request, preserving wait_callback.
func (p *Provider) ChargeSavedCard(ctx context.Context, a payments.Account, in payments.ChargeRequest) (payments.ChargeResult, error) {
	if in.Amount <= 0 || in.OrderRef == "" || in.Card.OwnerKey == "" || in.Card.Token == "" {
		return payments.ChargeResult{}, &payments.ProviderError{Code: "invalid_request"}
	}
	c := a.Credentials
	amount := money.New(in.Amount).String()
	currency := currencyCode(in.Currency)
	mode := testMode(a)
	basket, _ := json.Marshal([][]any{{in.ItemName, amount, 1}})
	v := url.Values{"merchant_id": {c["merchant_id"]}, "user_ip": {in.Buyer.IP}, "merchant_oid": {in.OrderRef}, "email": {in.Buyer.Email}, "payment_amount": {amount}, "payment_type": {"card"}, "installment_count": {"0"}, "currency": {currency}, "test_mode": {mode}, "non_3d": {"1"}, "recurring_payment": {"1"}, "utoken": {in.Card.OwnerKey}, "ctoken": {in.Card.Token}, "user_basket": {string(basket)}, "user_name": {in.Buyer.Name}, "user_address": {"Türkiye"}, "user_phone": {in.Buyer.Phone}, "merchant_ok_url": {c["success_url"]}, "merchant_fail_url": {c["fail_url"]}}
	v.Set("paytr_token", sign(c["merchant_key"], c["merchant_id"]+in.Buyer.IP+in.OrderRef+in.Buyer.Email+amount+"card0"+currency+mode+"1"+c["merchant_salt"]))
	var out struct {
		Status   string `json:"status"`
		TryAgain bool   `json:"try_again"`
	}
	if err := p.post(ctx, "/odeme", v, &out); err != nil {
		return payments.ChargeResult{}, err
	}
	switch out.Status {
	case "success":
		return payments.ChargeResult{ProviderPaymentID: in.OrderRef, PaidAmount: in.Amount, Test: a.Mode != "live"}, nil
	case "failed":
		return payments.ChargeResult{}, &payments.ProviderError{Code: "declined", Temporary: out.TryAgain}
	default:
		return payments.ChargeResult{}, &payments.ProviderError{Code: "await_callback", Temporary: true}
	}
}

// CheckCharge deliberately leaves "no successful payment found" pending: absence
// is not proof that an asynchronous Non3D charge cannot later settle.
func (p *Provider) CheckCharge(ctx context.Context, a payments.Account, in payments.ChargeRequest) (payments.Notification, error) {
	c := a.Credentials
	v := url.Values{"merchant_id": {c["merchant_id"]}, "merchant_oid": {in.OrderRef}, "paytr_token": {sign(c["merchant_key"], c["merchant_id"]+in.OrderRef+c["merchant_salt"])}}
	var out struct {
		Status   string `json:"status"`
		Amount   string `json:"payment_amount"`
		Total    string `json:"payment_total"`
		Currency string `json:"currency"`
	}
	if err := p.post(ctx, "/odeme/durum-sorgu", v, &out); err != nil {
		return payments.Notification{}, err
	}
	n := payments.Notification{OrderRef: in.OrderRef, Status: payments.StatusPending}
	if out.Status != "success" {
		return n, nil
	}
	amount, e1 := money.Parse(out.Amount)
	paid, e2 := money.Parse(out.Total)
	if e1 != nil || e2 != nil {
		return n, payments.ErrMalformed
	}
	n.Status = payments.StatusSucceeded
	n.Amount = amount.Minor()
	n.PaidAmount = paid.Minor()
	n.Currency = isoCurrency(out.Currency)
	n.ProviderPaymentID = in.OrderRef
	n.EventKey = in.OrderRef + ":success"
	n.Test = a.Mode != "live"
	return n, nil
}
func (p *Provider) RemoveSavedCard(ctx context.Context, a payments.Account, card payments.SavedCard) error {
	c := a.Credentials
	v := url.Values{"merchant_id": {c["merchant_id"]}, "utoken": {card.OwnerKey}, "ctoken": {card.Token}, "paytr_token": {sign(c["merchant_key"], card.Token+card.OwnerKey+c["merchant_salt"])}}
	var out struct {
		Status string `json:"status"`
	}
	if err := p.post(ctx, "/odeme/capi/delete", v, &out); err != nil {
		return err
	}
	if out.Status != "success" {
		return &payments.ProviderError{Code: "card_delete_failed"}
	}
	return nil
}
func (p *Provider) ListSavedCards(ctx context.Context, a payments.Account, owner string) ([]payments.SavedCard, error) {
	c := a.Credentials
	v := url.Values{"merchant_id": {c["merchant_id"]}, "utoken": {owner}, "paytr_token": {sign(c["merchant_key"], owner+c["merchant_salt"])}}
	var out []struct {
		Token string `json:"ctoken"`
		Last4 string `json:"last_4"`
		Brand string `json:"c_brand"`
	}
	if err := p.post(ctx, "/odeme/capi/list", v, &out); err != nil {
		return nil, err
	}
	cards := make([]payments.SavedCard, 0, len(out))
	for _, card := range out {
		cards = append(cards, payments.SavedCard{OwnerKey: owner, Token: card.Token, Last4: card.Last4, Brand: card.Brand})
	}
	return cards, nil
}
