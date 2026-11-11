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

const (
	// MaxToolRounds bounds the model turns that may call tools in one reply.
	// A turn may carry any number of parallel calls; they run in order.
	MaxToolRounds = 6
	// maxCallsPerTurn bounds the parallel calls of one turn (cost and latency).
	maxCallsPerTurn = 8
	// maxContinuations bounds the automatic "continue" turns after max_tokens.
	maxContinuations = 2
	// maxTurnsBytes bounds the transcript the loop keeps sending back.
	maxTurnsBytes = 64000
)

// finishPrompt is the last turn when the tool rounds are used up: the model
// may not call tools (ToolChoice "none") and answers with what it has.
const finishPrompt = "You have used all tool rounds. Do not call any tool now. Answer the user in their language with the results gathered so far; say briefly what you could not look up."

// continuePrompt follows an assistant turn cut off by max_tokens.
const continuePrompt = "Your previous message was cut off by the length limit. Continue exactly where it stopped, without repeating anything already written and without an introduction."

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

// truncatedByLimit reports a provider stop on the output limit; an emit error
// or a size guard also surfaces as ErrTruncated but carries no stop reason.
func truncatedByLimit(next Result, err error) bool {
	return errors.Is(err, ErrTruncated) && errors.Is(stopError(next.StopReason), ErrTruncated)
}

// chat runs up to rounds tool turns. Every turn may call several tools; when the
// rounds are used up the model gets one last turn without tools instead of an
// error. A turn cut off by max_tokens is continued (at most maxContinuations
// times) inside the same reply; a reply still cut off after that is returned
// with Truncated set rather than failed.
func chat(ctx context.Context, req Request, rounds int, run ToolRunner, generate func(context.Context, Request) (Result, error)) (Result, error) {
	if rounds < 1 || rounds > MaxToolRounds {
		rounds = MaxToolRounds
	}
	out := Result{Calls: []ToolCall{}}
	// prose already produced by turns that were cut off and continued
	var prefix string
	toolRounds, continuations := 0, 0
	final := false
	for {
		raw, _ := json.Marshal(req.Turns)
		if !final && (toolRounds >= rounds || len(raw) > maxTurnsBytes) {
			final = true
			req.ToolChoice = "none"
			req.Turns = append(req.Turns, Turn{Role: "user", Text: finishPrompt})
		}
		next, err := generate(ctx, req)
		if next.InputTokens < 0 || next.OutputTokens < 0 {
			out.FinalUsage = false
			return out, ErrInvalidOutput
		}
		out.FinalUsage = next.FinalUsage
		out.Provider = next.Provider
		out.Model = next.Model
		out.Text = prefix + next.Text
		out.CacheReadTokens += next.CacheReadTokens
		out.CacheCreateTokens += next.CacheCreateTokens
		out.StopReason = next.StopReason
		out.StopDetails = next.StopDetails
		out.InputTokens += next.InputTokens
		out.OutputTokens += next.OutputTokens
		if truncatedByLimit(next, err) {
			if continuations >= maxContinuations {
				out.Truncated = true
				return out, nil
			}
			continuations++
			prefix += next.Text
			req.Turns = append(req.Turns, Turn{Role: "assistant", Text: next.Text}, Turn{Role: "user", Text: continuePrompt})
			continue
		}
		if err != nil {
			return out, err
		}
		if len(next.Calls) == 0 {
			return out, nil
		}
		if final || len(next.Calls) > maxCallsPerTurn {
			return out, ErrTooManyRounds
		}
		toolRounds++
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
			finished = finished || (offered.EndsTurn && !result.IsError)
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
}
