package channels

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/google/uuid"
)

// Vault seals instance credentials with the platform SecretBox (AES-256-GCM,
// APP_ENCRYPTION_KEY). The sealed plaintext starts with a binding to the
// organization and instance, checked on Open, so a ciphertext copied onto
// another row does not decrypt there.
type Vault struct {
	box *crypto.SecretBox
}

// NewVault builds the vault.
func NewVault(box *crypto.SecretBox) (*Vault, error) {
	if box == nil {
		return nil, errors.New("channels: secret box required")
	}
	return &Vault{box: box}, nil
}

// ErrCredentialBinding: the ciphertext belongs to another instance.
var ErrCredentialBinding = errors.New("channels: credentials belong to another instance")

func binding(orgID int64, instance uuid.UUID) string {
	return "channel_instances|org:" + strconv.FormatInt(orgID, 10) + "|instance:" + instance.String() + "\n"
}

// Seal encrypts credentials for an instance ("" for none).
func (v *Vault) Seal(orgID int64, instance uuid.UUID, c Credentials) (string, error) {
	if len(c) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return v.box.Encrypt(binding(orgID, instance) + string(raw))
}

// Open decrypts the credentials of an instance.
func (v *Vault) Open(orgID int64, instance uuid.UUID, ciphertext string) (Credentials, error) {
	out := Credentials{}
	if ciphertext == "" {
		return out, nil
	}
	plain, err := v.box.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("channels: open credentials: %w", err)
	}
	prefix := binding(orgID, instance)
	if !strings.HasPrefix(plain, prefix) {
		return nil, ErrCredentialBinding
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(plain, prefix)), &out); err != nil {
		return nil, fmt.Errorf("channels: credentials: %w", err)
	}
	return out, nil
}
