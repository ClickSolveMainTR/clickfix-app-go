// Package reqid carries validated correlation identifiers across HTTP and jobs.
package reqid

import (
	"context"
	"regexp"
)

const Header = "X-Request-ID"

var valid = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)

type key struct{}

func Valid(id string) bool { return valid.MatchString(id) }
func With(ctx context.Context, id string) context.Context {
	if !Valid(id) {
		return ctx
	}
	return context.WithValue(ctx, key{}, id)
}
func From(ctx context.Context) string { id, _ := ctx.Value(key{}).(string); return id }
