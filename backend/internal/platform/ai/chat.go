package ai

import (
	"context"
	"encoding/json"
	"errors"
)

type ToolResult struct {
	Content string
	IsError bool
}
type ToolRunner func(context.Context, ToolCall) (ToolResult, error)

// Chat ports takwim's bounded tool loop. Authorization is rechecked by run for each call;
// high-risk approval must be supplied by the application, never by model arguments.
func Chat(ctx context.Context, p Provider, req Request, rounds int, run ToolRunner) (Result, error) {
	return chat(ctx, req, rounds, run, p.Generate)
}

// ChatStream uses the same bounded, schema-validated tool loop as Chat.
func ChatStream(ctx context.Context, p Provider, req Request, rounds int, run ToolRunner, emit Delta) (Result, error) {
	var text string
	out, err := chat(ctx, req, rounds, run, func(ctx context.Context, req Request) (Result, error) {
		return p.Stream(ctx, req, func(delta string) error {
			if err := emit(delta); err != nil {
				return err
			}
			text += delta
			return nil
		})
	})
	out.Text = text
	return out, err
}

func chat(ctx context.Context, req Request, rounds int, run ToolRunner, generate func(context.Context, Request) (Result, error)) (Result, error) {
	if rounds < 1 || rounds > 6 {
		rounds = 6
	}
	out := Result{Calls: []ToolCall{}}
	for range rounds {
		raw, _ := json.Marshal(req.Turns)
		if len(raw) > 64000 || len(out.Calls) > 6 {
			return out, ErrTooManyRounds
		}
		next, err := generate(ctx, req)
		if next.InputTokens < 0 || next.OutputTokens < 0 {
			out.FinalUsage = false
			return out, ErrInvalidOutput
		}
		out.FinalUsage = next.FinalUsage
		out.Provider = next.Provider
		out.Model = next.Model
		out.Text = next.Text
		out.CacheReadTokens += next.CacheReadTokens
		out.CacheCreateTokens += next.CacheCreateTokens
		out.StopReason = next.StopReason
		out.StopDetails = next.StopDetails
		out.InputTokens += next.InputTokens
		out.OutputTokens += next.OutputTokens
		if err != nil {
			return out, err
		}
		if len(next.Calls) > 2 {
			return out, ErrTooManyRounds
		}
		if len(next.Calls) == 0 {
			return out, nil
		}
		req.Turns = append(req.Turns, Turn{Role: "assistant", Text: next.Text, Calls: next.Calls})
		finished := false
		for _, call := range next.Calls {
			var offered *Tool
			for i := range req.Tools {
				if req.Tools[i].Name == call.Name {
					offered = &req.Tools[i]
					break
				}
			}
			if offered == nil || offered.RequiresApproval || run == nil {
				return out, errors.New("ai: tool denied")
			}
			schema := offered.Schema
			if len(schema) == 0 {
				schema = map[string]any{"type": "object"}
			}
			if err := validateJSON(schema, call.Input); err != nil {
				return out, err
			}
			result, err := run(ctx, call)
			if err != nil {
				return out, err
			}
			finished = finished || offered.EndsTurn
			out.Calls = append(out.Calls, call)
			if len(result.Content) > 4096 || Guard(result.Content) != nil {
				return out, ErrInjection
			}
			req.Turns = append(req.Turns, Turn{Role: "tool", Text: NewMasker().Mask(result.Content), ToolCallID: call.ID})
		}
		if finished {
			return out, nil
		}
	}
	return out, ErrTooManyRounds
}
