package usecase

import (
	"errors"
	"testing"
)

func TestStatusSlug(t *testing.T) {
	for in, want := range map[string]string{
		"İşleniyor":            "isleniyor",
		"Tedarikçi bekleniyor": "tedarikci-bekleniyor",
		"  Müşteri / Onay ":    "musteri-onay",
		"🙂":                    "status",
	} {
		if got := StatusSlug(in); got != want {
			t.Errorf("StatusSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidCustomStatus(t *testing.T) {
	if name, color, err := ValidCustomStatus("  İşleniyor ", " #3b82f6 ", "open"); err != nil || name != "İşleniyor" || color != "#3b82f6" {
		t.Fatalf("valid = %q %q %v", name, color, err)
	}
	for _, c := range []struct{ name, category string }{{"", "open"}, {"x", "snoozed"}, {"x", "closed"}, {"x", "nope"}} {
		if _, _, err := ValidCustomStatus(c.name, "", c.category); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: err = %v", c, err)
		}
	}
}
