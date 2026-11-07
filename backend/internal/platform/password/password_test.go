package password

import (
	"encoding/base64"
	"math/rand/v2"
	"strings"
	"testing"
)

func TestHashVerify(t *testing.T) {
	t.Parallel()

	hash, err := Hash("Password1")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	ok, err := Verify(hash, "Password1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}
	ok, err = Verify(hash, "WrongPass1")
	if err != nil {
		t.Fatalf("Verify wrong: %v", err)
	}
	if ok {
		t.Fatal("expected wrong password to fail")
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "ok", value: "Password1", wantErr: false},
		{name: "short", value: "Pa1", wantErr: true},
		{name: "no_digit", value: "Password", wantErr: true},
		{name: "no_upper", value: "password1", wantErr: true},
		{name: "no_lower", value: "PASSWORD1", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePassword(tc.value)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestVerifyRejectsInvalidHashLength(t *testing.T) {
	for _, n := range []int{0, 1025} {
		encoded := "$argon2id$v=19$m=8,t=1,p=1$c2FsdA$" + base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat("a", n)))
		if ok, err := Verify(encoded, "Password1"); ok || err == nil {
			t.Errorf("accepted hash length %d: ok=%v err=%v", n, ok, err)
		}
	}
}

// Accounts without a password store a hash nothing verifies against.
func TestHashUnusableVerifiesNoPassword(t *testing.T) {
	t.Parallel()

	first, err := HashUnusable()
	if err != nil {
		t.Fatalf("HashUnusable: %v", err)
	}
	second, err := HashUnusable()
	if err != nil {
		t.Fatalf("HashUnusable: %v", err)
	}
	if first == second {
		t.Fatal("two unusable hashes are equal: the secret or the salt is not random")
	}
	if !strings.HasPrefix(first, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %q", first)
	}
	for _, guess := range []string{"", " ", "Password1", "password", first, strings.Repeat("\x00", 32)} {
		ok, err := Verify(first, guess)
		if err != nil {
			t.Fatalf("Verify(%q): %v", guess, err)
		}
		if ok {
			t.Fatalf("guess %q verified against an unusable hash", guess)
		}
	}
}

// CF-181: random secrets used to go through Hash and its policy. The policy
// rejects a share of them (no digit among 43 base64url characters: about one
// in 1500), which failed passwordless sign-ups at random. A fixed seed makes
// the share reproducible; encode, which HashUnusable uses, takes any secret.
func TestRandomSecretsBypassThePasswordPolicy(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(181, 181))
	rejected := ""
	for range 20000 {
		raw := make([]byte, 32)
		for i := range raw {
			raw[i] = byte(rng.UintN(256))
		}
		secret := base64.RawURLEncoding.EncodeToString(raw)
		if ValidatePassword(secret) != nil {
			rejected = secret
			break
		}
	}
	if rejected == "" {
		t.Fatal("expected the policy to reject some random secrets")
	}
	if _, err := Hash(rejected); err == nil {
		t.Fatal("Hash must keep enforcing the policy on chosen passwords")
	}
	hash, err := encode([]byte(rejected))
	if err != nil {
		t.Fatalf("a secret the policy rejects must still hash: %v", err)
	}
	if ok, err := Verify(hash, rejected); err != nil || !ok {
		t.Fatalf("hash of the secret does not verify: %v", err)
	}
}
