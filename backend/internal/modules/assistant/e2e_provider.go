package assistant

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

// ScriptedProvider plays the assistant scenarios of the end-to-end suite
// (CF-194) without a real AI provider: it reads the user's last message
// and the tool results so far and answers with the next tool call or a
// final text. It is only wired when ASSISTANT_E2E_FAKE_AI=1 in a
// development or test environment (see httpserver); never in production.
type ScriptedProvider struct{}

// E2ERoutes are the assistant routes served by the scripted provider.
func E2ERoutes() map[string]ai.Route {
	r := ai.Route{Provider: ScriptedProvider{}, ProviderName: "e2e", Model: "scripted"}
	return map[string]ai.Route{"triage": r, "reply": r, "complex": r}
}

var (
	urlRxp    = regexp.MustCompile(`https://[^\s"'<>]+`)
	markerRxp = regexp.MustCompile(`e2e-[a-z0-9]+`)
)

// channelWithURL is the uuid of the listed channel posting to url.
func channelWithURL(result, url string) string {
	var found string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if x["outbound_url"] == url {
				if id, ok := x["uuid"].(string); ok && found == "" {
					found = id
				}
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	var v any
	if json.Unmarshal([]byte(result), &v) == nil {
		walk(v)
	}
	return found
}

// hostLabel is the first label of a URL's host ("shop-abc").
func hostLabel(raw string) string {
	host := strings.TrimPrefix(raw, "https://")
	host, _, _ = strings.Cut(host, "/")
	label, _, _ := strings.Cut(host, ".")
	return label
}

// turnState is the user's message and the tool results of this reply.
func turnState(req ai.Request) (string, []string) {
	user, results := "", []string{}
	for _, t := range req.Turns {
		switch t.Role {
		case "user":
			if !strings.HasPrefix(t.Text, "<conversation>") {
				user, results = strings.ToLower(t.Text), []string{}
			}
		case "tool":
			results = append(results, t.Text)
		}
	}
	return user, results
}

func firstUUID(result, key string) string {
	m := regexp.MustCompile(`"` + key + `":"([0-9a-f-]{36})"`).FindStringSubmatch(result)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func call(name string, input map[string]any) ai.Result {
	raw, _ := json.Marshal(input)
	return ai.Result{Calls: []ai.ToolCall{{ID: "e2e_" + name, Name: name, Input: raw}}}
}

func (ScriptedProvider) next(req ai.Request) ai.Result {
	user, results := turnState(req)
	step := len(results)
	switch {
	case strings.Contains(user, "test et"):
		switch step {
		case 0:
			return call("channels_list", map[string]any{"type": "api"})
		case 1:
			if id := firstUUID(results[0], "uuid"); id != "" {
				return call("channels_test", map[string]any{"summary": "Kanal test edilir", "params": map[string]any{"channel_uuid": id}})
			}
		}
		return ai.Result{Text: "Testi onayınıza sundum."}
	case strings.Contains(user, "rest api"):
		url := urlRxp.FindString(user)
		if url == "" {
			url = "https://shop.example.com/clickfix"
		}
		switch step {
		case 0:
			return call("channels_list", map[string]any{"type": "api"})
		case 1:
			// Only the channel of this address is updated; any other
			// address gets its own channel (named after its host).
			if id := channelWithURL(results[0], url); id != "" {
				return call("channels_api_update", map[string]any{"summary": "Mevcut REST API kanalının adresi güncellenir",
					"params": map[string]any{"channel_uuid": id, "webhook_url": url}})
			}
			return call("channels_api_create", map[string]any{"summary": "Sipariş sistemi için REST API kanalı kurulur",
				"params": map[string]any{"name": "Sipariş sistemi " + hostLabel(url), "webhook_url": url}})
		}
		return ai.Result{Text: "REST API kanalını onayınıza sundum. Onaylayınca imza anahtarı bir kez gösterilecek."}
	case strings.Contains(user, "iş akışı"):
		if step == 0 {
			name := "İade talepleri"
			if m := markerRxp.FindString(user); m != "" {
				name += " (" + m + ")"
			}
			return call("workflows_create", map[string]any{"summary": "İade içeren yeni mesajlarda öncelik yükselir",
				"params": map[string]any{"name": name, "trigger": "message.received", "enable": false,
					"condition": map[string]any{"all": []any{map[string]any{"field": "subject", "operator": "contains", "value": "iade"}}},
					"actions":   []any{map[string]any{"type": "set_priority", "value": "high"}}}})
		}
		return ai.Result{Text: "İş akışını taslak olarak hazırladım; kuru çalıştırıp onaylayabilirsiniz."}
	case strings.Contains(user, "nerede"):
		if step == 0 {
			return call("navigation_open_route", map[string]any{"summary": "Çalışma saatleri sekmesi",
				"params": map[string]any{"route": "/settings/sla", "target": "sla-business-hours"}})
		}
		return ai.Result{Text: "Sizi SLA sayfasındaki çalışma saatleri sekmesine götürebilirim."}
	}
	return ai.Result{Text: "Size nasıl yardımcı olabilirim?"}
}

// Generate implements ai.Provider (structured replies are JSON).
func (p ScriptedProvider) Generate(_ context.Context, req ai.Request) (ai.Result, error) {
	r := p.next(req)
	// Like a real model, only offered tools are called: a member without
	// the permission is told so instead.
	for _, c := range r.Calls {
		if !slices.ContainsFunc(req.Tools, func(t ai.Tool) bool { return t.Name == c.Name }) {
			r = ai.Result{Text: "Bu işlem için yetkiniz görünmüyor; organizasyon yöneticinizden isteyebilirsiniz."}
			break
		}
	}
	if len(r.Calls) == 0 && req.Schema != nil {
		raw, _ := json.Marshal(map[string]any{"reply": r.Text, "suggestions": []string{}})
		r.Text = string(raw)
	}
	if r.Calls == nil {
		r.Calls = []ai.ToolCall{}
	}
	r.FinalUsage, r.InputTokens, r.OutputTokens, r.Provider, r.Model = true, 1, 1, "e2e", "scripted"
	return r, nil
}

// Stream implements ai.Provider.
func (p ScriptedProvider) Stream(ctx context.Context, req ai.Request, emit ai.Delta) (ai.Result, error) {
	r, err := p.Generate(ctx, req)
	if err == nil && len(r.Calls) == 0 && r.Text != "" && emit != nil {
		err = emit(r.Text)
	}
	return r, err
}
