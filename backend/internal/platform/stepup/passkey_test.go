package stepup

import (
	"math"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/model"
)

func TestPasskeyCounterRange(t *testing.T) {
	for _, counter := range []int64{-1, math.MaxUint32 + 1, math.MaxInt64} {
		_, err := mapPasskeyCredential(model.PasskeyRecord{Counter: counter, CredentialID: "AQ", PublicKey: "Ag"})
		if err == nil {
			t.Errorf("accepted invalid counter %d", counter)
		}
	}
}

func TestPasskeyCounterEndpoints(t *testing.T) {
	for _, counter := range []int64{0, math.MaxUint32} {
		cred, err := mapPasskeyCredential(model.PasskeyRecord{Counter: counter, CredentialID: "AQ", PublicKey: "Ag"})
		if err != nil || int64(cred.Authenticator.SignCount) != counter {
			t.Errorf("counter %d: credential=%+v err=%v", counter, cred, err)
		}
	}
}
