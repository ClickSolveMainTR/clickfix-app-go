package ai

import (
	"strings"
	"testing"
)

// All values below are invented test data.
func TestAnonymizerRemovesStructuredAndPatternPII(t *testing.T) {
	a := NewAnonymizer()
	a.Known("NAME", "Deneme Kişioğlu")
	a.Known("COMPANY", "Örnek Kırtasiye")
	a.Known("PHONE", "+90 555 000 11 22")
	a.Known("TAXNO", "1234567890")
	in := "Merhaba, ben Deneme. Örnek Kırtasiye adına yazıyorum. Numaram 0555 000 11 22, " +
		"e-posta deneme@example.test. VKN: 1234567890. Adres: Lale Mah. Gül Sok. No: 7/3 Kadıköy. " +
		"Sayın Tülin Örnekçi ile görüştüm, Kemal Bey de aradı. Fatura Sahte Yazılım A.Ş. adına. " +
		"Faturada 1234567890 numarası geçmesin."
	got := a.Redact(in)
	for _, leak := range []string{"Deneme", "Kişioğlu", "Örnek Kırtasiye", "555 000", "deneme@example.test", "1234567890", "Lale Mah", "Gül Sok", "No: 7/3", "Tülin", "Örnekçi", "Kemal", "Sahte Yazılım"} {
		if strings.Contains(got, leak) {
			t.Errorf("leaked %q in %q", leak, got)
		}
	}
	for _, kept := range []string{"Merhaba", "adına yazıyorum", "Kadıköy", "[PII_NAME]", "[PII_COMPANY]", "[PII_ADDRESS]", "[PII_TAXNO]"} {
		if !strings.Contains(got, kept) {
			t.Errorf("expected %q in %q", kept, got)
		}
	}
}

func TestAnonymizerKeepsWordsThatContainAKnownValue(t *testing.T) {
	a := NewAnonymizer()
	a.Known("NAME", "Ali")
	got := a.Redact("Ali, Alican ve Kalite ekibi")
	if got != "[PII_NAME], Alican ve Kalite ekibi" {
		t.Fatalf("got %q", got)
	}
}

func TestSharedRulesCoverNewKindsInMasker(t *testing.T) {
	m := NewMasker()
	in := "Sayın Deneme Kişi, Sahte Ticaret Ltd. Şti. Papatya Cad. No: 3 vergi no 9876543210"
	masked := m.Mask(in)
	for _, leak := range []string{"Deneme Kişi", "Sahte Ticaret", "Papatya Cad", "9876543210"} {
		if strings.Contains(masked, leak) {
			t.Fatalf("leaked %q in %q", leak, masked)
		}
	}
	if back := m.Render(masked, true); back != in {
		t.Fatalf("render %q", back)
	}
}
