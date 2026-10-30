package brands

import (
	"net"
	"strings"
)

// normalizeHelpHost is adapted from olexfilms-ecosystem platform/brandctx.NormalizeHost.
// ClickFix resolves an explicitly configured org brand and never falls back to a global brand.
func normalizeHelpHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, ','); i >= 0 {
		raw = strings.TrimSpace(raw[:i])
	}
	if h, _, err := net.SplitHostPort(raw); err == nil {
		raw = h
	}
	raw = strings.TrimSuffix(strings.Trim(raw, "[]"), ".")
	return strings.ToLower(raw)
}
func validHelpHost(raw string) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 253 {
		return false
	}
	for _, label := range strings.Split(raw, ".") {
		if !subdomainRE.MatchString(label) {
			return false
		}
	}
	return true
}
