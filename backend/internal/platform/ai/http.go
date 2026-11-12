package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// HTTPProvider uses the Messages or OpenAI-compatible wire protocol. Keys and base URLs
// are composition-root inputs only; neither is accepted from customer requests.
type HTTPProvider struct {
	Kind           string
	BaseURL        string
	Key            string
	Client         *http.Client
	EmbeddingModel string
}

func NewAnthropic(key string) *HTTPProvider {
	return &HTTPProvider{Kind: ProviderAnthropic, BaseURL: "https://api.anthropic.com/v1", Key: key, Client: &http.Client{Timeout: 90 * time.Second}}
}

func NewOpenAI(key, baseURL, embeddingModel string) *HTTPProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &HTTPProvider{Kind: ProviderOpenAI, BaseURL: strings.TrimRight(baseURL, "/"), Key: key, EmbeddingModel: embeddingModel, Client: &http.Client{Timeout: 90 * time.Second}}
}

func (p *HTTPProvider) post(ctx context.Context, path string, body any) (*http.Response, error) {
	if p.Key == "" {
		return nil, ErrNotConfigured
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	model := ""
	if values, ok := body.(map[string]any); ok {
		model, _ = values["model"].(string)
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+path, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if p.Kind == ProviderAnthropic {
			req.Header.Set("x-api-key", p.Key)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("Authorization", "Bearer "+p.Key)
		}
		started := time.Now()
		var firstByte atomic.Int64
		trace := &httptrace.ClientTrace{GotFirstResponseByte: func() { firstByte.CompareAndSwap(0, time.Since(started).Microseconds()) }}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
		resp, err := p.Client.Do(req)
		slog.InfoContext(ctx, "ai_http_headers", "provider", p.Kind, "path", path, "model", model, "run_uuid", ctx.Value(runTraceKey{}),
			"attempt", attempt+1, "ttfb_us", firstByte.Load(), "headers_ms", time.Since(started).Milliseconds())
		// Transport errors may mean an accepted/billable request: never automatically replay them.
		if err != nil {
			return nil, ErrUnavailable
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		detail := rejectionDetail(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		retryable := resp.StatusCode == 429 || resp.StatusCode == 529 || resp.StatusCode == 503
		if !retryable || attempt == 2 {
			slog.Warn("ai_provider_rejected", "provider", p.Kind, "path", path, "status", resp.StatusCode, "detail", detail)
			return nil, &Rejected{Status: resp.StatusCode, Detail: detail}
		}
		delay := time.Duration(1<<attempt) * 250 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, ErrUnavailable
}

func (p *HTTPProvider) body(req Request, stream bool) (string, map[string]any) {
	if req.MaxTokens <= 0 {
		req.MaxTokens = 2048
	}
	body := map[string]any{"model": req.Model, "max_tokens": req.MaxTokens, "stream": stream}
	messages := []any{}
	if p.Kind == ProviderAnthropic {
		body["system"] = req.Instructions
		if req.CachePrefix {
			body["system"] = []any{map[string]any{"type": "text", "text": req.Instructions, "cache_control": map[string]any{"type": "ephemeral"}}}
		}
		config := map[string]any{}
		if req.Effort != "" {
			config["effort"] = req.Effort
		}
		if req.Model == "claude-sonnet-5-5" && (req.Effort == "low" || req.Effort == "medium") && len(req.Tools) == 0 {
			body["thinking"] = map[string]any{"type": "between_tools"}
		}
		for _, turn := range req.Turns {
			blocks := []any{}
			if turn.Text != "" && turn.Role != "tool" {
				blocks = append(blocks, map[string]any{"type": "text", "text": turn.Text})
			}
			for _, c := range turn.Calls {
				blocks = append(blocks, map[string]any{"type": "tool_use", "id": c.ID, "name": c.Name, "input": c.Input})
			}
			role := turn.Role
			if role == "tool" {
				role = "user"
				blocks = append(blocks, map[string]any{"type": "tool_result", "tool_use_id": turn.ToolCallID, "content": turn.Text})
			}
			if len(blocks) == 0 {
				continue // Anthropic rejects empty content.
			}
			messages = append(messages, map[string]any{"role": role, "content": blocks})
		}
		tools := []any{}
		for i, t := range req.Tools {
			tool := map[string]any{"name": t.Name, "description": t.Description, "input_schema": t.Schema}
			if req.CachePrefix && req.CacheTools > 0 && req.CacheTools < len(req.Tools) && i == req.CacheTools-1 {
				tool["cache_control"] = map[string]any{"type": "ephemeral"}
			}
			tools = append(tools, tool)
		}
		if len(tools) > 0 {
			body["tools"] = tools
			if req.ToolChoice == "none" {
				body["tool_choice"] = map[string]any{"type": "none"}
			}
		}
		if len(req.Schema) > 0 {
			config["format"] = map[string]any{"type": "json_schema", "schema": anthropicSchema(req.Schema)}
		}
		if len(config) > 0 {
			body["output_config"] = config
		}
		body["messages"] = messages
		return "/messages", body
	}
	messages = append(messages, map[string]any{"role": "system", "content": req.Instructions})
	for _, turn := range req.Turns {
		message := map[string]any{"role": turn.Role, "content": turn.Text}
		if turn.ToolCallID != "" {
			message["tool_call_id"] = turn.ToolCallID
		}
		calls := []any{}
		for _, c := range turn.Calls {
			calls = append(calls, map[string]any{"id": c.ID, "type": "function", "function": map[string]any{"name": c.Name, "arguments": string(c.Input)}})
		}
		if len(calls) > 0 {
			message["tool_calls"] = calls
		}
		messages = append(messages, message)
	}
	body["messages"] = messages
	tools := []any{}
	for _, t := range req.Tools {
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Schema}})
	}
	if len(tools) > 0 {
		body["tools"] = tools
		if req.ToolChoice == "none" {
			body["tool_choice"] = "none"
		}
	}
	if len(req.Schema) > 0 {
		body["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "answer", "strict": true, "schema": anthropicSchema(req.Schema)}}
	}
	if stream {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	return "/chat/completions", body
}

type wireCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
type wireMessage struct {
	Content   string     `json:"content"`
	Refusal   string     `json:"refusal"`
	ToolCalls []wireCall `json:"tool_calls"`
}
type wireResponse struct {
	Type        string          `json:"type"`
	Model       string          `json:"model"`
	StopReason  string          `json:"stop_reason"`
	StopDetails json.RawMessage `json:"stop_details"`
	Content     []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	Choices []struct {
		Message wireMessage `json:"message"`
		Delta   wireMessage `json:"delta"`
		Finish  string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		Input       int64 `json:"input_tokens"`
		Output      int64 `json:"output_tokens"`
		CacheRead   int64 `json:"cache_read_input_tokens"`
		CacheCreate int64 `json:"cache_creation_input_tokens"`
		Prompt      int64 `json:"prompt_tokens"`
		Completion  int64 `json:"completion_tokens"`
	} `json:"usage"`
}

func usage(w wireResponse) (int64, int64) {
	return w.Usage.Input + w.Usage.CacheRead + w.Usage.CacheCreate + w.Usage.Prompt, w.Usage.Output + w.Usage.Completion
}

func stopError(stop string) error {
	switch stop {
	case "refusal", "content_filter":
		return ErrRefused
	case "max_tokens", "length":
		return ErrTruncated
	}
	return nil
}

func validateResult(req Request, out Result) (Result, error) {
	for _, c := range out.Calls {
		if !json.Valid(c.Input) {
			return out, ErrInvalidOutput
		}
	}
	if len(req.Schema) > 0 && len(out.Calls) == 0 {
		if err := validateJSON(req.Schema, []byte(out.Text)); err != nil {
			return out, err
		}
	}

	return out, nil
}

func (p *HTTPProvider) generateOnce(ctx context.Context, req Request) (out Result, err error) {
	started := time.Now()
	defer func() {
		slog.InfoContext(ctx, "ai_model_attempt", "model", req.Model, "provider", p.Kind, "run_uuid", req.TraceID,
			"total_ms", time.Since(started).Milliseconds(), "stop_reason", out.StopReason,
			"stop_details_present", len(out.StopDetails) > 0 && string(out.StopDetails) != "null", "cache_read_input_tokens", out.CacheReadTokens,
			"cache_creation_input_tokens", out.CacheCreateTokens, "failed", err != nil)
	}()
	path, body := p.body(req, false)
	resp, err := p.post(ctx, path, body)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var wire wireResponse
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&wire); err != nil {
		return Result{}, ErrInvalidOutput
	}
	out = Result{FinalUsage: true, Provider: p.Kind, Model: wire.Model, Calls: []ToolCall{},
		CacheReadTokens: wire.Usage.CacheRead, CacheCreateTokens: wire.Usage.CacheCreate,
		StopReason: wire.StopReason, StopDetails: wire.StopDetails}
	out.InputTokens, out.OutputTokens = usage(wire)
	if p.Kind == ProviderAnthropic {
		for _, b := range wire.Content {
			if b.Type == "text" {
				out.Text += b.Text
			}
			if b.Type == "tool_use" {
				out.Calls = append(out.Calls, ToolCall{ID: b.ID, Name: b.Name, Input: b.Input})
			}
		}
		if err = stopError(wire.StopReason); err != nil {
			return out, err
		}
	} else {
		if len(wire.Choices) != 1 {
			return out, ErrInvalidOutput
		}
		choice := wire.Choices[0]
		out.StopReason = choice.Finish
		out.Text = choice.Message.Content
		if choice.Message.Refusal != "" {
			return out, ErrRefused
		}
		for _, c := range choice.Message.ToolCalls {
			out.Calls = append(out.Calls, ToolCall{ID: c.ID, Name: c.Function.Name, Input: json.RawMessage(c.Function.Arguments)})
		}
		if err = stopError(choice.Finish); err != nil {
			return out, err
		}
	}
	return validateResult(req, out)
}

// Stream aggregates tool JSON deltas and final usage. An interrupted stream is never
// retried: replaying text/tools could duplicate side effects and provider charges.
func (p *HTTPProvider) Stream(ctx context.Context, req Request, emit Delta) (result Result, streamErr error) {
	started := time.Now()
	var firstToken time.Duration
	defer func() {
		slog.InfoContext(ctx, "ai_model_stream", "model", req.Model, "provider", p.Kind, "run_uuid", req.TraceID,
			"total_ms", time.Since(started).Milliseconds(), "first_token_ms", firstToken.Milliseconds(),
			"cache_read_input_tokens", result.CacheReadTokens, "cache_creation_input_tokens", result.CacheCreateTokens, "failed", streamErr != nil)
	}()
	ctx = context.WithValue(ctx, runTraceKey{}, req.TraceID)
	if err := validateRequest(req); err != nil {
		return Result{}, &requestRejected{}
	}
	path, body := p.body(req, true)
	resp, err := p.post(ctx, path, body)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	out := Result{Provider: p.Kind, Model: req.Model, Calls: []ToolCall{}}
	calls := map[int]*ToolCall{}
	args := map[int]string{}
	complete := false
	var terminalErr error
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 4<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			complete = true
			break
		}
		var w wireResponse
		if json.Unmarshal([]byte(data), &w) != nil {
			return out, ErrInvalidOutput
		}
		delta := ""
		if p.Kind == ProviderAnthropic {
			var e struct {
				Type    string       `json:"type"`
				Index   int          `json:"index"`
				Message wireResponse `json:"message"`
				Block   struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
				Delta struct {
					Text        string          `json:"text"`
					Partial     string          `json:"partial_json"`
					Stop        string          `json:"stop_reason"`
					StopDetails json.RawMessage `json:"stop_details"`
				} `json:"delta"`
			}
			if json.Unmarshal([]byte(data), &e) != nil {
				return out, ErrInvalidOutput
			}
			switch e.Type {
			case "error":
				return out, ErrUnavailable
			case "message_start":
				out.Model = e.Message.Model
				out.InputTokens, out.OutputTokens = usage(e.Message)
				out.CacheReadTokens = e.Message.Usage.CacheRead
				out.CacheCreateTokens = e.Message.Usage.CacheCreate
			case "content_block_start":
				if e.Block.Type == "tool_use" {
					calls[e.Index] = &ToolCall{ID: e.Block.ID, Name: e.Block.Name}
				}
			case "content_block_delta":
				delta = e.Delta.Text
				args[e.Index] += e.Delta.Partial
			case "message_delta":
				out.OutputTokens = w.Usage.Output
				out.StopReason = e.Delta.Stop
				out.StopDetails = e.Delta.StopDetails
				terminalErr = stopError(e.Delta.Stop)
				out.FinalUsage = true
			case "message_stop":
				complete = true
			}
		} else {
			if w.Type == "error" {
				return out, ErrUnavailable
			}
			if w.Model != "" {
				out.Model = w.Model
			}
			in, output := usage(w)
			if in > 0 || output > 0 {
				out.InputTokens = in
				out.OutputTokens = output
				out.FinalUsage = true
			}
			for _, c := range w.Choices {
				delta += c.Delta.Content
				if c.Delta.Refusal != "" {
					terminalErr = ErrRefused
				}
				if err = stopError(c.Finish); err != nil {
					terminalErr = err
				}
				for _, wc := range c.Delta.ToolCalls {
					call := calls[wc.Index]
					if call == nil {
						call = &ToolCall{}
						calls[wc.Index] = call
					}
					call.ID += wc.ID
					call.Name += wc.Function.Name
					args[wc.Index] += wc.Function.Arguments
				}
			}
		}
		if delta != "" && firstToken == 0 {
			firstToken = time.Since(started)
		}
		out.Text += delta
		if len(out.Text) > 2<<20 {
			return out, ErrTruncated
		}
		if delta != "" && emit != nil {
			if err = emit(delta); err != nil {
				return out, err
			}
		}
	}
	if scanner.Err() != nil || !complete {
		return out, ErrUnavailable
	}
	if terminalErr != nil {
		return out, terminalErr
	}
	for i := 0; i < 1024; i++ {
		if c := calls[i]; c != nil {
			c.Input = json.RawMessage(args[i])
			if len(c.Input) == 0 {
				c.Input = json.RawMessage(`{}`)
			}
			out.Calls = append(out.Calls, *c)
		}
	}
	return validateResult(req, out)
}
func (p *HTTPProvider) Model() string { return p.EmbeddingModel }
func (p *HTTPProvider) Embed(ctx context.Context, input []string) (Embeddings, error) {
	if p.Kind != ProviderOpenAI || p.EmbeddingModel == "" {
		return Embeddings{}, ErrUnavailable
	}
	resp, err := p.post(ctx, "/embeddings", map[string]any{"model": p.EmbeddingModel, "input": input, "encoding_format": "float"})
	if err != nil {
		return Embeddings{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var wire struct {
		Usage struct {
			PromptTokens int64 `json:"prompt_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&wire); err != nil {
		return Embeddings{}, ErrInvalidOutput
	}
	if len(wire.Data) != len(input) {
		return Embeddings{}, ErrInvalidOutput
	}
	out := make([][]float32, len(input))
	for _, d := range wire.Data {
		if d.Index < 0 || d.Index >= len(out) || out[d.Index] != nil || len(d.Embedding) == 0 {
			return Embeddings{}, ErrInvalidOutput
		}
		out[d.Index] = d.Embedding
	}
	tokens := wire.Usage.PromptTokens
	if tokens == 0 {
		tokens = wire.Usage.TotalTokens
	}
	return Embeddings{Vectors: out, InputTokens: tokens}, nil
}

// denySchemaLoader prevents a schema reference from turning validation into network I/O.
type denySchemaLoader struct{}

func (denySchemaLoader) Load(string) (any, error) { return nil, ErrInvalidOutput }

func validateJSON(schema map[string]any, data []byte) error {
	if !json.Valid(data) {
		return ErrInvalidOutput
	}
	normalized, err := json.Marshal(schema)
	if err != nil {
		return ErrInvalidOutput
	}
	var document any
	if err = json.Unmarshal(normalized, &document); err != nil {
		return ErrInvalidOutput
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(denySchemaLoader{})
	if err = compiler.AddResource("https://clickfix.invalid/answer", document); err != nil {
		return ErrInvalidOutput
	}
	compiled, err := compiler.Compile("https://clickfix.invalid/answer")
	if err != nil {
		return ErrInvalidOutput
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err = decoder.Decode(&value); err != nil {
		return ErrInvalidOutput
	}
	removeOptionalNulls(document, value)
	if err = compiled.Validate(value); err != nil {
		return ErrInvalidOutput
	}
	return nil
}

// ValidateJSON validates structured agent output even for in-process fake providers.
func ValidateJSON(schema map[string]any, data []byte) error { return validateJSON(schema, data) }

// ValidateSchema compiles an application tool schema without network reference loading.
func ValidateSchema(schema map[string]any) error {
	raw, err := json.Marshal(schema)
	if err != nil {
		return ErrInvalidOutput
	}
	var document any
	if json.Unmarshal(raw, &document) != nil {
		return ErrInvalidOutput
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(denySchemaLoader{})
	if compiler.AddResource("https://clickfix.invalid/tool", document) != nil {
		return ErrInvalidOutput
	}
	if _, err = compiler.Compile("https://clickfix.invalid/tool"); err != nil {
		return ErrInvalidOutput
	}
	return nil
}

// rejectionDetail extracts "type: message" from a provider error body. Provider
// error messages describe the request shape, not its content.
func rejectionDetail(r io.Reader) string {
	var e struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.NewDecoder(r).Decode(&e) != nil || e.Error.Message == "" {
		return ""
	}
	detail := e.Error.Type + ": " + e.Error.Message
	if len(detail) > 300 {
		detail = detail[:300]
	}
	return detail
}
