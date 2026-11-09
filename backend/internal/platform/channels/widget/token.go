package widget

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Visitor token lifetimes. A token authorizes the visitor API for one widget
// and one visitor for TokenTTL; an expired token whose signature is valid
// still proves who the visitor was for RenewWindow after expiry, so bootstrap renews it
// for the same visitor instead of starting an anonymous stranger.
const (
	TokenTTL    = 12 * time.Hour
	RenewWindow = 90 * 24 * time.Hour
)

// Token errors.
var (
	ErrTokenInvalid = errors.New("widget: visitor token is invalid")
	ErrTokenExpired = errors.New("widget: visitor token has expired")
)

// Claims identify a visitor of one widget.
type Claims struct {
	// Instance is the widget (channel instance uuid) the token is bound to.
	Instance uuid.UUID `json:"i"`
	// Visitor is the anonymous visitor id kept in the browser.
	Visitor string `json:"v"`
	// User is the customer's own user id once identify verified its
	// user_hash ("" for anonymous visitors).
	User string `json:"u,omitempty"`
	// Issued: when the visitor first got a token (renewals keep it).
	Issued int64 `json:"t"`
	// Expires: unix seconds.
	Expires int64 `json:"e"`
}

// ExternalID is the contact identity id the visitor writes as: the verified
// user id, else the anonymous visitor id.
func (c Claims) ExternalID() string {
	if c.User != "" {
		return UserExternalID(c.User)
	}
	return "visitor:" + c.Visitor
}

// UserExternalID is the widget identity id of the customer's user id: the
// id a verified visitor writes and is written to as (CF-195 creates it
// ahead of the first visit for agent-started conversations).
func UserExternalID(userID string) string { return "user:" + userID }

// Verified reports whether the visitor proved a user id.
func (c Claims) Verified() bool { return c.User != "" }

// Signer issues and verifies visitor tokens (HMAC-SHA256, compact
// "<claims>.<mac>" in base64url). The key is derived from the application
// secret so it never equals a key used elsewhere.
type Signer struct {
	key []byte
	now func() time.Time
}

// NewSigner derives the token key from an application secret.
func NewSigner(appSecret string) *Signer {
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write([]byte("clickfix/widget-visitor-token/v1"))
	return &Signer{key: mac.Sum(nil), now: time.Now}
}

// WithClock replaces the clock (tests).
func (s *Signer) WithClock(now func() time.Time) *Signer { s.now = now; return s }

// Issue signs claims valid for TokenTTL from now.
func (s *Signer) Issue(c Claims) (string, time.Time) {
	now := s.now().UTC()
	if c.Issued == 0 {
		c.Issued = now.Unix()
	}
	exp := now.Add(TokenTTL)
	c.Expires = exp.Unix()
	body, _ := json.Marshal(c)
	enc := base64.RawURLEncoding.EncodeToString(body)
	return enc + "." + s.mac(enc), exp
}

// Verify checks a token for an instance. An expired but authentic token
// returns its claims with ErrTokenExpired so bootstrap can renew it.
func (s *Signer) Verify(token string, instance uuid.UUID) (Claims, error) {
	enc, sig, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok || enc == "" || !hmac.Equal([]byte(sig), []byte(s.mac(enc))) {
		return Claims{}, ErrTokenInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return Claims{}, ErrTokenInvalid
	}
	var c Claims
	if err := json.Unmarshal(body, &c); err != nil || c.Visitor == "" || c.Instance != instance {
		return Claims{}, ErrTokenInvalid
	}
	if s.now().Unix() >= c.Expires {
		return c, ErrTokenExpired
	}
	return c, nil
}

// Renewable reports whether an expired token may still be renewed (it
// expired less than RenewWindow ago).
func (s *Signer) Renewable(c Claims) bool {
	return c.Expires > 0 && s.now().Sub(time.Unix(c.Expires, 0)) < RenewWindow
}

func (s *Signer) mac(enc string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(enc))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
