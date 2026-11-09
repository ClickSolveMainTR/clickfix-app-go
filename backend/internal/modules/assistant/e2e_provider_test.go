package assistant

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

func TestScriptedProviderPlaysScenarioOne(t *testing.T) {
	p := ScriptedProvider{}
	req := ai.Request{Turns: []ai.Turn{{Role: "user", Text: "<conversation>\"ctx\"</conversation>"},
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
