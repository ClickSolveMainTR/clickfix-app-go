package mcp

import (
	"context"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
	authuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

var ErrForbidden = errors.New("mcp: forbidden")

type Principal struct {
	Layer    string
	UserID   int64
	UserUUID uuid.UUID
	Scope    *orgctx.Scope
	CanWrite bool
}
type Resolver struct {
	Q     *db.Queries
	Orgs  orgctx.Resolver
	Tools *Catalog
}

func (r Resolver) principal(ctx context.Context, sub model.Subject) (Principal, error) {
	if r.Orgs == nil || sub.UserID <= 0 || sub.OrganizationID <= 0 {
		return Principal{}, ErrForbidden
	}
	user, err := r.Q.GetUserByID(ctx, sub.UserID)
	if err != nil {
		return Principal{}, err
	}
	if user.Status != "active" || user.DeletedAt.Valid {
		return Principal{}, ErrForbidden
	}
	org, err := r.Q.GetOrganizationByID(ctx, sub.OrganizationID)
	if err != nil {
		return Principal{}, err
	}
	scope, err := r.Orgs.Resolve(ctx, org.Uuid.String(), sub.UserID)
	if err != nil {
		return Principal{}, fmt.Errorf("%w: organization access", ErrForbidden)
	}
	if !scope.Has(rbac.PermTenantMCPConnect) {
		return Principal{}, ErrForbidden
	}
	return Principal{Layer: model.LayerBusiness, UserID: user.ID, UserUUID: user.Uuid, Scope: &scope, CanWrite: scope.Has(rbac.PermTenantMCPWrite)}, nil
}
func (r Resolver) Resolve(ctx context.Context, t model.AccessToken) (Principal, error) {
	if t.Resource != model.ResourceBusiness {
		return Principal{}, ErrForbidden
	}
	p, err := r.principal(ctx, t.Subject)
	p.CanWrite = p.CanWrite && model.HasScope(t.Scopes, model.ScopeWrite)
	return p, err
}
func (r Resolver) Check(ctx context.Context, layer string, sub model.Subject) (authuc.GateResult, error) {
	if layer != model.LayerBusiness {
		return authuc.GateResult{}, nil
	}
	p, err := r.principal(ctx, sub)
	if errors.Is(err, ErrForbidden) {
		return authuc.GateResult{}, nil
	}
	if err != nil {
		return authuc.GateResult{}, err
	}
	return authuc.GateResult{Allowed: true, CanWrite: p.CanWrite, ToolCount: len(r.Tools.Available(p))}, nil
}
