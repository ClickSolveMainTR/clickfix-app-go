package usecase

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/google/uuid"
	"io"
	"net/http"
	"strconv"
	"strings"

	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"testing"
)

func TestWebhookSignatureAndTargetPolicy(t *testing.T) {
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write([]byte("123.{\"event\":\"message.received\"}"))
	body := []byte(`{"event":"message.received"}`)
	if got := SignWebhook("test-secret", 123, body); got != "v1="+hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal(got)
	}
	if SignWebhook("test-secret", 124, body) == SignWebhook("test-secret", 123, body) {
		t.Fatal("timestamp not authenticated")
	}
	for _, target := range []string{"http://example.com/hook", "https://127.0.0.1/hook", "https://[::1]/hook", "https://169.254.169.254/latest/meta-data", "https://user:pass@example.com/hook", "https://example.local/hook", "https://example.com/hook#frag"} {
		if ValidateURL(target) == nil {
			t.Errorf("accepted %s", target)
		}
	}
	if err := ValidateURL("https://hooks.example.com/events"); err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"127.0.0.1", "::1", "::ffff:192.168.1.1", "100.64.0.1", "169.254.169.254", "fc00::1"} {
		if !privateAddr(netip.MustParseAddr(ip)) {
			t.Errorf("allowed %s", ip)
		}
	}
}

type recordingTransport struct{ t *testing.T }

func (rt recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		rt.t.Fatal(err)
	}
	stamp, err := strconv.ParseInt(req.Header.Get("ClickFix-Webhook-Timestamp"), 10, 64)
	if err != nil {
		rt.t.Fatal(err)
	}
	if req.Header.Get("ClickFix-Webhook-Signature") != SignWebhook("test-secret", stamp, body) {
		rt.t.Fatal("unsigned delivery")
	}
	if req.Header.Get("ClickFix-Webhook-Id") == "" || req.Header.Get("ClickFix-Webhook-Event") != "message.received" {
		rt.t.Fatal("missing correlation headers")
	}
	return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
}
func TestSignedDelivery(t *testing.T) {
	box, err := crypto.NewSecretBox(strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := box.Encrypt("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Box: box, Client: &http.Client{Transport: recordingTransport{t}}}
	status, err := svc.post(context.Background(), db.OutboundWebhook{Url: "https://hooks.example.com/events", SecretEnc: enc}, db.OutboundWebhookDelivery{Uuid: uuid.New(), EventName: "message.received", Payload: []byte(`{"private":"not logged"}`)})
	if err != nil || status != 204 {
		t.Fatal(status, err)
	}
}
