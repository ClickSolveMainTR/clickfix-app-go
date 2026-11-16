package desktoplogin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrInvalidCode covers unknown and expired codes.
	ErrInvalidCode = errors.New("desktop login code is invalid or expired")
	// ErrCodeUsed is a second exchange of an already redeemed code.
	ErrCodeUsed = errors.New("desktop login code was already used")
	// ErrVerifierMismatch is a wrong PKCE code_verifier or state.
	ErrVerifierMismatch = errors.New("desktop login verifier does not match")
)

// Code is the Redis-held state of one desktop sign-in code.
type Code struct {
	UserUUID  string
	UserID    int64
	State     string
	Challenge string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Store keeps desktop sign-in codes in Redis (ephemeral, never in Postgres).
// The key holds only a hash of the code. Redeeming is one Lua script, so two
// concurrent exchanges of the same code cannot both win.
type Store struct {
	rdb *redis.Client
	env string
}

// NewStore creates a Redis-backed store.
func NewStore(rdb *redis.Client, appEnv string) *Store {
	if appEnv == "" {
		appEnv = "dev"
	}
	return &Store{rdb: rdb, env: appEnv}
}

func (s *Store) key(code string) string {
	return fmt.Sprintf("app:%s:desktoplogin:%s", s.env, hashValue(code))
}

func hashValue(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

// Create stores a new code that expires with ttl.
func (s *Store) Create(ctx context.Context, code string, c Code, ttl time.Duration) error {
	key := s.key(code)
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, key, map[string]any{
		"status":     "pending",
		"user_uuid":  c.UserUUID,
		"user_id":    c.UserID,
		"state":      c.State,
		"challenge":  c.Challenge,
		"created_at": c.CreatedAt.UTC().Unix(),
		"expires_at": c.ExpiresAt.UTC().Unix(),
		"attempts":   0,
	})
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// redeemScript marks a pending code used when the PKCE challenge and state
// match. The used marker stays until the key expires, so a replay inside the
// window is told apart from an unknown code (and audited). Wrong verifiers
// burn attempts; the code is deleted after maxAttempts.
// ARGV: S256(verifier), state, max attempts.
var redeemScript = redis.NewScript(`
local st = redis.call('HGET', KEYS[1], 'status')
if not st then return {'missing', '', ''} end
local uid = redis.call('HGET', KEYS[1], 'user_uuid') or ''
local iid = redis.call('HGET', KEYS[1], 'user_id') or ''
if st == 'used' then return {'used', uid, iid} end
if redis.call('HGET', KEYS[1], 'challenge') ~= ARGV[1] or redis.call('HGET', KEYS[1], 'state') ~= ARGV[2] then
  local n = redis.call('HINCRBY', KEYS[1], 'attempts', 1)
  if n >= tonumber(ARGV[3]) then redis.call('DEL', KEYS[1]) end
  return {'invalid', uid, iid}
end
redis.call('HSET', KEYS[1], 'status', 'used')
return {'ok', uid, iid}
`)

// Redeemed is the account behind a code. It is also filled for a reused code
// or a wrong verifier, so the attempt can be audited on that account.
type Redeemed struct {
	UserUUID string
	UserID   int64
}

// Redeem atomically validates and consumes a code.
func (s *Store) Redeem(ctx context.Context, code, challenge, state string, maxAttempts int) (Redeemed, error) {
	res, err := redeemScript.Run(ctx, s.rdb, []string{s.key(code)}, challenge, state, maxAttempts).StringSlice()
	if err != nil {
		return Redeemed{}, err
	}
	if len(res) < 3 {
		return Redeemed{}, ErrInvalidCode
	}
	id, _ := strconv.ParseInt(res[2], 10, 64)
	out := Redeemed{UserUUID: res[1], UserID: id}
	switch res[0] {
	case "ok":
		return out, nil
	case "used":
		return out, ErrCodeUsed
	case "invalid":
		return out, ErrVerifierMismatch
	default:
		return Redeemed{}, ErrInvalidCode
	}
}
