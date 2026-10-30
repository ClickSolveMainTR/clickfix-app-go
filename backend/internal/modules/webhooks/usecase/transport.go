// Ported from rentopia-app-go modules/publicapi/usecase/webhooks.go.
package usecase

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/netguard"
)

var ErrWebhookTarget = errors.New("webhook URL must be a public HTTPS address")

// privateAddr reports addresses a webhook must never reach (loopback,
// private, link-local, CGNAT, NAT64, reserved ranges: see netguard).
func privateAddr(a netip.Addr) bool {
	return !netguard.IsPublic(a)
}

// webhookTransport refuses private targets at dial time (after DNS), so a
// public name that resolves to an internal address is refused too.
func webhookTransport() http.RoundTripper {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			a, err := netip.ParseAddr(host)
			if err != nil || privateAddr(a) {
				return ErrWebhookTarget
			}
			return nil
		},
	}
	return &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 20,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
	}
}

// validateEndpointURL checks the target before it is stored.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return ErrWebhookTarget
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local") {
		return ErrWebhookTarget
	}
	if a, err := netip.ParseAddr(host); err == nil && privateAddr(a) {
		return ErrWebhookTarget
	}
	return nil
}

// PublicHTTPClient applies the same dial-time SSRF protection used for signed webhooks.
func PublicHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, Transport: webhookTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// ValidateMonitorURL permits public HTTP as well as HTTPS without credentials or fragments.
func ValidateMonitorURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return ErrWebhookTarget
	}
	if u.Scheme == "http" {
		u.Scheme = "https"
	}
	return ValidateURL(u.String())
}
