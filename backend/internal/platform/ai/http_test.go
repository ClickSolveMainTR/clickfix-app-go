package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamRejectedOutputPreservesFinalUsage(t *testing.T) {
	for _, tc := range []struct {
		name, kind, events string
		want               error
	}{
		{name: "anthropic truncated", kind: ProviderAnthropic, want: ErrTruncated, events: `data: {"type":"message_start","message":{"model":"fake","usage":{"input_tokens":9}}}

data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":4}}

data: {"type":"message_stop"}

`},
		{name: "openai refused", kind: ProviderOpenAI, want: ErrRefused, events: `data: {"choices":[{"delta":{"refusal":"declined"},"finish_reason":"stop"}]}

data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":4}}

data: [DONE]

`},
		{name: "openai invalid schema", kind: ProviderOpenAI, want: ErrInvalidOutput, events: `data: {"choices":[{"delta":{"content":"[]"},"finish_reason":"stop"}]}

data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":4}}

data: [DONE]

`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, tc.events)
			}))
			defer server.Close()
			p := NewOpenAI("fake", server.URL, "")
			p.Kind = tc.kind
			out, err := p.Stream(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "data"}}, Schema: map[string]any{"type": "object"}}, nil)
			if !errors.Is(err, tc.want) || !out.FinalUsage || out.InputTokens != 9 || out.OutputTokens != 4 {
				t.Fatalf("usage lost for rejected output: %+v, %v", out, err)
			}
		})
	}
}

// CF-211: the stable leading tools get their own cache breakpoint, so a
// page change (other trailing tools) still reuses them.
func TestAnthropicCachesStableTools(t *testing.T) {
	p := NewAnthropic("fake")
	tools := []Tool{{Name: "a", Schema: map[string]any{"type": "object"}}, {Name: "b", Schema: map[string]any{"type": "object"}}, {Name: "page", Schema: map[string]any{"type": "object"}}}
	_, body := p.body(Request{Instructions: "x", Tools: tools, CachePrefix: true, CacheTools: 2}, false)
	wire := body["tools"].([]any)
	if wire[1].(map[string]any)["cache_control"] == nil || wire[0].(map[string]any)["cache_control"] != nil || wire[2].(map[string]any)["cache_control"] != nil {
		t.Fatalf("breakpoint: %v", wire)
	}
	_, body = p.body(Request{Instructions: "x", Tools: tools, CacheTools: 2}, false)
	if body["tools"].([]any)[1].(map[string]any)["cache_control"] != nil {
		t.Fatal("cache breakpoint without CachePrefix")
	}
}
