package remotesupport

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// Hasher keys every stored digest (code, code prefix, join token, client
// secret) with a server secret, so a database dump alone cannot be used to
// brute-force a 9-digit code offline.
type Hasher struct{ key []byte }

func NewHasher(secret string) Hasher {
	sum := sha256.Sum256([]byte("clickfix/remote-support/v1:" + secret))
	return Hasher{key: sum[:]}
}

func (h Hasher) sum(domain, v string) string {
	m := hmac.New(sha256.New, h.key)
	m.Write([]byte(domain))
	m.Write([]byte{0})
	m.Write([]byte(v))
	return hex.EncodeToString(m.Sum(nil))
}

const (
	codeDigits = 9
	// lookupDigits of the code locate the pending session a guess is charged to.
	lookupDigits = 5
)

func (h Hasher) CodeLookup(code string) string { return h.sum("code-lookup", code[:lookupDigits]) }
func (h Hasher) Code(code string) string       { return h.sum("code", code) }
func (h Hasher) JoinToken(t string) string     { return h.sum("join", t) }
func (h Hasher) Secret(s string) string        { return h.sum("client", s) }

// Equal compares two digests in constant time.
func Equal(a, b string) bool {
	return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

var codeMax = big.NewInt(1_000_000_000)

// NewCode is a uniformly random 9-digit code (crypto/rand).
func NewCode() (string, error) {
	n, err := rand.Int(rand.Reader, codeMax)
	if err != nil {
		return "", fmt.Errorf("remote support: random: %w", err)
	}
	return fmt.Sprintf("%09d", n.Int64()), nil
}

// NewSecret is a 256-bit URL-safe random token (join link, client secret).
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("remote support: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

var (
	codePattern   = regexp.MustCompile(`^[0-9]{9}$`)
	secretPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

// NormalizeCode drops the spaces and dashes people type ("123 456 789").
func NormalizeCode(v string) (string, bool) {
	v = strings.NewReplacer(" ", "", "-", "", " ", "").Replace(strings.TrimSpace(v))
	return v, codePattern.MatchString(v)
}

func validSecret(v string) bool { return secretPattern.MatchString(v) }

// FormatCode groups the code for reading aloud: "123 456 789".
func FormatCode(code string) string {
	if len(code) != codeDigits {
		return code
	}
	return code[:3] + " " + code[3:6] + " " + code[6:]
}
