package twilioutil_test

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/twilioutil"
	"net/url"
	"testing"
)

// Ported from takwim platform/voice/twilio's documented Twilio test vector.
func TestDocumentedTwilioSignatureVector(t *testing.T) {
	t.Parallel()
	form := url.Values{"CallSid": {"CA1234567890ABCDE"},
		"Caller": {"+14158675309"},
		"Digits": {"1234"},
		"From":   {"+14158675309"},
		"To":     {"+18005551212"}}
	got := twilioutil.Sign("12345", "https://mycompany.com/myapp.php?foo=1&bar=2", form)
	if got != "RSOYDt4T1cUTdK1PDd93/VVr8B8=" {
		t.Fatalf("signature %q", got)
	}
}
