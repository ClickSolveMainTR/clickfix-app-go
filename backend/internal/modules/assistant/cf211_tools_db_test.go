package assistant_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// orgAPI is an API factory that records which organization every call
// was made for.
type orgAPI struct {
	mu   sync.Mutex
	api  *recAPI
	orgs []uuid.UUID
}

func (o *orgAPI) factory(_, org uuid.UUID) capability.API {
	o.mu.Lock()
	o.orgs = append(o.orgs, org)
	o.mu.Unlock()
	return o.api
}

func (o *orgAPI) reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.orgs = nil
	o.api.mu.Lock()
	o.api.calls, o.api.bodies = nil, nil
	o.api.mu.Unlock()
}

func (o *orgAPI) calls() []string {
	o.api.mu.Lock()
	defer o.api.mu.Unlock()
	return append([]string{}, o.api.calls...)
}

// Every CF-211 tool, run from the chat through use_tool:
//   - without its permission it is refused and no /v1 call is made;
//   - with it, a read calls its route once, for the caller's organization,
//     and is written to the audit log; a write only becomes a card;
//   - a card cannot be confirmed by a member without the permission, and
//     the confirmed call is made for the caller's organization.
func TestCF211ToolsFromTheChat(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), "DELETE FROM activity_events WHERE organization_id = $1", f.scope.ID)
	})
	reg := capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).Register(cf211Capabilities()...)
	f.svc.Caps = reg
	f.svc.Activity = activity.NewRecorder(f.q, slog.Default())
	rec := &orgAPI{api: &recAPI{}}
	f.svc.API = rec.factory
	all := map[string]bool{}
	for _, cp := range reg.All() {
		if cp.Permission != "" {
			all[cp.Permission] = true
		}
	}
	callerWithout := func(perm string) assistant.Caller {
		perms := []string{}
		for p := range all {
			if p != perm {
				perms = append(perms, p)
			}
		}
		return f.caller(perms...)
	}
	everything := callerWithout("")
	pc := assistant.Context{Surface: "web", Route: "/dashboard"}
	// Two replies per tool: above the default daily limit of the fixture.
	if _, err := f.q.AssistantSettingsSave(ctx, db.AssistantSettingsSaveParams{Enabled: true, ModelRoute: "reply", DailyLimitPerOrg: 1000}); err != nil {
		t.Fatal(err)
	}
	use := func(t *testing.T, c assistant.Caller, cp *capability.Capability, raw string) (assistant.SendResult, string, uuid.UUID) {
		t.Helper()
		th, err := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
		if err != nil {
			t.Fatal(err)
		}
		input, _ := json.Marshal(map[string]any{"name": cp.ToolName(), "params": json.RawMessage(raw), "summary": "Test"})
		f.prov.mu.Lock()
		f.prov.reqs = nil
		f.prov.steps = []ai.Result{call("u1", "use_tool", string(input)), {Text: `{"reply":"Tamam.","suggestions":[]}`}}
		f.prov.mu.Unlock()
		res, err := f.svc.Send(ctx, c, th.UUID, "Bunu yap: "+cp.Name, pc)
		if err != nil {
			t.Fatal(err)
		}
		f.prov.mu.Lock()
		defer f.prov.mu.Unlock()
		if len(f.prov.reqs) < 2 {
			t.Fatalf("%s: the model got no tool result", cp.Name)
		}
		return res, toolTexts(f.prov.reqs[1]), th.UUID
	}
	for _, cp := range reg.All() {
		tc, ok := cf211Cases[cp.Name]
		if !ok {
			continue // core capabilities have their own tests
		}
		t.Run(cp.Name, func(t *testing.T) {
			// Permission denied.
			rec.reset()
			res, out, _ := use(t, callerWithout(cp.Permission), cp, tc.params)
			if !strings.Contains(out, "unknown tool for this user") || len(res.Reply.Actions) != 0 || len(rec.calls()) != 0 {
				t.Fatalf("denied: result %s, actions %v, calls %v", out, res.Reply.Actions, rec.calls())
			}
			// Allowed.
			rec.reset()
			res, out, thread := use(t, everything, cp, tc.params)
			if !cp.Writes() {
				if calls := rec.calls(); len(calls) != 1 || calls[0] != tc.call || strings.Contains(out, `"error"`) {
					t.Fatalf("read: result %s, calls %v", out, calls)
				}
				for _, org := range rec.orgs {
					if org != f.scope.UUID {
						t.Fatalf("read ran for organization %s", org)
					}
				}
				var n int
				_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM activity_events WHERE organization_id = $1 AND action = $2 AND payload->>'capability' = $3",
					f.scope.ID, assistant.ActionToolCalled, cp.Name).Scan(&n)
				if n == 0 {
					t.Fatal("read tool not audited")
				}
				return
			}
			if len(rec.calls()) != 0 || len(res.Reply.Actions) != 1 || res.Reply.Actions[0].Capability != cp.Name {
				t.Fatalf("write: calls %v before confirm, actions %v, result %s", rec.calls(), res.Reply.Actions, out)
			}
			actionID := res.Reply.Actions[0].ID
			if _, err := f.svc.Confirm(ctx, callerWithout(cp.Permission), thread, actionID, nil); !errors.Is(err, assistant.ErrForbidden) {
				t.Fatalf("confirm without permission: %v", err)
			}
			if len(rec.calls()) != 0 {
				t.Fatalf("forbidden confirm called %v", rec.calls())
			}
			if _, err := f.svc.Confirm(ctx, everything, thread, actionID, nil); err != nil {
				t.Fatal(err)
			}
			if calls := rec.calls(); len(calls) != 1 || calls[0] != tc.call {
				t.Fatalf("confirm calls %v, want %s", calls, tc.call)
			}
			for _, org := range rec.orgs {
				if org != f.scope.UUID {
					t.Fatalf("write ran for organization %s", org)
				}
			}
		})
	}
}

// find_tools only lists what the member may use, and the loaded tools of
// one turn stay well below the registry.
func TestCF211FindTools(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	reg := capability.NewRegistry().Register(assistant.CoreCapabilities(f.q)...).Register(cf211Capabilities()...)
	f.svc.Caps = reg
	f.svc.API = (&orgAPI{api: &recAPI{}}).factory
	pc := assistant.Context{Surface: "web", Route: "/inbox"}
	c := f.caller(rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReply, rbac.PermTenantContactsRead, rbac.PermTenantMacrosRead)
	th, _ := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
	f.prov.steps = []ai.Result{call("f1", "find_tools", `{"query":"fatura ve randevu, kişi sil"}`), call("f2", "find_tools", `{"query":"makro"}`),
		{Text: `{"reply":"Tamam.","suggestions":[]}`}}
	if _, err := f.svc.Send(ctx, c, th.UUID, "Bekleyen konuşmaları göster", pc); err != nil {
		t.Fatal(err)
	}
	first := f.prov.reqs[0]
	names := map[string]bool{}
	for _, tl := range first.Tools {
		names[tl.Name] = true
	}
	if !names["conversations_search"] || !names["find_tools"] || names["contacts_delete"] || len(first.Tools) > 22 {
		t.Fatalf("loaded tools %v", names)
	}
	if first.CacheTools == 0 || first.Tools[first.CacheTools-1].Name != "use_tool" {
		t.Fatalf("stable tools are not first: %d", first.CacheTools)
	}
	out := toolTexts(f.prov.reqs[1])
	for _, forbidden := range []string{"billing_", "appointments_", "contacts_delete"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("find_tools leaked %s: %s", forbidden, out)
		}
	}
	if out2 := toolTexts(f.prov.reqs[2]); !strings.Contains(out2, "macros_list") && !strings.Contains(out2, "canned_responses_list") && !strings.Contains(out2, "Nothing else") {
		t.Fatalf("find_tools makro: %s", out2)
	}
}
