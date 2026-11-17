package remotesupport

import (
	"strings"
	"testing"
)

func TestNewCodeShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := NewCode()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := NormalizeCode(c); !ok {
			t.Fatalf("bad code %q", c)
		}
		seen[c] = true
	}
	if len(seen) < 195 {
		t.Fatalf("codes repeat too often: %d unique of 200", len(seen))
	}
}

func TestNormalizeCode(t *testing.T) {
	for in, want := range map[string]string{"123 456 789": "123456789", " 123-456-789 ": "123456789", "123 456 789": "123456789"} {
		got, ok := NormalizeCode(in)
		if !ok || got != want {
			t.Fatalf("%q -> %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "12345678", "1234567890", "12345678a", "١٢٣٤٥٦٧٨٩"} {
		if _, ok := NormalizeCode(in); ok {
			t.Fatalf("%q accepted", in)
		}
	}
	if FormatCode("123456789") != "123 456 789" {
		t.Fatal("format")
	}
}

func TestHasherDomainsAndKey(t *testing.T) {
	h := NewHasher("k1")
	code := "123456789"
	if h.Code(code) == h.CodeLookup(code) || h.Code(code) == h.JoinToken(code) || h.Secret(code) == h.JoinToken(code) {
		t.Fatal("digests of different kinds must differ")
	}
	if h.CodeLookup("123459999") != h.CodeLookup(code) || h.Code("123459999") == h.Code(code) {
		t.Fatal("lookup covers the 5-digit prefix only")
	}
	if NewHasher("k2").Code(code) == h.Code(code) {
		t.Fatal("digest must depend on the server key")
	}
	if strings.Contains(h.Code(code), code) || Equal("", "") || !Equal(h.Code(code), h.Code(code)) {
		t.Fatal("equal")
	}
}

func TestSecretShape(t *testing.T) {
	s, err := NewSecret()
	if err != nil || !validSecret(s) {
		t.Fatalf("%q %v", s, err)
	}
	if validSecret(s[:42]) || validSecret(s+"=") {
		t.Fatal("secret pattern too loose")
	}
}
