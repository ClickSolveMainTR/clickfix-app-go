package usecase

import (
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
)

// CF-196: workflow conditions read the contact's and company's list fields
// by option id, and fields hidden from workflows never reach the data map.
func TestContactAndCompanyCustomFieldConditions(t *testing.T) {
	defs := []customfields.Definition{
		{Key: "paket", Type: customfields.TypeSelect, Visibility: customfields.DefaultVisibility},
		{Key: "gizli", Type: customfields.TypeText, Visibility: customfields.Visibility{Agent: true}},
	}
	data := map[string]any{
		"contact": map[string]any{"custom_fields": customfields.FilterFor(defs, map[string]any{"paket": "pro", "gizli": "x"}, customfields.AudienceWorkflow)},
		"company": map[string]any{"custom_fields": map[string]any{"segment": []any{"kobi", "bayi"}}},
	}
	for raw, want := range map[string]bool{
		`{"field":"contact.custom_fields.paket","operator":"eq","value":"pro"}`:          true,
		`{"field":"contact.custom_fields.paket","operator":"in","value":["pro","gold"]}`: true,
		`{"field":"contact.custom_fields.gizli","operator":"exists"}`:                    false,
		`{"field":"company.custom_fields.segment","operator":"contains","value":"bayi"}`: true,
	} {
		c := condition(t, raw)
		n := 0
		if err := validateCondition(c, 0, &n); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if got := matches(c, data); got != want {
			t.Errorf("%s: got %t", raw, got)
		}
	}
	for _, bad := range []string{"contact.custom_fields.", "contact.custom_fields.a.b", "contact.password"} {
		if isCustomFieldPath(bad) {
			t.Errorf("accepted %s", bad)
		}
	}
}
