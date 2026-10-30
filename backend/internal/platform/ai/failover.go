package ai

import (
	"context"
	"errors"
	"fmt"
)

// Rejected is an explicit upstream rejection, before any response stream was accepted.
// Ambiguous transport failures deliberately do not match it.
// Rejected is a non-retryable upstream answer. Detail carries the provider's
// error type and message (never request content) for logs and run records.
type Rejected struct {
	Status int
	Detail string
}

func (e *Rejected) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("ai: upstream rejected (HTTP %d)", e.Status)
	}
	return fmt.Sprintf("ai: upstream rejected (HTTP %d): %s", e.Status, e.Detail)
}
func (e *Rejected) Unwrap() error { return ErrUnavailable }

// Failover ports Laravel's ordered provider chain, restricted to explicit transient
// rejection. Refusals and output guard failures cannot be bypassed using another model.
type Failover struct {
	Primary        Provider
	Secondary      Provider
	SecondaryModel string
}

func canFailover(err error) bool {
	var rejection *Rejected
	return errors.As(err, &rejection) && (rejection.Status == 429 || rejection.Status == 503 || rejection.Status == 529)
}

func (f *Failover) Generate(ctx context.Context, req Request) (Result, error) {
	out, err := f.Primary.Generate(ctx, req)
	if !canFailover(err) || f.Secondary == nil {
		return out, err
	}
	req.Model = f.SecondaryModel
	return f.Secondary.Generate(ctx, req)
}

func (f *Failover) Stream(ctx context.Context, req Request, emit Delta) (Result, error) {
	emitted := false
	wrapped := func(s string) error {
		emitted = true
		if emit != nil {
			return emit(s)
		}
		return nil
	}
	out, err := f.Primary.Stream(ctx, req, wrapped)
	if emitted || !canFailover(err) || f.Secondary == nil {
		return out, err
	}
	req.Model = f.SecondaryModel
	return f.Secondary.Stream(ctx, req, emit)
}

// NotAccepted identifies failures for which no provider request was accepted.
func NotAccepted(err error) bool {
	var rejected *Rejected
	var local *requestRejected
	return errors.As(err, &local) || errors.Is(err, ErrNotConfigured) || errors.As(err, &rejected)
}
