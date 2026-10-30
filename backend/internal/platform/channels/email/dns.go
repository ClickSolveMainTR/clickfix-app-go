package email

import (
	"context"
	"net"
	"strings"
)

// RecommendedDMARC is the record the domain wizard suggests when a domain
// has none (monitoring only, so nothing is rejected while it is set up).
const RecommendedDMARC = "v=DMARC1; p=none;"

// LookupDMARC returns the DMARC policy of a domain ("" when it has none).
// Resend verifies SPF and DKIM; DMARC is checked here for the wizard.
func LookupDMARC(ctx context.Context, r *net.Resolver, domain string) (string, error) {
	if r == nil {
		r = net.DefaultResolver
	}
	txts, err := r.LookupTXT(ctx, "_dmarc."+strings.TrimSuffix(domain, "."))
	if err != nil {
		var dnsErr *net.DNSError
		if ok := asDNSError(err, &dnsErr); ok && dnsErr.IsNotFound {
			return "", nil
		}
		return "", err
	}
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=dmarc1") {
			return strings.TrimSpace(t), nil
		}
	}
	return "", nil
}

func asDNSError(err error, target **net.DNSError) bool {
	e, ok := err.(*net.DNSError)
	if ok {
		*target = e
	}
	return ok
}
