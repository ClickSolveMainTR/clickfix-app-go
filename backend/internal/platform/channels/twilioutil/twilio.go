// Package twilioutil shares Twilio's signature and account protocol between SMS and voice.
package twilioutil

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // Twilio's mandated request signature algorithm.
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
)

// Sign is ported from takwim-app-go platform/voice/twilio.Sign.
func Sign(token, fullURL string, form url.Values) string {
	var b strings.Builder
	b.WriteString(fullURL)
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		vs := slices.Clone(form[k])
		slices.Sort(vs)
		vs = slices.Compact(vs)
		for _, v := range vs {
			b.WriteString(k)
			b.WriteString(v)
		}
	}
	m := hmac.New(sha1.New, []byte(token))
	m.Write([]byte(b.String()))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}
func Verify(r *http.Request, b []byte, inst channels.Instance, publicBase string) error {
	form, err := url.ParseQuery(string(b))
	if err != nil {
		return channels.ErrSignature
	}
	if inst.Credentials["auth_token"] == "" || publicBase == "" {
		return channels.ErrSignature
	}
	// Use the configured public origin and the actual path/query. Never trust forwarded host headers.
	full := strings.TrimRight(publicBase, "/") + r.URL.RequestURI()
	if !hmac.Equal([]byte(Sign(inst.Credentials["auth_token"], full, form)), []byte(r.Header.Get("X-Twilio-Signature"))) {
		return channels.ErrSignature
	}
	if form.Get("AccountSid") != inst.Credentials["account_sid"] {
		return channels.ErrSignature
	}
	return nil
}
func E164(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if r != '+' && r != ' ' && r != '(' && r != ')' && r != '-' {
			return ""
		}
	}
	n := b.String()
	n = strings.TrimPrefix(n, "00")
	if len(n) == 10 && n[0] == '5' {
		n = "90" + n
	}
	if len(n) == 11 && n[0] == '0' {
		n = "9" + n
	}
	if len(n) < 8 || len(n) > 15 || n[0] == '0' {
		return ""
	}
	return "+" + n
}
func AccountPath(inst channels.Instance) string {
	return "/2010-04-01/Accounts/" + url.PathEscape(inst.Credentials["account_sid"])
}
func Request(ctx context.Context,
	inst channels.Instance,
	method,
	target string,
	form url.Values) (*http.Request,
	error) {
	req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.New("invalid Twilio request")
	}
	req.SetBasicAuth(inst.Credentials["account_sid"], inst.Credentials["auth_token"])
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, nil
}
func Check(ctx context.Context, c channelhttp.Client, base string, inst channels.Instance) error {
	if err := channelhttp.Required(inst, "account_sid", "auth_token", "from_number"); err != nil {
		return err
	}
	if len(inst.Credentials["account_sid"]) != 34 ||
		!strings.HasPrefix(inst.Credentials["account_sid"],
			"AC") ||
		E164(inst.Credentials["from_number"]) == "" {
		return errors.New("invalid Twilio account or caller number")
	}
	req, err := Request(ctx, inst, http.MethodGet, base+AccountPath(inst)+".json", nil)
	if err != nil {
		return err
	}
	return channelhttp.Do(c, req, nil)
}
