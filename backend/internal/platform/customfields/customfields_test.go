package customfields

import (
	"context"
	"errors"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func ptr[T any](v T) *T { return &v }

func testDefs() []Definition {
	return []Definition{
		{Key: "plan", Label: "Plan", Type: TypeSelect, Required: true, Default: "basic", Options: []Option{
			{Value: "basic", Label: "Basic"}, {Value: "pro", Label: "Pro"}, {Value: "legacy", Label: "Legacy", Archived: true},
		}},
		{Key: "seats", Label: "Seats", Type: TypeNumber, Config: Config{Min: ptr(1.0), Max: ptr(500.0), Precision: ptr(0)}},
		{Key: "renewal", Label: "Renewal", Type: TypeDate},
		{Key: "tags", Label: "Tags", Type: TypeMultiselect, Options: []Option{{Value: "vip", Label: "VIP"}, {Value: "beta", Label: "Beta"}, {Value: "churn", Label: "Churn"}}},
		{Key: "pro_contact", Label: "Pro contact", Type: TypeEmail, Required: true, Condition: &Condition{Field: "plan", Operator: CondEquals, Value: "pro"}},
		{Key: "mrr", Label: "MRR", Type: TypeMoney},
		{Key: "site", Label: "Site", Type: TypeURL},
		{Key: "phone", Label: "Phone", Type: TypePhone},
		{Key: "active", Label: "Active", Type: TypeBoolean},
		{Key: "note", Label: "Note", Type: TypeText, Config: Config{MaxLength: 5}},
		{Key: "old", Label: "Old", Type: TypeText, Archived: true},
	}
}

func fieldErrors(t *testing.T, err error) map[string]string {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	out := map[string]string{}
	for _, e := range ve.Errors {
		out[e.Field] = e.Code
	}
	return out
}

func TestValidateNormalizesAndAppliesDefaults(t *testing.T) {
	got, err := Validate(testDefs(), nil, map[string]any{
		"seats":   "12",
		"renewal": "2026-11-30T10:00:00Z",
		"tags":    []any{"beta", "vip", "beta"},
		"mrr":     149.9,
		"site":    "https://clickfix.example",
		"phone":   "0090 (532) 111-22-33",
		"active":  "true",
		"note":    "  hi  ",
	}, ModeCreate)
	if err != nil {
		t.Fatal(err)
	}
	if got["plan"] != "basic" {
		t.Fatalf("default not applied: %v", got["plan"])
	}
	if got["seats"] != 12.0 || got["renewal"] != "2026-11-30" || got["phone"] != "+905321112233" || got["active"] != true || got["note"] != "hi" {
		t.Fatalf("normalized = %#v", got)
	}
	if tags := got["tags"].([]any); len(tags) != 2 || tags[0] != "vip" || tags[1] != "beta" {
		t.Fatalf("multiselect not deduped in option order: %v", tags)
	}
	if m := got["mrr"].(map[string]any); m["amount_minor"] != int64(14990) || m["currency"] != "TRY" {
		t.Fatalf("money = %v", m)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	_, err := Validate(testDefs(), nil, map[string]any{
		"seats":  2.5,
		"plan":   "gold",
		"site":   "ftp://x",
		"note":   "toolong",
		"old":    "x",
		"nope":   1,
		"active": "maybe",
	}, ModeCreate)
	errs := fieldErrors(t, err)
	want := map[string]string{"seats": CodeInvalid, "plan": CodeInvalid, "site": CodeInvalid, "note": CodeInvalid, "old": CodeArchived, "nope": CodeUnknown, "active": CodeInvalid}
	for k, code := range want {
		if errs[k] != code {
			t.Fatalf("%s: got %q want %q (all %v)", k, errs[k], code, errs)
		}
	}
}

func TestConditionalRequiredAndClearing(t *testing.T) {
	defs := testDefs()
	_, err := Validate(defs, nil, map[string]any{"plan": "pro"}, ModeCreate)
	if errs := fieldErrors(t, err); errs["pro_contact"] != CodeRequired {
		t.Fatalf("conditional field must be required when plan=pro: %v", errs)
	}
	stored, err := Validate(defs, nil, map[string]any{"plan": "pro", "pro_contact": "Ops@Acme.io"}, ModeCreate)
	if err != nil {
		t.Fatal(err)
	}
	if stored["pro_contact"] != "ops@acme.io" {
		t.Fatalf("email not normalized: %v", stored["pro_contact"])
	}
	// Switching the plan hides the conditional field and drops its value.
	updated, err := Validate(defs, stored, map[string]any{"plan": "basic"}, ModeUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updated["pro_contact"]; ok {
		t.Fatal("hidden conditional value must be cleared")
	}
	// Required field cannot be cleared on update.
	if _, err := Validate(defs, updated, map[string]any{"plan": nil}, ModeUpdate); fieldErrors(t, err)["plan"] != CodeRequired {
		t.Fatal("clearing a required field must fail")
	}
}

func TestArchivedOptionKeptButNotPicked(t *testing.T) {
	defs := testDefs()
	if _, err := Validate(defs, map[string]any{"plan": "basic"}, map[string]any{"plan": "legacy"}, ModeUpdate); err == nil {
		t.Fatal("picking an archived option must fail")
	}
	if _, err := Validate(defs, map[string]any{"plan": "legacy"}, map[string]any{"plan": "legacy", "seats": 3}, ModeUpdate); err != nil {
		t.Fatalf("re-sending the stored archived option must pass: %v", err)
	}
}

func TestValidateDefinition(t *testing.T) {
	defs := testDefs()
	cases := map[string]Definition{
		"bad key":         {Key: "Bad Key", Label: "x", Type: TypeText},
		"no options":      {Key: "s", Label: "x", Type: TypeSelect},
		"dup options":     {Key: "s", Label: "x", Type: TypeSelect, Options: []Option{{Value: "a", Label: "A"}, {Value: "a", Label: "B"}}},
		"min > max":       {Key: "n", Label: "x", Type: TypeNumber, Config: Config{Min: ptr(5.0), Max: ptr(1.0)}},
		"bad default":     {Key: "n", Label: "x", Type: TypeNumber, Default: "abc"},
		"self condition":  {Key: "n", Label: "x", Type: TypeText, Condition: &Condition{Field: "n", Operator: CondIsSet}},
		"unknown ref":     {Key: "n", Label: "x", Type: TypeText, Condition: &Condition{Field: "zzz", Operator: CondIsSet}},
		"bad cond value":  {Key: "n", Label: "x", Type: TypeText, Condition: &Condition{Field: "plan", Operator: CondEquals, Value: "gold"}},
		"unknown type":    {Key: "n", Label: "x", Type: "color"},
		"options on text": {Key: "n", Label: "x", Type: TypeText, Options: []Option{{Value: "a", Label: "A"}}},
	}
	for name, d := range cases {
		if errs := ValidateDefinition(d, defs); len(errs) == 0 {
			t.Errorf("%s: expected errors", name)
		}
	}
	ok := Definition{Key: "tier_note", Label: "Tier note", Type: TypeTextarea, Condition: &Condition{Field: "plan", Operator: CondIn, Value: []any{"pro", "basic"}}}
	if errs := ValidateDefinition(ok, defs); len(errs) != 0 {
		t.Fatalf("valid definition rejected: %v", errs)
	}
	// a -> b -> a cycle.
	a := Definition{Key: "a", Label: "A", Type: TypeText, Condition: &Condition{Field: "b", Operator: CondIsSet}}
	b := Definition{Key: "b", Label: "B", Type: TypeText, Condition: &Condition{Field: "a", Operator: CondIsSet}}
	if errs := ValidateDefinition(a, []Definition{b}); len(errs) == 0 {
		t.Fatal("cycle not detected")
	}
}

func TestParseFiltersAndSQL(t *testing.T) {
	q := url.Values{
		"cf.plan":          {"pro"},
		"cf.seats.gte":     {"10"},
		"cf.tags.in":       {"vip,beta"},
		"cf.renewal.lt":    {"2027-01-01"},
		"cf.note.contains": {"50%"},
		"cf.site.set":      {"false"},
		"limit":            {"10"},
	}
	filters, err := ParseFilters(q, testDefs())
	if err != nil {
		t.Fatal(err)
	}
	if len(filters) != 6 {
		t.Fatalf("filters = %d", len(filters))
	}
	where, args := SQL("r.custom_fields", filters, 3)
	for _, frag := range []string{
		`(r.custom_fields @> $`,
		`r.custom_fields->>'note' ILIKE $`,
		`NOT (r.custom_fields ? $`,
		`((r.custom_fields->>'renewal')::date) < $`,
		`::numeric`,
		`(r.custom_fields->'tags') ?| $`,
	} {
		if !strings.Contains(where, frag) {
			t.Fatalf("missing %q in %s", frag, where)
		}
	}
	if !slices.Contains(args, any(`%50\%%`)) {
		t.Fatalf("LIKE wildcard not escaped: %v", args)
	}

	for _, bad := range []url.Values{
		{"cf.nope": {"1"}},
		{"cf.plan.gt": {"pro"}},
		{"cf.seats": {"abc"}},
		{"cf.plan": {"a", "b"}},
	} {
		if _, err := ParseFilters(bad, testDefs()); err == nil {
			t.Fatalf("expected error for %v", bad)
		}
	}
	if expr, ok := SortExpr("r.custom_fields", "cf.seats", testDefs()); !ok || !strings.Contains(expr, "::numeric") {
		t.Fatalf("sort expr = %q %v", expr, ok)
	}
	if _, ok := SortExpr("r.custom_fields", "cf.tags", testDefs()); ok {
		t.Fatal("multiselect must not be sortable")
	}
}

// TestFiltersAgainstPostgres runs the generated SQL on real JSONB rows.
func TestFiltersAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE cf_rows (name text, custom_fields jsonb NOT NULL DEFAULT '{}') ON COMMIT DROP`); err != nil {
		t.Fatal(err)
	}
	defs := testDefs()
	rows := map[string]map[string]any{
		"a": {"plan": "pro", "pro_contact": "a@b.co", "seats": 20, "tags": []any{"vip"}, "renewal": "2026-12-01", "mrr": map[string]any{"amount_minor": 5000, "currency": "TRY"}},
		"b": {"plan": "basic", "seats": 5, "tags": []any{"beta", "churn"}, "renewal": "2027-03-01", "note": "50%!"},
		"c": {"plan": "basic", "site": "https://c.example"},
	}
	for name, values := range rows {
		stored, err := Validate(defs, nil, values, ModeCreate)
		if err != nil {
			t.Fatal(name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO cf_rows (name, custom_fields) VALUES ($1, $2)`, name, stored); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		query url.Values
		sort  string
		want  []string
	}{
		{url.Values{"cf.plan": {"basic"}}, "name", []string{"b", "c"}},
		{url.Values{"cf.plan.neq": {"basic"}}, "name", []string{"a"}},
		{url.Values{"cf.seats.gte": {"10"}}, "name", []string{"a"}},
		{url.Values{"cf.tags.in": {"vip,churn"}}, "name", []string{"a", "b"}},
		{url.Values{"cf.tags": {"beta,churn"}}, "name", []string{"b"}},
		{url.Values{"cf.renewal.lt": {"2027-01-01"}}, "name", []string{"a"}},
		{url.Values{"cf.note.contains": {"50%"}}, "name", []string{"b"}},
		{url.Values{"cf.site.set": {"true"}}, "name", []string{"c"}},
		{url.Values{"cf.mrr.gt": {"49.99"}}, "name", []string{"a"}},
		{url.Values{"cf.plan.in": {"pro,basic"}}, "cf.seats", []string{"b", "a", "c"}},
	}
	for _, tc := range cases {
		filters, err := ParseFilters(tc.query, defs)
		if err != nil {
			t.Fatal(tc.query, err)
		}
		where, args := SQL("custom_fields", filters, 1)
		order := tc.sort
		if expr, ok := SortExpr("custom_fields", tc.sort, defs); ok {
			order = expr + " ASC NULLS LAST"
		}
		r, err := tx.Query(ctx, "SELECT name FROM cf_rows WHERE "+where+" ORDER BY "+order, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", tc.query, err, where)
		}
		got, err := pgx.CollectRows(r, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, tc.want) {
			t.Fatalf("%v: got %v want %v (%s)", tc.query, got, tc.want, where)
		}
	}
}
