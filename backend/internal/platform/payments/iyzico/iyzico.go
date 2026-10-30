// Package iyzico ports Takwim hosted checkout and recurring-card APIs, with
// Rentopia V3 webhook verification and exact signed-number normalization.
package iyzico

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
)

// ProviderCode is the provider code of iyzico.
const ProviderCode = "iyzico"

// API origins of the two environments.
const (
	SandboxBaseURL = "https://sandbox-api.iyzipay.com"
	LiveBaseURL    = "https://api.iyzipay.com"
)

// Provider implements payments.Provider for iyzico.
type Provider struct {
	// BaseURL overrides the environment origin (tests); empty picks it from
	// the account's mode.
	BaseURL string
	Client  *http.Client
}

// New builds the adapter (empty base = by account mode).
func New(baseURL string, client *http.Client) *Provider {
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	return &Provider{BaseURL: strings.TrimRight(baseURL, "/"), Client: client}
}

var (
	_ payments.Provider      = (*Provider)(nil)
	_ payments.Confirmer     = (*Provider)(nil)
	_ payments.Refunder      = (*Provider)(nil)
	_ payments.CardCharger   = (*Provider)(nil)
	_ payments.ChargeChecker = (*Provider)(nil)
	_ payments.CardRemover   = (*Provider)(nil)
)

func (p *Provider) Code() string { return ProviderCode }

func (p *Provider) base(account payments.Account) string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	if account.Mode == "live" {
		return LiveBaseURL
	}
	return SandboxBaseURL
}

// Check implements integrations.Adapter with a BIN lookup: an
// authenticated call that moves no money.
func (p *Provider) Check(ctx context.Context, account payments.Account) error {
	if account.Credentials["api_key"] == "" || account.Credentials["secret_key"] == "" {
		return errors.New("iyzico: api_key and secret_key are required")
	}
	var out apiResult
	if err := p.call(ctx, account, "/payment/bin/check", map[string]any{
		"locale": "tr", "conversationId": "takwimcheck", "binNumber": "554960",
	}, &out); err != nil {
		return err
	}
	if out.Status != "success" {
		return fmt.Errorf("iyzico: %s %s", out.ErrorCode, out.ErrorMessage)
	}
	return nil
}

// apiResult is the common head of every iyzico answer.
type apiResult struct {
	Status       string `json:"status"`
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
	ErrorGroup   string `json:"errorGroup"`
	Signature    string `json:"signature"`
}

func (r apiResult) refusal() *payments.ProviderError {
	msg := r.ErrorMessage
	if msg == "" {
		msg = "refused"
	}
	return &payments.ProviderError{Code: r.ErrorCode, Message: msg}
}

// Authorization is the IYZWSv2 header of a request: base64("apiKey:" key
// "&randomKey:" rnd "&signature:" hex(HMAC-SHA256(secret, rnd + path +
// body))). Exported for tests.
func Authorization(apiKey, secretKey, rnd, path string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(rnd + path))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	params := "apiKey:" + apiKey + "&randomKey:" + rnd + "&signature:" + sig
	return "IYZWSv2 " + base64.StdEncoding.EncodeToString([]byte(params))
}

// ResponseSignature is the signature iyzico puts on an answer:
// hex(HMAC-SHA256(secret, values joined with ":")). Exported for tests.
func ResponseSignature(secretKey string, values ...string) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(strings.Join(values, ":")))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomKey() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return strconv.FormatInt(time.Now().UnixMilli(), 10) + hex.EncodeToString(b)
}

// errTransport marks an answer that never came or could not be read: the
// result of the call is unknown.
func transportErr(msg string) error {
	return &payments.ProviderError{Message: msg, Temporary: true}
}

// call posts a signed JSON request and decodes the answer into out. A
// transport failure, a 5xx or an unreadable body is a Temporary
// ProviderError (unknown result); a 4xx answer is decoded like a 200,
// because iyzico puts its refusals in the body.
func (p *Provider) call(ctx context.Context, account payments.Account, path string, body any, out any) error {
	return p.do(ctx, account, http.MethodPost, path, body, out)
}

func (p *Provider) do(ctx context.Context, account payments.Account, method, path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base(account)+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	rnd := randomKey()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-iyzi-rnd", rnd)
	req.Header.Set("Authorization", Authorization(account.Credentials["api_key"], account.Credentials["secret_key"], rnd, path, raw))
	resp, err := p.Client.Do(req)
	if err != nil {
		return transportErr(err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return transportErr(err.Error())
	}
	if resp.StatusCode >= 500 {
		return &payments.ProviderError{Code: strconv.Itoa(resp.StatusCode), Message: "provider unavailable", Temporary: true}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return transportErr("unreadable answer")
	}
	return nil
}

// jsText is the text JavaScript gives a JSON value (iyzico signs the
// values as its own SDKs print them): numbers in their shortest form
// (1.20 → "1.2"), strings as they are.
func jsText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return t.String()
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return fmt.Sprint(v)
}

func amountOf(v any) (money.Money, error) {
	s := jsText(v)
	if s == "" {
		return money.Zero(), errors.New("empty amount")
	}
	return money.Parse(s)
}

func currencyOf(c string) string {
	if c == "" || c == "TL" {
		return "TRY"
	}
	return strings.ToUpper(c)
}

func locale(l string) string {
	if l == "en" {
		return "en"
	}
	return "tr"
}

// price is the decimal text iyzico takes ("1250.50").
func price(m money.Money) string { return m.String() }

type buyer struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Surname             string `json:"surname"`
	IdentityNumber      string `json:"identityNumber"`
	Email               string `json:"email"`
	GsmNumber           string `json:"gsmNumber,omitempty"`
	RegistrationAddress string `json:"registrationAddress"`
	City                string `json:"city"`
	Country             string `json:"country"`
	IP                  string `json:"ip"`
}

type address struct {
	ContactName string `json:"contactName"`
	City        string `json:"city"`
	Country     string `json:"country"`
	Address     string `json:"address"`
}

type basketItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Category1 string `json:"category1"`
	ItemType  string `json:"itemType"`
	Price     string `json:"price"`
}

// splitName gives iyzico's name and surname (both required).
func splitName(full string) (string, string) {
	full = strings.Join(strings.Fields(full), " ")
	if full == "" {
		return "ClickFix", "Musteri"
	}
	i := strings.LastIndex(full, " ")
	if i <= 0 {
		return full, full
	}
	return full[:i], full[i+1:]
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// payer builds the buyer and address blocks. iyzico requires an identity
// number, an address and an e-mail a booking may not have: placeholders
// stand in (the card holder is verified by the bank, not by these).
func payer(ref string, b payments.Buyer) (buyer, address) {
	name, surname := splitName(b.Name)
	email := strings.TrimSpace(b.Email)
	if email == "" {
		email = "musteri+" + strings.ToLower(ref) + "@example.invalid"
	}
	ip := b.IP
	if ip == "" {
		ip = "127.0.0.1"
	}
	id := ref
	if b.Name != "" {
		id = "c" + ref
	}
	contact := strings.TrimSpace(name + " " + surname)
	return buyer{
			ID: clip(id, 64), Name: clip(name, 80), Surname: clip(surname, 80), IdentityNumber: "11111111111",
			Email: email, GsmNumber: b.Phone, RegistrationAddress: "Türkiye", City: "Istanbul", Country: "Turkey", IP: ip,
		}, address{
			ContactName: clip(contact, 120), City: "Istanbul", Country: "Turkey", Address: "Türkiye",
		}
}

func basket(ref, name string, amount money.Money) []basketItem {
	if strings.TrimSpace(name) == "" {
		name = "ClickFix"
	}
	return []basketItem{{ID: clip(ref, 64), Name: clip(name, 200), Category1: "Hizmet", ItemType: "VIRTUAL", Price: price(amount)}}
}

func group(recurring bool) string {
	if recurring {
		return "SUBSCRIPTION"
	}
	return "PRODUCT"
}

// checkoutInit is the Checkout Form initialize body.
type checkoutInit struct {
	Locale              string       `json:"locale"`
	ConversationID      string       `json:"conversationId"`
	Price               string       `json:"price"`
	BasketID            string       `json:"basketId"`
	PaymentGroup        string       `json:"paymentGroup"`
	Buyer               buyer        `json:"buyer"`
	ShippingAddress     address      `json:"shippingAddress"`
	BillingAddress      address      `json:"billingAddress"`
	BasketItems         []basketItem `json:"basketItems"`
	CallbackURL         string       `json:"callbackUrl"`
	Currency            string       `json:"currency"`
	PaidPrice           string       `json:"paidPrice"`
	CardUserKey         string       `json:"cardUserKey,omitempty"`
	EnabledInstallments []int        `json:"enabledInstallments"`
}

// CheckoutBody builds the initialize request of a checkout (exported for
// tests).
func CheckoutBody(in payments.CheckoutRequest) (any, error) {
	if in.OrderRef == "" || in.Amount <= 0 {
		return nil, &payments.ProviderError{Code: "invalid_request", Message: "order reference and amount are required"}
	}
	if in.ReturnURL == "" {
		return nil, &payments.ProviderError{Code: "invalid_request", Message: "return URL is required"}
	}
	b, addr := payer(in.OrderRef, in.Buyer)
	return checkoutInit{
		Locale: locale(in.Locale), ConversationID: in.OrderRef, Price: price(money.New(in.Amount)), BasketID: in.OrderRef,
		PaymentGroup: group(in.Recurring), Buyer: b, ShippingAddress: addr, BillingAddress: addr,
		BasketItems: basket(in.OrderRef, in.ItemName, money.New(in.Amount)), CallbackURL: in.ReturnURL,
		Currency: currencyOf(in.Currency), PaidPrice: price(money.New(in.Amount)), CardUserKey: in.CardOwnerKey,
		EnabledInstallments: []int{1},
	}, nil
}

// CreateCheckout implements payments.Provider (Checkout Form initialize).
func (p *Provider) CreateCheckout(ctx context.Context, account payments.Account, in payments.CheckoutRequest) (payments.Checkout, error) {
	body, err := CheckoutBody(in)
	if err != nil {
		return payments.Checkout{}, err
	}
	var out struct {
		apiResult
		Token          string `json:"token"`
		PaymentPageURL string `json:"paymentPageUrl"`
		ConversationID string `json:"conversationId"`
	}
	if err := p.call(ctx, account, "/payment/iyzipos/checkoutform/initialize/auth/ecom", body, &out); err != nil {
		return payments.Checkout{}, err
	}
	if out.Status != "success" || out.Token == "" || out.PaymentPageURL == "" {
		return payments.Checkout{}, out.refusal()
	}
	if out.Signature != "" && !hmac.Equal([]byte(out.Signature), []byte(ResponseSignature(account.Credentials["secret_key"], out.ConversationID, out.Token))) {
		return payments.Checkout{}, payments.ErrInvalidSignature
	}
	return payments.Checkout{PageURL: out.PaymentPageURL, Token: out.Token}, nil
}

// paymentResult is the payment part of a retrieve, charge or detail answer.
type paymentResult struct {
	apiResult
	PaymentStatus  string `json:"paymentStatus"`
	PaymentID      any    `json:"paymentId"`
	Price          any    `json:"price"`
	PaidPrice      any    `json:"paidPrice"`
	Currency       string `json:"currency"`
	BasketID       string `json:"basketId"`
	ConversationID string `json:"conversationId"`
	Token          string `json:"token"`
	FraudStatus    any    `json:"fraudStatus"`
	CardUserKey    string `json:"cardUserKey"`
	CardToken      string `json:"cardToken"`
	BinNumber      string `json:"binNumber"`
	LastFour       string `json:"lastFourDigits"`
	CardAssoc      string `json:"cardAssociation"`
	CardFamily     string `json:"cardFamily"`
	CardBank       string `json:"cardBankName"`
	CardAlias      string `json:"cardAlias"`
}

// notification maps a payment answer of our reference onto the layer's
// notification. iyzico answers status "failure" both for a declined
// payment (paymentStatus FAILURE) and for a call it could not serve; only
// the first is a result.
func notification(account payments.Account, ref string, r paymentResult) (payments.Notification, error) {
	n := payments.Notification{
		OrderRef: ref, Currency: currencyOf(r.Currency), ProviderPaymentID: jsText(r.PaymentID), Token: r.Token,
		Test: account.Mode != "live",
	}
	if r.PaymentStatus == "FAILURE" {
		n.Status = payments.StatusFailed
		n.FailureCode, n.FailureMessage = r.ErrorCode, r.ErrorMessage
		n.EventKey = ref + ":" + payments.StatusFailed
		return n, nil
	}
	if r.Status != "success" {
		e := r.refusal()
		e.Temporary = true
		return payments.Notification{}, e
	}
	if r.PaymentStatus != "SUCCESS" {
		// INIT_THREEDS, CALLBACK_THREEDS, BKM_POS_SELECTED …: not final.
		n.Status = payments.StatusPending
		return n, nil
	}
	switch jsText(r.FraudStatus) {
	case "-1":
		n.Status = payments.StatusFailed
		n.FailureCode, n.FailureMessage = "fraud", "rejected by the provider's fraud check"
		n.EventKey = ref + ":" + payments.StatusFailed
		return n, nil
	case "0":
		n.Status = payments.StatusPending // fraud review
		return n, nil
	}
	amount, err := amountOf(r.Price)
	if err != nil {
		return payments.Notification{}, payments.ErrMalformed
	}
	paid, err := amountOf(r.PaidPrice)
	if err != nil {
		return payments.Notification{}, payments.ErrMalformed
	}
	n.Status, n.Amount, n.PaidAmount = payments.StatusSucceeded, amount.Minor(), paid.Minor()
	n.EventKey = ref + ":" + payments.StatusSucceeded
	if r.CardUserKey != "" && r.CardToken != "" {
		n.Card = &payments.SavedCard{
			OwnerKey: r.CardUserKey, Token: r.CardToken, Last4: r.LastFour, Bin: r.BinNumber,
			Brand: r.CardAssoc, Family: r.CardFamily, Bank: r.CardBank, Alias: r.CardAlias,
		}
	}
	return n, nil
}

// Confirm implements payments.Confirmer: Checkout Form retrieve. The
// answer must be about our reference and carry iyzico's signature.
func (p *Provider) Confirm(ctx context.Context, account payments.Account, orderRef, token string) (payments.Notification, error) {
	if orderRef == "" || token == "" || len(token) > 200 {
		return payments.Notification{}, payments.ErrMalformed
	}
	var out paymentResult
	if err := p.call(ctx, account, "/payment/iyzipos/checkoutform/auth/ecom/detail", map[string]any{
		"locale": "tr", "conversationId": orderRef, "token": token,
	}, &out); err != nil {
		return payments.Notification{}, err
	}
	// Only an answer about our reference counts (a token of another
	// payment sent to this one's return address is refused).
	if out.BasketID != "" && out.BasketID != orderRef {
		return payments.Notification{}, payments.ErrInvalidSignature
	}
	if out.Status == "success" {
		// A successful payment is only believed with our basket and iyzico's
		// response signature (HMAC-SHA256 over paymentStatus, paymentId,
		// currency, basketId, conversationId, paidPrice, price, token): it is
		// what credits an account.
		if out.PaymentStatus == "SUCCESS" {
			if out.BasketID == "" || !validPaymentSignature(account.Credentials["secret_key"], out, true) {
				return payments.Notification{}, payments.ErrInvalidSignature
			}
		} else if out.BasketID == "" || out.Signature != "" && !validPaymentSignature(account.Credentials["secret_key"], out, true) {
			// An unfinished or abandoned form answers without our basket (or
			// with a result that does not verify): no result yet. Pending
			// neither credits nor fails a payment, and lets a stale checkout
			// expire.
			return payments.Notification{OrderRef: orderRef, Token: token, Status: payments.StatusPending, Test: account.Mode != "live"}, nil
		}
	}

	return notification(account, orderRef, out)
}

// webhook is iyzico's merchant notification (only its references are read).
type webhook struct {
	PaymentConversationID string `json:"paymentConversationId"`
	Token                 string `json:"token"`
	Status                string `json:"status"`
	EventType             string `json:"iyziEventType"`
}

// ParseNotification implements payments.Provider: the webhook names our
// reference and the page token; its status is not trusted (the service
// confirms through the API).
func (p *Provider) ParseNotification(_ http.Header, body []byte) (payments.Notification, error) {
	var w webhook
	if err := json.Unmarshal(bytes.TrimSpace(body), &w); err != nil || w.PaymentConversationID == "" || w.Token == "" {
		return payments.Notification{}, payments.ErrMalformed
	}
	return payments.Notification{
		OrderRef: w.PaymentConversationID, Token: w.Token, Status: payments.StatusPending,
		EventKey: w.PaymentConversationID + ":" + strings.ToLower(w.Status),
	}, nil
}

// VerifyNotification requires the merchant V3 signature before any API confirmation.
func (p *Provider) VerifyNotification(account payments.Account, _ payments.Notification, header http.Header, body []byte) error {
	return p.verifyWebhook(account, header, body)
}

// Acknowledge implements payments.Provider: iyzico stops on any 2xx.
func (p *Provider) Acknowledge(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
}

// Refund implements payments.Refunder (refund v2: by payment id). A
// transport failure, a 5xx or an unreadable answer leaves the result
// unknown (Temporary).
func (p *Provider) Refund(ctx context.Context, account payments.Account, in payments.RefundRequest) (payments.RefundResult, error) {
	if in.ProviderPaymentID == "" || in.Amount <= 0 {
		return payments.RefundResult{}, &payments.ProviderError{Code: "invalid_request", Message: "payment id and amount are required"}
	}
	conv := in.RefundRef
	if conv == "" {
		conv = in.OrderRef
	}
	var out struct {
		apiResult
		PaymentID      any    `json:"paymentId"`
		ConversationID string `json:"conversationId"`
	}
	if err := p.call(ctx, account, "/v2/payment/refund", map[string]any{
		"locale": "tr", "conversationId": conv, "paymentId": in.ProviderPaymentID,
		"price": price(money.New(in.Amount)), "currency": currencyOf(in.Currency), "ip": "127.0.0.1",
	}, &out); err != nil {
		return payments.RefundResult{}, err
	}
	switch out.Status {
	case "success":
		return payments.RefundResult{ProviderRefundID: conv, Test: account.Mode != "live"}, nil
	case "failure":
		return payments.RefundResult{}, out.refusal()
	}
	return payments.RefundResult{}, transportErr("unexpected refund answer")
}

type chargeBody struct {
	Locale          string       `json:"locale"`
	ConversationID  string       `json:"conversationId"`
	Price           string       `json:"price"`
	PaidPrice       string       `json:"paidPrice"`
	Installment     int          `json:"installment"`
	PaymentChannel  string       `json:"paymentChannel"`
	BasketID        string       `json:"basketId"`
	PaymentGroup    string       `json:"paymentGroup"`
	PaymentCard     storedCard   `json:"paymentCard"`
	Buyer           buyer        `json:"buyer"`
	ShippingAddress address      `json:"shippingAddress"`
	BillingAddress  address      `json:"billingAddress"`
	BasketItems     []basketItem `json:"basketItems"`
	Currency        string       `json:"currency"`
}

type storedCard struct {
	CardToken   string `json:"cardToken"`
	CardUserKey string `json:"cardUserKey"`
}

// ChargeBody builds the saved card payment (exported for tests).
func ChargeBody(in payments.ChargeRequest) (any, error) {
	if in.OrderRef == "" || in.Amount <= 0 || in.Card.OwnerKey == "" || in.Card.Token == "" {
		return nil, &payments.ProviderError{Code: "invalid_request", Message: "order reference, amount and card are required"}
	}
	b, addr := payer(in.OrderRef, in.Buyer)
	return chargeBody{
		Locale: locale(in.Locale), ConversationID: in.OrderRef, Price: price(money.New(in.Amount)), PaidPrice: price(money.New(in.Amount)),
		Installment: 1, PaymentChannel: "WEB", BasketID: in.OrderRef, PaymentGroup: group(in.Recurring),
		PaymentCard: storedCard{CardToken: in.Card.Token, CardUserKey: in.Card.OwnerKey},
		Buyer:       b, ShippingAddress: addr, BillingAddress: addr,
		BasketItems: basket(in.OrderRef, in.ItemName, money.New(in.Amount)), Currency: currencyOf(in.Currency),
	}, nil
}

// ChargeSavedCard implements payments.CardCharger (non-3-D Secure payment
// with cardUserKey + cardToken; the merchant must have it enabled).
func (p *Provider) ChargeSavedCard(ctx context.Context, account payments.Account, in payments.ChargeRequest) (payments.ChargeResult, error) {
	body, err := ChargeBody(in)
	if err != nil {
		return payments.ChargeResult{}, err
	}
	var out paymentResult
	if err := p.call(ctx, account, "/payment/auth", body, &out); err != nil {
		return payments.ChargeResult{}, err
	}
	switch out.Status {
	case "failure":
		return payments.ChargeResult{}, out.refusal()
	case "success":
	default:
		return payments.ChargeResult{}, transportErr("unexpected charge answer")
	}
	if out.Signature != "" && !validPaymentSignature(account.Credentials["secret_key"], out, false) {
		return payments.ChargeResult{}, transportErr("charge answer signature mismatch")
	}
	if out.BasketID != in.OrderRef || out.ConversationID != in.OrderRef || jsText(out.PaymentID) == "" || currencyOf(out.Currency) != in.Currency || jsText(out.FraudStatus) == "0" {
		return payments.ChargeResult{}, transportErr("charge result requires reconciliation")
	}

	if jsText(out.FraudStatus) == "-1" {
		return payments.ChargeResult{}, &payments.ProviderError{Code: "fraud", Message: "rejected by the provider's fraud check"}
	}
	original, err := amountOf(out.Price)
	if err != nil || original.Minor() != in.Amount {
		return payments.ChargeResult{}, transportErr("charge original amount mismatch")
	}
	paid, err := amountOf(out.PaidPrice)
	if err != nil {
		return payments.ChargeResult{}, transportErr("charge amount unreadable")
	}
	return payments.ChargeResult{ProviderPaymentID: jsText(out.PaymentID), PaidAmount: paid.Minor(), Test: account.Mode != "live"}, nil
}

// CheckCharge implements payments.ChargeChecker (payment detail by our
// conversation id). iyzico answers a failure when it has no such payment.
func (p *Provider) CheckCharge(ctx context.Context, account payments.Account, in payments.ChargeRequest) (payments.Notification, error) {
	orderRef := in.OrderRef
	if orderRef == "" {
		return payments.Notification{}, payments.ErrMalformed
	}
	var out paymentResult
	if err := p.call(ctx, account, "/payment/detail", map[string]any{
		"locale": "tr", "conversationId": orderRef, "paymentConversationId": orderRef,
	}, &out); err != nil {
		return payments.Notification{}, err
	}
	if out.Status == "failure" && out.PaymentStatus == "" {
		// Missing records and API errors do not prove that a timed-out capture failed.
		return payments.Notification{
			OrderRef: orderRef, Status: payments.StatusPending, FailureCode: out.ErrorCode, FailureMessage: out.ErrorMessage,
			EventKey: orderRef + ":" + payments.StatusFailed,
		}, nil
	}
	if out.Status == "success" && out.PaymentStatus == "" {
		out.PaymentStatus = "SUCCESS"
	}
	if out.BasketID != orderRef || (out.ConversationID != "" && out.ConversationID != orderRef) {
		return payments.Notification{}, payments.ErrInvalidSignature
	}
	if out.Signature != "" && !validPaymentSignature(account.Credentials["secret_key"], out, false) {
		return payments.Notification{}, payments.ErrInvalidSignature
	}
	return notification(account, orderRef, out)
}

// RemoveSavedCard implements payments.CardRemover (card storage delete).
func (p *Provider) RemoveSavedCard(ctx context.Context, account payments.Account, card payments.SavedCard) error {
	if card.OwnerKey == "" || card.Token == "" {
		return nil
	}
	var out apiResult
	if err := p.do(ctx, account, http.MethodDelete, "/cardstorage/card", map[string]any{
		"locale": "tr", "conversationId": "clickfixcard", "cardUserKey": card.OwnerKey, "cardToken": card.Token,
	}, &out); err != nil {
		return err
	}
	if out.Status != "success" {
		return out.refusal()
	}
	return nil
}
