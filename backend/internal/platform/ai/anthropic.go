package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

func (p *HTTPProvider) anthropicMessages() *anthropic.MessageService {
	client := anthropic.NewClient(
		option.WithoutEnvironmentDefaults(),
		option.WithBaseURL(p.BaseURL),
		option.WithAPIKey(p.Key),
		option.WithHTTPClient(p.httpClient()),
		// The SDK's own retries replay transport errors; ours never do.
		option.WithMaxRetries(0),
		option.WithRequestTimeout(p.timeout()),
		option.WithMiddleware(p.middleware),
	)
	return &client.Messages
}

// anthropicParams maps a Request onto the Messages API. Prompt caching,
// effort, thinking and structured output use the SDK's typed fields.
func anthropicParams(req Request) anthropic.MessageNewParams {
	if req.MaxTokens <= 0 {
		req.MaxTokens = 2048
	}
	params := anthropic.MessageNewParams{Model: anthropic.Model(req.Model), MaxTokens: req.MaxTokens, Messages: []anthropic.MessageParam{}}
	if req.Instructions != "" {
		system := anthropic.TextBlockParam{Text: req.Instructions}
		if req.CachePrefix {
			system.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		params.System = []anthropic.TextBlockParam{system}
	}
	if req.Effort != "" {
		params.OutputConfig.Effort = anthropic.OutputConfigEffort(req.Effort)
	}
	params.Thinking = fastThinking(req)
	for _, turn := range req.Turns {
		blocks := []anthropic.ContentBlockParamUnion{}
		if turn.Text != "" && turn.Role != "tool" {
			blocks = append(blocks, anthropic.NewTextBlock(turn.Text))
		}
		for _, c := range turn.Calls {
			blocks = append(blocks, anthropic.NewToolUseBlock(c.ID, c.Input, c.Name))
		}
		role := anthropic.MessageParamRole(turn.Role)
		if turn.Role == "tool" {
			role = anthropic.MessageParamRoleUser
			blocks = append(blocks, anthropic.NewToolResultBlock(turn.ToolCallID, turn.Text, turn.ToolError))
		}
		if len(blocks) == 0 {
			continue // Anthropic rejects empty content.
		}
		params.Messages = append(params.Messages, anthropic.MessageParam{Role: role, Content: blocks})
	}
	for i, t := range req.Tools {
		tool := anthropic.ToolParam{Name: t.Name, Description: anthropic.String(t.Description)}
		if len(t.Schema) > 0 {
			// The application schema is sent as written (local validation uses it too).
			tool.InputSchema = param.Override[anthropic.ToolInputSchemaParam](t.Schema)
		}
		if req.CachePrefix && req.CacheTools > 0 && req.CacheTools < len(req.Tools) && i == req.CacheTools-1 {
			tool.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		params.Tools = append(params.Tools, anthropic.ToolUnionParam{OfTool: &tool})
	}
	if len(params.Tools) > 0 && req.ToolChoice == "none" {
		params.ToolChoice = anthropic.ToolChoiceUnionParam{OfNone: &anthropic.ToolChoiceNoneParam{}}
	}
	if len(req.Schema) > 0 {
		params.OutputConfig.Format = anthropic.JSONOutputFormatParam{Schema: anthropicSchema(req.Schema)}
	}
	return params
}

// fastThinking turns extended thinking off for latency-bound tasks (effort low
// or medium: triage, customer replies, copilot). Leaving the field out is not
// "off" on the current models: they think adaptively, which adds seconds to
// every turn and spends max_tokens before the answer starts (CF-251: a reply
// cut off at max_tokens after 15 s). Sonnet 5.5 takes between_tools, with or
// without tools; Haiku 5.5 takes disabled. Other models keep their default.
func fastThinking(req Request) anthropic.ThinkingConfigParamUnion {
	if req.Effort != "low" && req.Effort != "medium" {
		return anthropic.ThinkingConfigParamUnion{}
	}
	switch req.Model {
	case "claude-sonnet-5-5":
		return anthropic.ThinkingConfigParamUnion{OfBetweenTools: &anthropic.ThinkingConfigBetweenToolsParam{}}
	case "claude-haiku-5-5":
		return anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
	}
	return anthropic.ThinkingConfigParamUnion{}
}

// thinkingRefused reports a request the provider refused before running it
// (HTTP 400 that names the thinking field) while it carried our thinking
// override. Such a request is sent once more with the model's default
// thinking: a slower answer is better than none if a model stops accepting
// the override. Any other 400 is not replayed.
func thinkingRefused(params anthropic.MessageNewParams, err error) bool {
	var rejected *Rejected
	return errors.As(err, &rejected) && rejected.Status == http.StatusBadRequest &&
		strings.Contains(strings.ToLower(rejected.Detail), "thinking") &&
		(params.Thinking.OfBetweenTools != nil || params.Thinking.OfDisabled != nil)
}

func (p *HTTPProvider) anthropicGenerate(ctx context.Context, req Request) (Result, error) {
	params := anthropicParams(req)
	msg, err := p.anthropicMessages().New(ctx, params)
	if thinkingRefused(params, err) {
		slog.WarnContext(ctx, "ai_thinking_override_refused", "model", req.Model, "run_uuid", req.TraceID)
		params.Thinking = anthropic.ThinkingConfigParamUnion{}
		msg, err = p.anthropicMessages().New(ctx, params)
	}
	if err != nil {
		return Result{}, providerError(err)
	}
	out := Result{FinalUsage: true, Provider: p.Kind, Model: string(msg.Model), Calls: []ToolCall{},
		CacheReadTokens: msg.Usage.CacheReadInputTokens, CacheCreateTokens: msg.Usage.CacheCreationInputTokens,
		StopReason: string(msg.StopReason), StopDetails: rawJSON(msg.JSON.StopDetails.Raw()),
		InputTokens:  usageIn(msg.Usage.InputTokens, msg.Usage.CacheReadInputTokens, msg.Usage.CacheCreationInputTokens),
		OutputTokens: msg.Usage.OutputTokens}
	for _, b := range msg.Content {
		switch b.Type {
		case "text":
			out.Text += b.Text
		case "tool_use":
			out.Calls = append(out.Calls, ToolCall{ID: b.ID, Name: b.Name, Input: b.Input})
		}
	}
	return out, stopError(string(msg.StopReason))
}

// anthropicStreamBody sends a streaming request and hands back the raw SSE body.
func (p *HTTPProvider) anthropicStreamBody(ctx context.Context, req Request) (io.ReadCloser, error) {
	var resp *http.Response
	params := anthropicParams(req)
	_, err := p.anthropicMessages().New(ctx, params, option.WithJSONSet("stream", true), option.WithResponseBodyInto(&resp))
	if thinkingRefused(params, err) {
		slog.WarnContext(ctx, "ai_thinking_override_refused", "model", req.Model, "run_uuid", req.TraceID)
		params.Thinking = anthropic.ThinkingConfigParamUnion{}
		_, err = p.anthropicMessages().New(ctx, params, option.WithJSONSet("stream", true), option.WithResponseBodyInto(&resp))
	}
	if err != nil {
		return nil, providerError(err)
	}
	return resp.Body, nil
}

// anthropicEvent applies one Messages stream event and returns its text delta.
func (s *streamState) anthropicEvent(data string) (string, error) {
	var e anthropic.MessageStreamEventUnion
	if json.Unmarshal([]byte(data), &e) != nil {
		return "", ErrInvalidOutput
	}
	switch e.Type {
	case "error":
		return "", ErrUnavailable
	case "message_start":
		u := e.Message.Usage
		s.out.Model = string(e.Message.Model)
		s.out.InputTokens = usageIn(u.InputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens)
		s.out.OutputTokens = u.OutputTokens
		s.out.CacheReadTokens = u.CacheReadInputTokens
		s.out.CacheCreateTokens = u.CacheCreationInputTokens
	case "content_block_start":
		if e.ContentBlock.Type == "tool_use" {
			s.calls[int(e.Index)] = &ToolCall{ID: e.ContentBlock.ID, Name: e.ContentBlock.Name}
		}
	case "content_block_delta":
		s.args[int(e.Index)] += e.Delta.PartialJSON
		return e.Delta.Text, nil
	case "message_delta":
		s.out.OutputTokens = e.Usage.OutputTokens
		s.out.StopReason = string(e.Delta.StopReason)
		s.out.StopDetails = rawJSON(e.Delta.JSON.StopDetails.Raw())
		s.terminal = stopError(string(e.Delta.StopReason))
		s.out.FinalUsage = true
	case "message_stop":
		s.complete = true
	}
	return "", nil
}
