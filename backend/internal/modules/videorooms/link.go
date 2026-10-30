package videorooms

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"math"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
)

// Join link roles. A guest goes through the lobby; an agent link only works
// for a signed-in member of the organization.
const (
	LinkGuest = "guest"
	LinkAgent = "agent"
)

const (
	linkVersion = 1
	payloadLen  = 22
)

var (
	linkRoles   = []string{LinkGuest, LinkAgent}
	linkRoleIDs = map[string]byte{LinkGuest: 0, LinkAgent: 1}
)

// Link is the content of a signed join code.
type Link struct {
	Room      uuid.UUID
	Role      string
	ExpiresAt time.Time
}

// Signer issues and verifies join codes: version | room | role | expiry
// (unix seconds, uint32), followed by a truncated HMAC-SHA256. 51 URL-safe
// characters.
type Signer struct{ key []byte }

func NewSigner(secret string) Signer {
	sum := sha256.Sum256([]byte("clickfix/video-link/v1:" + secret))
	return Signer{key: sum[:]}
}

func (s Signer) Sign(l Link) (string, error) {
	role, ok := linkRoleIDs[l.Role]
	if !ok || l.Room == uuid.Nil || l.ExpiresAt.Unix() <= 0 || l.ExpiresAt.Unix() > math.MaxUint32 {
		return "", ErrInvalid
	}
	buf := make([]byte, 0, payloadLen+16)
	buf = append(buf, linkVersion)
	buf = append(buf, l.Room[:]...)
	buf = append(buf, role)
	buf = binary.BigEndian.AppendUint32(buf, safeconv.ToUint32(l.ExpiresAt.Unix()))
	buf = append(buf, s.mac(buf)...)
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s Signer) mac(b []byte) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write(b)
	return m.Sum(nil)[:16]
}

// Verify returns the link when the code is authentic and not expired.
func (s Signer) Verify(code string, now time.Time) (Link, error) {
	raw, err := base64.RawURLEncoding.DecodeString(code)
	if err != nil || len(raw) != payloadLen+16 || raw[0] != linkVersion {
		return Link{}, ErrLinkInvalid
	}
	if !hmac.Equal(raw[payloadLen:], s.mac(raw[:payloadLen])) {
		return Link{}, ErrLinkInvalid
	}
	if int(raw[17]) >= len(linkRoles) {
		return Link{}, ErrLinkInvalid
	}
	room, _ := uuid.FromBytes(raw[1:17])
	exp := int64(binary.BigEndian.Uint32(raw[18:payloadLen]))
	l := Link{Room: room, Role: linkRoles[raw[17]], ExpiresAt: time.Unix(exp, 0).UTC()}
	if !now.Before(l.ExpiresAt) {
		return l, ErrLinkExpired
	}
	return l, nil
}

// newSecret returns 32 random bytes, URL-safe encoded (guest tickets, E2EE keys).
func newSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashTicket(ticket string) string {
	sum := sha256.Sum256([]byte("video-ticket:" + ticket))
	return hex.EncodeToString(sum[:])
}
