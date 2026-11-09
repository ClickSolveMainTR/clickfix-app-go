package apichannel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
)

func TestTestDeliverySignsAndRefusesPrivateTargets(t *testing.T) {
	secret := strings.Repeat("s", 40)
	var got *http.Request
	var body string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b := make([]byte, 512)
		n, _ := r.Body.Read(b)
		body = string(b[:n])
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	// A private target is refused before any request.
	a := New(nil)
	if _, err := a.TestDelivery(context.Background(), channels.Instance{Credentials: channels.Credentials{"signing_secret": secret, "webhook_url": srv.URL}}); err == nil {
		t.Fatal("private target accepted")
	}
	// The public-client transport refuses at dial time too.
	out, err := a.TestDelivery(context.Background(), channels.Instance{Credentials: channels.Credentials{"signing_secret": secret, "webhook_url": "https://localtest.me/x"}})
	if err != nil || out.OK {
		t.Fatalf("dial guard: %+v %v", out, err)
	}
	// Signed delivery to a reachable endpoint (test client; the URL keeps
	// passing the public-address rules).
	inst := channels.Instance{Credentials: channels.Credentials{"signing_secret": secret, "webhook_url": "https://example.com/hook"}}
	a = New(rewrite{c: srv.Client(), to: srv.URL})
	out, err = a.TestDelivery(context.Background(), inst)
	if err != nil || !out.OK || out.StatusCode != 202 || got == nil || got.Header.Get("X-ClickFix-Event") != "test" {
		t.Fatalf("%+v %v", out, err)
	}
	if got.Header.Get("X-ClickFix-Signature") != channelhttp.Sign(secret, got.Header.Get("X-ClickFix-Timestamp"), []byte(body)) || strings.Contains(body, secret) {
		t.Fatal("bad signature or secret in body")
	}
}

// rewrite sends every request to the test server.
type rewrite struct {
	c  *http.Client
	to string
}

func (r rewrite) Do(req *http.Request) (*http.Response, error) {
	u := *req.URL
	u.Scheme, u.Host = "https", strings.TrimPrefix(r.to, "https://")
	clone := req.Clone(req.Context())
	clone.URL = &u
	clone.Host = ""
	return r.c.Do(clone)
}
