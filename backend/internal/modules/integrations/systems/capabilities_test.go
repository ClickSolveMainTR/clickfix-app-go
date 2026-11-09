package systems_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcp"
	mcpmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Defined actions reach the assistant as a confirmation card and MCP as a
// write tool, gated by tenant.integrations.use and mcp:write.
func TestCapabilitiesRegisterForAssistantAndMCP(t *testing.T) {
	reg := capability.NewRegistry().Register(systems.Capabilities()...)
	run, _ := reg.Get("integrations.actions.run")
	if run == nil || !run.Writes() || run.Permission != rbac.PermTenantIntegrationsUse || run.DryRun == nil {
		t.Fatalf("run capability: %+v", run)
	}
	if !run.OnSurface(capability.SurfaceWeb) || !run.OnSurface(capability.SurfaceMCP) || run.OnSurface(capability.SurfaceWidget) {
		t.Fatal("surfaces")
	}
	if len(run.Form("tr")) == 0 {
		t.Fatal("no card form")
	}
	// Card proposals without a customer are corrected by the model.
	if err := run.Check(capability.Params{"action": "fatura_tekrar_gonder"}); !errors.Is(err, capability.ErrInvalid) {
		var pe *capability.ParamError
		if !errors.As(err, &pe) {
			t.Fatalf("missing customer accepted: %v", err)
		}
	}
	cat := mcp.NewCatalog(reg)
	has := func(perms ...string) orgctx.Scope { return orgctx.Scope{ID: 1, Permissions: perms} }
	member := has(rbac.PermTenantIntegrationsRead, rbac.PermTenantIntegrationsUse)
	names := func(p mcp.Principal) map[string]bool {
		out := map[string]bool{}
		for _, tool := range cat.Available(p) {
			out[tool.Name] = tool.Write
		}
		return out
	}
	writer := names(mcp.Principal{Layer: mcpmodel.LayerBusiness, Scope: &member, CanWrite: true})
	if _, ok := writer["integrations_actions_run"]; !ok || !writer["integrations_actions_run"] {
		t.Fatalf("MCP write tool missing: %v", writer)
	}
	if _, ok := writer["integrations_concepts_lookup"]; !ok {
		t.Fatalf("MCP lookup missing: %v", writer)
	}
	reader := names(mcp.Principal{Layer: mcpmodel.LayerBusiness, Scope: &member})
	if _, ok := reader["integrations_actions_run"]; ok {
		t.Fatal("write tool offered without mcp:write")
	}
	outsider := has(rbac.PermTenantIntegrationsRead)
	if _, ok := names(mcp.Principal{Layer: mcpmodel.LayerBusiness, Scope: &outsider, CanWrite: true})["integrations_actions_run"]; ok {
		t.Fatal("write tool offered without tenant.integrations.use")
	}
}

type fakeAPI struct {
	calls []string
	body  string
}

func (f *fakeAPI) Call(_ context.Context, method, path string, _ any) (capability.Response, error) {
	f.calls = append(f.calls, method+" "+path)
	return capability.Response{Status: http.StatusOK, OK: true, Data: []byte(f.body)}, nil
}

// The card's dry run reads the definition and never posts.
func TestActionDryRunReadsOnly(t *testing.T) {
	reg := capability.NewRegistry().Register(systems.Capabilities()...)
	run, _ := reg.Get("integrations.actions.run")
	api := &fakeAPI{body: `{"connection":null,"concepts":[],"playbooks":[],"calls":[],"actions":[{"key":"iade_baslat","title":"İade başlat","method":"POST","path_template":"/iadeler","params":[],"requires_approval":true,"allowed_for":["agent"],"enabled":true}]}`}
	e := capability.Exec{Locale: "tr", API: capability.ReadOnly(api)}
	res, err := run.DryRun(context.Background(), e, capability.Params{"action": "iade_baslat", "external_id": "CUST-42"})
	if err != nil || res.Summary == "" || len(api.calls) != 1 || api.calls[0] != "GET "+systems.Base {
		t.Fatalf("dry run: %v %+v %v", err, res, api.calls)
	}
	api.body = `{"connection":null,"concepts":[],"playbooks":[],"calls":[],"actions":[{"key":"sadece_ai","title":"x","method":"POST","path_template":"/x","params":[],"requires_approval":false,"allowed_for":["ai"],"enabled":true}]}`
	if _, err := run.DryRun(context.Background(), e, capability.Params{"action": "sadece_ai", "external_id": "CUST-42"}); err == nil {
		t.Fatal("AI-only action offered to a member")
	}
}
