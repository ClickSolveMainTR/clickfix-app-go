package middleware

import "net/http"

// SecurityHeaders sets API headers without relying on a reverse proxy.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// HSTS belongs to the TLS terminator; never trust arbitrary forwarded headers here.
		next.ServeHTTP(w, r)
	})
}
