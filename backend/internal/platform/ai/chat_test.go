package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// finishProvider calls a tool on every turn until it is forbidden to, then answers.
type finishProvider struct{ seen []Request }

func (p *finishProvider) Generate(_ context.Context, req Request) (Result, error) {
	p.seen = append(p.seen, req)
	r := Result{InputTokens: 1, OutputTokens: 1, FinalUsage: true, Calls: []ToolCall{}}
	if req.ToolChoice == "none" {
		r.Text = "answer from the results so far"
		return r, nil
	}
	r.Calls = []ToolCall{{ID: "a", Name: "lookup", Input: json.RawMessage(`{}`)}, {ID: "b", Name: "lookup", Input: json.RawMessage(`{}`)}}
	return r, nil
}
func (p *finishProvider) Stream(ctx context.Context, r Request, _ Delta) (Result, error) {
	return p.Generate(ctx, r)
}

// Several calls in one turn are one round; after the round limit the last turn
// forbids tools and the loop returns its text instead of ErrTooManyRounds.
func TestChatParallelCallsAndFinalTurnWithoutTools(t *testing.T) {
	p := &finishProvider{}
	ran := 0
	out, err := Chat(context.Background(), p, Request{Turns: []Turn{{Role: "user", Text: "x"}}, Tools: []Tool{{Name: "lookup", Schema: map[string]any{"type": "object"}}}}, 3,
		func(context.Context, ToolCall) (ToolResult, error) { ran++; return ToolResult{Content: "ok"}, nil })
	if err != nil || out.Text != "answer from the results so far" || ran != 6 || len(out.Calls) != 6 {
		t.Fatalf("err=%v text=%q ran=%d calls=%d", err, out.Text, ran, len(out.Calls))
	}
	if len(p.seen) != 4 || p.seen[3].ToolChoice != "none" || p.seen[2].ToolChoice != "" {
		t.Fatalf("final turn not marked: %d", len(p.seen))
	}
	if last := p.seen[3].Turns[len(p.seen[3].Turns)-1]; last.Role != "user" || last.Text != finishPrompt {
		t.Fatalf("finish prompt missing: %+v", last)
	}
	if out.InputTokens != 4 {
		t.Fatalf("usage of every turn must be kept: %d", out.InputTokens)
	}
}

// truncatingProvider stops on max_tokens once, then finishes the sentence.
type truncatingProvider struct{ count int }

func (p *truncatingProvider) Generate(_ context.Context, req Request) (Result, error) {
	p.count++
	if p.count == 1 {
		return Result{InputTokens: 1, OutputTokens: 1, FinalUsage: true, Text: "first half", StopReason: "max_tokens", Calls: []ToolCall{}}, ErrTruncated
	}
	if last := req.Turns[len(req.Turns)-1]; last.Text != continuePrompt {
		return Result{}, errors.New("no continuation prompt")
	}
	return Result{InputTokens: 1, OutputTokens: 1, FinalUsage: true, Text: " second half", StopReason: "end_turn", Calls: []ToolCall{}}, nil
}
func (p *truncatingProvider) Stream(ctx context.Context, r Request, _ Delta) (Result, error) {
	return p.Generate(ctx, r)
}

func TestChatContinuesAfterMaxTokens(t *testing.T) {
	out, err := Chat(context.Background(), &truncatingProvider{}, Request{Turns: []Turn{{Role: "user", Text: "x"}}, Tools: []Tool{{Name: "lookup"}}}, 2, nil)
	if err != nil || out.Text != "first half second half" || out.Truncated {
		t.Fatalf("err=%v text=%q truncated=%v", err, out.Text, out.Truncated)
	}
	// An emit/size guard ErrTruncated carries no stop reason and is not continued.
	guard := &fakeProvider{err: ErrTruncated, result: Result{Text: "x"}}
	if _, err = Chat(context.Background(), guard, Request{Turns: []Turn{{Role: "user", Text: "x"}}, Tools: []Tool{{Name: "lookup"}}}, 2, nil); !errors.Is(err, ErrTruncated) {
		t.Fatalf("guard truncation continued: %v", err)
	}
}

// tool_choice none is sent on both wire formats, only while tools are defined.
func TestToolChoiceNoneOnTheWire(t *testing.T) {
	req := Request{Model: "m", Turns: []Turn{{Role: "user", Text: "x"}}, Tools: []Tool{{Name: "lookup", Schema: map[string]any{"type": "object"}}}, ToolChoice: "none"}
	_, body := (&HTTPProvider{Kind: ProviderAnthropic}).body(req, false)
	if choice, _ := body["tool_choice"].(map[string]any); choice["type"] != "none" {
		t.Fatalf("anthropic tool_choice: %v", body["tool_choice"])
	}
	_, body = (&HTTPProvider{Kind: ProviderOpenAI}).body(req, false)
	if body["tool_choice"] != "none" {
		t.Fatalf("openai tool_choice: %v", body["tool_choice"])
	}
	req.Tools = nil
	_, body = (&HTTPProvider{Kind: ProviderAnthropic}).body(req, false)
	if _, ok := body["tool_choice"]; ok {
		t.Fatal("tool_choice without tools")
	}
}
