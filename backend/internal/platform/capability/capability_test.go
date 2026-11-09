package capability

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func testCaps() []*Capability {
	run := func(context.Context, Exec, Params) (Result, error) { return Result{Summary: "ok"}, nil }
	return []*Capability{
		{Name: "channels.list", Module: "channels", Kind: Read, Permission: "tenant.channels.read", Routes: []string{"/channels"}, Run: run},
		{Name: "channels.api.create", Module: "channels", Kind: Write, Permission: "tenant.channels.manage", Routes: []string{"/channels"}, Apply: run,
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "webhook_url"},
				"properties": map[string]any{
					"name":           map[string]any{"type": "string", "title": "Name", "maxLength": 80},
					"webhook_url":    map[string]any{"type": "string", "format": "uri", "title": "Webhook URL"},
					"retries":        map[string]any{"type": "integer", "minimum": 0, "maximum": 5},
					"signing_secret": map[string]any{"type": "string"},
				}},
			Secrets: []Secret{{Key: "signing_secret", Generate: RandomSecret}}},
		{Name: "workflows.list", Module: "automation", Kind: Read, Permission: "tenant.automations.read", Routes: []string{"/automation"}, Run: run},
		{Name: "help.open", Kind: Read, Surfaces: []string{SurfaceWeb, SurfaceWidget}, Run: run},
	}
}

func names(cs []*Capability) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func TestAvailableFiltersPermissionSurfaceAndOrdersPageFirst(t *testing.T) {
	r := NewRegistry().Register(testCaps()...)
	has := func(perms ...string) func(string) bool {
		return func(p string) bool { return slices.Contains(perms, p) }
	}
	got := names(r.Available(Filter{Surface: SurfaceWeb, Route: "/automation/workflows", Has: has("tenant.channels.read", "tenant.automations.read")}))
	if !slices.Equal(got, []string{"workflows.list", "channels.list", "help.open"}) {
		t.Fatalf("filtered %v", got)
	}
	// No permission: only the open capability.
	if got := names(r.Available(Filter{Surface: SurfaceWeb, Has: has()})); !slices.Equal(got, []string{"help.open"}) {
		t.Fatalf("no perms %v", got)
	}
	// The widget only gets what allows it explicitly, whatever the permissions.
	all := has("tenant.channels.read", "tenant.channels.manage", "tenant.automations.read")
	if got := names(r.Available(Filter{Surface: SurfaceWidget, Has: all})); !slices.Equal(got, []string{"help.open"}) {
		t.Fatalf("widget %v", got)
	}
	// The cap keeps the page's own capabilities.
	if got := names(r.Available(Filter{Surface: SurfaceWeb, Route: "/channels", Has: all, Max: 2})); !slices.Equal(got, []string{"channels.api.create", "channels.list"}) {
		t.Fatalf("cap %v", got)
	}
	if c, ok := r.Get("channels_api_create"); !ok || c.Name != "channels.api.create" {
		t.Fatal("tool name lookup")
	}
}

func TestCheckRejectsBadParamsAndSecrets(t *testing.T) {
	r := NewRegistry().Register(testCaps()...)
	c, _ := r.Get("channels.api.create")
	cases := map[string]Params{
		"missing":   {"name": "Uygulama"},
		"type":      {"name": 42, "webhook_url": "https://example.com/hook"},
		"extra":     {"name": "a", "webhook_url": "https://example.com/hook", "admin": true},
		"range":     {"name": "a", "webhook_url": "https://example.com/hook", "retries": 9},
		"fraction":  {"name": "a", "webhook_url": "https://example.com/hook", "retries": 1.5},
		"too long":  {"name": strings.Repeat("x", 81), "webhook_url": "https://example.com/hook"},
		"secret in": {"name": "a", "webhook_url": "https://example.com/hook", "signing_secret": "from-the-model-0123456789"},
	}
	for name, p := range cases {
		err := c.Check(p)
		var pe *ParamError
		if !errors.As(err, &pe) || !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
	if err := c.Check(Params{"name": "a", "webhook_url": "https://example.com/hook", "retries": float64(2)}); err != nil {
		t.Fatal(err)
	}
	if err := c.Check(Params{"name": "a"}); err == nil || !strings.Contains(err.Error(), "ask the user") {
		t.Fatalf("missing field should tell the model to ask: %v", err)
	}
	// The model never sees secret fields, nor "required" (Check reports it).
	schema := c.ToolSchema()
	props := schema["properties"].(map[string]any)
	if _, ok := props["signing_secret"]; ok || schema["required"] != nil || schema["additionalProperties"] != false {
		t.Fatalf("tool schema %v", schema)
	}
	form := c.Form("tr")
	if last := form[len(form)-1]; last.Key != "signing_secret" || last.Type != "secret" || !last.Generated || last.Required {
		t.Fatalf("form %+v", form)
	}
	if f := form[slices.IndexFunc(form, func(f Field) bool { return f.Key == "webhook_url" })]; f.Type != "url" || !f.Required || f.Label != "Webhook URL" {
		t.Fatalf("url field %+v", f)
	}
}

func TestSecretsSplitAndFill(t *testing.T) {
	r := NewRegistry().Register(testCaps()...)
	c, _ := r.Get("channels.api.create")
	params, secrets := c.SplitSecrets(Params{"name": "a", "signing_secret": "typed-by-the-user-in-the-card-0123456789"})
	if _, ok := params["signing_secret"]; ok || secrets["signing_secret"] == "" {
		t.Fatal("split")
	}
	generated := map[string]string{}
	if err := c.FillSecrets(generated); err != nil || len(generated["signing_secret"]) != 64 {
		t.Fatalf("generated %v %v", generated, err)
	}
}

func TestRegisterRejectsBadDefinitions(t *testing.T) {
	run := func(context.Context, Exec, Params) (Result, error) { return Result{}, nil }
	bad := []*Capability{
		{Name: "Bad Name", Kind: Read, Run: run},
		{Name: "x.write", Kind: Write, Run: run},
		{Name: "x.kind", Kind: "maybe", Run: run},
		{Name: "x.open", Kind: Read, Run: run, Input: map[string]any{"type": "object"}},
		{Name: "x.secret", Kind: Write, Apply: run, Secrets: []Secret{{Key: "k"}}},
		{Name: "x.mcp", Kind: Write, Apply: run, Surfaces: []string{SurfaceMCP}, Secrets: []Secret{{Key: "k"}},
			Input: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"k": map[string]any{"type": "string"}}}},
	}
	for _, c := range bad {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s registered", c.Name)
				}
			}()
			NewRegistry().Register(c)
		}()
	}
}

func TestRedactSecrets(t *testing.T) {
	in := "API anahtarım api_key=abcd1234efgh5678 ve Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig " +
		"ayrıca sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWX ve 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08. Webhook https://api.example.com/hook"
	out := RedactSecrets(in)
	for _, leaked := range []string{"abcd1234efgh5678", "eyJhbGciOiJIUzI1NiJ9", "sk-ant-api03", "9f86d081884c7d659a2f"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("leaked %s: %s", leaked, out)
		}
	}
	if !strings.Contains(out, "api_key=") || !strings.Contains(out, "https://api.example.com/hook") {
		t.Fatalf("over-redacted: %s", out)
	}
}

type recordingAPI struct{ methods []string }

func (r *recordingAPI) Call(_ context.Context, method, _ string, _ any) (Response, error) {
	r.methods = append(r.methods, method)
	return Response{OK: true, Data: []byte(`{}`)}, nil
}

func TestReadOnlyAPIRefusesWrites(t *testing.T) {
	rec := &recordingAPI{}
	api := ReadOnly(rec)
	if _, err := api.Call(context.Background(), "GET", "/v1/tenant/channels", nil); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if _, err := api.Call(context.Background(), m, "/v1/tenant/channels", nil); !errors.Is(err, ErrReadOnly) {
			t.Fatalf("%s passed", m)
		}
	}
	if !slices.Equal(rec.methods, []string{"GET"}) {
		t.Fatalf("calls %v", rec.methods)
	}
	sim := ReadOnly(rec, "POST /v1/tenant/workflows/{uuid}/test")
	if _, err := sim.Call(context.Background(), "POST", "/v1/tenant/workflows/0b6f2c1e-5d4a-4b3c-9a8b-7c6d5e4f3a2b/test", nil); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/v1/tenant/workflows/0b6f/test", "/v1/tenant/workflows/../test", "/v1/tenant/workflows/0b6f2c1e-5d4a-4b3c-9a8b-7c6d5e4f3a2b/publish", "/v1/tenant/workflows/a/b/test", "/v1/tenant/workflows/0b6f/test?x=1"} {
		if _, err := sim.Call(context.Background(), "POST", p, nil); !errors.Is(err, ErrReadOnly) {
			t.Fatalf("%s passed", p)
		}
	}
	if _, err := sim.Call(context.Background(), "PUT", "/v1/tenant/workflows/0b6f/test", nil); !errors.Is(err, ErrReadOnly) {
		t.Fatal("PUT passed")
	}
}

func TestReadResponse(t *testing.T) {
	ok := ReadResponse(200, []byte(`{"success":true,"data":{"items":[]}}`), false)
	if !ok.OK || string(ok.Data) != `{"items":[]}` {
		t.Fatalf("%+v", ok)
	}
	bad := ReadResponse(422, []byte(`{"success":false,"error":{"code":"VALIDATION_ERROR","message":"invalid","details":[{"field":"webhook_url","message":"must be https"}]}}`), false)
	if bad.OK || bad.Code != "VALIDATION_ERROR" || !strings.Contains(bad.Message, "webhook_url: must be https") || bad.Err() == nil {
		t.Fatalf("%+v", bad)
	}
	if f := ReadResponse(403, []byte(`{}`), false); f.Code != "FORBIDDEN" {
		t.Fatalf("%+v", f)
	}
}

// Long path segments of a URL are not secrets; the same token alone is.
func TestRedactSecretsKeepsURLPaths(t *testing.T) {
	seg := strings.Repeat("a1B2", 12)
	if out := RedactSecrets("adres https://shop.example.com/hooks/" + seg + "/in"); !strings.Contains(out, seg) {
		t.Fatalf("URL path masked: %s", out)
	}
	if out := RedactSecrets("anahtar " + seg); strings.Contains(out, seg) {
		t.Fatalf("bare token kept: %s", out)
	}
	if out := RedactSecrets("https://x.example.com/h?token=" + seg); strings.Contains(out, seg) {
		t.Fatalf("query token kept: %s", out)
	}
}

// MCP tool names are unique, and MCP tools always carry a permission.
func TestRegistryMCPNameAndPermission(t *testing.T) {
	run := func(context.Context, Exec, Params) (Result, error) { return Result{}, nil }
	mk := func(name, mcp, perm string) *Capability {
		return &Capability{Name: name, MCPName: mcp, Kind: Read, Run: run, Permission: perm, Surfaces: []string{SurfaceWeb, SurfaceMCP}}
	}
	mustPanic := func(what string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s: no panic", what)
			}
		}()
		f()
	}
	mustPanic("duplicate MCP name", func() {
		NewRegistry().Register(mk("a.reply", "reply", "p"), mk("b.reply", "reply", "p"))
	})
	mustPanic("MCP name of another tool name", func() {
		NewRegistry().Register(mk("a.list", "", "p"), mk("b.list", "a_list", "p"))
	})
	mustPanic("MCP without permission", func() { NewRegistry().Register(mk("a.list", "", "")) })
	// Off the MCP surface the MCP name is not reserved.
	web := mk("c.reply", "reply", "")
	web.Surfaces = []string{SurfaceWeb}
	NewRegistry().Register(mk("a.reply", "reply", "p"), web)
}

// Errors without an envelope code keep the MCP tools' codes.
func TestReadResponseCodes(t *testing.T) {
	cases := map[int]string{401: "FORBIDDEN", 403: "FORBIDDEN", 404: "NOT_FOUND", 409: "CONFLICT", 429: "RATE_LIMITED", 500: "TOOL_FAILED", 502: "TOOL_FAILED", 422: "INVALID_REQUEST"}
	for status, want := range cases {
		if r := ReadResponse(status, []byte(`{"success":false}`), false); r.Code != want {
			t.Errorf("%d: %s", status, r.Code)
		}
		if r := ReadResponse(status, []byte(`<html>`), false); r.Code != want {
			t.Errorf("%d unreadable: %s", status, r.Code)
		}
	}
	if r := ReadResponse(200, []byte(`<html>`), false); r.Code != "TOOL_FAILED" {
		t.Errorf("unreadable 200: %s", r.Code)
	}
	if r := ReadResponse(404, []byte(`{"success":false,"error":{"code":"WEBHOOK_NOT_FOUND","message":"x"}}`), false); r.Code != "WEBHOOK_NOT_FOUND" {
		t.Errorf("envelope code: %s", r.Code)
	}
	if r := ReadResponse(200, nil, true); r.Code != "RESULT_TOO_LARGE" || !strings.Contains(r.Message, "limit") {
		t.Errorf("too large: %+v", r)
	}
}
