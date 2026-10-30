package payments

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"

	"github.com/google/uuid"
)

// CardVault ports Takwim's scope/row/field-bound provider token vault.
// Only provider tokens are accepted; PAN and CVV never belong in this layer.
type CardVault struct {
	gcm      cipher.AEAD
	indexKey [32]byte
}

func NewCardVault(key string) (*CardVault, error) {
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(raw) != 32 {
		raw = []byte(key)
	}
	if len(raw) != 32 {
		return nil, errors.New("payments: 32 byte vault key required")
	}
	enc := hmac.New(sha256.New, raw)
	enc.Write([]byte("payment.cards.encryption"))
	block, err := aes.NewCipher(enc.Sum(nil))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	idx := hmac.New(sha256.New, raw)
	idx.Write([]byte("payment.cards.index"))
	v := &CardVault{gcm: gcm}
	copy(v.indexKey[:], idx.Sum(nil))
	return v, nil
}
func cardAAD(scope string, row uuid.UUID, field string) []byte {
	return []byte("payment_cards." + field + "|" + scope + "|card:" + row.String())
}
func (v *CardVault) seal(scope string, row uuid.UUID, field, value string) (string, error) {
	nonce := make([]byte, v.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(v.gcm.Seal(nonce, nonce, []byte(value), cardAAD(scope, row, field))), nil
}
func (v *CardVault) open(scope string, row uuid.UUID, field, value string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(data) < v.gcm.NonceSize() {
		return "", errors.New("payments: invalid token ciphertext")
	}
	n := v.gcm.NonceSize()
	out, err := v.gcm.Open(nil, data[:n], data[n:], cardAAD(scope, row, field))
	return string(out), err
}
func (v *CardVault) Seal(scope string, row uuid.UUID, card SavedCard) (string, string, error) {
	if scope == "" || row == uuid.Nil || card.OwnerKey == "" || card.Token == "" {
		return "", "", errors.New("payments: card scope and keys required")
	}
	owner, err := v.seal(scope, row, "owner", card.OwnerKey)
	if err != nil {
		return "", "", err
	}
	token, err := v.seal(scope, row, "token", card.Token)
	return owner, token, err
}
func (v *CardVault) Open(scope string, row uuid.UUID, ownerEnc, tokenEnc string) (string, string, error) {
	owner, err := v.open(scope, row, "owner", ownerEnc)
	if err != nil {
		return "", "", err
	}
	token, err := v.open(scope, row, "token", tokenEnc)
	return owner, token, err
}
func (v *CardVault) TokenHash(scope, provider, token string) string {
	h := hmac.New(sha256.New, v.indexKey[:])
	h.Write([]byte(scope + "|" + provider + "|" + token))
	return hex.EncodeToString(h.Sum(nil))
}

// OpenOwner reads the provider owner key used to list saved cards on checkout.
func (v *CardVault) OpenOwner(scope string, row uuid.UUID, ownerEnc string) (string, error) {
	return v.open(scope, row, "owner", ownerEnc)
}
