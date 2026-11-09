package assistant

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

func TestScriptedProviderPlaysScenarioOne(t *testing.T) {
	p := ScriptedProvider{}
	tools := []ai.Tool{{Name: "channels_list"}, {Name: "channels_api_create"}}
	req := ai.Request{Tools: tools, Turns: []ai.Turn{{Role: "user", Text: "<conversation>\"ctx\"</conversation>"},
		{Role: "user", Text: "REST API kanalını bağla: https://shop.example.com/clickfix"}}}
	r, _ := p.Generate(context.Background(), req)
	if len(r.Calls) != 1 || r.Calls[0].Name != "channels_list" {
		t.Fatalf("%+v", r)
	}
	req.Turns = append(req.Turns, ai.Turn{Role: "assistant", Calls: r.Calls}, ai.Turn{Role: "tool", Text: `{"data":{"items":[],"total":0}}`})
	r, _ = p.Generate(context.Background(), req)
	if len(r.Calls) != 1 || r.Calls[0].Name != "channels_api_create" || !strings.Contains(string(r.Calls[0].Input), "shop.example.com/clickfix") {
		t.Fatalf("%+v", r)
	}
	req.Turns = append(req.Turns, ai.Turn{Role: "assistant", Calls: r.Calls}, ai.Turn{Role: "tool", Text: `{"status":"proposed"}`})
	req.Schema = map[string]any{"type": "object"}
	if r, _ = p.Generate(context.Background(), req); len(r.Calls) != 0 || !strings.HasPrefix(r.Text, `{"reply":`) {
		t.Fatalf("%+v", r)
	}
}

// Only the channel of the same address is updated; another channel is left alone.
func TestScriptedProviderTouchesOnlyItsOwnChannel(t *testing.T) {
	list := `{"data":{"items":[{"uuid":"11111111-1111-1111-1111-111111111111","outbound_url":"https://demo.example.com/x"},` +
		`{"uuid":"22222222-2222-2222-2222-222222222222","outbound_url":"https://shop-abc.example.com/clickfix"}]}}`
	if id := channelWithURL(list, "https://shop-abc.example.com/clickfix"); id != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("own channel %q", id)
	}
	if id := channelWithURL(list, "https://shop-new.example.com/clickfix"); id != "" {
		t.Fatalf("foreign channel %q", id)
	}
	if got := hostLabel("https://shop-abc.example.com/clickfix"); got != "shop-abc" {
		t.Fatalf("label %q", got)
	}
}

// A tool that is not offered (no permission) is never called.
func TestScriptedProviderOnlyCallsOfferedTools(t *testing.T) {
	req := ai.Request{Turns: []ai.Turn{{Role: "user", Text: "REST API kanalını bağla"}}}
	r, _ := ScriptedProvider{}.Generate(context.Background(), req)
	if len(r.Calls) != 0 || !strings.Contains(r.Text, "yetkiniz") {
		t.Fatalf("%+v", r)
	}
}
