package remotesupport

import (
	"context"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
)

// Authorizer grants remote_support:{session} to the session's agent and to
// remote support managers of the session's organization (both holding
// tenant.remote_support.use there). Other channels go to Next.
type Authorizer struct {
	Next     realtime.ChannelAuthorizer
	Resolver orgctx.Resolver
	Queries  *db.Queries
}

func (a Authorizer) CanSubscribeChannel(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, channel string) (bool, error) {
	raw, ok := strings.CutPrefix(channel, ChannelPrefix)
	if !ok {
		if a.Next == nil {
			return false, nil
		}
		return a.Next.CanSubscribeChannel(ctx, userUUID, isSuperAdmin, channel)
	}
	id, err := uuid.Parse(raw)
	if err != nil || a.Queries == nil || a.Resolver == nil {
		return false, nil
	}
	row, err := a.Queries.RemoteSupportSessionByUUID(ctx, id)
	if err != nil {
		return false, nil
	}
	user, err := a.Queries.GetUserByUUID(ctx, userUUID)
	if err != nil {
		return false, nil
	}
	o, err := a.Queries.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return false, nil
	}
	scope, err := a.Resolver.Resolve(ctx, o.Uuid.String(), user.ID)
	if err != nil || scope.ID != row.OrganizationID || !scope.Has(rbac.PermTenantRemoteSupportUse) {
		return false, nil
	}
	mine := row.AgentUserID.Valid && row.AgentUserID.Int64 == user.ID
	return mine || scope.Has(rbac.PermTenantRemoteSupportSettings), nil
}
