// Package netguard keeps server-side HTTP calls to user-supplied URLs off the
// private network (SSRF). Callers validate the URL before storing it and use
// Client (or DialControl) for the request itself: the check runs again at
// dial time on the resolved address, so a public name that resolves (or is
// re-bound) to an internal address is refused too. Redirects are not
// followed, so a public target cannot bounce the request inward.
package netguard

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrForbiddenTarget is returned for URLs or addresses on a non-public network.
var ErrForbiddenTarget = errors.New("netguard: target must be a public address")

// blocked lists special-purpose ranges that the netip predicates do not cover
// (RFC 6890 and the IANA special-purpose registries). Cloud metadata
// endpoints live in link-local (169.254.169.254, fd00:ec2::254 via fc00::/7)
// and CGNAT (100.100.100.200) space.
var blocked = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{
		"0.0.0.0/8",       // "this network"; 0.x routes to the local host on Linux
		"100.64.0.0/10",   // shared address space (CGNAT, some cloud metadata)
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // TEST-NET-1
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // reserved, includes 255.255.255.255
		"64:ff9b::/96",    // NAT64: would reach any IPv4 address, private included
		"64:ff9b:1::/48",  // local-use NAT64
		"100::/64",        // discard-only
		"2001::/32",       // Teredo (embeds an IPv4 address)
		"2001:db8::/32",   // documentation
		"2002::/16",       // 6to4 (embeds an IPv4 address)
		"fc00::/7",        // unique local
		"fec0::/10",       // deprecated site-local
	} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

// IsPublic reports whether a is a globally routable unicast address that a
// server-side request may reach.
func IsPublic(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() ||
		a.IsLinkLocalUnicast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	for _, p := range blocked {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// ValidateURL checks a user-supplied URL before it is stored or fetched:
// https only (http too when allowHTTP), no credentials or fragment, no
// internal host names and no literal non-public address. DNS is checked at
// dial time by Client / DialControl.
func ValidateURL(raw string, allowHTTP bool) error {
	if len(raw) > 2048 {
		return ErrForbiddenTarget
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || u.Hostname() == "" {
		return ErrForbiddenTarget
	}
	if u.Scheme != "https" && (!allowHTTP || u.Scheme != "http") {
		return ErrForbiddenTarget
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || !strings.Contains(host, ".") && !strings.Contains(host, ":") {
		return ErrForbiddenTarget
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".lan", ".home.arpa"} {
		if strings.HasSuffix(host, suffix) {
			return ErrForbiddenTarget
		}
	}
	if a, err := netip.ParseAddr(host); err == nil && !IsPublic(a) {
		return ErrForbiddenTarget
	}
	return nil
}

// DialControl is a net.Dialer Control hook that refuses non-public addresses.
// It sees the resolved IP of every connection attempt.
func DialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrForbiddenTarget
	}
	a, err := netip.ParseAddr(host)
	if err != nil || !IsPublic(a) {
		return ErrForbiddenTarget
	}
	return nil
}

// Transport ignores proxy environment variables (a proxy would dial on our
// behalf, bypassing DialControl) and refuses non-public addresses.
func Transport() *http.Transport {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: DialControl}
	return &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 20,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
	}
}

// Client returns an HTTP client for user-supplied URLs: public addresses only
// and redirects are returned to the caller instead of being followed.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout, Transport: Transport(),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
