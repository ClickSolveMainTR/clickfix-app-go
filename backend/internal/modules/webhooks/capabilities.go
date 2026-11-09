package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Assistant capabilities and page state of outbound webhooks (CF-187,
// scenario 3: "webhook test delivery"). Creating and editing
// subscriptions comes with CF-190.

const hooksPath = "/v1/tenant/webhooks"

type hookView struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	URL     string    `json:"url"`
	Events  []string  `json:"events"`
	Enabled bool      `json:"enabled"`
}

type delivery struct {
	EventID    uuid.UUID `json:"event_id"`
	EventName  string    `json:"event_name"`
	Status     string    `json:"status"`
	Attempts   int32     `json:"attempts"`
	LastStatus int32     `json:"last_status"`
	LastError  string    `json:"last_error"`
}

func tr(e capability.Exec, trText, en string, a ...any) string {
	if e.En() {
		return fmt.Sprintf(en, a...)
	}
	return fmt.Sprintf(trText, a...)
}

func listHooks(ctx context.Context, api capability.API) ([]hookView, error) {
	r, err := api.Call(ctx, http.MethodGet, hooksPath+"?limit=30", nil)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []hookView `json:"items"`
	}
	decErr := r.Decode(&page)
	return page.Items, decErr
}

func recentDeliveries(ctx context.Context, api capability.API) ([]delivery, error) {
	r, err := api.Call(ctx, http.MethodGet, "/v1/tenant/webhook-deliveries?limit=20", nil)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []delivery `json:"items"`
	}
	decErr := r.Decode(&page)
	return page.Items, decErr
}

// Capabilities are the webhook capabilities.
func Capabilities() []*capability.Capability {
	routes := []string{"/settings/integrations"}
	return []*capability.Capability{
		{
			Name: "webhooks.list", Surfaces: capability.ReadSurfaces, Module: "webhooks", Kind: capability.Read, Permission: rbac.PermTenantWebhooksRead, Routes: routes,
			Title:       map[string]string{"tr": "Webhook aboneliklerini listele", "en": "List webhooks"},
			Description: "List the organization's outbound webhook subscriptions (uuid, name, URL, events, enabled) and the status of the latest deliveries.",
			Run: func(ctx context.Context, e capability.Exec, _ capability.Params) (capability.Result, error) {
				hooks, err := listHooks(ctx, e.API)
				if err != nil {
					return capability.Result{}, err
				}
				deliveries, _ := recentDeliveries(ctx, e.API)
				return capability.Result{Summary: fmt.Sprintf("%d webhooks", len(hooks)), Data: map[string]any{"webhooks": hooks, "recent_deliveries": deliveries}}, nil
			},
		},
		{
			Name: "webhooks.test", Module: "webhooks", Kind: capability.Write, Permission: rbac.PermTenantWebhooksWrite, Routes: routes,
			Title:       map[string]string{"tr": "Webhook test teslimatı", "en": "Webhook test delivery"},
			Description: "Send a signed test event (webhook.test) to a webhook subscription and report the delivery result. Contacts the organization's endpoint, so it is confirmed.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"webhook_uuid"},
				"properties": map[string]any{"webhook_uuid": map[string]any{"type": "string", "title": "Webhook", "pattern": "^[0-9a-fA-F-]{36}$"}}},
			Validate: func(p capability.Params) error {
				id, _ := p["webhook_uuid"].(string)
				if _, err := uuid.Parse(id); err != nil {
					return capability.Invalid("webhook_uuid must be a UUID")
				}
				return nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id, _ := p["webhook_uuid"].(string)
				r, err := e.API.Call(ctx, http.MethodPost, hooksPath+"/"+strings.TrimSpace(id)+"/test", nil)
				if err != nil {
					return capability.Result{}, err
				}
				var out struct {
					EventID uuid.UUID `json:"event_id"`
				}
				if err := r.Decode(&out); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: out.EventID.String(), Route: "/settings/integrations",
					Details: []capability.Detail{{Label: tr(e, "Test olayı", "Test event"), Value: out.EventID.String()}}}, nil
			},
			// The delivery runs in the background worker: look for it a few
			// times and report what is known.
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, res capability.Result) capability.Check {
				for i := range 4 {
					if i > 0 {
						select {
						case <-ctx.Done():
							return capability.Check{Status: capability.VerifySkipped}
						case <-time.After(750 * time.Millisecond):
						}
					}
					items, err := recentDeliveries(ctx, e.API)
					if err != nil {
						return capability.Check{Status: capability.VerifySkipped}
					}
					for _, d := range items {
						if d.EventID.String() != res.Summary {
							continue
						}
						switch d.Status {
						case "succeeded":
							return capability.Check{Status: capability.VerifyOK, Message: tr(e, "Teslim edildi (HTTP %d)", "Delivered (HTTP %d)", d.LastStatus)}
						case "failed":
							return capability.Check{Status: capability.VerifyFailed, Message: d.LastError}
						}
						if d.Attempts > 0 && d.LastError != "" {
							return capability.Check{Status: capability.VerifyFailed, Message: tr(e, "İlk deneme başarısız (%s); yeniden denenecek", "First attempt failed (%s); it will be retried", d.LastError)}
						}
					}
				}
				return capability.Check{Status: capability.VerifySkipped, Message: tr(e, "Kuyrukta; sonucu Entegrasyonlar sayfasındaki teslimat kaydında görünecek.", "Queued; the result will show in the delivery log on the Integrations page.")}
			},
		},
	}
}

// Pages is the webhook part of the integrations page.
func Pages() []*capability.Page {
	return []*capability.Page{{
		Module: "webhooks", Routes: []string{"/settings/integrations"}, Permission: rbac.PermTenantWebhooksRead,
		State: func(ctx context.Context, e capability.Exec) (capability.PageState, error) {
			st := capability.PageState{Module: "webhooks", Title: tr(e, "Entegrasyonlar ve webhook'lar", "Integrations and webhooks")}
			hooks, err := listHooks(ctx, e.API)
			if err != nil {
				return st, err
			}
			byID := map[string]string{}
			for i, h := range hooks {
				byID[h.UUID.String()] = h.Name
				if !h.Enabled {
					st.Warnings = append(st.Warnings, capability.Step{Key: "disabled:" + h.UUID.String(),
						Title: tr(e, "“%s” kapalı, olay gönderilmiyor", "“%s” is disabled; no events are sent", h.Name)})
				}
				if i < 3 {
					st.Tests = append(st.Tests, capability.Test{Capability: "webhooks.test",
						Title:  tr(e, "“%s” için test teslimatı", "Test delivery to “%s”", h.Name),
						Prompt: tr(e, "“%s” webhook'una test olayı gönder", "Send a test event to the “%s” webhook", h.Name)})
				}
			}
			if deliveries, err := recentDeliveries(ctx, e.API); err == nil {
				failed := 0
				for _, d := range deliveries {
					if d.Status == "failed" {
						failed++
					}
				}
				if failed > 0 {
					st.Warnings = append(st.Warnings, capability.Step{Key: "failed_deliveries", Capability: "webhooks.list",
						Title:  tr(e, "Son teslimatlardan %d tanesi başarısız", "%d of the latest deliveries failed", failed),
						Prompt: tr(e, "Başarısız webhook teslimatlarının nedenini açıkla", "Explain why the webhook deliveries failed")})
				}
			}
			return st, nil
		},
	}}
}

// Webhook subscription writes (CF-190). The signing secret is created by
// the route on create and shown once on the card; it is never stored in
// the chat.

func validURL(p capability.Params) error {
	if u, _ := p["url"].(string); u != "" {
		if err := usecase.ValidateURL(strings.TrimSpace(u)); err != nil {
			return capability.Invalid("url must be a public https address (no localhost, private IP or user:password)")
		}
	}
	return nil
}

func hookByID(ctx context.Context, api capability.API, id string) (hookView, error) {
	if _, err := uuid.Parse(id); err != nil {
		return hookView{}, capability.Invalid("webhook_uuid must be a UUID")
	}
	hooks, err := listHooks(ctx, api)
	if err != nil {
		return hookView{}, err
	}
	for _, h := range hooks {
		if h.UUID.String() == id {
			return h, nil
		}
	}
	return hookView{}, capability.Invalid("webhook %s not found", id)
}

func strList(v any) []string {
	items, _ := v.([]any)
	out := []string{}
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// WriteCapabilities are the webhook subscription changes.
func WriteCapabilities() []*capability.Capability {
	routes := []string{"/settings/integrations"}
	events := map[string]any{"type": "array", "title": "Events", "minItems": 1, "maxItems": 50,
		"items": map[string]any{"type": "string", "enum": events.WebhookEvents()}}
	idField := map[string]any{"type": "string", "title": "Webhook", "pattern": "^[0-9a-fA-F-]{36}$"}
	props := map[string]any{
		"name":    map[string]any{"type": "string", "title": "Name", "minLength": 1, "maxLength": 120},
		"url":     map[string]any{"type": "string", "title": "URL", "format": "uri", "maxLength": 2048},
		"events":  events,
		"enabled": map[string]any{"type": "boolean", "title": "Enabled"},
	}
	withID := map[string]any{"webhook_uuid": idField}
	for k, v := range props {
		withID[k] = v
	}
	return []*capability.Capability{
		{
			Name: "webhooks.create", Module: "webhooks", Kind: capability.Write, Permission: rbac.PermTenantWebhooksWrite, Routes: routes,
			Title: map[string]string{"tr": "Webhook aboneliği oluştur", "en": "Create a webhook"},
			Description: "Subscribe the organization's endpoint (public https URL) to events, e.g. conversation.created. The signing secret is " +
				"created by the server and shown to the user once; never ask for it. Offer webhooks_test afterwards.",
			Undo:     "/settings/integrations",
			Input:    map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "url", "events"}, "properties": props},
			Validate: validURL,
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				hooks, err := listHooks(ctx, e.API)
				if err != nil {
					return capability.Result{}, err
				}
				for _, h := range hooks {
					if h.URL == str(p, "url") {
						return capability.Result{}, capability.Invalid("%s", tr(e, "Bu adrese giden bir abonelik zaten var (%s); güncellemeyi öneririm.", "A webhook to this URL already exists (%s); update it instead.", h.Name))
					}
				}
				return capability.Result{Summary: tr(e, "“%s” aboneliği %s adresine %d olay gönderecek; imza anahtarı bir kez gösterilecek.",
					"“%s” will post to %s (%d events); the signing secret is shown once.", str(p, "name"), str(p, "url"), len(strList(p["events"]))),
					Details: []capability.Detail{{Label: tr(e, "Olaylar", "Events"), Value: strings.Join(strList(p["events"]), ", ")}}}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				enabled := true
				if v, ok := p["enabled"].(bool); ok {
					enabled = v
				}
				r, err := e.API.Call(ctx, http.MethodPost, hooksPath, map[string]any{"name": str(p, "name"), "url": str(p, "url"), "events": strList(p["events"]), "enabled": enabled})
				if err != nil {
					return capability.Result{}, err
				}
				var out struct {
					UUID   uuid.UUID `json:"uuid"`
					Secret string    `json:"secret"`
				}
				if err := r.Decode(&out); err != nil {
					return capability.Result{}, err
				}
				res := capability.Result{Summary: out.UUID.String(), Route: "/settings/integrations",
					Details: []capability.Detail{{Label: tr(e, "İmza doğrulama", "Signature check"),
						Value: tr(e, "ClickFix-Webhook-Signature = v1= + hex(HMAC-SHA256(anahtar, zaman damgası + \".\" + gövde)); ClickFix-Webhook-Timestamp ile birlikte gelir.",
							"ClickFix-Webhook-Signature = v1= + hex(HMAC-SHA256(secret, timestamp + \".\" + body)); sent with ClickFix-Webhook-Timestamp.")}}}
				if out.Secret != "" {
					res.Secrets = map[string]string{"signing_secret": out.Secret}
				}
				return res, nil
			},
			Verify: func(ctx context.Context, e capability.Exec, _ capability.Params, res capability.Result) capability.Check {
				if _, err := hookByID(ctx, e.API, res.Summary); err != nil {
					return capability.Check{Status: capability.VerifyFailed}
				}
				return capability.Check{Status: capability.VerifyOK, Message: tr(e, "Abonelik kayıtlı; test teslimatı gönderebilirim.", "Saved; I can send a test delivery.")}
			},
		},
		{
			Name: "webhooks.update", Module: "webhooks", Kind: capability.Write, Permission: rbac.PermTenantWebhooksWrite, Routes: routes,
			Title:       map[string]string{"tr": "Webhook aboneliğini güncelle", "en": "Update a webhook"},
			Description: "Change the name, URL, events or enabled state of a webhook (webhook_uuid from webhooks_list); fields left out stay.",
			Input:       map[string]any{"type": "object", "additionalProperties": false, "required": []string{"webhook_uuid"}, "properties": withID},
			Validate:    validURL,
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				h, err := hookByID(ctx, e.API, str(p, "webhook_uuid"))
				if err != nil {
					return capability.Result{}, err
				}
				body := map[string]any{"name": h.Name, "url": h.URL, "events": h.Events, "enabled": h.Enabled}
				if v := str(p, "name"); v != "" {
					body["name"] = v
				}
				if v := str(p, "url"); v != "" {
					body["url"] = v
				}
				if v := strList(p["events"]); len(v) > 0 {
					body["events"] = v
				}
				if v, ok := p["enabled"].(bool); ok {
					body["enabled"] = v
				}
				r, err := e.API.Call(ctx, http.MethodPut, hooksPath+"/"+h.UUID.String(), body)
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: h.UUID.String(), Route: "/settings/integrations"}, r.Err()
			},
		},
		{
			Name: "webhooks.delete", Module: "webhooks", Kind: capability.Destructive, Permission: rbac.PermTenantWebhooksWrite, Routes: routes,
			Title:       map[string]string{"tr": "Webhook aboneliğini sil", "en": "Delete a webhook"},
			Description: "Delete a webhook subscription for good; its endpoint stops receiving events.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"webhook_uuid"},
				"properties": map[string]any{"webhook_uuid": idField}},
			DryRun: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				h, err := hookByID(ctx, e.API, str(p, "webhook_uuid"))
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: tr(e, "“%s” (%s) kalıcı olarak silinecek.", "“%s” (%s) will be deleted for good.", h.Name, h.URL)}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				h, err := hookByID(ctx, e.API, str(p, "webhook_uuid"))
				if err != nil {
					return capability.Result{}, err
				}
				r, err := e.API.Call(ctx, http.MethodDelete, hooksPath+"/"+h.UUID.String(), nil)
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: h.Name, Route: "/settings/integrations"}, r.Err()
			},
		},
	}
}

func str(p capability.Params, k string) string {
	s, _ := p[k].(string)
	return strings.TrimSpace(s)
}
