package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/netguard"
)

// Outbound calls to an organization's own system. Every call goes through
// the same guard: the URL is built from the saved base URL and a path
// template (placeholders are escaped and can never change the host), it
// must pass netguard (https, public host; the dialer re-checks every
// resolved IP, so DNS rebinding is refused too), redirects are not
// followed, the organization's timeout applies, the response is capped,
// and the organization is rate limited. Logged paths have secrets masked.

const (
	// MaxResponseBytes caps what is read from the organization's system.
	MaxResponseBytes = 256 << 10
	// CallsPerMinute is the per-organization budget for outbound calls.
	CallsPerMinute  = 60
	maxRequestBytes = 32 << 10
)

// Error codes of a failed call.
const (
	CodeSSRF            = "SSRF_BLOCKED"
	CodeTimeout         = "TIMEOUT"
	CodeRateLimited     = "RATE_LIMITED"
	CodeNetwork         = "NETWORK_ERROR"
	CodeHTTP            = "HTTP_ERROR"
	CodeTooLarge        = "TOO_LARGE"
	CodeInvalidJSON     = "INVALID_JSON"
	CodeIdentityMissing = "IDENTITY_MISSING"
	CodeTemplate        = "INVALID_TEMPLATE"
	CodeNotConnected    = "NOT_CONNECTED"
)

// CallError is a failed outbound call with a stable code.
type CallError struct {
	Code    string
	Status  int
	Message string
}

func (e *CallError) Error() string { return e.Code + ": " + e.Message }

func callError(code string, status int, format string, a ...any) *CallError {
	return &CallError{Code: code, Status: status, Message: fmt.Sprintf(format, a...)}
}

// CodeOf returns the code of a call error ("" when err is nil).
func CodeOf(err error) string {
	var ce *CallError
	if errors.As(err, &ce) {
		return ce.Code
	}
	if err != nil {
		return CodeNetwork
	}
	return ""
}

// Limiter is the ratelimit.Limiter subset used here.
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Credentials are a connection's decrypted auth settings.
type Credentials struct {
	Type   string
	Header string
	Secret string
}

// Request is one outbound call.
type Request struct {
	Org      int64
	BaseURL  string
	Path     string // path template, relative to BaseURL
	Vars     map[string]string
	Method   string
	Body     any
	Auth     Credentials
	Timeout  time.Duration
	Idempote string // Idempotency-Key for writes
}

// Response is the outcome of an outbound call.
type Response struct {
	Status    int
	Body      any
	Raw       []byte
	Truncated bool
	Duration  time.Duration
	URL       string // masked, for display and logs
}

// Caller performs outbound calls. The zero value is not usable; use
// NewCaller. Tests replace HTTP and Validate to reach a local server.
type Caller struct {
	HTTP     func(timeout time.Duration) *http.Client
	Validate func(raw string) error
	Limiter  Limiter
	Now      func() time.Time
}

// NewCaller returns a Caller with the netguard client and URL rules.
func NewCaller(l Limiter) *Caller {
	if l == nil {
		l = newMemoryLimiter()
	}
	return &Caller{
		HTTP:     netguard.Client,
		Validate: func(raw string) error { return netguard.ValidateURL(raw, false) },
		Limiter:  l,
		Now:      time.Now,
	}
}

var placeholderRxp = regexp.MustCompile(`\{([a-z_][a-z0-9_.]{0,63})\}`)

// ValidTemplate checks a path template without values.
func ValidTemplate(tmpl string) error {
	if !strings.HasPrefix(tmpl, "/") || strings.HasPrefix(tmpl, "//") || len(tmpl) > 500 {
		return callError(CodeTemplate, 0, "path must start with a single /")
	}
	for _, bad := range []string{"..", "@", "\\", "#", " ", "\n", "\r", "\t"} {
		if strings.Contains(tmpl, bad) {
			return callError(CodeTemplate, 0, "path must not contain %q", bad)
		}
	}
	rest := placeholderRxp.ReplaceAllString(tmpl, "")
	if strings.ContainsAny(rest, "{}") {
		return callError(CodeTemplate, 0, "placeholders look like {external_id}")
	}
	return nil
}

// Placeholders lists the {names} of a template.
func Placeholders(tmpl string) []string {
	out := []string{}
	for _, m := range placeholderRxp.FindAllStringSubmatch(tmpl, -1) {
		out = append(out, m[1])
	}
	return out
}

// BuildURL joins base and an expanded template. Values are escaped for
// where they stand (path or query), so they cannot add segments, query
// keys or change the host. A placeholder without a value is an error.
func BuildURL(base, tmpl string, vars map[string]string) (string, error) {
	if err := ValidTemplate(tmpl); err != nil {
		return "", err
	}
	b, err := url.Parse(strings.TrimRight(strings.TrimSpace(base), "/"))
	if err != nil || b.Host == "" || b.RawQuery != "" || b.Fragment != "" || b.User != nil {
		return "", callError(CodeTemplate, 0, "base URL is invalid")
	}
	q := strings.Index(tmpl, "?")
	var missing []string
	expanded := placeholderRxp.ReplaceAllStringFunc(tmpl, func(m string) string {
		name := m[1 : len(m)-1]
		v := strings.TrimSpace(vars[name])
		if v == "" {
			missing = append(missing, name)
			return ""
		}
		// PathEscape keeps "." and ".."; as a whole segment they would move
		// the path, so they are refused like a missing value.
		if v == "." || v == ".." {
			missing = append(missing, name)
			return ""
		}
		idx := strings.Index(tmpl, m)
		if q >= 0 && idx > q {
			return url.QueryEscape(v)
		}
		return url.PathEscape(v)
	})
	if len(missing) > 0 {
		return "", callError(CodeIdentityMissing, 0, "no value for %s", strings.Join(missing, ", "))
	}
	full := b.String() + expanded
	u, err := url.Parse(full)
	if err != nil || u.Host != b.Host || u.Scheme != b.Scheme {
		return "", callError(CodeTemplate, 0, "path changes the host")
	}
	return full, nil
}

var secretQueryKey = regexp.MustCompile(`(?i)(key|token|secret|pass|auth|sig|session|credential)`)

// MaskURL hides secret query values and any known secret in a URL.
// Query values are masked by key; the path goes through the shared
// secret redactor (long tokens, known key prefixes).
func MaskURL(raw string, secrets ...string) string {
	out := raw
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		parts := []string{}
		for _, pair := range strings.Split(u.RawQuery, "&") {
			if pair == "" {
				continue
			}
			k, v, _ := strings.Cut(pair, "=")
			if secretQueryKey.MatchString(k) {
				v = capability.Masked
			} else {
				v = capability.RedactSecrets(v)
			}
			parts = append(parts, k+"="+v)
		}
		out = u.Scheme + "://" + u.Host + capability.RedactSecrets(u.EscapedPath())
		if len(parts) > 0 {
			out += "?" + strings.Join(parts, "&")
		}
	}
	for _, s := range secrets {
		for _, part := range []string{s, url.QueryEscape(s), url.PathEscape(s)} {
			if len(part) >= 4 {
				out = strings.ReplaceAll(out, part, capability.Masked)
			}
		}
	}
	return out
}

func (a Credentials) apply(h http.Header) {
	switch a.Type {
	case "bearer":
		if a.Secret != "" {
			h.Set("Authorization", "Bearer "+a.Secret)
		}
	case "header":
		if a.Secret != "" && a.Header != "" {
			h.Set(a.Header, a.Secret)
		}
	case "basic":
		if a.Secret != "" {
			h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(a.Secret)))
		}
	}
}

func (a Credentials) secrets() []string {
	if a.Secret == "" {
		return nil
	}
	out := []string{a.Secret, base64.StdEncoding.EncodeToString([]byte(a.Secret))}
	if user, pass, ok := strings.Cut(a.Secret, ":"); ok && a.Type == "basic" {
		out = append(out, user, pass)
	}
	return out
}

// Do performs one call. It never returns the secret in an error or URL.
func (c *Caller) Do(ctx context.Context, r Request) (Response, error) {
	target, err := BuildURL(r.BaseURL, r.Path, r.Vars)
	if err != nil {
		return Response{}, err
	}
	masked := MaskURL(target, r.Auth.secrets()...)
	resp := Response{URL: masked}
	if err := c.Validate(target); err != nil {
		return resp, callError(CodeSSRF, 0, "the address is not a public https address")
	}
	if ok, _ := c.Limiter.Allow(ctx, "integration_call", fmt.Sprint(r.Org), CallsPerMinute, time.Minute); !ok {
		return resp, callError(CodeRateLimited, 0, "too many calls to the connected system; try again in a minute")
	}
	var body io.Reader
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodDelete {
		raw, err := json.Marshal(r.Body)
		if err != nil || len(raw) > maxRequestBytes {
			return resp, callError(CodeTemplate, 0, "request body is too large")
		}
		body = bytes.NewReader(raw)
	}
	timeout := r.Timeout
	if timeout <= 0 || timeout > 15*time.Second {
		timeout = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, r.Method, target, body)
	if err != nil {
		return resp, callError(CodeTemplate, 0, "request could not be built")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ClickFix-Connect/1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.Idempote != "" {
		req.Header.Set("Idempotency-Key", r.Idempote)
	}
	r.Auth.apply(req.Header)
	start := c.Now()
	res, err := c.HTTP(timeout).Do(req)
	resp.Duration = c.Now().Sub(start)
	if err != nil {
		var ne net.Error
		switch {
		case errors.Is(err, netguard.ErrForbiddenTarget) || strings.Contains(err.Error(), netguard.ErrForbiddenTarget.Error()):
			return resp, callError(CodeSSRF, 0, "the address resolves to a non-public network")
		case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout():
			return resp, callError(CodeTimeout, 0, "no answer within %s", timeout)
		default:
			return resp, callError(CodeNetwork, 0, "connection failed")
		}
	}
	defer func() { _ = res.Body.Close() }()
	resp.Status = res.StatusCode
	raw, err := io.ReadAll(io.LimitReader(res.Body, MaxResponseBytes+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return resp, callError(CodeTimeout, res.StatusCode, "answer did not finish within %s", timeout)
		}
		return resp, callError(CodeNetwork, res.StatusCode, "answer could not be read")
	}
	if len(raw) > MaxResponseBytes {
		return resp, callError(CodeTooLarge, res.StatusCode, "answer is larger than %d KB", MaxResponseBytes>>10)
	}
	resp.Raw = raw
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &resp.Body); err != nil {
			resp.Body = clipText(string(raw), 2000)
			if res.StatusCode < 300 && r.Method == http.MethodGet {
				return resp, callError(CodeInvalidJSON, res.StatusCode, "answer is not JSON")
			}
		}
	}
	if res.StatusCode >= 300 {
		return resp, callError(CodeHTTP, res.StatusCode, "the system answered HTTP %d", res.StatusCode)
	}
	return resp, nil
}

func clipText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// memoryLimiter is the in-process fallback when Redis is not wired (tests,
// single-process tools). It is a fixed one-minute window per subject.
type memoryLimiter struct {
	mu     sync.Mutex
	window map[string]memoryWindow
}

type memoryWindow struct {
	start time.Time
	n     int
}

func newMemoryLimiter() *memoryLimiter { return &memoryLimiter{window: map[string]memoryWindow{}} }

func (m *memoryLimiter) Allow(_ context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := action + ":" + subject
	now := time.Now()
	w := m.window[k]
	if now.Sub(w.start) >= window {
		w = memoryWindow{start: now}
	}
	w.n++
	m.window[k] = w
	if w.n > limit {
		return false, window - now.Sub(w.start)
	}
	return true, 0
}
