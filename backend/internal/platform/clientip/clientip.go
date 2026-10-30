// Package clientip returns the caller address used for per-IP rate limits.
//
// In production only the Next.js BFF reaches the API (compose.prod.yml: the
// backend is "expose"d, never published). The BFF derives the browser's IP
// from the edge proxy and sends it as a single X-Forwarded-For value
// (frontend/src/lib/server/upstream.ts clientIpFromHeaders). Without it every
// public request would carry the BFF's own address and share one global
// rate-limit bucket.
package clientip

import (
	"net"
	"net/http"
	"strings"
)

// FromRequest returns the left-most X-Forwarded-For hop set by the BFF, else
// the peer address.
func FromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
