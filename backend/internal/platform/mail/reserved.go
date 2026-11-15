package mail

import "strings"

// reservedDomains are second-level names reserved for documentation by
// RFC 2606 (and their subdomains).
var reservedDomains = []string{"example.com", "example.net", "example.org"}

// reservedTLDs are top-level names reserved by RFC 2606 / RFC 6761.
var reservedTLDs = []string{"test", "invalid", "localhost", "example"}

// IsReservedAddress reports whether addr is on a reserved domain (RFC 2606:
// example.com/.net/.org, *.test, *.invalid, *.localhost, *.example). Mail to
// these addresses can never be delivered; providers such as Resend reject it
// permanently. An address without a domain is not reserved (it is invalid).
func IsReservedAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if i := strings.LastIndex(addr, "<"); i >= 0 {
		addr = strings.TrimSuffix(addr[i+1:], ">")
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return false
	}
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(addr[at+1:])), ".")
	if domain == "" {
		return false
	}
	for _, d := range reservedDomains {
		if domain == d || strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	tld := domain
	if i := strings.LastIndex(domain, "."); i >= 0 {
		tld = domain[i+1:]
	}
	for _, t := range reservedTLDs {
		if tld == t {
			return true
		}
	}
	return false
}
