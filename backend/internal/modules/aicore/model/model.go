// Package model defines the product AI execution API. Customers cannot choose prompts,
// credentials, tool privileges, provider URLs or prices.
package model

import "errors"

var (
	ErrInvalid     = errors.New("ai: invalid request")
	ErrDisabled    = errors.New("ai: disabled")
	ErrBudget      = errors.New("ai: budget exceeded")
	ErrCredits     = errors.New("ai: insufficient credits")
	ErrConcurrency = errors.New("ai: concurrency exceeded")
	ErrDaily       = errors.New("ai: daily calls exceeded")
)

type RunInput struct {
	Task      string `json:"task"`
	Input     string `json:"input"`
	Locale    string `json:"locale"`
	Retrieve  bool   `json:"retrieve"`
	MaxTokens int64  `json:"max_tokens"`
	Stream    bool   `json:"stream"`
	Unmask    bool   `json:"unmask"`
}

func (in *RunInput) Validate() error {
	if in.Task != "triage" && in.Task != "reply" && in.Task != "complex" {
		return ErrInvalid
	}
	if len(in.Input) == 0 || len(in.Input) > 24000 {
		return ErrInvalid
	}
	if in.Locale == "" {
		in.Locale = "tr"
	}
	if in.Locale != "tr" && in.Locale != "en" {
		return ErrInvalid
	}
	if in.MaxTokens == 0 {
		in.MaxTokens = 2048
	}
	if in.MaxTokens < 1 || in.MaxTokens > 8192 {
		return ErrInvalid
	}
	return nil
}
