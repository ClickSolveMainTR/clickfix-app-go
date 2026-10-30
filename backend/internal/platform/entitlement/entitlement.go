// Package entitlement is the shared, fail-closed feature/quota contract.
// Adapted from comcode quota.go/errors.go and Takwim's CheckAdd.
package entitlement

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

const (
	CodeFeatureRequired = "ENTITLEMENT_REQUIRED"
	CodeLimitExceeded   = "PLAN_LIMIT_EXCEEDED"
	CodeReadOnly        = "SUBSCRIPTION_READ_ONLY"
	UpgradePath         = "/billing"
)

var ErrDenied = errors.New("entitlement denied")

type Error struct {
	Code    string
	Feature string
	Limit   int64
	Used    int64
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s (%d/%d)", e.Code, e.Feature, e.Used, e.Limit)
}
func (e *Error) Unwrap() error { return ErrDenied }

type Value struct {
	Feature     string `json:"feature"`
	Kind        string `json:"kind"`
	Enabled     bool   `json:"enabled"`
	Limit       int64  `json:"limit"`
	Unlimited   bool   `json:"unlimited"`
	Enforcement string `json:"enforcement"`
	WarnPercent int32  `json:"warn_percent"`
	Aggregation string `json:"aggregation"`
	Period      string `json:"period"`
}
type Decision struct {
	Allowed        bool   `json:"allowed"`
	WarningPercent int    `json:"warning_percent"`
	Limit          int64  `json:"limit"`
	Used           int64  `json:"used"`
	Unlimited      bool   `json:"unlimited"`
	UpgradePath    string `json:"upgrade_path"`
}

func Check(v Value, used, delta int64) (Decision, error) {
	d := Decision{Limit: v.Limit, Used: used, Unlimited: v.Unlimited, UpgradePath: UpgradePath}
	if used < 0 || delta < 0 || used > math.MaxInt64-delta || v.Kind != "limit" {
		return d, fmt.Errorf("invalid quota arguments")
	}
	next := used + delta
	d.Allowed = v.Unlimited || next <= v.Limit || v.Enforcement == "soft"
	if !v.Unlimited {
		if next >= v.Limit {
			d.WarningPercent = 100
		} else {
			// ceil(limit*percent/100) without integer overflow.
			warn := v.Limit/100*int64(v.WarnPercent) + (v.Limit%100*int64(v.WarnPercent)+99)/100
			if next >= warn {
				d.WarningPercent = int(v.WarnPercent)
			}
		}
	}
	if !d.Allowed {
		return d, &Error{Code: CodeLimitExceeded, Feature: v.Feature, Limit: v.Limit, Used: used}
	}
	return d, nil
}

type Engine interface {
	Require(context.Context, int64, string) error
	CheckLimit(context.Context, int64, string, int64, int64) (Decision, error)
	RequireWritable(context.Context, int64) error
}

// Require is fail-closed when a module forgot to inject the engine.
func Require(ctx context.Context, e Engine, org int64, key string) error {
	if e == nil {
		return &Error{Code: CodeFeatureRequired, Feature: key}
	}
	return e.Require(ctx, org, key)
}
func Middleware(e Engine, key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := orgctx.From(r.Context())
			if !ok {
				response.Forbidden(w, r, "")
				return
			}
			err := Require(r.Context(), e, scope.ID, key)
			if err != nil {
				if !WriteError(w, r, err) {
					response.Internal(w, r, "")
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Writable is for outbound replies/resource writes only; never put it on inbound webhooks or reads.
func Writable(e Engine) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := orgctx.From(r.Context())
			if !ok {
				response.Forbidden(w, r, "")
				return
			}
			var err error
			if e == nil {
				err = &Error{Code: CodeReadOnly}
			} else {
				err = e.RequireWritable(r.Context(), scope.ID)
			}
			if err != nil {
				if !WriteError(w, r, err) {
					response.Internal(w, r, "")
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
func WriteError(w http.ResponseWriter, r *http.Request, err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	locale := i18n.LocaleTR
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Accept-Language")), "en") {
		locale = i18n.LocaleEN
	}
	response.ErrorWithDetails(w, r, http.StatusForbidden, e.Code, i18n.Translate(locale, "billing.errors."+e.Code), []response.Detail{
		{Field: e.Feature, Code: e.Code, Message: fmt.Sprintf("limit=%d used=%d", e.Limit, e.Used)},
		{Field: "upgrade_path", Code: "UPGRADE", Message: UpgradePath},
	})
	return true
}
