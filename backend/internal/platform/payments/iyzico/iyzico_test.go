package iyzico_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/money"
)

const secret = "sandbox-secret-key"

func account() payments.Account {
	return payments.Account{Mode: "sandbox", Credentials: map[string]string{"api_key": "sandbox-api-key", "secret_key": secret}}
}

// fake answers every path with the handler's JSON after checking the
// IYZWSv2 header against the body.
func fake(t *testing.T, handle func(path string, body map[string]any) (int, any)) *iyzico.Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rnd := r.Header.Get("x-iyzi-rnd")
		if rnd == "" || r.Header.Get("Authorization") != iyzico.Authorization("sandbox-api-key", secret, rnd, r.URL.Path, raw) {
			t.Errorf("%s: bad authorization header", r.URL.Path)
		}
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		status, out := handle(r.URL.Path, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return iyzico.New(srv.URL, srv.Client())
}

func TestAuthorizationHeader(t *testing.T) {
	h := iyzico.Authorization("key", "secret", "123", "/payment/test", []byte(`{"a":1}`))
	if !strings.HasPrefix(h, "IYZWSv2 ") {
		t.Fatalf("header %q", h)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(h, "IYZWSv2 "))
	if err != nil {
		t.Fatal(err)
	}
	// hex(HMAC-SHA256("secret", "123/payment/test{\"a\":1}"))
	want := "apiKey:key&randomKey:123&signature:" + iyzico.ResponseSignature("secret", `123/payment/test{"a":1}`)
	if string(raw) != want {
		t.Fatalf("got %q want %q", raw, want)
	}
}

func TestCheckoutAndConfirm(t *testing.T) {
	ref := "abc123"
	var initBody map[string]any
	answer := map[string]any{}
	p := fake(t, func(path string, body map[string]any) (int, any) {
		switch path {
		case "/payment/iyzipos/checkoutform/initialize/auth/ecom":
			initBody = body
			return 200, map[string]any{
				"status": "success", "token": "tok-1", "paymentPageUrl": "https://sandbox-cpp.iyzipay.com?token=tok-1",
				"conversationId": ref, "signature": iyzico.ResponseSignature(secret, ref, "tok-1"),
			}
		case "/payment/iyzipos/checkoutform/auth/ecom/detail":
			if body["token"] != "tok-1" {
				t.Errorf("retrieve token %v", body["token"])
			}
			return 200, answer
		}
		t.Errorf("unexpected path %s", path)
		return 404, nil
	})
	ctx := context.Background()
	c, err := p.CreateCheckout(ctx, account(), payments.CheckoutRequest{
		OrderRef: ref, Amount: money.MustParse("1250.50").Minor(), Currency: "TRY", ItemName: "Kesim · #12",
		Buyer: payments.Buyer{Name: "Ayşe Nur Yılmaz", IP: "1.2.3.4"}, ReturnURL: "https://takwim.link/pay/return/iyzico/" + ref,
		CardOwnerKey: "owner-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "tok-1" || !strings.Contains(c.PageURL, "tok-1") {
		t.Fatalf("checkout %+v", c)
	}
	buyer := initBody["buyer"].(map[string]any)
	if initBody["price"] != "1250.50" || initBody["paidPrice"] != "1250.50" || initBody["basketId"] != ref ||
		initBody["cardUserKey"] != "owner-1" || buyer["name"] != "Ayşe Nur" || buyer["surname"] != "Yılmaz" ||
		initBody["callbackUrl"] != "https://takwim.link/pay/return/iyzico/"+ref || initBody["paymentGroup"] != "PRODUCT" {
		t.Fatalf("init body %+v", initBody)
	}

	signed := func(m map[string]any) map[string]any {
		m["signature"] = iyzico.ResponseSignature(secret, m["paymentStatus"].(string), "22416035", "TRY", ref, ref, "1250.5", "1250.5", "tok-1")
		return m
	}
	// Success with a saved card (numbers signed as JavaScript prints them).
	answer = signed(map[string]any{
		"status": "success", "paymentStatus": "SUCCESS", "paymentId": "22416035", "price": 1250.50, "paidPrice": 1250.5,
		"currency": "TRY", "basketId": ref, "conversationId": ref, "token": "tok-1", "fraudStatus": 1,
		"cardUserKey": "owner-1", "cardToken": "card-9", "lastFourDigits": "0016", "binNumber": "552879",
		"cardAssociation": "MASTER_CARD", "cardFamily": "Paraf", "cardBankName": "Halkbank",
	})
	n, err := p.Confirm(ctx, account(), ref, "tok-1")
	if err != nil {
		t.Fatal(err)
	}
	if n.Status != payments.StatusSucceeded || money.New(n.Amount).String() != "1250.50" || n.ProviderPaymentID != "22416035" ||
		n.EventKey != ref+":succeeded" || n.Card == nil || n.Card.Token != "card-9" || n.Card.Last4 != "0016" || !n.Test {
		t.Fatalf("notification %+v card %+v", n, n.Card)
	}

	// A forged signature is refused.
	answer["signature"] = "00"
	if _, err := p.Confirm(ctx, account(), ref, "tok-1"); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("forged: %v", err)
	}
	// A token of another payment is refused.
	answer = signed(map[string]any{
		"status": "success", "paymentStatus": "SUCCESS", "paymentId": "22416035", "price": 1250.5, "paidPrice": 1250.5,
		"currency": "TRY", "basketId": "other", "conversationId": ref, "token": "tok-1",
	})
	if _, err := p.Confirm(ctx, account(), ref, "tok-1"); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("other basket: %v", err)
	}
	// A success without our basket is refused.
	answer = signed(map[string]any{
		"status": "success", "paymentStatus": "SUCCESS", "paymentId": "22416035", "price": 1250.5, "paidPrice": 1250.5,
		"currency": "TRY", "conversationId": ref, "token": "tok-1",
	})
	if _, err := p.Confirm(ctx, account(), ref, "tok-1"); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("success without basket: %v", err)
	}
	// An abandoned form (no basket, no payment result): pending, not an error.
	answer = map[string]any{"status": "success", "token": "tok-1", "callbackUrl": "https://takwim.link/pay/return/iyzico/" + ref, "conversationId": ref}
	if n, err := p.Confirm(ctx, account(), ref, "tok-1"); err != nil || n.Status != payments.StatusPending || n.OrderRef != ref {
		t.Fatalf("abandoned: %+v %v", n, err)
	}
	// A failure that does not verify is not believed: pending.
	answer = map[string]any{
		"status": "success", "paymentStatus": "FAILURE", "price": 1250.5, "paidPrice": 1250.5, "currency": "TRY",
		"basketId": ref, "conversationId": ref, "token": "tok-1", "signature": "00",
	}
	if n, err := p.Confirm(ctx, account(), ref, "tok-1"); err != nil || n.Status != payments.StatusPending {
		t.Fatalf("unverified failure: %+v %v", n, err)
	}
	// 3-D Secure still open: pending.
	answer = signed(map[string]any{
		"status": "success", "paymentStatus": "INIT_THREEDS", "paymentId": "22416035", "price": 1250.5, "paidPrice": 1250.5,
		"currency": "TRY", "basketId": ref, "conversationId": ref, "token": "tok-1",
	})
	if n, err := p.Confirm(ctx, account(), ref, "tok-1"); err != nil || n.Status != payments.StatusPending {
		t.Fatalf("pending: %+v %v", n, err)
	}
	// Declined.
	answer = map[string]any{"status": "failure", "paymentStatus": "FAILURE", "errorCode": "10051", "errorMessage": "Kart limiti yetersiz", "basketId": ref}
	if n, err := p.Confirm(ctx, account(), ref, "tok-1"); err != nil || n.Status != payments.StatusFailed || n.FailureCode != "10051" {
		t.Fatalf("declined: %+v %v", n, err)
	}
	// A call iyzico could not serve is not a result.
	answer = map[string]any{"status": "failure", "errorCode": "5", "errorMessage": "Sistem hatası"}
	var pe *payments.ProviderError
	if _, err := p.Confirm(ctx, account(), ref, "tok-1"); !errors.As(err, &pe) || !pe.Temporary {
		t.Fatalf("api failure: %v", err)
	}
}

// iyzico answers amounts with eight decimals but signs them as JavaScript
// prints them (shape of a real sandbox checkout form retrieve).
func TestConfirmVerifiesEightDecimalAmounts(t *testing.T) {
	ref := "d4e5b87d428545e3c950e53dd45cafa6"
	p := fake(t, func(string, map[string]any) (int, any) {
		return 200, map[string]any{
			"status": "success", "paymentStatus": "SUCCESS", "paymentId": "38584245", "phase": "AUTH",
			"price": json.Number("3600.00000000"), "paidPrice": json.Number("3600.00000000"), "currency": "TRY",
			"basketId": ref, "conversationId": ref, "token": "tok-1", "fraudStatus": 1,
			"iyziCommissionRateAmount": json.Number("125.64000000"), "merchantCommissionRate": json.Number("0E-8"),
			"signature": iyzico.ResponseSignature(secret, "SUCCESS", "38584245", "TRY", ref, ref, "3600", "3600", "tok-1"),
		}
	})
	n, err := p.Confirm(context.Background(), account(), ref, "tok-1")
	if err != nil || n.Status != payments.StatusSucceeded || money.New(n.Amount).String() != "3600.00" || money.New(n.PaidAmount).String() != "3600.00" {
		t.Fatalf("%+v %v", n, err)
	}
}

func TestWebhookIsOnlyAReference(t *testing.T) {
	p := iyzico.New("http://unused", nil)
	n, err := p.ParseNotification(nil, []byte(`{"paymentConversationId":"abc123","token":"tok-1","status":"SUCCESS","iyziEventType":"CHECKOUT_FORM_AUTH","merchantId":1}`))
	if err != nil || n.OrderRef != "abc123" || n.Token != "tok-1" || n.Status != payments.StatusPending {
		t.Fatalf("%+v %v", n, err)
	}
	if _, err := p.ParseNotification(nil, []byte(`{"token":"x"}`)); !errors.Is(err, payments.ErrMalformed) {
		t.Fatalf("missing ref: %v", err)
	}
}

func TestRefund(t *testing.T) {
	answer := map[string]any{"status": "success", "paymentId": "22416035", "conversationId": "r1"}
	status := 200
	p := fake(t, func(path string, body map[string]any) (int, any) {
		if path != "/v2/payment/refund" || body["paymentId"] != "22416035" || body["price"] != "100.00" {
			t.Errorf("refund %s %+v", path, body)
		}
		return status, answer
	})
	ctx := context.Background()
	in := payments.RefundRequest{OrderRef: "abc", ProviderPaymentID: "22416035", Amount: money.MustParse("100").Minor(), Currency: "TRY", RefundRef: "r1"}
	if r, err := p.Refund(ctx, account(), in); err != nil || r.ProviderRefundID != "r1" {
		t.Fatalf("%+v %v", r, err)
	}
	answer = map[string]any{"status": "failure", "errorCode": "5092", "errorMessage": "İade edilemez"}
	var pe *payments.ProviderError
	if _, err := p.Refund(ctx, account(), in); !errors.As(err, &pe) || pe.Temporary || pe.Code != "5092" {
		t.Fatalf("refused: %v", err)
	}
	status = 502
	if _, err := p.Refund(ctx, account(), in); !errors.As(err, &pe) || !pe.Temporary {
		t.Fatalf("5xx: %v", err)
	}
}

func TestChargeSavedCardAndCheck(t *testing.T) {
	ref := "chg1"
	answer := map[string]any{}
	status := 200
	p := fake(t, func(path string, body map[string]any) (int, any) {
		switch path {
		case "/payment/auth":
			card := body["paymentCard"].(map[string]any)
			if card["cardUserKey"] != "owner-1" || card["cardToken"] != "card-9" || body["paymentGroup"] != "SUBSCRIPTION" {
				t.Errorf("charge body %+v", body)
			}
		case "/payment/detail":
			if body["paymentConversationId"] != ref {
				t.Errorf("detail body %+v", body)
			}
		case "/cardstorage/card":
		default:
			t.Errorf("unexpected path %s", path)
		}
		return status, answer
	})
	ctx := context.Background()
	in := payments.ChargeRequest{
		OrderRef: ref, Amount: money.MustParse("499.90").Minor(), Currency: "TRY", ItemName: "Üyelik · Kasım",
		Card: payments.SavedCard{OwnerKey: "owner-1", Token: "card-9"}, Recurring: true,
	}
	answer = map[string]any{
		"status": "success", "paymentId": "9001", "price": 499.9, "paidPrice": 499.9, "currency": "TRY", "basketId": ref,
		"conversationId": ref, "fraudStatus": 1,
		"signature": iyzico.ResponseSignature(secret, "9001", "TRY", ref, ref, "499.9", "499.9"),
	}
	r, err := p.ChargeSavedCard(ctx, account(), in)
	if err != nil || r.ProviderPaymentID != "9001" || money.New(r.PaidAmount).String() != "499.90" {
		t.Fatalf("charge %+v %v", r, err)
	}
	var pe *payments.ProviderError
	answer = map[string]any{"status": "failure", "errorCode": "10051", "errorMessage": "Yetersiz bakiye"}
	if _, err := p.ChargeSavedCard(ctx, account(), in); !errors.As(err, &pe) || pe.Temporary {
		t.Fatalf("declined: %v", err)
	}
	status = 504
	if _, err := p.ChargeSavedCard(ctx, account(), in); !errors.As(err, &pe) || !pe.Temporary {
		t.Fatalf("timeout: %v", err)
	}

	status = 200
	answer = map[string]any{
		"status": "success", "paymentStatus": "SUCCESS", "paymentId": "9001", "price": 499.9, "paidPrice": 499.9,
		"currency": "TRY", "basketId": ref, "conversationId": ref,
	}
	if n, err := p.CheckCharge(ctx, account(), in); err != nil || n.Status != payments.StatusSucceeded || n.ProviderPaymentID != "9001" {
		t.Fatalf("check found: %+v %v", n, err)
	}
	answer = map[string]any{"status": "failure", "errorCode": "5023", "errorMessage": "Ödeme bulunamadı"}
	if n, err := p.CheckCharge(ctx, account(), in); err != nil || n.Status != payments.StatusPending {
		t.Fatalf("check missing: %+v %v", n, err)
	}

	answer = map[string]any{"status": "success"}
	if err := p.RemoveSavedCard(ctx, account(), payments.SavedCard{OwnerKey: "owner-1", Token: "card-9"}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckUsesSandboxAndFields(t *testing.T) {
	p := fake(t, func(path string, _ map[string]any) (int, any) {
		if path != "/payment/bin/check" {
			t.Errorf("check path %s", path)
		}
		return 200, map[string]any{"status": "success", "binNumber": "554960"}
	})
	if err := p.Check(context.Background(), account()); err != nil {
		t.Fatal(err)
	}
	if err := p.Check(context.Background(), payments.Account{Credentials: map[string]string{"api_key": "x"}}); err == nil {
		t.Fatal("missing secret accepted")
	}
	if iyzico.New("", nil).Code() != "iyzico" {
		t.Fatal("code")
	}
}

func TestV3SignatureAndCancel(t *testing.T) {
	p := iyzico.New("", nil)
	raw := []byte(`{"paymentConversationId":"ref","iyziEventType":"CHECKOUT_FORM_AUTH","iyziPaymentId":"123","token":"tok","status":"SUCCESS"}`)
	n, err := p.ParseNotification(nil, raw)
	if err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	if err = p.VerifyNotification(account(), n, h, raw); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("missing signature %v", err)
	}
	h.Set(iyzico.SignatureHeader, iyzico.WebhookSignature(secret, payments.Webhook{EventType: "CHECKOUT_FORM_AUTH", PaymentID: "123", Token: "tok", ConversationID: "ref", Status: "SUCCESS"}))
	if err = p.VerifyNotification(account(), n, h, raw); err != nil {
		t.Fatal(err)
	}
	if err = p.VerifyNotification(account(), n, h, []byte(strings.ReplaceAll(string(raw), `"SUCCESS"`, `"FAILURE"`))); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("tampered %v", err)
	}
	p = fake(t, func(path string, body map[string]any) (int, any) {
		if path != "/payment/cancel" || body["paymentId"] != "123" {
			t.Errorf("cancel %s %+v", path, body)
		}
		return 200, map[string]any{"status": "success"}
	})
	if out, err := p.CancelPayment(context.Background(), account(), payments.RefundRequest{ProviderPaymentID: "123", RefundRef: "cancel1"}); err != nil || out.ProviderRefundID != "cancel1" {
		t.Fatalf("cancel %+v %v", out, err)
	}
}
