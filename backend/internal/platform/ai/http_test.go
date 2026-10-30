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
