package automation

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Page states and rule capabilities of the automation module (CF-187,
// scenario 3): what is missing on the workflows and rules pages and the
// dry runs the assistant can start there.

const rulesPath = "/v1/tenant/automations"

type ruleView struct {
	UUID         uuid.UUID `json:"uuid"`
	Name         string    `json:"name"`
	TriggerEvent string    `json:"trigger_event"`
	Enabled      bool      `json:"enabled"`
}

func listRules(ctx context.Context, api capability.API) ([]ruleView, int64, error) {
	r, err := api.Call(ctx, http.MethodGet, rulesPath+"?limit=30", nil)
	if err != nil {
		return nil, 0, err
	}
	var page struct {
		Items []ruleView `json:"items"`
		Total int64      `json:"total"`
	}
	decErr := r.Decode(&page)
	return page.Items, page.Total, decErr
}

func listWorkflows(ctx context.Context, api capability.API) ([]workflowView, error) {
	r, err := api.Call(ctx, http.MethodGet, workflowsPath+"?limit=30", nil)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []workflowView `json:"items"`
	}
	decErr := r.Decode(&page)
	return page.Items, decErr
}

// RuleCapabilities are the automation rule capabilities.
func RuleCapabilities() []*capability.Capability {
	routes := []string{"/rules"}
	return []*capability.Capability{
		{
			Name: "automations.list", Surfaces: capability.ReadSurfaces, Module: "automation", Kind: capability.Read, Permission: rbac.PermTenantAutomationRead, Routes: routes,
			Title:       map[string]string{"tr": "Otomasyon kurallarını listele", "en": "List automation rules"},
			Description: "List the organization's automation rules: uuid, name, trigger (sla.breached rules are SLA rules) and whether it is enabled.",
			Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				items, total, err := listRules(ctx, e.API)
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: fmt.Sprintf("%d rules", total), Data: map[string]any{"items": items, "total": total}}, nil
			},
		},
		{
			Name: "automations.dry_run", Surfaces: capability.ReadSurfaces, Module: "automation", Kind: capability.Read, Permission: rbac.PermTenantAutomationWrite, Routes: routes,
			Title:       map[string]string{"tr": "Kuralı kuru çalıştır", "en": "Dry-run a rule"},
			Description: "Check whether an automation rule's conditions match a conversation (default: the latest) and which actions it would run, without running them.",
			Simulations: []string{"POST " + rulesPath + "/{uuid}/test"},
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"rule_uuid"},
				"properties": map[string]any{
					"rule_uuid":         map[string]any{"type": "string", "pattern": "^[0-9a-fA-F-]{36}$"},
					"conversation_uuid": map[string]any{"type": "string", "pattern": "^[0-9a-fA-F-]{36}$"},
				}},
			Run: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := str(p, "rule_uuid")
				if _, err := uuid.Parse(id); err != nil {
					return capability.Result{}, capability.Invalid("rule_uuid must be a UUID")
				}
				conv := str(p, "conversation_uuid")
				if conv == "" {
					conv = latestConversation(ctx, e.API)
				}
				if conv == "" {
					return capability.Result{Summary: "There is no conversation to test on yet."}, nil
				}
				r, err := e.API.Call(ctx, http.MethodPost, rulesPath+"/"+id+"/test", map[string]any{"conversation_uuid": conv})
				if err != nil {
					return capability.Result{}, err
				}
				var out struct {
					Matched bool `json:"matched"`
					Actions any  `json:"actions"`
				}
				if err := r.Decode(&out); err != nil {
					return capability.Result{}, err
				}
				summary := t(e, "Koşullar bu konuşmada sağlanmadı; eylem çalışmazdı.", "The conditions do not match this conversation; no action would run.")
				if out.Matched {
					summary = t(e, "Koşullar sağlandı; eylemler çalışırdı.", "The conditions match; the actions would run.")
				}
				return capability.Result{Summary: summary, Data: map[string]any{"conversation_uuid": conv, "matched": out.Matched, "actions": out.Actions}}, nil
			},
		},
	}
}

// Pages are the page-state providers of the module.
func Pages() []*capability.Page {
	return []*capability.Page{
		{
			Module: "automation", Routes: []string{"/workflows"}, Permission: rbac.PermTenantAutomationRead,
			State: func(ctx context.Context, e capability.Exec) (capability.PageState, error) {
				st := capability.PageState{Module: "automation", Title: t(e, "İş akışları", "Workflows")}
				items, err := listWorkflows(ctx, e.API)
				if err != nil {
					return st, err
				}
				if len(items) == 0 {
					st.Missing = append(st.Missing, capability.Step{Key: "create_workflow", Capability: "workflows.create",
						Title:  t(e, "Henüz iş akışı yok", "No workflow yet"),
						Prompt: t(e, "Gelen mesajlar için ilk iş akışımı kurmama yardım et", "Help me set up my first workflow for incoming messages")})
				}
				for i, w := range items {
					if w.PublishedVersion == nil {
						st.Warnings = append(st.Warnings, capability.Step{Key: "unpublished:" + w.UUID.String(), Capability: "workflows.enable",
							Title:  t(e, "“%s” taslakta, çalışmıyor", "“%s” is a draft and does not run", w.Name),
							Prompt: t(e, "“%s” iş akışını yayınla ve etkinleştir", "Publish and enable the “%s” workflow", w.Name),
							Focus:  &capability.Focus{Route: "/workflows/" + w.UUID.String()}})
					}
					if i < 3 {
						st.Tests = append(st.Tests, capability.Test{Capability: "workflows.dry_run",
							Title:  t(e, "“%s” iş akışını kuru çalıştır", "Dry-run “%s”", w.Name),
							Prompt: t(e, "“%s” iş akışını son konuşmada kuru çalıştır", "Dry-run the “%s” workflow on the latest conversation", w.Name)})
					}
				}
				return st, nil
			},
		},
		{
			Module: "automation", Routes: []string{"/rules"}, Permission: rbac.PermTenantAutomationRead,
			State: func(ctx context.Context, e capability.Exec) (capability.PageState, error) {
				st := capability.PageState{Module: "automation", Title: t(e, "Otomasyon kuralları", "Automation rules")}
				items, _, err := listRules(ctx, e.API)
				if err != nil {
					return st, err
				}
				if len(items) == 0 {
					st.Missing = append(st.Missing, capability.Step{Key: "create_rule", Capability: "automations.create",
						Title:  t(e, "Henüz otomasyon kuralı yok", "No automation rule yet"),
						Prompt: t(e, "Hangi otomasyon kurallarıyla başlamalıyım? Birini birlikte kuralım", "Which automation rules should I start with? Let's set one up")})
				}
				for i, r := range items {
					if i >= 3 {
						break
					}
					title := t(e, "“%s” kuralını kuru çalıştır", "Dry-run “%s”", r.Name)
					if r.TriggerEvent == "sla.breached" {
						title = t(e, "“%s” SLA kuralını kuru çalıştır", "Dry-run the SLA rule “%s”", r.Name)
					}
					st.Tests = append(st.Tests, capability.Test{Capability: "automations.dry_run", Title: title,
						Prompt: t(e, "“%s” kuralını son konuşmada kuru çalıştır", "Dry-run the “%s” rule on the latest conversation", r.Name)})
				}
				return st, nil
			},
		},
	}
}
