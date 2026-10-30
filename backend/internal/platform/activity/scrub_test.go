package activity

import (
	"reflect"
	"testing"
)

func TestScrub(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"email":        "ayse.yilmaz@example.com",
		"owner_email":  "x@y.tr",
		"name":         "Ayşe",
		"owner_name":   "Ayşe Yılmaz",
		"phone":        "+90555",
		"tax_number":   "1234567890",
		"password":     "secret",
		"roles":        []any{"agent"},
		"user_uuid":    "4a0b",
		"fields":       []string{"company", "name"},
		"nested":       map[string]any{"customer_phone": "1", "status": "ok", "emails": []any{"b@c.d"}},
		"slug":         "acme",
		"already_role": "owner",
	}
	got := Scrub(in)
	want := map[string]any{
		"name":         "Ayşe",
		"email":        "a***@example.com",
		"owner_email":  "x***@y.tr",
		"roles":        []any{"agent"},
		"user_uuid":    "4a0b",
		"fields":       []string{"company", "name"},
		"nested":       map[string]any{"status": "ok", "emails": []any{"b***@c.d"}},
		"slug":         "acme",
		"already_role": "owner",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Scrub =\n%#v\nwant\n%#v", got, want)
	}
	if Scrub(nil) != nil {
		t.Fatal("nil payload")
	}
	if MaskEmail("no-at") != "***" || MaskEmail("") != "" {
		t.Fatal("MaskEmail edge cases")
	}
}
