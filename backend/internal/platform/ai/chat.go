package ai

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
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

// toolOutcome is what one call hands back to the model.
type toolOutcome struct {
	ToolResult
	// ends: a trusted EndsTurn tool ran without error.
	ends bool
}

// maxToolResultBytes bounds one tool result the model may read.
const maxToolResultBytes = 4096

// Tool failures the model is told about. They never carry the cause: an
// internal error text or a withheld result must not reach the model.
const (
	toolErrUnknown  = "Tool error: this tool is not available. Answer without it."
	toolErrApproval = "Tool error: this action needs a person's approval and was NOT executed. Do not say it was done; hand the request to a human."
	toolErrInput    = "Tool error: the arguments do not match the tool's schema. Nothing was executed."
	toolErrFailed   = "Tool error: the tool failed and returned nothing. Answer without it and do not guess its result."
	toolErrWithheld = "Tool error: the result was withheld (too large or unsafe). Answer without it and do not guess its result."
)

// runTool executes one call. A call that cannot run (unknown or approval-only
// tool, invalid arguments, failed execution, oversized or unsafe result)
// becomes an error result the model reads and recovers from (CF-251): one bad
// call used to fail the whole reply with an empty answer. Only a cancelled
// context ends the loop. executed reports that the application ran the tool.
func runTool(ctx context.Context, tools []Tool, run ToolRunner, call ToolCall) (out toolOutcome, executed bool, err error) {
	fail := func(reason, message string) (toolOutcome, bool, error) {
		slog.WarnContext(ctx, "ai_tool_call_failed", "tool", call.Name, "reason", reason, "run_uuid", ctx.Value(runTraceKey{}))
		return toolOutcome{ToolResult: ToolResult{Content: message, IsError: true}}, false, nil
	}
	var offered *Tool
	for i := range tools {
		if tools[i].Name == call.Name {
			offered = &tools[i]
			break
		}
	}
	if offered == nil || run == nil {
		return fail("unknown_tool", toolErrUnknown)
	}
	// Approval is supplied by the application, never by model arguments.
	if offered.RequiresApproval {
		return fail("approval_required", toolErrApproval)
	}
	schema := offered.Schema
	if len(schema) == 0 {
		schema = map[string]any{"type": "object"}
	}
	if err := validateJSON(schema, call.Input); err != nil {
		return fail("invalid_arguments", toolErrInput)
	}
	result, err := run(ctx, call)
	if ctx.Err() != nil {
		return toolOutcome{}, false, ctx.Err()
	}
	if err != nil {
		return fail("execution_failed", toolErrFailed)
	}
	if len(result.Content) > maxToolResultBytes || Guard(result.Content) != nil {
		// The tool ran; its output never reaches the model.
		slog.WarnContext(ctx, "ai_tool_call_failed", "tool", call.Name, "reason", "result_withheld", "run_uuid", ctx.Value(runTraceKey{}))
		return toolOutcome{ToolResult: ToolResult{Content: toolErrWithheld, IsError: true}}, true, nil
	}
	result.Content = NewMasker().Mask(result.Content)
	return toolOutcome{ToolResult: result, ends: offered.EndsTurn && !result.IsError}, true, nil
}

// chat runs up to rounds tool turns. Every turn may call several tools; when the
// rounds are used up the model gets one last turn without tools instead of an
// error. A turn cut off by max_tokens is continued (at most maxContinuations
// times) inside the same reply; a reply still cut off after that is returned
// with Truncated set rather than failed. A tool call that cannot run is an
// error result for the model, not the end of the reply (runTool).
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
		// A structured answer cannot be continued: the next turn would start a
		// second JSON document. Generate already retried it with a larger limit.
		if truncatedByLimit(next, err) && len(req.Schema) == 0 {
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
			result, executed, err := runTool(ctx, req.Tools, run, call)
			if err != nil {
				return out, err
			}
			if executed {
				out.Calls = append(out.Calls, call)
			}
			finished = finished || (executed && result.ends)
			req.Turns = append(req.Turns, Turn{Role: "tool", Text: result.Content, ToolCallID: call.ID, ToolError: result.IsError})
		}
		if finished {
			return out, nil
		}
	}
}
