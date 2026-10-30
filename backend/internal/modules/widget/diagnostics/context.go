// Package diagnostics validates capture metadata and masks technical context before persistence.
package diagnostics

import (
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var ErrInvalid = errors.New("diagnostics: invalid context or recording")

type NetworkFailure struct {
	URL    string `json:"url"`
	Method string `json:"method"`
	Status int    `json:"status"`
	Error  string `json:"error"`
}

// Context deliberately excludes request/response bodies, headers, cookies and storage.
// SDKs must collect only after visitor consent and mask visible PII before capturing pixels.
type Context struct {
	ConsoleErrors   []string         `json:"console_errors"`
	NetworkFailures []NetworkFailure `json:"network_failures"`
	Browser         string           `json:"browser"`
	OS              string           `json:"os"`
	URL             string           `json:"url"`
	Metadata        map[string]any   `json:"metadata"`
}

var secretRE = regexp.MustCompile(`(?i)(?:bearer\s+|["']?(?:(?:access[_-]?)?token|password|secret|api[_-]?key|authorization|cookie|full[_-]?name|name|ssn|iban)["']?\s*[:=]\s*(?:bearer\s+)?)(?:"[^"]*"|'[^']*'|[^\s,;}]+)`)
var phoneRE = regexp.MustCompile(`\+?[0-9][0-9 ()-]{7,}[0-9]`)
var urlRE = regexp.MustCompile(`https?://[^\s<>"']+`)

func MaskText(s string) string {
	s = urlRE.ReplaceAllStringFunc(s, MaskURL)
	s = ai.RedactPII(s)
	s = secretRE.ReplaceAllString(s, "[secret]")
	s = phoneRE.ReplaceAllString(s, "[number]")
	if utf8.RuneCountInString(s) > 2000 {
		s = string([]rune(s)[:2000])
	}
	return s
}
func MaskURL(s string) string {
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "[url]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	u.RawFragment = ""
	u.ForceQuery = false
	// Paths may contain customer IDs, e-mails or tokens. Keep only the origin.
	u.Path = ""
	u.RawPath = ""
	return u.String()
}
func sensitive(k string) bool {
	k = strings.ToLower(k)
	for _, part := range []string{"email", "phone", "name", "address", "token", "secret", "password", "auth", "cookie", "session", "credit", "card", "body", "header", "storage", "ip", "user", "customer", "contact", "account", "ident", "iban", "ssn"} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}
func maskValue(v any, depth int) (any, error) {
	if depth > 4 {
		return nil, ErrInvalid
	}
	switch v := v.(type) {
	case nil, bool:
		return v, nil
	case float64:
		if v >= 100000000 || v <= -100000000 {
			return "[number]", nil
		}
		return v, nil
	case string:
		return MaskText(v), nil
	case []any:
		if len(v) > 50 {
			return nil, ErrInvalid
		}
		out := make([]any, 0, len(v))
		for _, x := range v {
			y, err := maskValue(x, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, y)
		}
		return out, nil
	case map[string]any:
		if len(v) > 50 {
			return nil, ErrInvalid
		}
		out := map[string]any{}
		for k, x := range v {
			if len(k) > 80 {
				return nil, ErrInvalid
			}
			if sensitive(k) {
				out[k] = "[redacted]"
				continue
			}
			y, err := maskValue(x, depth+1)
			if err != nil {
				return nil, err
			}
			out[MaskText(k)] = y
		}
		return out, nil
	default:
		return nil, ErrInvalid
	}
}
func (c Context) Mask() (Context, error) {
	if c.ConsoleErrors == nil {
		c.ConsoleErrors = []string{}
	}
	if c.NetworkFailures == nil {
		c.NetworkFailures = []NetworkFailure{}
	}
	if len(c.ConsoleErrors) > 50 || len(c.NetworkFailures) > 50 || len(c.Browser) > 200 || len(c.OS) > 200 || len(c.URL) > 4000 {
		return Context{}, ErrInvalid
	}
	for i, v := range c.ConsoleErrors {
		if len(v) > 10000 {
			return Context{}, ErrInvalid
		}
		c.ConsoleErrors[i] = MaskText(v)
	}
	for i, v := range c.NetworkFailures {
		if len(v.URL) > 4000 || len(v.Error) > 10000 || v.Status < 0 || v.Status > 599 {
			return Context{}, ErrInvalid
		}
		switch v.Method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "":
		default:
			return Context{}, ErrInvalid
		}
		v.URL = MaskURL(v.URL)
		v.Error = MaskText(v.Error)
		c.NetworkFailures[i] = v
	}
	c.URL = MaskURL(c.URL)
	c.Browser = MaskText(c.Browser)
	c.OS = MaskText(c.OS)
	if c.Metadata == nil {
		c.Metadata = map[string]any{}
	}
	masked, err := maskValue(c.Metadata, 0)
	if err != nil {
		return Context{}, err
	}
	c.Metadata = masked.(map[string]any)
	raw, err := json.Marshal(c)
	if err != nil || len(raw) > 64<<10 {
		return Context{}, ErrInvalid
	}
	return c, nil
}
