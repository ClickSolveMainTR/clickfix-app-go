package channels_test

import (
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/testchannel"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/google/uuid"
)

func TestRegistryAndCredentials(t *testing.T) {
	a := testchannel.New()
	r := channels.NewRegistry(a, nil)
	if got, ok := r.Get(channels.TypeTest); !ok || got != a {
		t.Fatal("test adapter not registered")
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("unknown type resolved")
	}
	creds, missing := channels.ValidateCredentials(a, channels.Credentials{"extra": "x"})
	if len(missing) != 1 || missing[0] != testchannel.SecretField || len(creds) != 0 {
		t.Fatalf("validate: %v %v", creds, missing)
	}
	hint := channels.Hint(a, channels.Credentials{testchannel.SecretField: "supersecretvalue1234"})
	if hint[testchannel.SecretField] != "supe••••1234" {
		t.Fatalf("hint = %v", hint)
	}
}

func TestVaultBindsCiphertextToInstance(t *testing.T) {
	box, err := crypto.NewSecretBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	v, err := channels.NewVault(box)
	if err != nil {
		t.Fatal(err)
	}
	inst := uuid.New()
	sealed, err := v.Seal(7, inst, channels.Credentials{"token": "abc"})
	if err != nil || sealed == "" {
		t.Fatalf("seal: %v", err)
	}
	got, err := v.Open(7, inst, sealed)
	if err != nil || got["token"] != "abc" {
		t.Fatalf("open: %v %v", got, err)
	}
	if _, err := v.Open(8, inst, sealed); !errors.Is(err, channels.ErrCredentialBinding) {
		t.Fatalf("other org opened the credentials: %v", err)
	}
	if _, err := v.Open(7, uuid.New(), sealed); !errors.Is(err, channels.ErrCredentialBinding) {
		t.Fatalf("other instance opened the credentials: %v", err)
	}
	if empty, err := v.Seal(7, inst, nil); err != nil || empty != "" {
		t.Fatal("empty credentials seal to empty string")
	}
}

func TestPermanentErrors(t *testing.T) {
	if !channels.IsPermanent(channels.Permanent("bad_recipient", errors.New("x"))) {
		t.Fatal("permanent")
	}
	if channels.IsPermanent(&channels.SendError{Err: errors.New("timeout")}) {
		t.Fatal("temporary marked permanent")
	}
	if !channels.IsPermanent(channels.ErrWindowClosed) {
		t.Fatal("closed window is permanent")
	}
}

func TestWindowOpen(t *testing.T) {
	now := time.Now()
	meta := channels.Capabilities{ReplyWindow: 24 * time.Hour}
	if !channels.WindowOpen(meta, now.Add(-time.Hour), now) || channels.WindowOpen(meta, now.Add(-25*time.Hour), now) {
		t.Fatal("24h window")
	}
	if channels.WindowOpen(meta, time.Time{}, now) {
		t.Fatal("no inbound = closed window")
	}
	if !channels.WindowOpen(channels.Capabilities{}, time.Time{}, now) {
		t.Fatal("no window = always open")
	}
}
