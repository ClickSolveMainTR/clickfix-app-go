package usecase

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
	"strings"
	"testing"
)

func TestPKCEAndRedirectPolicy(t *testing.T) {
	// RFC 7636 Appendix B published test vector.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	if !validVerifier(verifier) || S256(verifier) != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatal("PKCE vector")
	}
	for _, v := range []string{"short", strings.Repeat("a", 129), strings.Repeat("!", 43)} {
		if validVerifier(v) {
			t.Fatal("invalid verifier accepted")
		}
	}
	for _, u := range []string{"http://example.com/cb", "javascript:alert(1)", "https://user@example.com/cb", "https://example.com/cb#fragment"} {
		if validRedirect(u) {
			t.Fatal(u)
		}
	}
	for _, u := range []string{"https://client.example/cb", "http://127.0.0.1:9876/cb", "http://[::1]/cb"} {
		if !validRedirect(u) {
			t.Fatal(u)
		}
	}
	svc := New(nil, nil, "https://api.example", nil)
	if _, ok := svc.resourcePath("https://evil.example/mcp/business"); ok {
		t.Fatal("foreign audience")
	}
	if _, ok := svc.resourcePath("https://api.example/mcp/platform"); ok {
		t.Fatal("unsupported platform layer")
	}
	if path, ok := svc.resourcePath("https://api.example" + model.ResourceBusiness); !ok || path != model.ResourceBusiness {
		t.Fatal("organization audience")
	}
}
