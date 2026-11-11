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

// Set-up capabilities: the assistant fills the "Sistemini bağla" screen
// by talking (CF-197 scope 5). When a member shares the system's API
// document (CF-191 document input), the model proposes concepts,
// endpoints, actions and playbooks as cards; nothing is saved before the
// member confirms. Secrets never come from chat: the connection's secret
// is entered on the screen only.

func conceptUUID(o model.Overview, key string) (string, bool) {
	for _, c := range o.Concepts {
		if c.Key == key {
			return c.ObjectTypeUUID.String(), true
		}
	}
	return "", false
}

func strList(p capability.Params, k string) []string {
	raw, _ := p[k].([]any)
	out := []string{}
	for _, v := range raw {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func boolOr(p capability.Params, k string, def bool) bool {
	if v, ok := p[k].(bool); ok {
		return v
	}
	return def
}

var setupRoutes = []string{"/settings/integrations/connect"}

var fieldsProp = map[string]any{"type": "array", "maxItems": 30, "title": "Alanlar", "description": "Response paths to keep, with the name the AI sees.",
	"items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"path", "label"},
		"properties": map[string]any{"path": map[string]any{"type": "string", "maxLength": 520}, "label": map[string]any{"type": "string", "maxLength": 80}}}}

var paramsProp = map[string]any{"type": "array", "maxItems": 20, "title": "Parametreler", "description": "Inputs of the action; usable in the path as {key}.",
	"items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"key", "label", "type", "required"},
		"properties": map[string]any{"key": map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_]{0,39}$"}, "label": map[string]any{"type": "string", "maxLength": 80},
			"type": map[string]any{"type": "string", "enum": []string{"string", "number", "boolean"}}, "required": map[string]any{"type": "boolean"}}}}

// SetupCapabilities are the write capabilities that fill the screen.
func SetupCapabilities() []*capability.Capability {
	return []*capability.Capability{
		{
			Name: "integrations.concepts.create", Module: "integrations", Kind: capability.Write, Permission: rbac.PermTenantCustomFieldsManage, Routes: setupRoutes,
			Title:       map[string]string{"tr": "Kavram ekle", "en": "Add a concept"},
			Description: "Add a concept of the organization's system (a custom object type such as Fatura / Invoice, Lisans, Sipariş). Propose this when the member describes or shares a document of their system.",
			Undo:        "/settings/custom-objects",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"key", "singular_label", "plural_label"},
				"properties": map[string]any{
					"key":            map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_]{0,62}$", "title": "Anahtar", "description": "Lowercase key, e.g. fatura."},
					"singular_label": map[string]any{"type": "string", "maxLength": 80, "title": "Tekil ad"},
					"plural_label":   map[string]any{"type": "string", "maxLength": 80, "title": "Çoğul ad"},
				}},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				o, err := overview(ctx, e.API)
				if err != nil {
					return capability.Result{}, err
				}
				if _, ok := conceptUUID(o, str(p, "key")); ok {
					return capability.Result{}, capability.Invalid("concept %q already exists", str(p, "key"))
				}
				return capability.Result{Summary: tr(e, "%s kavramı eklenecek", "The %s concept will be added", str(p, "singular_label"))}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				r, err := e.API.Call(ctx, http.MethodPost, "/v1/tenant/custom-objects", map[string]any{"key": str(p, "key"), "singular_label": str(p, "singular_label"), "plural_label": str(p, "plural_label")})
				if err != nil {
					return capability.Result{}, err
				}
				if err := r.Err(); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: str(p, "singular_label"), Route: "/settings/integrations/connect", Focus: &capability.Focus{Route: "/settings/integrations/connect"}}, nil
			},
		},
		{
			Name: "integrations.sources.save", Module: "integrations", Kind: capability.Write, Permission: rbac.PermTenantIntegrationsManage, Routes: setupRoutes,
			Title:       map[string]string{"tr": "Kavramın ucunu ayarla", "en": "Set a concept's endpoint"},
			Description: "Set where a concept is read from in the organization's system: an endpoint relative to the connection's base URL that holds {external_id} (or {company_external_id}), where the records are in the answer and which fields to keep. Take the endpoint from the member's API document; ask the member to press Dene on the screen to check it.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"concept", "path_template"},
				"properties": map[string]any{
					"concept":       map[string]any{"type": "string", "pattern": keyPattern, "title": "Kavram"},
					"path_template": map[string]any{"type": "string", "maxLength": 500, "title": "Uç", "description": "e.g. /faturalar?musteri={external_id}"},
					"identity_key":  map[string]any{"type": "string", "enum": []string{model.KeyExternalID, model.KeyCompanyExternalID}, "title": "Kimlik"},
					"items_path":    map[string]any{"type": "string", "maxLength": 200, "title": "Kayıt yolu", "description": "Dotted path of the records array, e.g. data.invoices."},
					"fields":        fieldsProp,
				}},
			Validate: func(p capability.Params) error {
				path := str(p, "path_template")
				if !strings.HasPrefix(path, "/") || (!strings.Contains(path, "{external_id}") && !strings.Contains(path, "{company_external_id}")) {
					return capability.Invalid("path_template must start with / and hold {external_id} or {company_external_id}")
				}
				return nil
			},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				o, err := overview(ctx, e.API)
				if err != nil {
					return capability.Result{}, err
				}
				if _, ok := conceptUUID(o, str(p, "concept")); !ok {
					return capability.Result{}, capability.Invalid("concept %q does not exist; propose integrations_concepts_create first", str(p, "concept"))
				}
				note := ""
				if o.Connection == nil {
					note = tr(e, " (önce bağlantı kaydedilmeli)", " (save the connection first)")
				}
				return capability.Result{Summary: tr(e, "%s, GET %s ile okunacak%s", "%s will be read with GET %s%s", str(p, "concept"), str(p, "path_template"), note)}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				o, err := overview(ctx, e.API)
				if err != nil {
					return capability.Result{}, err
				}
				id, ok := conceptUUID(o, str(p, "concept"))
				if !ok {
					return capability.Result{}, capability.Invalid("concept %q does not exist", str(p, "concept"))
				}
				body := map[string]any{"path_template": str(p, "path_template"), "identity_key": str(p, "identity_key"), "items_path": str(p, "items_path"), "enabled": true, "fields": []any{}}
				if body["identity_key"] == "" {
					body["identity_key"] = model.KeyExternalID
				}
				if f, ok := p["fields"].([]any); ok {
					body["fields"] = f
				}
				r, err := e.API.Call(ctx, http.MethodPut, Base+"/sources/"+id, body)
				if err != nil {
					return capability.Result{}, err
				}
				if err := r.Err(); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: str(p, "concept"), Focus: &capability.Focus{Route: "/settings/integrations/connect"}}, nil
			},
		},
		{
			Name: "integrations.actions.save", Module: "integrations", Kind: capability.Write, Permission: rbac.PermTenantIntegrationsManage, Routes: setupRoutes,
			Title:       map[string]string{"tr": "Yapılabilecek iş tanımla", "en": "Define an action"},
			Description: "Define an action of the organization's system (e.g. resend an invoice, start a refund): endpoint, params, whether a member must approve, and who may use it (ai, agent, workflow). Default to requires_approval true and allowed_for [agent] unless the member says otherwise.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"key", "title", "path_template"},
				"properties": map[string]any{
					"key":               map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_]{1,49}$", "title": "Anahtar"},
					"title":             map[string]any{"type": "string", "maxLength": 120, "title": "Ad"},
					"description":       map[string]any{"type": "string", "maxLength": 1000, "title": "Açıklama"},
					"concept":           map[string]any{"type": "string", "title": "Kavram"},
					"method":            map[string]any{"type": "string", "enum": []string{"POST", "PUT", "PATCH", "DELETE"}, "title": "Yöntem"},
					"path_template":     map[string]any{"type": "string", "maxLength": 500, "title": "Uç"},
					"params":            paramsProp,
					"requires_approval": map[string]any{"type": "boolean", "title": "Onay gerekir"},
					"allowed_for":       map[string]any{"type": "array", "title": "Kim kullanabilir", "items": map[string]any{"type": "string", "enum": model.Channels}},
				}},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				method := str(p, "method")
				if method == "" {
					method = http.MethodPost
				}
				return capability.Result{Summary: tr(e, "%s: %s %s", "%s: %s %s", str(p, "title"), method, str(p, "path_template"))}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				allowed := strList(p, "allowed_for")
				if len(allowed) == 0 {
					allowed = []string{model.ChannelAgent}
				}
				body := map[string]any{"title": str(p, "title"), "description": str(p, "description"), "concept_key": str(p, "concept"), "method": str(p, "method"),
					"path_template": str(p, "path_template"), "params": []any{}, "requires_approval": boolOr(p, "requires_approval", true), "allowed_for": allowed, "enabled": true}
				if v, ok := p["params"].([]any); ok {
					body["params"] = v
				}
				r, err := e.API.Call(ctx, http.MethodPut, Base+"/actions/"+url.PathEscape(str(p, "key")), body)
				if err != nil {
					return capability.Result{}, err
				}
				if err := r.Err(); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: str(p, "title"), Focus: &capability.Focus{Route: "/settings/integrations/connect"}}, nil
			},
		},
		{
			Name: "integrations.playbooks.save", Module: "integrations", Kind: capability.Write, Permission: rbac.PermTenantIntegrationsManage, Routes: setupRoutes,
			Title:       map[string]string{"tr": "AI tarifi yaz", "en": "Write an AI playbook"},
			Description: "Write a plain-language playbook telling the AI how to handle one kind of customer request (e.g. 'my invoice did not arrive': look at Invoice; if the status is Error read the error text and hand off). The intent is classified by the AI from free sentences.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"intent", "title", "instructions"},
				"properties": map[string]any{
					"intent":       map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_]{1,63}$", "title": "Niyet"},
					"title":        map[string]any{"type": "string", "maxLength": 200, "title": "Müşteri ne ister?"},
					"instructions": map[string]any{"type": "string", "maxLength": 3000, "title": "Tarif"},
					"concepts":     map[string]any{"type": "array", "maxItems": 10, "title": "Kavramlar", "items": map[string]any{"type": "string"}},
					"actions":      map[string]any{"type": "array", "maxItems": 10, "title": "İşler", "items": map[string]any{"type": "string"}},
				}},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				o, err := overview(ctx, e.API)
				if err != nil {
					return capability.Result{}, err
				}
				for _, k := range strList(p, "concepts") {
					if _, ok := conceptUUID(o, k); !ok {
						return capability.Result{}, capability.Invalid("concept %q does not exist", k)
					}
				}
				for _, k := range strList(p, "actions") {
					if _, ok := findAction(o, k); !ok {
						return capability.Result{}, capability.Invalid("action %q does not exist", k)
					}
				}
				return capability.Result{Summary: fmt.Sprintf("%s (%s)", str(p, "title"), str(p, "intent"))}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				body := map[string]any{"title": str(p, "title"), "instructions": str(p, "instructions"), "concept_keys": strList(p, "concepts"), "action_keys": strList(p, "actions"), "enabled": true}
				r, err := e.API.Call(ctx, http.MethodPut, Base+"/playbooks/"+url.PathEscape(str(p, "intent")), body)
				if err != nil {
					return capability.Result{}, err
				}
				if err := r.Err(); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: str(p, "title"), Focus: &capability.Focus{Route: "/settings/integrations/connect"}}, nil
			},
		},
	}
}
