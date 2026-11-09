package usecase

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
)

// CF-196: only AI-visible, non-sensitive custom fields reach the model.
func TestCustomerFieldsSkipSensitiveAndHidden(t *testing.T) {
	contactDefs := []customfields.Definition{
		{Key: "paket", Label: "Paket", Type: customfields.TypeSelect, Visibility: customfields.DefaultVisibility,
			Options: []customfields.Option{{Value: "pro", Label: "Pro Paket"}}},
		{Key: "tc_kimlik", Label: "TC kimlik", Type: customfields.TypeText, Visibility: customfields.DefaultVisibility, Sensitive: true},
		{Key: "ic_not", Label: "İç not", Type: customfields.TypeText, Visibility: customfields.Visibility{Agent: true}},
	}
	companyDefs := []customfields.Definition{
		{Key: "sube_sayisi", Label: "Şube sayısı", Type: customfields.TypeNumber, Visibility: customfields.DefaultVisibility},
	}
	got := customerFields(contactDefs, map[string]any{"paket": "pro", "tc_kimlik": "12345678901", "ic_not": "gizli not"},
		companyDefs, map[string]any{"sube_sayisi": 12.0})
	for _, want := range []string{"Paket (paket): Pro Paket", "Şube sayısı (sube_sayisi): 12"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	for _, leak := range []string{"12345678901", "tc_kimlik", "gizli not", "ic_not"} {
		if strings.Contains(got, leak) {
			t.Fatalf("AI context leaks %q: %q", leak, got)
		}
	}
	if customerFields(nil, nil, nil, nil) != "" {
		t.Fatal("empty fields should add nothing")
	}
}
