package iyzico_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/iyzico"
)

// Fixed vectors for the iyzico webhook V3 signature (X-IYZ-SIGNATURE-V3):
// hex(HMAC-SHA256(secretKey, secretKey + iyziEventType + paymentId + token +
// paymentConversationId + status)); token is empty for non Checkout Form
// payments. iyzico publishes the formula, not sample digests, so the expected
// values were computed independently with
//
//	printf '%s' "<message>" | openssl dgst -sha256 -hmac "<secret>"
func TestWebhookSignatureVectors(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   payments.Webhook
		want string
	}{
		{"checkout form", payments.Webhook{EventType: "CHECKOUT_FORM_AUTH", PaymentID: "20000001", Token: "tok-1", ConversationID: "conv1", Status: "SUCCESS"}, "dc8f0c321c4e9907fe37a59dd288faff76c4d1d32d4c1094b246e93f4215806f"},
		{"api payment", payments.Webhook{EventType: "PAYMENT_API", PaymentID: "12345", ConversationID: "conv2", Status: "FAILURE"}, "7a044221027659cc123c0ca7508792b141dabd591abc568e79c6ab004b41519e"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := iyzico.WebhookSignature("sandbox-secret-key", tc.in); got != tc.want {
				t.Fatalf("got %s", got)
			}
		})
	}
}

func TestWebhookVerificationRejectsTampering(t *testing.T) {
	p := iyzico.New("", nil)
	account := payments.Account{Credentials: map[string]string{"secret_key": "sandbox-secret-key"}}
	body, _ := json.Marshal(map[string]any{"iyziEventType": "CHECKOUT_FORM_AUTH", "iyziPaymentId": 20000001, "token": "tok-1", "paymentConversationId": "conv1", "status": "SUCCESS"})
	n, err := p.ParseNotification(nil, body)
	if err != nil {
		t.Fatal(err)
	}
	good := "dc8f0c321c4e9907fe37a59dd288faff76c4d1d32d4c1094b246e93f4215806f"
	for _, tc := range []struct {
		name, header, secret string
		ok                   bool
	}{
		{"valid", good, "sandbox-secret-key", true},
		{"upper-case hex", "DC8F0C321C4E9907FE37A59DD288FAFF76C4D1D32D4C1094B246E93F4215806F", "sandbox-secret-key", true},
		{"other secret", good, "other-secret", false},
		{"flipped digit", good[:63] + "e", "sandbox-secret-key", false},
		{"missing", "", "sandbox-secret-key", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.header != "" {
				h.Set(iyzico.SignatureHeader, tc.header)
			}
			account.Credentials["secret_key"] = tc.secret
			err := p.VerifyNotification(account, n, h, body)
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
