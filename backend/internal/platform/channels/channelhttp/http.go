package channelhttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client is the small transport port used by adapters and their fake servers.
type Client interface {
	Do(*http.Request) (*http.Response, error)
}

// DefaultClient bounds provider calls and refuses redirects carrying credentials.
func DefaultClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request,
			[]*http.Request) error {
			return http.ErrUseLastResponse
		}}
}

// Do reads a bounded provider response; errors never include URLs or provider bodies.
func Do(c Client, r *http.Request, out any) error {
	res, err := c.Do(r)
	if err != nil {
		return &channels.SendError{Code: "transport", Err: errors.New("provider request failed")}
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return errors.New("provider response unreadable or too large")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		e := fmt.Errorf("provider HTTP %d", res.StatusCode)
		if res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != 429 && res.StatusCode != 408 {
			return channels.Permanent("provider_rejected", e)
		}
		return &channels.SendError{Code: "provider_unavailable", Err: e}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.New("provider returned invalid JSON")
	}
	return nil
}

// Rejected reports whether err is a provider's 4xx answer (Do's
// "provider_rejected"): the request itself was refused, so a formatted
// message may be worth one more try as plain text.
func Rejected(err error) bool {
	var se *channels.SendError
	return errors.As(err, &se) && se.Code == "provider_rejected"
}

// Sign signs a timestamp and the exact body. Used by the custom channel in both directions.
func Sign(secret, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(timestamp + "."))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// Verify rejects missing secrets, changed bodies and requests outside the replay window.
func Verify(r *http.Request, body []byte, secret string, now time.Time) error {
	ts := r.Header.Get("X-ClickFix-Timestamp")
	if secret == "" ||
		!Fresh(ts,
			now) ||
		!hmac.Equal([]byte(Sign(secret,
			ts,
			body)),
			[]byte(r.Header.Get("X-ClickFix-Signature"))) {
		return channels.ErrSignature
	}
	return nil
}

// Fresh reports whether a Unix timestamp falls within five minutes of now.
func Fresh(ts string, now time.Time) bool {
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	return n >= now.Unix()-300 && n <= now.Unix()+300
}

// Fields builds credential metadata. Secret field keys are explicitly listed.
func Fields(required, optional, secret string) []channels.Field {
	fields := []channels.Field{}
	for _, group := range []struct {
		keys     string
		required bool
	}{{required, true}, {optional, false}} {
		for _, key := range strings.Fields(group.keys) {
			fields = append(fields,
				channels.Field{Key: key,
					LabelKey: "channels.fields." + key,
					Required: group.required,
					Secret: strings.Contains(" "+secret+" ",
						" "+key+" ")})
		}
	}
	return fields
}

// Required validates provider-dependent fields in addition to the common form fields.
func Required(inst channels.Instance, keys ...string) error {
	for _, key := range keys {
		if strings.TrimSpace(inst.Credentials[key]) == "" {
			return fmt.Errorf("missing credential: %s", key)
		}
	}
	return nil
}
