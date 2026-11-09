package widget

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Web widget set-up from the assistant (CF-189): create a widget, change
// its colour, greeting and allowed domains (draft or publish) and give the
// install code. The widget id of the snippet is the channel instance uuid.

func say(e capability.Exec, tr, en string, a ...any) string {
	if e.En() {
		return fmt.Sprintf(en, a...)
	}
	return fmt.Sprintf(tr, a...)
}

func text(p capability.Params, k string) string { s, _ := p[k].(string); return strings.TrimSpace(s) }

// Snippet is the embed code (frontend/src/features/widget/lib/settings.ts).
func Snippet(origin, id string) string {
	return `<script src="` + strings.TrimSuffix(origin, "/") + `/widget.js" data-widget="` + id + `" async></script>`
}

func installDetails(e capability.Exec, origin, id string) []capability.Detail {
	return []capability.Detail{
		{Label: say(e, "Kurulum kodu (sitenizin </body> etiketinden önce)", "Install code (before your site's </body>)"), Code: Snippet(origin, id)},
		{Label: say(e, "Widget kimliği", "Widget id"), Value: id},
	}
}

var widgetID = map[string]any{"type": "string", "title": "Widget", "pattern": "^[0-9a-fA-F-]{36}$"}

func appearance() map[string]any {
	return map[string]any{
		"color":          map[string]any{"type": "string", "title": "Colour", "pattern": "^#[0-9a-fA-F]{6}$"},
		"greeting_title": map[string]any{"type": "string", "title": "Greeting title", "maxLength": 80},
		"greeting_text":  map[string]any{"type": "string", "title": "Greeting text", "maxLength": 500},
		// An empty list would allow every site: at least one domain (removing
		// the restriction stays a panel decision).
		"allowed_domains": map[string]any{"type": "array", "title": "Allowed domains", "minItems": 1, "maxItems": 50,
			"items": map[string]any{"type": "string", "minLength": 1, "maxLength": 255}},
	}
}

func validDomains(p capability.Params) error {
	items, _ := p["allowed_domains"].([]any)
	for _, it := range items {
		d, _ := it.(string)
		d = strings.TrimSpace(d)
		if u, err := url.Parse("https://" + d); d == "" || err != nil || u.Host != d || strings.ContainsAny(d, "/:@ ") {
			return capability.Invalid("%q is not a domain (write example.com, without https:// or a path)", d)
		}
	}
	return nil
}

// Capabilities are the widget capabilities; appOrigin is the panel's
// public URL that serves /widget.js.
func Capabilities(appOrigin string) []*capability.Capability {
	routes := []string{"/widget"}
	return []*capability.Capability{
		{
			Name: "widget.install_code", Module: "widget", Kind: capability.Read, Permission: rbac.PermTenantChannelsRead, Routes: routes,
			Surfaces:    capability.ReadSurfaces,
			Title:       map[string]string{"tr": "Widget kurulum kodu", "en": "Widget install code"},
			Description: "The embed code of a web widget (widget_uuid = a channel of type widget from channels_list). Paste it to the user as a code block.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"widget_uuid"},
				"properties": map[string]any{"widget_uuid": widgetID}},
			Run: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := text(p, "widget_uuid")
				if _, err := uuid.Parse(id); err != nil {
					return capability.Result{}, capability.Invalid("widget_uuid must be a UUID")
				}
				r, err := e.API.Call(ctx, http.MethodGet, "/v1/tenant/widgets/"+id+"/config", nil)
				if err != nil {
					return capability.Result{}, err
				}
				if err := r.Err(); err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: id, Data: map[string]any{"snippet": Snippet(appOrigin, id)}, Details: installDetails(e, appOrigin, id)}, nil
			},
		},
		{
			Name: "widget.create", Module: "widget", Kind: capability.Write, Permission: rbac.PermTenantChannelsManage, Routes: routes,
			Title:       map[string]string{"tr": "Web widget'ı oluştur", "en": "Create the web widget"},
			Description: "Create a web chat widget for the organization's site and give its install code. Ask for the site name if unclear.",
			Undo:        "/widget",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name"},
				"properties": map[string]any{"name": map[string]any{"type": "string", "title": "Name", "minLength": 1, "maxLength": 80}}},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				r, err := e.API.Call(ctx, http.MethodPost, "/v1/tenant/channels", map[string]any{"type": "widget", "name": text(p, "name")})
				if err != nil {
					return capability.Result{}, err
				}
				var v struct {
					UUID uuid.UUID `json:"uuid"`
				}
				if err := r.Decode(&v); err != nil {
					return capability.Result{}, err
				}
				id := v.UUID.String()
				return capability.Result{Summary: id, Details: installDetails(e, appOrigin, id), Route: "/widget", Focus: &capability.Focus{Route: "/widget"}}, nil
			},
		},
		{
			Name: "widget.update", Module: "widget", Kind: capability.Write, Permission: rbac.PermTenantChannelsManage, Routes: routes,
			Title: map[string]string{"tr": "Widget görünümünü güncelle", "en": "Update the widget"},
			Description: "Change a widget's colour (#RRGGBB), greeting and allowed domains. publish=true makes it live at once (needs the widget " +
				"publish permission); otherwise it is saved as a draft to review in the editor.",
			Input: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"widget_uuid"},
				"properties": func() map[string]any {
					props := appearance()
					props["widget_uuid"] = widgetID
					props["publish"] = map[string]any{"type": "boolean", "title": "Publish now"}
					return props
				}()},
			Validate: func(p capability.Params) error {
				if text(p, "color") == "" && text(p, "greeting_title") == "" && text(p, "greeting_text") == "" && p["allowed_domains"] == nil {
					return capability.Invalid("give at least one change: ask the user what to change")
				}
				return validDomains(p)
			},
			DryRun: func(_ context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				details := []capability.Detail{}
				for _, k := range []string{"color", "greeting_title", "greeting_text"} {
					if v := text(p, k); v != "" {
						details = append(details, capability.Detail{Label: k, Value: v})
					}
				}
				if d, ok := p["allowed_domains"].([]any); ok {
					details = append(details, capability.Detail{Label: "allowed_domains", Value: fmt.Sprint(d...)})
				}
				state := say(e, "taslak olarak kaydedilecek", "saved as a draft")
				if pub, _ := p["publish"].(bool); pub {
					state = say(e, "hemen yayınlanacak", "published at once")
				}
				return capability.Result{Summary: say(e, "Widget %s.", "The widget will be %s.", state), Details: details}, nil
			},
			Apply: func(ctx context.Context, e capability.Exec, p capability.Params) (capability.Result, error) {
				id := text(p, "widget_uuid")
				if _, err := uuid.Parse(id); err != nil {
					return capability.Result{}, capability.Invalid("widget_uuid must be a UUID")
				}
				r, err := e.API.Call(ctx, http.MethodGet, "/v1/tenant/widgets/"+id+"/config", nil)
				if err != nil {
					return capability.Result{}, err
				}
				var cfg struct {
					Version   int            `json:"version"`
					Published map[string]any `json:"published"`
					Draft     *struct {
						Settings    map[string]any `json:"settings"`
						BaseVersion int            `json:"base_version"`
					} `json:"draft"`
					Defaults map[string]any `json:"defaults"`
				}
				if err := r.Decode(&cfg); err != nil {
					return capability.Result{}, err
				}
				settings := map[string]any{}
				for _, src := range []map[string]any{cfg.Defaults, cfg.Published} {
					for k, v := range src {
						settings[k] = v
					}
				}
				if cfg.Draft != nil {
					for k, v := range cfg.Draft.Settings {
						settings[k] = v
					}
				}
				for _, k := range []string{"color", "greeting_title", "greeting_text"} {
					if v := text(p, k); v != "" {
						settings[k] = v
					}
				}
				if d, ok := p["allowed_domains"]; ok {
					settings["allowed_domains"] = d
				}
				path, body := "/v1/tenant/widgets/"+id+"/draft", map[string]any{"settings": settings, "base_version": cfg.Version}
				method := http.MethodPut
				if pub, _ := p["publish"].(bool); pub {
					path, method = "/v1/tenant/widgets/"+id+"/publish", http.MethodPost
					body["note"] = say(e, "Asistan ile güncellendi", "Updated with the assistant")
				}
				r, err = e.API.Call(ctx, method, path, body)
				if err != nil {
					return capability.Result{}, err
				}
				return capability.Result{Summary: id, Route: "/widget", Focus: &capability.Focus{Route: "/widget"}}, r.Err()
			},
		},
	}
}
