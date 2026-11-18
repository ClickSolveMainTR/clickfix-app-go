package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// HTTPProvider talks to the Messages API (anthropic-sdk-go) or an
// OpenAI-compatible API (openai-go) (CF-213). The SDKs build, authenticate and
// send the requests; retry, rejection mapping, size limits and logs stay in
// this package (middleware below) so failover, masking and metering behave
// as before. Keys and base URLs are composition-root inputs only; neither is
// accepted from customer requests, and the SDKs' environment defaults are off.
type HTTPProvider struct {
	Kind string
	// BaseURL is the API root: "https://api.anthropic.com" (the SDK adds
	// "/v1/messages") or an OpenAI-compatible root that already ends in its
	// version, e.g. "https://api.openai.com/v1".
	BaseURL        string
	Key            string
	Client         *http.Client
	EmbeddingModel string
}

const requestTimeout = 90 * time.Second

func NewAnthropic(key string) *HTTPProvider {
	return &HTTPProvider{Kind: ProviderAnthropic, BaseURL: "https://api.anthropic.com", Key: key, Client: &http.Client{Timeout: requestTimeout}}
}

func NewOpenAI(key, baseURL, embeddingModel string) *HTTPProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &HTTPProvider{Kind: ProviderOpenAI, BaseURL: strings.TrimRight(baseURL, "/"), Key: key, EmbeddingModel: embeddingModel, Client: &http.Client{Timeout: requestTimeout}}
}

func (p *HTTPProvider) httpClient() *http.Client {
	if p.Client == nil {
		return &http.Client{Timeout: requestTimeout}
	}
	return p.Client
}

func (p *HTTPProvider) timeout() time.Duration {
	if p.Client != nil && p.Client.Timeout > 0 {
		return p.Client.Timeout
	}
	return requestTimeout
}

// call describes one SDK request for the middleware.
type call struct {
	model    string
	attempts int   // 0 means 3
	limit    int64 // response body bound; 0 means 4 MiB
}

type callKey struct{}

func withCall(ctx context.Context, c call) context.Context {
	return context.WithValue(ctx, callKey{}, c)
}

// middleware is shared by both SDKs (their Middleware types are the same
// func signature). Explicit 429/503/529 rejections are retried with a short
// backoff; transport errors are never replayed because the provider may have
// accepted a billable request. Any other non-2xx answer becomes *Rejected.
func (p *HTTPProvider) middleware(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	ctx := req.Context()
	c, _ := ctx.Value(callKey{}).(call)
	attempts := c.attempts
	if attempts <= 0 {
		attempts = 3
	}
	limit := c.limit
	if limit <= 0 {
		limit = 4 << 20
	}
	for attempt := 0; attempt < attempts; attempt++ {
		r := req
		if attempt > 0 {
			if req.GetBody == nil {
				return nil, ErrUnavailable
			}
			body, err := req.GetBody()
			if err != nil {
				return nil, ErrUnavailable
			}
			r = req.Clone(ctx)
			r.Body = body
		}
		started := time.Now()
		var firstByte atomic.Int64
		trace := &httptrace.ClientTrace{GotFirstResponseByte: func() { firstByte.CompareAndSwap(0, time.Since(started).Microseconds()) }}
		r = r.WithContext(httptrace.WithClientTrace(r.Context(), trace))
		resp, err := next(r)
		slog.InfoContext(ctx, "ai_http_headers", "provider", p.Kind, "path", req.URL.Path, "model", c.model, "run_uuid", ctx.Value(runTraceKey{}),
			"attempt", attempt+1, "ttfb_us", firstByte.Load(), "headers_ms", time.Since(started).Milliseconds())
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body = &limitedBody{r: io.LimitReader(resp.Body, limit+1), c: resp.Body, left: limit}
			// The body is decoded as JSON whatever type it declares (as before
			// CF-213); some OpenAI-compatible servers send text/plain.
			if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "json") && !strings.Contains(ct, "event-stream") {
				resp.Header.Set("Content-Type", "application/json")
			}
			return resp, nil
		}
		detail := rejectionDetail(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		retryable := resp.StatusCode == 429 || resp.StatusCode == 529 || resp.StatusCode == 503
		if !retryable || attempt == attempts-1 {
			slog.Warn("ai_provider_rejected", "provider", p.Kind, "path", req.URL.Path, "status", resp.StatusCode, "detail", detail)
			return nil, &Rejected{Status: resp.StatusCode, Detail: detail}
		}
		timer := time.NewTimer(time.Duration(1<<attempt) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, ErrUnavailable
}

// limitedBody fails the read past its bound, so an oversized answer becomes a
// decode error (ErrInvalidOutput) instead of a silently cut document.
type limitedBody struct {
	r    io.Reader
	c    io.Closer
	left int64
}

var errBodyTooLarge = errors.New("ai: response too large")

func (b *limitedBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.left -= int64(n)
	if b.left < 0 {
		return n, errBodyTooLarge
	}
	return n, err
}
func (b *limitedBody) Close() error { return b.c.Close() }

// providerError maps an SDK call error to this package's errors. Rejections,
// unavailability and context ends pass through; anything else (a body the SDK
// could not decode, an oversized body) is invalid provider output.
func providerError(err error) error {
	var rejected *Rejected
	switch {
	case err == nil:
		return nil
	case errors.As(err, &rejected):
		return rejected
	case errors.Is(err, ErrUnavailable):
		return ErrUnavailable
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// The request may have been accepted: same class as a transport error.
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return ErrInvalidOutput
}

func usageIn(input, cacheRead, cacheCreate int64) int64 { return input + cacheRead + cacheCreate }

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
		out.Text = conformJSON(req.Schema, out.Text)
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
	if p.Key == "" {
		return Result{}, ErrNotConfigured
	}
	ctx = withCall(ctx, call{model: req.Model})
	if p.Kind == ProviderAnthropic {
		out, err = p.anthropicGenerate(ctx, req)
	} else {
		out, err = p.openaiGenerate(ctx, req)
	}
	if err != nil {
		return out, err
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
	if p.Key == "" {
		return Result{}, ErrNotConfigured
	}
	ctx = withCall(ctx, call{model: req.Model})
	var body io.ReadCloser
	var err error
	if p.Kind == ProviderAnthropic {
		body, err = p.anthropicStreamBody(ctx, req)
	} else {
		body, err = p.openaiStreamBody(ctx, req)
	}
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = body.Close() }()
	s := &streamState{out: Result{Provider: p.Kind, Model: req.Model, Calls: []ToolCall{}}, calls: map[int]*ToolCall{}, args: map[int]string{}}
	var decode func(string) (string, error)
	if p.Kind == ProviderAnthropic {
		decode = s.anthropicEvent
	} else {
		decode = s.openaiChunk
	}
	err = eachData(body, func(data string) (bool, error) {
		if data == "[DONE]" {
			s.complete = true
			return false, nil
		}
		delta, err := decode(data)
		if err != nil {
			return false, err
		}
		if delta != "" && firstToken == 0 {
			firstToken = time.Since(started)
		}
		s.out.Text += delta
		if len(s.out.Text) > 2<<20 {
			return false, ErrTruncated
		}
		if delta != "" && emit != nil {
			if err := emit(delta); err != nil {
				return false, err
			}
		}
		return true, nil
	})
	out := s.out
	if err != nil {
		return out, err
	}
	if !s.complete {
		return out, ErrUnavailable
	}
	if s.terminal != nil {
		return out, s.terminal
	}
	for i := 0; i < 1024; i++ {
		if c := s.calls[i]; c != nil {
			c.Input = json.RawMessage(s.args[i])
			if len(c.Input) == 0 {
				c.Input = json.RawMessage(`{}`)
			}
			out.Calls = append(out.Calls, *c)
		}
	}
	return validateResult(req, out)
}

// streamState accumulates one streamed answer across provider events.
type streamState struct {
	out      Result
	calls    map[int]*ToolCall
	args     map[int]string
	complete bool
	terminal error
}

// eachData feeds the payload of every SSE "data:" line to fn until fn stops
// or the body ends. The SDKs' own stream readers are not used: the Messages
// reader drops events without an "event:" line and neither reader tells an
// interrupted stream from a finished one, which metering depends on.
func eachData(body io.Reader, fn func(string) (bool, error)) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		more, err := fn(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		if err != nil || !more {
			return err
		}
	}
	if scanner.Err() != nil {
		return ErrUnavailable
	}
	return nil
}

func (p *HTTPProvider) Model() string { return p.EmbeddingModel }

// denySchemaLoader prevents a schema reference from turning validation into network I/O.
type denySchemaLoader struct{}

func (denySchemaLoader) Load(string) (any, error) { return nil, ErrInvalidOutput }

func validateJSON(schema map[string]any, data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return &invalidOutput{reason: "empty"}
	}
	if !json.Valid(data) {
		return &invalidOutput{reason: "not_json"}
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
		return &invalidOutput{reason: "not_json"}
	}
	removeOptionalNulls(document, value)
	if err = compiled.Validate(value); err != nil {
		return &invalidOutput{reason: schemaViolation(err)}
	}
	return nil
}

// invalidOutput is ErrInvalidOutput with the reason for logs: which schema
// keyword failed where. It never carries a value of the document.
type invalidOutput struct{ reason string }

func (e *invalidOutput) Error() string { return "ai: invalid output (" + e.reason + ")" }
func (e *invalidOutput) Unwrap() error { return ErrInvalidOutput }

// InvalidReason is the loggable cause of an ErrInvalidOutput ("" when unknown).
func InvalidReason(err error) string {
	var invalid *invalidOutput
	if errors.As(err, &invalid) {
		return invalid.reason
	}
	return ""
}

// schemaViolation names the first failed keyword and its instance path,
// e.g. "/summary:maxLength".
func schemaViolation(err error) string {
	var failed *jsonschema.ValidationError
	if !errors.As(err, &failed) {
		return "schema"
	}
	for len(failed.Causes) > 0 {
		failed = failed.Causes[0]
	}
	keyword := "schema"
	if failed.ErrorKind != nil {
		if path := failed.ErrorKind.KeywordPath(); len(path) > 0 {
			keyword = path[len(path)-1]
		}
	}
	reason := "/" + strings.Join(failed.InstanceLocation, "/") + ":" + keyword
	if len(reason) > 120 {
		reason = reason[:120]
	}
	return reason
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

// rawJSON turns an SDK raw field into a RawMessage; absent fields stay nil.
func rawJSON(raw string) json.RawMessage {
	if raw == "" {
		return nil
	}
	return json.RawMessage(raw)
}
