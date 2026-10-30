package repository

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/jackc/pgx/v5"
)

// CheckConversation preserves inbox team visibility on AI entry points. Trusted
// event workers have no HTTP scope and already carry the organization on events.
func (s *Store) CheckConversation(ctx context.Context, org, id int64) error {
	scope, scoped := orgctx.From(ctx)
	if !scoped {
		return nil
	}
	p, ok := authctx.PrincipalFrom(ctx)
	if !ok || scope.ID != org || !access.CanRead(scope) {
		return pgx.ErrNoRows
	}
	viewer, err := access.For(ctx, s.Q, scope, p.UserInternal)
	if err != nil {
		return err
	}
	allowed, err := viewer.CanSee(ctx, s.Q, org, id)
	if err != nil {
		return err
	}
	if !allowed {
		return pgx.ErrNoRows
	}
	return nil
}
