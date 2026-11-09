package capability

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
)

// RandomSecret is 32 random bytes as hex (64 characters).
func RandomSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Masked replaces a secret in stored params and audit payloads.
const Masked = "[SECRET]"

var secretRules = []*regexp.Regexp{
	// key=value / key: value pairs whose key names a secret.
	regexp.MustCompile(`(?i)\b((?:api[_-]?key|apikey|secret|signing[_-]?secret|client[_-]?secret|token|access[_-]?token|refresh[_-]?token|password|passwd|parola|şifre|sifre|anahtar)["']?\s*[:=]\s*["']?)([^\s"',;]{6,})`),
	// Authorization headers.
	regexp.MustCompile(`(?i)\b(bearer\s+|basic\s+|token\s+)([A-Za-z0-9._~+/=-]{16,})`),
	// Well-known key prefixes (OpenAI, Anthropic, Stripe, GitHub, Slack, AWS, Google).
	regexp.MustCompile(`()\b((?:sk|pk|rk)[-_](?:live|test|ant|proj)?[-_]?[A-Za-z0-9_-]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{30,})`),
	// Long opaque tokens (hex or base64url) of 32+ characters.
	regexp.MustCompile(`()\b([A-Fa-f0-9]{32,}|[A-Za-z0-9_-]{40,})\b`),
}

// RedactSecrets masks secret-looking values in user text before it is
// stored or sent to a model. It keeps the key name so the assistant can
// still say "you shared an API key; enter it in the card instead".
func RedactSecrets(text string) string {
	for i, rule := range secretRules {
		if i == len(secretRules)-1 {
			text = redactOutsideURLs(rule, text)
			continue
		}
		text = rule.ReplaceAllString(text, "${1}"+Masked)
	}
	return text
}

var urlRxp = regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>]+`)

// redactOutsideURLs applies the long-token rule outside URLs: long path
// segments of a webhook URL are not secrets (query tokens are already
// masked by the key=value rule).
func redactOutsideURLs(rule *regexp.Regexp, text string) string {
	urls := urlRxp.FindAllStringIndex(text, -1)
	inURL := func(i int) bool {
		for _, u := range urls {
			if i >= u[0] && i < u[1] {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	last := 0
	for _, m := range rule.FindAllStringIndex(text, -1) {
		if inURL(m[0]) {
			continue
		}
		b.WriteString(text[last:m[0]])
		b.WriteString(Masked)
		last = m[1]
	}
	b.WriteString(text[last:])
	return b.String()
}
