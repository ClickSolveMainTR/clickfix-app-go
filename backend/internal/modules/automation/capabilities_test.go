package automation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
)

func capByName(name string) *capability.Capability {
	c, _ := capability.NewRegistry().Register(Capabilities()...).Get(name)
	return c
}

func validDraft() capability.Params {
	return capability.Params{
		"name": "İade talepleri", "trigger": "message.received", "enable": true,
		"condition": map[string]any{"all": []any{map[string]any{"field": "subject", "operator": "contains", "value": "iade"}}},
		"actions": []any{
			map[string]any{"type": "set_priority", "value": "high"},
			map[string]any{"type": "add_note", "content": "İade talebi: muhasebeye bilgi verin."},
		},
	}
}

func TestDraftBuildsAValidGraph(t *testing.T) {
	c := capByName("workflows.create")
	p := validDraft()
	if err := c.Check(p); err != nil {
		t.Fatal(err)
	}
	d, _ := readDraft(p)
	g := BuildGraph(d)
	if err := usecase.ValidateGraph(g); err != nil || len(g.Nodes) != 5 || len(g.Edges) != 5 {
		t.Fatalf("graph %d nodes %d edges: %v", len(g.Nodes), len(g.Edges), err)
	}
	// Without a condition: trigger → actions → end.
	delete(p, "condition")
	d, _ = readDraft(p)
	if g := BuildGraph(d); usecase.ValidateGraph(g) != nil || len(g.Nodes) != 4 {
		t.Fatalf("no-condition graph %+v", g)
	}
	res, err := c.DryRun(context.Background(), capability.Exec{Locale: "tr"}, validDraft())
	if err != nil || !strings.Contains(res.Summary, "geçerli") || len(res.Details) != 4 || !strings.Contains(res.Details[1].Value, "subject contains") {
		t.Fatalf("dry run %+v %v", res, err)
	}
}

func TestDraftRejectsWhatTheEngineWouldReject(t *testing.T) {
	c := capByName("workflows.create")
	mutate := map[string]func(capability.Params){
		"unknown trigger": func(p capability.Params) { p["trigger"] = "message.deleted" },
		"no actions":      func(p capability.Params) { p["actions"] = []any{} },
		"unknown action":  func(p capability.Params) { p["actions"] = []any{map[string]any{"type": "delete_everything"}} },
		"bad priority": func(p capability.Params) {
			p["actions"] = []any{map[string]any{"type": "set_priority", "value": "asap"}}
		},
		"tag without uuid": func(p capability.Params) { p["actions"] = []any{map[string]any{"type": "add_tag"}} },
		"bad field": func(p capability.Params) {
			p["condition"] = map[string]any{"field": "password", "operator": "eq", "value": "x"}
		},
		"bad operator": func(p capability.Params) {
			p["condition"] = map[string]any{"field": "subject", "operator": "matches", "value": "x"}
		},
		"schedule without minutes": func(p capability.Params) { p["trigger"] = "scheduled" },
		"extra field":              func(p capability.Params) { p["sql"] = "DROP TABLE" },
		"wrong type":               func(p capability.Params) { p["enable"] = "yes" },
	}
	for name, m := range mutate {
		p := validDraft()
		m(p)
		if err := c.Check(p); !errors.Is(err, capability.ErrInvalid) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
}

func TestDryRunCapabilityMayOnlySimulate(t *testing.T) {
	c := capByName("workflows.dry_run")
	if c.Writes() || len(c.Simulations) != 1 {
		t.Fatal("dry run must be a read with one simulation endpoint")
	}
	if c := capByName("workflows.create"); !c.Writes() || c.DryRun == nil || c.Verify == nil {
		t.Fatal("create must be a confirmed write with dry run and verify")
	}
}
