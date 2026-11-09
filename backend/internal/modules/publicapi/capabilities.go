package publicapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// API key capabilities of the assistant (CF-190). A new key is created by
// the route and shown once on the card (never stored in the chat); the
// model only ever sees the key's prefix.

const keysPath = "/v1/tenant/api-keys"

func say(e capability.Exec, tr, en string, a ...any) string {
	if e.En() {
		return fmt.Sprintf(en, a...)
	}
	return fmt.Sprintf(tr, a...)
}

func text(p capability.Params, k string) string { s, _ := p[k].(string); return strings.TrimSpace(s) }

// UsageExample is a request with the key as a placeholder.
func UsageExample(apiURL string) string {
	if apiURL == "" {
		apiURL = "https://<api>"
	}
	return `curl -H "Authorization: Bearer $CLICKFIX_API_KEY" "` + strings.TrimSuffix(apiURL, "/") + `/v1/public-api/conversations?limit=5"`
}

// Capabilities are the API key capabilities; apiURL is PUBLIC_API_URL.
func Capabilities(apiURL string) []*capability.Capability {
	routes := []string{"/settings/integrations"}
	return []*capability.Capability{
		{
			Name: "api_keys.list", Module: "api_keys", Kind: capability.Read, Permission: rbac.PermTenantAPIKeysManage, Routes: routes,
			Title:       map[string]string{"tr": "API anahtarlarını listele", "en": "List API keys"},
			Description: "List the organization's API keys: name, prefix, scopes, rate limit, expiry, last use. Secrets are never shown.",
			Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				r, err := e.API.Call(ctx, http.MethodGet, keysPath+"?limit=50", nil)
				if err != nil {
					return capability.Result{}, err
				}
				var out json.RawMessage
				decErr := r.Decode(&out)
				return capability.Result{Summary: "API keys", Data: out}, decErr
			},
		},
		{
			Name: "api_keys.create", Module: "api_keys", Kind: capability.Write, Permission: rbac.PermTenantAPIKeysManage, Routes: routes,
			Title: map[string]string{"tr": "API anahtarı oluştur", "en": "Create an API key"},
			Description: "Create a public API key with the least scopes the user needs (e.g. conversations:read). The key is created by the " +
				"server and shown to the user once; never ask for it or repeat it.",
			Undo: "api_keys.revoke",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "scopes"},
				"properties": map[string]any{
					"name":            map[string]any{"type": "string", "title": "Name", "minLength": 1, "maxLength": 120},
					"scopes":          map[string]any{"type": "array", "title": "Scopes", "minItems": 1, "maxItems": len(model.Scopes), "items": map[string]any{"type": "string", "enum": model.Scopes}},
					"rate_limit":      map[string]any{"type": "integer", "title": "Requests per minute", "minimum": 1, "maximum": 6000},
					"expires_in_days": map[string]any{"type": "integer", "title": "Expires in (days)", "minimum": 1, "maximum": 730},
				}},
			DryRun: func(_ context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				scopes, _ := json.Marshal(p["scopes"])
				return capability.Result{Summary: say(e, "“%s” anahtarı oluşturulacak; anahtar yalnız bir kez gösterilecek.", "Key “%s” will be created; it is shown only once.", text(p, "name")),
					Details: []capability.Detail{{Label: say(e, "Yetki kapsamları", "Scopes"), Value: string(scopes)}}}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				body := map[string]any{"name": text(p, "name"), "scopes": p["scopes"]}
				if v, ok := p["rate_limit"].(float64); ok {
					body["rate_limit"] = int(v)
				}
				if v, ok := p["expires_in_days"].(float64); ok {
					body["expires_at"] = time.Now().UTC().Add(time.Duration(v) * 24 * time.Hour)
				}
				r, err := e.API.Call(ctx, http.MethodPost, keysPath, body)
				if err != nil {
					return capability.Result{}, err
				}
				var out struct {
					UUID uuid.UUID `json:"uuid"`
					Key  string    `json:"key"`
				}
				if err := r.Decode(&out); err != nil {
					return capability.Result{}, err
				}
				res := capability.Result{Summary: out.UUID.String(), Route: "/settings/integrations",
					Details: []capability.Detail{{Label: say(e, "Kullanım örneği", "Example request"), Code: UsageExample(apiURL)}}}
				if out.Key != "" {
					res.Secrets = map[string]string{"api_key": out.Key}
				}
				return res, nil
			},
		},
		{
			Name: "api_keys.revoke", Module: "api_keys", Kind: capability.Destructive, Permission: rbac.PermTenantAPIKeysManage, Routes: routes,
			Title:       map[string]string{"tr": "API anahtarını iptal et", "en": "Revoke an API key"},
			Description: "Revoke an API key (key_uuid from api_keys_list) at once; every integration using it stops working.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"key_uuid"},
				"properties": map[string]any{"key_uuid": map[string]any{"type": "string", "title": "Key", "pattern": "^[0-9a-fA-F-]{36}$"}}},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := text(p, "key_uuid")
				if _, err := uuid.Parse(id); err != nil {
					return capability.Result{}, capability.Invalid("key_uuid must be a UUID")
				}
				r, err := e.API.Call(ctx, http.MethodDelete, keysPath+"/"+id, nil)
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: id, Route: "/settings/integrations"}, r.Err()
			},
		},
	}
}
