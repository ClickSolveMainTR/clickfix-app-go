package systems

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Assistant and MCP capabilities of the connected system (CF-197).
//
// The capability registry is built once at start-up, but each
// organization defines its own actions. So the actions are not one
// capability each: one write capability, integrations.actions.run, takes
// the organization's action key. Its DryRun and Validate resolve the
// organization's definition through the API, and the route applies the
// action's own rules (allowed_for must hold "agent", an approval-requiring
// action needs the card's confirmation). integrations.system.describe
// tells the model which concepts and action keys exist.

var routes = []string{"/settings/integrations"}

const keyPattern = "^[a-z][a-z0-9_]{1,63}$"

func tr(e capability.Exec, trText, en string, a ...any) string {
	if e.En() {
		return fmt.Sprintf(en, a...)
	}
	return fmt.Sprintf(trText, a...)
}

func str(p capability.Params, k string) string {
	v, _ := p[k].(string)
	return strings.TrimSpace(v)
}

func overview(ctx context.Context, api capability.API) (model.Overview, error) {
	r, err := api.Call(ctx, http.MethodGet, Base, nil)
	if err != nil {
		return model.Overview{}, err
	}
	var out model.Overview
	return out, r.Decode(&out)
}

func findAction(o model.Overview, key string) (model.Action, bool) {
	for _, a := range o.Actions {
		if a.Key == key {
			return a, true
		}
	}
	return model.Action{}, false
}

var customerProps = map[string]any{
	"contact_uuid": map[string]any{"type": "string", "format": "uuid", "title": "Kişi", "description": "Contact UUID of the customer (from the conversation)."},
	"external_id":  map[string]any{"type": "string", "maxLength": 255, "title": "Dış kimlik", "description": "The customer's id in the organization's own system, when no contact is known."},
}

func withCustomer(props map[string]any) map[string]any {
	for k, v := range customerProps {
		props[k] = v
	}
	return props
}

func validCustomer(p capability.Params) error {
	if str(p, "contact_uuid") == "" && str(p, "external_id") == "" {
		return capability.Invalid("contact_uuid or external_id is required: ask which customer")
	}
	return nil
}

// Capabilities are the connected system capabilities.
func Capabilities() []*capability.Capability {
	return []*capability.Capability{
		{
			Name: "integrations.system.describe", Module: "integrations", Kind: capability.Read, Surfaces: capability.ReadSurfaces,
			Permission: rbac.PermTenantIntegrationsRead, Routes: routes,
			Title:       map[string]string{"tr": "Bağlı sistemi anlat", "en": "Describe the connected system"},
			Description: "Describe the organization's connected system: its concepts (custom object types such as Invoice or License, with whether a data source is set), the actions that can be run (key, title, params, whether approval is needed, who may use them) and the AI playbooks. Call this before integrations_concepts_lookup or integrations_actions_run to learn the keys.",
			Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				o, err := overview(ctx, e.API)
				if err != nil {
					return capability.Result{}, err
				}
				concepts := []map[string]any{}
				for _, c := range o.Concepts {
					concepts = append(concepts, map[string]any{"key": c.Key, "label": c.Label, "has_source": c.Source != nil && c.Source.Enabled})
				}
				actions := []map[string]any{}
				for _, a := range o.Actions {
					if a.Enabled {
						actions = append(actions, map[string]any{"key": a.Key, "title": a.Title, "description": a.Description, "params": a.Params,
							"requires_approval": a.RequiresApproval, "allowed_for": a.AllowedFor, "concept": a.ConceptKey})
					}
				}
				playbooks := []map[string]any{}
				for _, p := range o.Playbooks {
					playbooks = append(playbooks, map[string]any{"intent": p.Intent, "title": p.Title, "concepts": p.ConceptKeys, "enabled": p.Enabled})
				}
				connected := o.Connection != nil
				return capability.Result{
					Summary: tr(e, "%d kavram, %d iş, %d tarif", "%d concepts, %d actions, %d playbooks", len(concepts), len(actions), len(playbooks)),
					Data:    map[string]any{"connected": connected, "concepts": concepts, "actions": actions, "playbooks": playbooks},
					Route:   "/settings/integrations/connect",
				}, nil
			},
		},
		{
			Name: "integrations.concepts.lookup", Module: "integrations", Kind: capability.Read, Surfaces: capability.ReadSurfaces,
			Permission: rbac.PermTenantIntegrationsUse, Routes: routes,
			Title:       map[string]string{"tr": "Bağlı sistemden bilgi getir", "en": "Look up connected system data"},
			Description: "Read one concept (for example invoices or licenses) of a customer from the organization's own system, by the customer's external id. Give the concept key from integrations_system_describe and the conversation's contact_uuid (or the external_id).",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"concept"},
				"properties": withCustomer(map[string]any{
					"concept": map[string]any{"type": "string", "pattern": keyPattern, "title": "Kavram", "description": "Concept key (custom object type key), e.g. invoice."},
				})},
			Validate: validCustomer,
			Run: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				q := url.Values{}
				if v := str(p, "contact_uuid"); v != "" {
					q.Set("contact_uuid", v)
				}
				if v := str(p, "external_id"); v != "" {
					q.Set("external_id", v)
				}
				r, err := e.API.Call(ctx, http.MethodGet, Base+"/concepts/"+url.PathEscape(str(p, "concept"))+"/lookup?"+q.Encode(), nil)
				if err != nil {
					return capability.Result{}, err
				}
				var out model.Lookup
				if err := r.Decode(&out); err != nil {
					return capability.Result{}, err
				}
				summary := tr(e, "%s: %d kayıt", "%s: %d records", out.Concept, len(out.Items))
				if out.Status != "ok" {
					summary = tr(e, "%s okunamadı: %s", "%s could not be read: %s", out.Concept, out.Reason)
				}
				return capability.Result{Summary: summary, Data: out}, nil
			},
		},
		{
			Name: "integrations.actions.run", Module: "integrations", Kind: capability.Write,
			Surfaces:   []string{capability.SurfaceWeb, capability.SurfaceMobile, capability.SurfaceMCP},
			Permission: rbac.PermTenantIntegrationsUse, Routes: routes,
			Title:       map[string]string{"tr": "Bağlı sistemde iş yap", "en": "Run a connected system action"},
			Description: "Run one of the organization's actions in its own system (for example resend an invoice or start a refund) for a customer. Use an action key from integrations_system_describe whose allowed_for holds \"agent\"; params are the action's params. The member confirms the card before anything is sent.",
			Undo:        "Bağlı sistemde işlemi geri alın / revert it in the connected system",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"action"},
				"properties": withCustomer(map[string]any{
					"action": map[string]any{"type": "string", "pattern": keyPattern, "title": "İş", "description": "Action key, e.g. resend_invoice."},
					"params": map[string]any{"type": "object", "title": "Parametreler", "description": "The action's params by key."},
				})},
			Validate: validCustomer,
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				o, err := overview(ctx, e.API)
				if err != nil {
					return capability.Result{}, err
				}
				a, ok := findAction(o, str(p, "action"))
				if !ok || !a.Enabled {
					return capability.Result{}, capability.Invalid("action %q does not exist or is turned off; call integrations_system_describe", str(p, "action"))
				}
				agent := false
				for _, c := range a.AllowedFor {
					agent = agent || c == model.ChannelAgent
				}
				if !agent {
					return capability.Result{}, capability.Invalid("action %q is not allowed for members", a.Key)
				}
				approval := tr(e, "onay gerekmez", "no approval needed")
				if a.RequiresApproval {
					approval = tr(e, "onay gerekir", "approval required")
				}
				return capability.Result{
					Summary: tr(e, "%s: %s %s (%s)", "%s: %s %s (%s)", a.Title, a.Method, a.PathTemplate, approval),
					Details: []capability.Detail{{Label: tr(e, "Uç", "Endpoint"), Value: a.Method + " " + a.PathTemplate}},
				}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				body := map[string]any{"approved": true, "params": map[string]any{}}
				if v, ok := p["params"].(map[string]any); ok {
					body["params"] = v
				}
				if v := str(p, "contact_uuid"); v != "" {
					body["contact_uuid"] = v
				}
				if v := str(p, "external_id"); v != "" {
					body["external_id"] = v
				}
				r, err := e.API.Call(ctx, http.MethodPost, Base+"/actions/"+url.PathEscape(str(p, "action"))+"/run", body)
				if err != nil {
					return capability.Result{}, err
				}
				var out model.RunResult
				if err := r.Decode(&out); err != nil {
					return capability.Result{}, err
				}
				if !out.OK {
					return capability.Result{}, fmt.Errorf("connected system: %s (%s)", out.Summary, out.ErrorCode)
				}
				return capability.Result{Summary: out.Summary, Data: out, Details: []capability.Detail{{Label: tr(e, "İstek", "Request"), Value: out.Request}}}, nil
			},
		},
	}
}
