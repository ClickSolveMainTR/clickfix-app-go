package customfields

import (
	"reflect"
	"testing"
)

func catalogDefs() []Definition {
	return []Definition{
		{Key: "paket", Label: "Paket", Type: TypeSelect, Visibility: DefaultVisibility, Options: []Option{
			{Value: "pro", Label: "Pro Paket"}, {Value: "basic", Label: "Temel"},
		}},
		{Key: "tc_kimlik", Label: "TC kimlik", Type: TypeText, Visibility: DefaultVisibility, Sensitive: true},
		{Key: "ic_not", Label: "İç not", Type: TypeText, Visibility: Visibility{Agent: true}},
		{Key: "aktif", Label: "Aktif", Type: TypeBoolean, Visibility: DefaultVisibility},
	}
}

func TestResolveOptionAcceptsIDsAndLabels(t *testing.T) {
	d := catalogDefs()[0]
	for in, want := range map[string]string{"pro": "pro", "PRO": "pro", "Pro Paket": "pro", " pro paket ": "pro", "temel": "basic"} {
		got, ok := ResolveOption(d, in)
		if !ok || got != want {
			t.Fatalf("ResolveOption(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := ResolveOption(d, "enterprise"); ok {
		t.Fatal("unknown option resolved")
	}
	out, err := Validate([]Definition{d}, nil, map[string]any{"paket": "Pro Paket"}, ModeCreate)
	if err != nil || out["paket"] != "pro" {
		t.Fatalf("label input should store the id: %v %v", out, err)
	}
}

func TestProjectHonoursVisibilityAndSensitive(t *testing.T) {
	values := map[string]any{"paket": "pro", "tc_kimlik": "12345678901", "ic_not": "x", "aktif": true}
	keys := func(fvs []FieldValue) []string {
		out := []string{}
		for _, fv := range fvs {
			out = append(out, fv.Key)
		}
		return out
	}
	ai := Project(catalogDefs(), values, AudienceAI)
	if got := keys(ai); !reflect.DeepEqual(got, []string{"paket", "aktif"}) {
		t.Fatalf("AI keys = %v", got)
	}
	if ai[0].Display != "Pro Paket" || ai[1].Display != "Evet" {
		t.Fatalf("AI displays = %+v", ai)
	}
	reports := Project(catalogDefs(), values, AudienceReports)
	if got := keys(reports); !reflect.DeepEqual(got, []string{"paket", "aktif"}) {
		t.Fatalf("report keys = %v", got)
	}
	agent := Project(catalogDefs(), values, AudienceAgent)
	if got := keys(agent); !reflect.DeepEqual(got, []string{"paket", "tc_kimlik", "ic_not", "aktif"}) {
		t.Fatalf("agent keys = %v", got)
	}
	if !agent[1].Masked || agent[1].Value != nil || agent[1].Display != Mask {
		t.Fatalf("sensitive value not masked for agents: %+v", agent[1])
	}
	if f := FilterFor(catalogDefs(), values, AudienceAI); f["tc_kimlik"] != nil || f["ic_not"] != nil || f["paket"] != "pro" {
		t.Fatalf("Filter(AI) = %v", f)
	}
	vars := TemplateVars("contact", catalogDefs(), values, AudienceAI)
	if vars["contact.paket"] != "Pro Paket" || vars["contact.tc_kimlik"] != "" {
		t.Fatalf("template vars = %v", vars)
	}
}

func TestMatchesUseOptionIDsAndSurviveRenames(t *testing.T) {
	defs := map[string][]Definition{EntityContact: catalogDefs()}
	defsOf := func(e string) []Definition { return defs[e] }
	// Old free-text rule ("Pro Paket") normalizes to the id.
	matches, errs := NormalizeMatches([]FieldMatch{{Entity: EntityContact, Key: "paket", Options: []string{"Pro Paket"}}}, defsOf)
	if len(errs) > 0 || !reflect.DeepEqual(matches[0].Options, []string{"pro"}) {
		t.Fatalf("normalize = %v %v", matches, errs)
	}
	// Renaming the option label keeps the rule bound to the id.
	defs[EntityContact][0].Options[0].Label = "Profesyonel"
	values := map[string]any{"paket": "pro"}
	if !Matches(matches, func(string) map[string]any { return values }) {
		t.Fatal("rule broke after the option was renamed")
	}
	if Matches(matches, func(string) map[string]any { return map[string]any{"paket": "basic"} }) {
		t.Fatal("other option matched")
	}
	if Matches(matches, func(string) map[string]any { return nil }) {
		t.Fatal("missing entity matched")
	}
	if _, errs := NormalizeMatches([]FieldMatch{{Entity: EntityContact, Key: "tc_kimlik", Options: []string{"x"}}}, defsOf); len(errs) == 0 {
		t.Fatal("non-list field accepted")
	}
}

func TestSuggestKeyAndValueKind(t *testing.T) {
	for in, want := range map[string]string{"lisans_bitis": "lisans_bitis", "Lisans Bitiş": "lisans_bitis", "subeSayisi": "sube_sayisi", "1st-plan": "st_plan", "!!": ""} {
		if got := SuggestKey(in); got != want {
			t.Fatalf("SuggestKey(%q) = %q want %q", in, got, want)
		}
	}
	for v, want := range map[any]string{"2026-12-01": "date", 3.0: "number", true: "boolean", "pro": "text"} {
		if got := ValueKind(v); got != want {
			t.Fatalf("ValueKind(%v) = %q want %q", v, got, want)
		}
	}
}
