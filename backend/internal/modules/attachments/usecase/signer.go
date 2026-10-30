package usecase

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

// ErrInvalidToken means a signed URL token is malformed, forged or expired.
var ErrInvalidToken = errors.New("attachments: invalid or expired token")

// tokenPrefix versions the token format.
const tokenPrefix = "v1"

type tokenClaims struct {
	Org     int64     `json:"o"`
	Item    uuid.UUID `json:"a"`
	Variant string    `json:"v"`
	Exp     int64     `json:"e"`
}

// tokenSigner issues and verifies short-lived HMAC-SHA256 download tokens.
// The key is derived from the application secret with a purpose label, so a
// token can never be replayed against another signature use of that secret.
type tokenSigner struct {
	key []byte
	now func() time.Time
}

func newTokenSigner(secret string, now func() time.Time) *tokenSigner {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("clickfix/attachments/signed-url/v1"))
	return &tokenSigner{key: mac.Sum(nil), now: now}
}

func (s *tokenSigner) sign(c tokenClaims) string {
	payload, _ := json.Marshal(c)
	body := base64.RawURLEncoding.EncodeToString(payload)
	return tokenPrefix + "." + body + "." + s.mac(body)
}

func (s *tokenSigner) mac(body string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(tokenPrefix + "." + body))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (s *tokenSigner) verify(token string) (tokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != tokenPrefix {
		return tokenClaims{}, ErrInvalidToken
	}
	if !hmac.Equal([]byte(parts[2]), []byte(s.mac(parts[1]))) {
		return tokenClaims{}, ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, ErrInvalidToken
	}
	var c tokenClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return tokenClaims{}, ErrInvalidToken
	}
	if s.now().Unix() >= c.Exp {
		return tokenClaims{}, ErrInvalidToken
	}
	return c, nil
}
