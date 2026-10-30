package presence

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
)

// Authorizer grants org:{uuid} to members of the organization and
// conversation:{uuid} to members of the owning organization who can read
// conversations. Other channels go to Next (user:, inbox:, ...). Widget
// channels are never granted here: the widget module issues those tokens.
type Authorizer struct {
	Next     realtime.ChannelAuthorizer
	Resolver orgctx.Resolver
	Queries  *db.Queries
}

// CanSubscribeChannel implements realtime.ChannelAuthorizer.
func (a Authorizer) CanSubscribeChannel(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, channel string) (bool, error) {
	if orgUUID, ok := realtime.ParseOrgChannel(channel); ok {
		_, member := a.scope(ctx, userUUID, orgUUID)
		return member, nil
	}
	if convUUID, ok := realtime.ParseConversationChannel(channel); ok {
		if a.Queries == nil {
			return false, nil
		}
		owner, err := a.Queries.GetConversationOrganization(ctx, convUUID)
		if err != nil {
			return false, nil
		}
		scope, member := a.scope(ctx, userUUID, owner.OrganizationUuid)
		if !member || !access.CanRead(scope) {
			return false, nil
		}
		return a.canSee(ctx, scope, userUUID, convUUID)
	}
	if a.Next == nil {
		return false, nil
	}
	return a.Next.CanSubscribeChannel(ctx, userUUID, isSuperAdmin, channel)
}

// canSee applies the team visibility rule (CF-44) to the conversation.
func (a Authorizer) canSee(ctx context.Context, scope orgctx.Scope, userUUID, convUUID uuid.UUID) (bool, error) {
	user, err := a.Queries.GetUserByUUID(ctx, userUUID)
	if err != nil {
		return false, nil
	}
	v, err := access.For(ctx, a.Queries, scope, user.ID)
	if err != nil {
		return false, err
	}
	conv, err := a.Queries.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: scope.ID, Uuid: convUUID})
	if err != nil {
		return false, nil
	}
	return v.CanSee(ctx, a.Queries, scope.ID, conv.ID)
}

func (a Authorizer) scope(ctx context.Context, userUUID, orgUUID uuid.UUID) (orgctx.Scope, bool) {
	if a.Resolver == nil || a.Queries == nil {
		return orgctx.Scope{}, false
	}
	user, err := a.Queries.GetUserByUUID(ctx, userUUID)
	if err != nil {
		return orgctx.Scope{}, false
	}
	scope, err := a.Resolver.Resolve(ctx, orgUUID.String(), user.ID)
	if err != nil {
		return orgctx.Scope{}, false
	}
	return scope, true
}
