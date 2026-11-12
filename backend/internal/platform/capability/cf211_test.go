package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestCompactShortensListsAndSaysSo(t *testing.T) {
	items := []any{}
	for i := 0; i < 200; i++ {
		items = append(items, map[string]any{"uuid": fmt.Sprintf("id-%03d", i), "name": "Konuşma", "avatar_url": "https://x", "note": nil, "tags": []any{},
			"preview": strings.Repeat("uzun metin ", 100)})
	}
	raw, _ := json.Marshal(map[string]any{"items": items, "total": 200})
	c, ok := Compact(json.RawMessage(raw), 3000)
	if !ok {
		t.Fatal("did not fit")
	}
	out, _ := json.Marshal(c)
	if len(out) > 3000 || strings.Contains(string(out), "avatar_url") || strings.Contains(string(out), `"tags"`) {
		t.Fatalf("not compacted: %d %s", len(out), out[:200])
	}
	if !strings.Contains(c.Note, "items: first") || !strings.Contains(c.Note, "of 200") || !strings.Contains(string(out), "id-000") {
		t.Fatalf("note %q", c.Note)
	}
	small, ok := Compact(map[string]any{"total": 1, "items": []any{map[string]any{"uuid": "a"}}}, 3000)
	if !ok || small.Note != "" {
		t.Fatal(small)
	}
	// Nested lists are shortened in every item.
	nested := map[string]any{"items": []any{map[string]any{"msgs": make([]any, 300)}, map[string]any{"msgs": make([]any, 300)}}}
	for _, it := range nested["items"].([]any) {
		m := it.(map[string]any)["msgs"].([]any)
		for i := range m {
			m[i] = "mesaj"
		}
	}
	if c, ok := Compact(nested, 1500); !ok || !strings.Contains(c.Note, "items[].msgs") {
		t.Fatalf("nested: %v %q", ok, c.Note)
	}
}

func TestIntentModules(t *testing.T) {
	for text, want := range map[string]string{
		"Bekleyen konuşmalarımı göster":         "conversations",
		"REST API kanalını bağlamak istiyorum":  "channels",
		"Bu ayki FATURALARIMI görebilir miyim?": "billing",
		"yarın 10:00'a randevu oluştur":         "appointments",
		"İade makalesini yayınla":               "knowledge",
		"Show me this week's CSAT report":       "reports",
		"Ayşe Yılmaz adlı kişiyi bul":           "contacts",
	} {
		if got := IntentModules(text); len(got) == 0 || !slices.Contains(got[:min(3, len(got))], want) {
			t.Errorf("%q -> %v, want %s", text, got, want)
		}
	}
	if got := IntentModules("merhaba"); len(got) != 0 {
		t.Errorf("greeting matched %v", got)
	}
	if Fold("İŞLEMLERİ Çağır") != "islemleri cagir" {
		t.Fatal(Fold("İŞLEMLERİ Çağır"))
	}
}

func testCap(name, module string, routes ...string) *Capability {
	return &Capability{Name: name, Module: module, Kind: Read, Permission: "tenant.x.read", Routes: routes, Description: name + " tool",
		Run: func(context.Context, Exec, Params) (Result, error) { return Result{}, nil }}
}

func TestSelectLoadsPageIntentAndAlways(t *testing.T) {
	reg := NewRegistry().Register(
		testCap("navigation.open_route", "navigation"),
		testCap("billing.invoices", "billing", "/billing"),
		testCap("contacts.list", "contacts", "/contacts"),
		testCap("contacts.get", "contacts", "/contacts"),
		testCap("macros.list", "macros"),
		testCap("audit.list", "audit"),
	)
	has := func(string) bool { return true }
	sel := reg.Select(Filter{Surface: SurfaceWeb, Route: "/billing", Has: has, Max: 3}, []string{"contacts"}, "navigation.open_route")
	names := []string{}
	for _, c := range sel.Loaded {
		names = append(names, c.Name)
	}
	if !slices.Equal(names, []string{"billing.invoices", "contacts.get", "navigation.open_route"}) {
		t.Fatalf("loaded %v", names)
	}
	if len(sel.Other) != 3 {
		t.Fatalf("other %d", len(sel.Other))
	}
	if found := Search(sel.Other, "makro", 5); len(found) != 1 || found[0].Name != "macros.list" {
		t.Fatalf("search %v", found)
	}
	// Everything fits: all loaded, no meta tools needed.
	if sel := reg.Select(Filter{Surface: SurfaceWeb, Has: has, Max: 20}, nil); len(sel.Loaded) != 6 || len(sel.Other) != 0 {
		t.Fatal(sel)
	}
	// Without permission nothing is selected.
	if sel := reg.Select(Filter{Surface: SurfaceWeb, Has: func(string) bool { return false }, Max: 3}, []string{"contacts"}); len(sel.Loaded)+len(sel.Other) != 0 {
		t.Fatal(sel)
	}
}

func TestEndpointRequest(t *testing.T) {
	ep := Endpoint{Method: "PATCH", Path: "/v1/tenant/x/{x_uuid}", Query: []string{"dry"}, Rename: map[string]string{"user_uuid": "assignee"}, Fixed: map[string]any{"private": true}}
	path, body, err := ep.Request(Params{"x_uuid": "a b/c", "dry": true, "user_uuid": "u", "note": "n"})
	if err != nil || path != "/v1/tenant/x/a%20b%2Fc?dry=true" {
		t.Fatal(path, err)
	}
	b := body.(map[string]any)
	if b["assignee"] != "u" || b["note"] != "n" || b["private"] != true || b["x_uuid"] != nil || b["dry"] != nil {
		t.Fatal(b)
	}
	if _, _, err := ep.Request(Params{}); err == nil {
		t.Fatal("missing path param accepted")
	}
	get := Endpoint{Method: "GET", Path: "/v1/tenant/y", Query: []string{"q", "limit"}}
	if path, body, _ := get.Request(Params{"q": "ayşe", "limit": float64(5), "ignored": "x"}); path != "/v1/tenant/y?limit=5&q=ay%C5%9Fe" || body != nil {
		t.Fatal(path, body)
	}
}

func TestToolSchemaDropsFormLabels(t *testing.T) {
	c := &Capability{Input: Object(nil, map[string]any{"name": Titled("Ad", Text(10, "d"))})}
	props := c.ToolSchema()["properties"].(map[string]any)
	if _, ok := props["name"].(map[string]any)["title"]; ok {
		t.Fatal("title sent to the model")
	}
	if c.Form("tr")[0].Label != "Ad" {
		t.Fatal("form lost its label")
	}
	sig := (&Capability{Input: Object([]string{"a"}, map[string]any{"a": UUID(""), "s": Enum("x", "y"), "n": Int(1, 2)})}).Signature()
	if sig != "a*, n:integer, s:x|y" {
		t.Fatal(sig)
	}
}
