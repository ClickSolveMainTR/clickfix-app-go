package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestIsPublic(t *testing.T) {
	for _, ip := range []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200",
		"0.0.0.0", "0.1.2.3", "198.18.0.1", "240.0.0.1", "255.255.255.255", "224.0.0.1", "192.0.0.170",
		"::", "::1", "fe80::1", "fd00:ec2::254", "fc00::1", "ff02::1", "64:ff9b::a9fe:a9fe", "2002:a9fe:a9fe::1",
		"2001:0:4136:e378:8000:63bf:3fff:fdd2", "::ffff:127.0.0.1", "::ffff:169.254.169.254", "2001:db8::1",
	} {
		if IsPublic(netip.MustParseAddr(ip)) {
			t.Errorf("IsPublic(%s) = true, want false", ip)
		}
	}
	for _, ip := range []string{"1.1.1.1", "8.8.8.8", "93.184.216.34", "2606:4700:4700::1111", "::ffff:8.8.8.8"} {
		if !IsPublic(netip.MustParseAddr(ip)) {
			t.Errorf("IsPublic(%s) = false, want true", ip)
		}
	}
}

func TestValidateURL(t *testing.T) {
	for _, raw := range []string{
		"http://example.com/x", "ftp://example.com", "https://user:pw@example.com", "https://example.com/#f",
		"https://localhost/x", "https://api.localhost", "https://metadata.google.internal/", "https://printer.local",
		"https://redis:6379", "https://127.0.0.1/", "https://[::1]/", "https://169.254.169.254/latest/meta-data",
		"https://[fd00:ec2::254]/", "https://10.0.0.1:8443/", "https://0.0.0.0/", "javascript:alert(1)", "https:///x",
	} {
		if err := ValidateURL(raw, false); !errors.Is(err, ErrForbiddenTarget) {
			t.Errorf("ValidateURL(%q) = %v, want ErrForbiddenTarget", raw, err)
		}
	}
	for _, raw := range []string{"https://example.com/hook", "https://fcm.googleapis.com/fcm/send/abc", "https://1.1.1.1/"} {
		if err := ValidateURL(raw, false); err != nil {
			t.Errorf("ValidateURL(%q) = %v", raw, err)
		}
	}
	if err := ValidateURL("http://example.com/", true); err != nil {
		t.Errorf("allowHTTP: %v", err)
	}
}

// TestClientRefusesPrivateAfterResolution: a URL that passes string checks
// but resolves to loopback (DNS rebinding) is refused at dial time.
func TestClientRefusesPrivateAfterResolution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	c := Client(2 * time.Second)
	// Point a public-looking name at the loopback test server.
	tr := c.Transport.(*http.Transport)
	base := tr.DialContext
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return base(ctx, network, srv.Listener.Addr().String())
	}
	req, _ := http.NewRequest(http.MethodGet, "http://public.example.com/", nil)
	resp, err := c.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("request to a loopback address succeeded")
	}
	if !errors.Is(err, ErrForbiddenTarget) {
		t.Fatalf("err = %v, want ErrForbiddenTarget", err)
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	c := Client(time.Second)
	if c.CheckRedirect == nil || c.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("client must not follow redirects")
	}
	if c.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("client must ignore proxy environment variables")
	}
}
