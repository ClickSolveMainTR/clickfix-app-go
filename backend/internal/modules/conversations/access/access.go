// Package access is the team visibility rule of the inbox (CF-44,
// docs/design/tenant-core.md §9).
//
// A member holding tenant.conversations.read_all sees every conversation of
// the organization. A member with only tenant.conversations.read sees a
// conversation when one of these holds:
//
//   - it is assigned to the member (assignee_user_id),
//   - it is assigned to one of the member's teams (assignee_team_id),
//   - it is unrouted: no agent and no team (the shared queue new inbound
//     conversations land in),
//   - the member is a participant (added or @mentioned).
//
// The same rule is written once in SQL (queries/conversations.sql, the
// see_all / viewer_id / viewer_team_ids arguments) and applied by the inbox
// list, badges, detail and every per-conversation endpoint, the palette
// search and the realtime channel authorizers.
package access

import (
	"context"
	"errors"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Resource is the team-scoped resource key of orgctx.Scope.CanSeeAll.
const Resource = "conversations"

// Viewer is who reads the inbox.
type Viewer struct {
	UserID int64
	// All: the viewer holds tenant.conversations.read_all.
	All bool
	// TeamIDs are the viewer's teams (empty when All).
	TeamIDs []int64
}

// Queries is the slice of db.Queries the rule reads.
type Queries interface {
	ListTeamIDsForUser(ctx context.Context, arg db.ListTeamIDsForUserParams) ([]int64, error)
	ConversationVisibleTo(ctx context.Context, arg db.ConversationVisibleToParams) (bool, error)
	FilterVisibleConversationUUIDs(ctx context.Context, arg db.FilterVisibleConversationUUIDsParams) ([]uuid.UUID, error)
}

// For builds the viewer of a member in scope. A scope without
// tenant.conversations.read sees nothing (Viewer with no teams is still
// restricted; callers guard read with RequireTenantPermission first).
func For(ctx context.Context, q Queries, scope orgctx.Scope, userID int64) (Viewer, error) {
	v := Viewer{UserID: userID, TeamIDs: []int64{}}
	if scope.CanSeeAll(Resource) {
		v.All = true
		return v, nil
	}
	ids, err := q.ListTeamIDsForUser(ctx, db.ListTeamIDsForUserParams{OrganizationID: scope.ID, UserID: userID})
	if err != nil {
		return Viewer{}, err
	}
	if ids != nil {
		v.TeamIDs = ids
	}
	return v, nil
}

// CanRead reports whether scope may read conversations at all.
func CanRead(scope orgctx.Scope) bool {
	return scope.Has(rbac.PermTenantConversationsRead) || scope.CanSeeAll(Resource)
}

// Teams returns TeamIDs as a non-nil slice (sqlc array argument).
func (v Viewer) Teams() []int64 {
	if v.TeamIDs == nil {
		return []int64{}
	}
	return v.TeamIDs
}

// CanSee reports whether v may see the conversation conversationID of orgID.
func (v Viewer) CanSee(ctx context.Context, q Queries, orgID, conversationID int64) (bool, error) {
	if v.All {
		return true, nil
	}
	return q.ConversationVisibleTo(ctx, db.ConversationVisibleToParams{
		OrganizationID: orgID, ID: conversationID, SeeAll: false, ViewerID: v.UserID, ViewerTeamIds: v.Teams(),
	})
}

// Visible returns the subset of ids v may see (order not kept).
func (v Viewer) Visible(ctx context.Context, q Queries, orgID int64, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.FilterVisibleConversationUUIDs(ctx, db.FilterVisibleConversationUUIDsParams{
		OrganizationID: orgID, Uuids: ids, SeeAll: v.All, ViewerID: v.UserID, ViewerTeamIds: v.Teams(),
	})
	if err != nil {
		return nil, err
	}
	for _, id := range rows {
		out[id] = true
	}
	return out, nil
}

type ctxKey struct{}

// WithViewer stores the request's viewer. The inbox routes set it for every
// request (conversations.RegisterRoutes); usecase methods called without one
// (workers, inbound webhooks) are not restricted.
func WithViewer(ctx context.Context, v Viewer) context.Context {
	return context.WithValue(ctx, ctxKey{}, v)
}

// From returns the request's viewer.
func From(ctx context.Context) (Viewer, bool) {
	v, ok := ctx.Value(ctxKey{}).(Viewer)
	return v, ok
}

// AudienceQueries is the slice of db.Queries Audience reads.
type AudienceQueries interface {
	GetConversationByUUID(ctx context.Context, arg db.GetConversationByUUIDParams) (db.Conversation, error)
	ListConversationAudience(ctx context.Context, arg db.ListConversationAudienceParams) ([]uuid.UUID, error)
}

// Audience returns the members (user uuids) who see the conversation under
// the team rule: its assignee, its team's members, its participants, or
// every active member while it is unrouted. Realtime fans inbox events out
// to their inbox:{org}:{user} channels; members holding read_all listen on
// inbox:{org} and get a copy there too (they do not subscribe to theirs).
func Audience(ctx context.Context, q AudienceQueries, orgID int64, conversationUUID uuid.UUID) ([]uuid.UUID, error) {
	conv, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: conversationUUID})
	if err != nil {
		return nil, err
	}
	return q.ListConversationAudience(ctx, db.ListConversationAudienceParams{
		OrganizationID: orgID, AssigneeUserID: conv.AssigneeUserID, AssigneeTeamID: conv.AssigneeTeamID, ConversationID: conv.ID,
	})
}

// OwnerQueries is the slice of db.Queries AttachmentGuard reads.
type OwnerQueries interface {
	Queries
	GetMessageByUUID(ctx context.Context, arg db.GetMessageByUUIDParams) (db.Message, error)
	GetConversationByUUID(ctx context.Context, arg db.GetConversationByUUIDParams) (db.Conversation, error)
}

// Attachment owner types the inbox owns (attachments model.OwnerMessage /
// model.OwnerConversation; repeated here to keep the packages independent).
const (
	ownerMessage      = "message"
	ownerConversation = "conversation"
)

// AttachmentGuard applies the team rule to files linked to a message or a
// conversation: a member may read such a file only when it may see the
// conversation (docs/design/tenant-core.md §14). Files of other owners
// (tickets, invoices, unattached uploads) are left to their own permission.
type AttachmentGuard struct {
	Q OwnerQueries
}

// CanSeeOwner reports whether the member userID of scope may read a file
// owned by (ownerType, ownerID). A missing owner is hidden from restricted
// members (fail closed) and visible to read_all members.
func (g AttachmentGuard) CanSeeOwner(ctx context.Context, scope orgctx.Scope, userID int64, ownerType string, ownerID uuid.UUID) (bool, error) {
	if ownerType != ownerMessage && ownerType != ownerConversation {
		return true, nil
	}
	v, err := For(ctx, g.Q, scope, userID)
	if err != nil {
		return false, err
	}
	if v.All {
		return true, nil
	}
	var conversationID int64
	switch ownerType {
	case ownerMessage:
		m, err := g.Q.GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: scope.ID, Uuid: ownerID})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		conversationID = m.ConversationID
	default:
		c, err := g.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: scope.ID, Uuid: ownerID})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		conversationID = c.ID
	}
	return v.CanSee(ctx, g.Q, scope.ID, conversationID)
}
