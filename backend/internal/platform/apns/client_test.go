package apns

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// ephemeralKey generates a throwaway P-256 key as .p8 PEM; no key is committed.
func ephemeralKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestNewDisabledWithoutKey(t *testing.T) {
	c, err := New(Config{KeyID: "K", TeamID: "", PrivateKey: "x", Topic: "com.getclickfix.app"}, nil)
	if err != nil || c != nil {
		t.Fatalf("want nil client, got %v %v", c, err)
	}
	var nilClient *Client
	if _, err := nilClient.SendLiveActivity(context.Background(), Notification{Token: "ab"}); err == nil {
		t.Fatal("nil client must refuse to send")
	}
}

func TestParsePrivateKeyForms(t *testing.T) {
	_, pemText := ephemeralKey(t)
	for name, raw := range map[string]string{
		"pem":     pemText,
		"base64":  base64.StdEncoding.EncodeToString([]byte(pemText)),
		"escaped": strings.ReplaceAll(pemText, "\n", `\n`),
	} {
		if _, err := ParsePrivateKey(raw); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := ParsePrivateKey("not a key"); err == nil {
		t.Error("garbage must fail")
	}
}

func TestProviderTokenSignedAndCached(t *testing.T) {
	key, pemText := ephemeralKey(t)
	c, err := New(Config{KeyID: "KEY123", TeamID: "TEAM456", PrivateKey: pemText, Topic: "com.getclickfix.app"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	c.now = func() time.Time { return now }
	first, err := c.providerToken()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := jwtlib.Parse(first, func(tok *jwtlib.Token) (any, error) {
		if tok.Method != jwtlib.SigningMethodES256 {
			t.Fatalf("alg %v", tok.Method)
		}
		return &key.PublicKey, nil
	}, jwtlib.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Header["kid"] != "KEY123" {
		t.Fatalf("kid %v", parsed.Header["kid"])
	}
	claims := parsed.Claims.(jwtlib.MapClaims)
	if claims["iss"] != "TEAM456" || int64(claims["iat"].(float64)) != now.Unix() {
		t.Fatalf("claims %v", claims)
	}
	now = now.Add(49 * time.Minute)
	if again, _ := c.providerToken(); again != first {
		t.Fatal("token must be cached within 50 minutes")
	}
	now = now.Add(2 * time.Minute)
	if again, _ := c.providerToken(); again == first {
		t.Fatal("token must be re-signed after 50 minutes")
	}
}

func TestSendLiveActivityRequest(t *testing.T) {
	_, pemText := ephemeralKey(t)
	var got *http.Request
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("apns-id", "abc")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c, err := New(Config{KeyID: "K", TeamID: "T", PrivateKey: pemText, Topic: "com.getclickfix.app"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	c.SetHost(Sandbox, srv.URL)
	exp := time.Unix(1_900_000_000, 0)
	res, err := c.SendLiveActivity(context.Background(), Notification{
		Token: "a1b2c3", Environment: Sandbox, Priority: PriorityLow, Expiration: exp, Payload: []byte(`{"aps":{}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || res.APNsID != "abc" {
		t.Fatalf("result %+v", res)
	}
	if got.Method != http.MethodPost || got.URL.Path != "/3/device/a1b2c3" {
		t.Fatalf("request %s %s", got.Method, got.URL.Path)
	}
	want := map[string]string{
		"apns-push-type":  "liveactivity",
		"apns-topic":      "com.getclickfix.app.push-type.liveactivity",
		"apns-priority":   "5",
		"apns-expiration": "1900000000",
		"content-type":    "application/json",
	}
	for h, v := range want {
		if got.Header.Get(h) != v {
			t.Errorf("%s = %q, want %q", h, got.Header.Get(h), v)
		}
	}
	if !strings.HasPrefix(got.Header.Get("authorization"), "bearer ey") {
		t.Errorf("authorization %q", got.Header.Get("authorization"))
	}
	if body != `{"aps":{}}` {
		t.Errorf("body %q", body)
	}
}

func TestSendLiveActivityRejections(t *testing.T) {
	_, pemText := ephemeralKey(t)
	status, reason := http.StatusGone, "Unregistered"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"reason":"`+reason+`"}`)
	}))
	defer srv.Close()
	c, _ := New(Config{KeyID: "K", TeamID: "T", PrivateKey: pemText, Topic: "com.getclickfix.app"}, srv.Client())
	c.SetHost(Production, srv.URL)
	send := func() Result {
		res, err := c.SendLiveActivity(context.Background(), Notification{Token: "ff", Environment: Production, Payload: []byte(`{}`)})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if res := send(); res.OK() || !res.TokenInvalid() || res.Reason != "Unregistered" {
		t.Fatalf("410: %+v", res)
	}
	status, reason = http.StatusBadRequest, "BadDeviceToken"
	if res := send(); !res.TokenInvalid() {
		t.Fatalf("BadDeviceToken: %+v", res)
	}
	status, reason = http.StatusTooManyRequests, "TooManyRequests"
	if res := send(); res.TokenInvalid() || !res.Retryable() {
		t.Fatalf("429: %+v", res)
	}
	status, reason = http.StatusForbidden, "ExpiredProviderToken"
	_ = send()
	if c.token != "" {
		t.Fatal("expired provider token must be dropped")
	}
	if _, err := c.SendLiveActivity(context.Background(), Notification{Token: "ff", Payload: make([]byte, 5000)}); err != ErrPayloadTooLarge {
		t.Fatalf("large payload: %v", err)
	}
}
