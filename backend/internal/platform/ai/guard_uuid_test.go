package ai

import (
	"strings"
	"testing"
)

// A resource id is never personal data: UUIDs whose groups are all digits used
// to be masked as card numbers, which broke tool params carrying them.
func TestPIIRulesLeaveUUIDsAlone(t *testing.T) {
	ids := []string{
		"12345678-1234-4123-8123-123456789012",
		"c50ea5a4-19e6-47de-8128-555809443722",
		"A1B2C3D4-0000-4000-8000-000000000000",
	}
	for _, id := range ids {
		in := "webhook " + id + " paused"
		if got := RedactPII(in); got != in {
			t.Fatalf("RedactPII changed %q to %q", in, got)
		}
		if got := NewMasker().Mask(in); got != in {
			t.Fatalf("Mask changed %q to %q", in, got)
		}
	}
	// Real card numbers next to a UUID are still masked.
	in := "id 12345678-1234-4123-8123-123456789012 card 4111 1111 1111 1111"
	got := RedactPII(in)
	if !strings.Contains(got, "12345678-1234-4123-8123-123456789012") || !strings.Contains(got, "[PII_CARD]") || strings.Contains(got, "4111 1111") {
		t.Fatalf("got %q", got)
	}
	m := NewMasker()
	masked := m.Mask(in)
	if !strings.Contains(masked, "[PII_CARD_") || !strings.Contains(masked, "12345678-1234-4123-8123-123456789012") {
		t.Fatalf("masked %q", masked)
	}
	if back := m.Render(masked, true); back != in {
		t.Fatalf("render %q", back)
	}
}
