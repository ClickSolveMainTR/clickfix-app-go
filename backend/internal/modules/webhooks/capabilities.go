package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
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
	return page.Items, r.Decode(&page)
}

func recentDeliveries(ctx context.Context, api capability.API) ([]delivery, error) {
	r, err := api.Call(ctx, http.MethodGet, "/v1/tenant/webhook-deliveries?limit=20", nil)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []delivery `json:"items"`
	}
	return page.Items, r.Decode(&page)
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
