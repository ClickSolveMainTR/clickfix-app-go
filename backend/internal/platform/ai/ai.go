// Package ai adapts takwim's trusted-instructions/untrusted-input provider contract.
// Product code supplies tools; model output is never authority to execute a tool.
package ai

import (
	"context"
	"encoding/json"
	"errors"
)

const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	DefaultModel      = "claude-sonnet-5-5"
)

var (
	ErrNotConfigured = errors.New("ai: provider not configured")
	ErrRefused       = errors.New("ai: refused")
	ErrTruncated     = errors.New("ai: truncated")
	ErrInvalidOutput = errors.New("ai: invalid output")
	ErrUnavailable   = errors.New("ai: provider unavailable")
	ErrTooManyRounds = errors.New("ai: too many tool rounds")
)

type Tool struct {
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	Schema           map[string]any `json:"schema"`
	RequiresApproval bool           `json:"requires_approval"`
	// EndsTurn: a trusted application tool may finish the reply after its
	// validated execution. An error result (ToolResult.IsError) never ends
	// the turn: the model reads the error and continues.
	EndsTurn bool `json:"-"`
}
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}
type Turn struct {
	Role       string     `json:"role"`
	Text       string     `json:"text"`
	Calls      []ToolCall `json:"calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}
type Request struct {
	TraceID      string
	Model        string
	Instructions string
	Turns        []Turn
	Schema       map[string]any
	Tools        []Tool
	MaxTokens    int64
	Effort       string
	CachePrefix  bool
	// CacheTools is the number of leading tools that are the same on every
	// turn; with CachePrefix the provider caches them separately, so a
	// page change (different trailing tools) still reuses them (CF-211).
	CacheTools int
	// ToolChoice "none" keeps the tools defined (earlier tool_use turns need
	// them) but forbids new calls; the tool loop sets it for its last turn.
	ToolChoice string
}
type Result struct {
	CacheReadTokens   int64           `json:"-"`
	CacheCreateTokens int64           `json:"-"`
	StopReason        string          `json:"-"`
	StopDetails       json.RawMessage `json:"-"`
	FinalUsage        bool            `json:"-"`
	// Truncated: the answer still hit max_tokens after the loop's automatic
	// continuations; Text holds everything that was produced.
	Truncated    bool       `json:"-"`
	Provider     string     `json:"provider"`
	Text         string     `json:"text"`
	Model        string     `json:"model"`
	Calls        []ToolCall `json:"tool_calls"`
	InputTokens  int64      `json:"input_tokens"`
	OutputTokens int64      `json:"output_tokens"`
}

// Delta is ephemeral provider text; callers must guard accumulated output before rendering it.
type Delta func(string) error

type Provider interface {
	Generate(context.Context, Request) (Result, error)
	Stream(context.Context, Request, Delta) (Result, error)
}
type Embeddings struct {
	Vectors     [][]float32
	InputTokens int64
}
type Embedder interface {
	Embed(context.Context, []string) (Embeddings, error)
	Model() string
}

func ModelForTask(task string) string {
	switch task {
	case "triage":
		return "claude-haiku-5-5"
	case "complex":
		return "claude-opus-5-5"
	default:
		return DefaultModel
	}
}

// Route has explicit operator-provided prices in USD micro-units per million tokens.
// Missing prices disable execution; silently recording zero cost would bypass budgets.
type Route struct {
	Provider         Provider
	ProviderName     string
	Model            string
	InputPrice       int64
	OutputPrice      int64
	CacheReadPrice   int64
	CacheCreatePrice int64
}

func (r Route) Cost(input, output int64) int64 {
	return (input*r.InputPrice + output*r.OutputPrice + 999999) / 1000000
}

// ResultCost uses optional operator-configured cache prices. Absent rates retain
// the existing effective input price (a conservative accounting fallback).
func (r Route) ResultCost(out Result) int64 {
	readPrice, writePrice := r.CacheReadPrice, r.CacheCreatePrice
	if readPrice <= 0 {
		readPrice = r.InputPrice
	}
	if writePrice <= 0 {
		writePrice = r.InputPrice
	}
	regular := max(int64(0), out.InputTokens-out.CacheReadTokens-out.CacheCreateTokens)
	return (regular*r.InputPrice + out.CacheReadTokens*readPrice + out.CacheCreateTokens*writePrice + out.OutputTokens*r.OutputPrice + 999999) / 1000000
}

func EffortForTask(task string) string {
	if task == "complex" {
		return ""
	}
	return "low"
}
