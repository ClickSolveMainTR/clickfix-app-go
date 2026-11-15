package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// CF-213: keys and base URLs come only from the composition root. The SDKs'
// environment defaults (ANTHROPIC_AUTH_TOKEN, OPENAI_ORG_ID, ...) must not
// reach a provider request.
func TestSDKIgnoresEnvironmentCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "env-token")
	t.Setenv("ANTHROPIC_BASE_URL", "http://env.invalid")
	t.Setenv("OPENAI_ORG_ID", "env-org")
	t.Setenv("OPENAI_PROJECT_ID", "env-project")
	t.Setenv("OPENAI_BASE_URL", "http://env.invalid")
	var seen atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		if r.Header.Get("Authorization") != "" && r.Header.Get("Authorization") != "Bearer fake" {
			t.Errorf("environment auth leaked: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("OpenAI-Organization") != "" || r.Header.Get("OpenAI-Project") != "" {
			t.Error("environment organization leaked")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/messages") {
			_, _ = fmt.Fprint(w, `{"model":"m","stop_reason":"end_turn","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"model":"m","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer server.Close()
	a := NewAnthropic("fake")
	a.BaseURL = server.URL
	o := NewOpenAI("fake", server.URL, "")
	for _, p := range []*HTTPProvider{a, o} {
		if _, err := p.Generate(context.Background(), Request{Model: "m", Turns: []Turn{{Role: "user", Text: "x"}}}); err != nil {
			t.Fatal(p.Kind, err)
		}
	}
	if seen.Load() != 2 {
		t.Fatalf("requests=%d (base URL from environment?)", seen.Load())
	}
}

// The request the SDK sends carries the Claude features the layer relies on.
func TestAnthropicSDKWire(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = fmt.Fprint(w, `{"model":"claude-sonnet-5-5","stop_reason":"end_turn","stop_details":null,"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":4,"output_tokens":1}}`)
	}))
	defer server.Close()
	p := NewAnthropic("fake")
	p.BaseURL = server.URL
	tools := []Tool{{Name: "a", Schema: map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string", "minLength": 1}}, "additionalProperties": false}}, {Name: "b", Schema: map[string]any{"type": "object"}}}
	req := Request{Model: "claude-sonnet-5-5", Effort: "low", Instructions: "trusted", CachePrefix: true, CacheTools: 1, Tools: tools, ToolChoice: "none",
		Turns: []Turn{{Role: "user", Text: "q"}, {Role: "assistant", Calls: []ToolCall{{ID: "t1", Name: "a", Input: json.RawMessage(`{"q":"x"}`)}}}, {Role: "tool", ToolCallID: "t1", Text: "result"}}}
	if _, err := p.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	for _, want := range []string{`"effort":"low"`, `"tool_choice":{"type":"none"}`, `"tool_use_id":"t1"`, `"minLength":1`, `"additionalProperties":false`, `"max_tokens":2048`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
	if !strings.Contains(fmt.Sprint(body["system"]), "ephemeral") {
		t.Error("system prompt not cached")
	}
	wire := body["tools"].([]any)
	if wire[0].(map[string]any)["cache_control"] == nil || wire[1].(map[string]any)["cache_control"] != nil {
		t.Errorf("tool breakpoint: %v", wire)
	}
	if body["thinking"] != nil {
		t.Error("thinking must stay off while tools are offered")
	}
	req.Tools, req.ToolChoice, req.Turns = nil, "", req.Turns[:1]
	if _, err := p.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(body["thinking"]) != "map[type:between_tools]" {
		t.Errorf("thinking=%v", body["thinking"])
	}
}

// A transport failure is never replayed (the request may be billable), an
// oversized body is invalid output, and a real Messages stream (with event
// lines) parses the same as the data-only fixtures.
func TestSDKTransportLimitsAndEventStream(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	p := NewOpenAI("fake", server.URL, "")
	_, err := p.Generate(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "x"}}})
	server.Close()
	if !errors.Is(err, ErrUnavailable) || calls.Load() != 1 || NotAccepted(err) {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}

	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"model":"m","choices":[{"message":{"content":"`+strings.Repeat("a", 5<<20)+`"},"finish_reason":"stop"}]}`)
	}))
	defer big.Close()
	if _, err = NewOpenAI("fake", big.URL, "").Generate(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "x"}}}); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("oversized body: %v", err)
	}

	stream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-5-5\",\"usage\":{\"input_tokens\":5,\"cache_read_input_tokens\":3}}}\n\n"+
			"event: ping\ndata: {\"type\":\"ping\"}\n\n"+
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"merhaba\"}}\n\n"+
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n"+
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer stream.Close()
	a := NewAnthropic("fake")
	a.BaseURL = stream.URL
	out, err := a.Stream(context.Background(), Request{Model: "claude-sonnet-5-5", Turns: []Turn{{Role: "user", Text: "x"}}}, nil)
	if err != nil || out.Text != "merhaba" || out.InputTokens != 8 || out.CacheReadTokens != 3 || out.OutputTokens != 2 || !out.FinalUsage {
		t.Fatalf("%+v %v", out, err)
	}
}
