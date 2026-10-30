package paytr_test

import (
	"net/url"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/payments/paytr"
)

// PayTR iFrame API step 2 (bildirim URL'si): hash =
// base64(HMAC-SHA256(merchant_key, merchant_oid + merchant_salt + status +
// total_amount)). Expected value computed independently with
//
//	printf '%s' 'ORDER1SALT456success10000' | openssl dgst -sha256 -hmac KEY123 -binary | base64
func TestNotificationHashVector(t *testing.T) {
	a := payments.Account{Credentials: map[string]string{"merchant_id": "123456", "merchant_key": "KEY123", "merchant_salt": "SALT456"}}
	if got := paytr.NotificationHash(a, "ORDER1", "success", "10000"); got != "dh+XJXp27hj8OhBwFG5eYhmxjgzqcpq9zN5+4gOBiW0=" {
		t.Fatalf("got %s", got)
	}
	p := paytr.New("", nil)
	form := url.Values{"merchant_oid": {"ORDER1"}, "status": {"success"}, "total_amount": {"10000"}, "hash": {"dh+XJXp27hj8OhBwFG5eYhmxjgzqcpq9zN5+4gOBiW0="}}
	n, err := p.ParseNotification(nil, []byte(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	if err = p.VerifyNotification(a, n, nil, []byte(form.Encode())); err != nil {
		t.Fatalf("valid hash rejected: %v", err)
	}
	// Changing any signed field (amount, status, order) breaks the hash.
	for field, value := range map[string]string{"total_amount": "1", "status": "failed", "merchant_oid": "ORDER2"} {
		tampered := url.Values{}
		for k, v := range form {
			tampered[k] = v
		}
		tampered.Set(field, value)
		if err = p.VerifyNotification(a, n, nil, []byte(tampered.Encode())); err == nil {
			t.Fatalf("tampered %s accepted", field)
		}
	}
}
