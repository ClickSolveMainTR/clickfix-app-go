package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

// cardOnlyProvider answers with a single tool call and no prose, then ends the turn silently.
type cardOnlyProvider struct{ turn int }

func (p *cardOnlyProvider) Generate(ctx context.Context, req ai.Request) (ai.Result, error) {
	return p.Stream(ctx, req, func(string) error { return nil })
}

func (p *cardOnlyProvider) Stream(_ context.Context, _ ai.Request, _ ai.Delta) (ai.Result, error) {
	p.turn++
	if p.turn == 1 {
		return ai.Result{FinalUsage: true, InputTokens: 10, OutputTokens: 5, StopReason: "tool_use",
			Calls: []ai.ToolCall{{ID: "c1", Name: "noop", Input: json.RawMessage(`{}`)}}}, nil
	}
	return ai.Result{FinalUsage: true, InputTokens: 10, OutputTokens: 1, StopReason: "end_turn", Calls: []ai.ToolCall{}}, nil
}

// A streamed turn that only proposes a card has no prose; that is a valid answer, not ErrInvalidOutput.
func TestPlatformAIAcceptsToolOnlyTurn(t *testing.T) {
	p := &PlatformAI{Routes: map[string]ai.Route{"reply": {Provider: &cardOnlyProvider{}, ProviderName: "test", Model: "m"}}}
	req := ai.Request{Turns: []ai.Turn{{Role: "user", Text: "SLA politikası kuralım"}}, Tools: []ai.Tool{{Name: "noop", Schema: map[string]any{"type": "object"}}}}
	runner := func(context.Context, ai.ToolCall) (ai.ToolResult, error) {
		return ai.ToolResult{Content: `{"status":"proposed"}`}, nil
	}
	res, err := p.Run(context.Background(), Call{Route: "reply", Req: req, Rounds: 3, Runner: runner, Emit: func(string) error { return nil }})
	if err != nil {
		t.Fatalf("tool-only turn rejected: %v", err)
	}
	if len(res.Calls) != 1 || res.Text != "" {
		t.Fatalf("unexpected result: calls=%d text=%q", len(res.Calls), res.Text)
	}
}

// A turn with neither prose nor tool calls is still invalid output.
type silentProvider struct{}

func (silentProvider) Generate(context.Context, ai.Request) (ai.Result, error) {
	return ai.Result{FinalUsage: true, InputTokens: 1, OutputTokens: 0, Calls: []ai.ToolCall{}}, nil
}
func (s silentProvider) Stream(ctx context.Context, req ai.Request, _ ai.Delta) (ai.Result, error) {
	return s.Generate(ctx, req)
}

func TestPlatformAIRejectsSilentTurn(t *testing.T) {
	p := &PlatformAI{Routes: map[string]ai.Route{"reply": {Provider: silentProvider{}, ProviderName: "test", Model: "m"}}}
	_, err := p.Run(context.Background(), Call{Route: "reply", Req: ai.Request{Turns: []ai.Turn{{Role: "user", Text: "x"}}}, Emit: func(string) error { return nil }})
	if !errors.Is(err, ai.ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput, got %v", err)
	}
}

func TestCardOnlyText(t *testing.T) {
	if cardOnlyText("tr", 1) == "" || cardOnlyText("en", 2) == "" || cardOnlyText("tr", 1) == cardOnlyText("tr", 2) {
		t.Fatal("card-only lead-in texts must exist and distinguish one card from several")
	}
}
