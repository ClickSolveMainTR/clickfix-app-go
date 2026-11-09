package automation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
)

type pageAPI struct{ calls []string }

func (a *pageAPI) Call(_ context.Context, method, path string, _ any) (capability.Response, error) {
	a.calls = append(a.calls, method+" "+path)
	switch {
	case strings.HasPrefix(path, workflowsPath):
		return capability.Response{OK: true, Data: json.RawMessage(`{"items":[{"uuid":"0b6f2c1e-5d4a-4b3c-9a8b-7c6d5e4f3a2b","name":"İade","revision":2,"published_version":null,"enabled":false}]}`)}, nil
	case strings.HasPrefix(path, rulesPath+"?"):
		return capability.Response{OK: true, Data: json.RawMessage(`{"items":[{"uuid":"1b6f2c1e-5d4a-4b3c-9a8b-7c6d5e4f3a2b","name":"Geciken yanıt","trigger_event":"sla.breached","enabled":true}],"total":1}`)}, nil
	case strings.HasPrefix(path, "/v1/tenant/conversations"):
		return capability.Response{OK: true, Data: json.RawMessage(`{"items":[{"uuid":"2b6f2c1e-5d4a-4b3c-9a8b-7c6d5e4f3a2b"}]}`)}, nil
	case strings.HasSuffix(path, "/test"):
		return capability.Response{OK: true, Data: json.RawMessage(`{"matched":true,"actions":[{"type":"notify"}],"dry_run":true}`)}, nil
	}
	return capability.Response{Status: 404, Code: "NOT_FOUND"}, nil
}

func TestAutomationPagesAndRuleDryRun(t *testing.T) {
	api := &pageAPI{}
	e := capability.Exec{API: capability.ReadOnly(api), Locale: "tr"}
	pages := Pages()
	wf, err := pages[0].State(context.Background(), e)
	if err != nil || len(wf.Warnings) != 1 || wf.Warnings[0].Capability != "workflows.enable" || len(wf.Tests) != 1 {
		t.Fatalf("workflows %+v %v", wf, err)
	}
	rules, err := pages[1].State(context.Background(), e)
	if err != nil || len(rules.Tests) != 1 || !strings.Contains(rules.Tests[0].Title, "SLA kuralını") {
		t.Fatalf("rules %+v %v", rules, err)
	}
	// The rule dry run may only use its simulation endpoint.
	var dry *capability.Capability
	for _, c := range RuleCapabilities() {
		if c.Name == "automations.dry_run" {
			dry = c
		}
	}
	capability.NewRegistry().Register(RuleCapabilities()...)
	res, err := dry.Run(context.Background(), capability.Exec{API: capability.ReadOnly(api, dry.Simulations...), Locale: "en"},
		capability.Params{"rule_uuid": "1b6f2c1e-5d4a-4b3c-9a8b-7c6d5e4f3a2b"})
	if err != nil || !strings.Contains(res.Summary, "match") {
		t.Fatalf("dry run %+v %v", res, err)
	}
	if _, err := dry.Run(context.Background(), capability.Exec{API: capability.ReadOnly(api), Locale: "en"},
		capability.Params{"rule_uuid": "1b6f2c1e-5d4a-4b3c-9a8b-7c6d5e4f3a2b"}); err == nil {
		t.Fatal("dry run passed a GET-only API without its simulation")
	}
}
