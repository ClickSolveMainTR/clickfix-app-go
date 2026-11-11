package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/screenctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// API-owned attachment paths (same as the attachments module).
const (
	attachmentDownloadPath  = "/v1/tenant/attachments/%s/download"
	attachmentThumbnailPath = "/v1/tenant/attachments/%s/download?variant=thumb"
)

// Page limits.
const (
	MaxListLimit     = 100
	MaxMessagesLimit = 100
)

// Actor is the agent doing something in the inbox.
type Actor struct {
	ID   int64
	UUID uuid.UUID
	Name string
}

// Named fills the actor's display name.
func (s *Service) Named(ctx context.Context, a Actor) Actor {
	if a.Name != "" || a.ID == 0 {
		return a
	}
	if u, err := s.store.Q().GetUserByID(ctx, a.ID); err == nil {
		a.Name = strings.TrimSpace(u.Name + " " + u.Surname)
	}
	return a
}

// MentionNotifier tells agents they were mentioned or assigned.
type MentionNotifier interface {
	Mentioned(ctx context.Context, orgID int64, userIDs []int64, by Actor, conv db.Conversation, preview string)
	Assigned(ctx context.Context, orgID int64, userID int64, by Actor, conv db.Conversation)
}

// WithRegistry lets the inbox read channel capabilities (reply window).
func (s *Service) WithRegistry(r *channels.Registry) *Service { s.registry = r; return s }

// ResolveGuard can refuse resolving a conversation (CF-52: request form
// fields required before resolving). It runs in the update transaction.
type ResolveGuard interface {
	CheckResolve(ctx context.Context, conv db.Conversation) error
}

// WithResolveGuard sets the resolve guard.
func (s *Service) WithResolveGuard(g ResolveGuard) *Service { s.resolveGuard = g; return s }

// WithMentions sets the mention / assignment notifier.
func (s *Service) WithMentions(m MentionNotifier) *Service { s.mentions = m; return s }

// ListFilter filters the inbox list.
type ListFilter struct {
	BrandID      *uuid.UUID
	Statuses     []string
	Priorities   []string
	ChannelTypes []string
	Inboxes      []uuid.UUID
	// Assignee: "" any, "me", "unassigned" or an agent uuid.
	Assignee  string
	Tags      []uuid.UUID
	Contact   *uuid.UUID
	Company   *uuid.UUID
	Mentioned bool
	// Team filters by one team; MyTeams by the viewer's teams.
	Team        *uuid.UUID
	MyTeams     bool
	Unread      bool
	SLABreached *bool
	// SLADueWithin keeps open conversations whose next SLA target is due
	// within this duration (and not breached yet).
	SLADueWithin time.Duration
	// IdentityState keeps conversations whose contact is verified,
	// anonymous or self_named (CF-195); "" for any.
	IdentityState string
	Q             string
	Sort          string
	Desc          bool
	Limit         int32
	Offset        int32
}

// List returns one page of the inbox for viewer.
func (s *Service) List(ctx context.Context, orgID, viewerID int64, f ListFilter) ([]model.Conversation, int64, error) {
	q := s.store.Q()
	v := viewerOf(ctx, viewerID)
	p := db.ListConversationsParams{
		BrandID: nullableUUID(f.BrandID), ViewerID: viewerID, OrganizationID: orgID, SeeAll: v.All, ViewerTeamIds: v.Teams(), Statuses: nonNilStrings(f.Statuses), Priorities: nonNilStrings(f.Priorities),
		ChannelTypes: nonNilStrings(f.ChannelTypes), ChannelInstanceIds: []int64{}, TagIds: []int64{},
		Mentioned: f.Mentioned, UnreadOnly: f.Unread, SortField: f.Sort, SortDesc: f.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	}
	if f.IdentityState != "" {
		if !model.ValidIdentityState(f.IdentityState) {
			return nil, 0, fmt.Errorf("%w: unknown identity state %q", ErrInvalid, f.IdentityState)
		}
		p.IdentityState = pgText(f.IdentityState)
	}
	for _, st := range p.Statuses {
		if !model.Valid(st, model.Statuses) {
			return nil, 0, fmt.Errorf("%w: unknown status %q", ErrInvalid, st)
		}
	}
	for _, pr := range p.Priorities {
		if !model.Valid(pr, model.Priorities) {
			return nil, 0, fmt.Errorf("%w: unknown priority %q", ErrInvalid, pr)
		}
	}
	switch p.SortField {
	case "", "last_message_at":
		p.SortField = "last_message_at"
	case "created_at", "waiting_since", "priority":
	default:
		return nil, 0, fmt.Errorf("%w: unknown sort %q", ErrInvalid, p.SortField)
	}
	if p.RowLimit <= 0 || p.RowLimit > MaxListLimit {
		p.RowLimit = 50
	}
	if p.RowOffset < 0 {
		p.RowOffset = 0
	}
	switch f.Assignee {
	case "", "me", "unassigned":
		p.Assignee = f.Assignee
	default:
		id, err := uuid.Parse(f.Assignee)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: assignee must be me, unassigned or a user uuid", ErrInvalid)
		}
		agent, err := q.GetOrganizationAgentByUUID(ctx, db.GetOrganizationAgentByUUIDParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return []model.Conversation{}, 0, nil
		}
		p.Assignee, p.AssigneeUserID = "user", pgInt8(agent.ID)
	}
	p.MyTeams = f.MyTeams
	if f.Team != nil {
		t, err := q.GetTeamRefByUUID(ctx, db.GetTeamRefByUUIDParams{OrganizationID: orgID, Uuid: *f.Team})
		if err != nil {
			return []model.Conversation{}, 0, nil
		}
		p.TeamID = pgInt8(t.ID)
	}
	if len(f.Inboxes) > 0 {
		rows, err := q.GetChannelInstancesByUUIDs(ctx, db.GetChannelInstancesByUUIDsParams{OrganizationID: orgID, Uuids: f.Inboxes})
		if err != nil {
			return nil, 0, err
		}
		if len(rows) == 0 {
			return []model.Conversation{}, 0, nil
		}
		for _, r := range rows {
			p.ChannelInstanceIds = append(p.ChannelInstanceIds, r.ID)
		}
	}
	if len(f.Tags) > 0 {
		rows, err := q.GetTagsByUUIDs(ctx, db.GetTagsByUUIDsParams{OrganizationID: orgID, Uuids: f.Tags})
		if err != nil {
			return nil, 0, err
		}
		if len(rows) == 0 {
			return []model.Conversation{}, 0, nil
		}
		for _, r := range rows {
			p.TagIds = append(p.TagIds, r.ID)
		}
	}
	if f.Contact != nil {
		c, err := q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: orgID, Uuid: *f.Contact})
		if err != nil {
			return []model.Conversation{}, 0, nil
		}
		p.ContactID = pgInt8(c.ID)
	}
	if f.Company != nil {
		c, err := q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: *f.Company})
		if err != nil {
			return []model.Conversation{}, 0, nil
		}
		p.CompanyID = pgInt8(c.ID)
	}
	if f.SLABreached != nil {
		p.SlaBreached = pgtype.Bool{Bool: *f.SLABreached, Valid: true}
	}
	if f.SLADueWithin > 0 {
		p.SlaDueBefore = pgTime(s.now().Add(f.SLADueWithin))
	}
	p.MatchUuids = []uuid.UUID{}
	var matches map[uuid.UUID]SearchMatch
	if qs := strings.TrimSpace(f.Q); qs != "" {
		p.Q = pgText(strings.TrimPrefix(qs, "#"))
		matches = s.searchMatches(ctx, orgID, qs)
		for id := range matches {
			p.MatchUuids = append(p.MatchUuids, id)
		}
	}
	rows, err := q.ListConversations(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountConversations(ctx, db.CountConversationsParams{
		BrandID: p.BrandID, ViewerID: p.ViewerID, OrganizationID: p.OrganizationID, SeeAll: p.SeeAll, ViewerTeamIds: p.ViewerTeamIds, Statuses: p.Statuses, Priorities: p.Priorities,
		ChannelTypes: p.ChannelTypes, ChannelInstanceIds: p.ChannelInstanceIds, Assignee: p.Assignee,
		AssigneeUserID: p.AssigneeUserID, TeamID: p.TeamID, ContactID: p.ContactID, CompanyID: p.CompanyID,
		TagIds: p.TagIds, Mentioned: p.Mentioned, MyTeams: p.MyTeams, UnreadOnly: p.UnreadOnly, SlaBreached: p.SlaBreached, SlaDueBefore: p.SlaDueBefore, Q: p.Q,
		MatchUuids: p.MatchUuids, IdentityState: p.IdentityState,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Conversation, 0, len(rows))
	for _, r := range rows {
		c := s.toConversation(r.Conversation, int(r.UnreadCount), r.TagUuids)
		c.Contact = contactRef(r.ContactUuid, r.ContactName, r.ContactEmail, r.ContactPhone, r.ContactExternalID, r.ContactVerifiedAt)
		c.Company = companyRef(r.CompanyUuid, r.CompanyName.String, r.CompanyExternalID)
		c.Inbox = ref(r.ChannelInstanceUuid, r.ChannelInstanceName.String)
		c.Assignee = ref(r.AssigneeUuid, fullName(r.AssigneeName, r.AssigneeSurname))
		c.Team = ref(r.TeamUuid, r.TeamName.String)
		if m, ok := matches[c.UUID]; ok && m.Snippet != "" {
			c.SearchMatch = &model.SearchMatch{Snippet: m.Snippet, Private: m.Private}
		}
		out = append(out, c)
	}
	return out, total, nil
}

// searchMatches asks the search index which conversations match qs. It
// fails soft: without a finder or on an index error the list falls back to
// the database match alone.
func (s *Service) searchMatches(ctx context.Context, orgID int64, qs string) map[uuid.UUID]SearchMatch {
	if s.finder == nil {
		return nil
	}
	found, err := s.finder.MatchConversations(ctx, orgID, qs)
	if err != nil {
		s.log.Warn("inbox_search_index_failed", "org", orgID, "error", err)
		return nil
	}
	out := make(map[uuid.UUID]SearchMatch, len(found))
	for _, m := range found {
		if m.ConversationUUID == uuid.Nil {
			continue
		}
		// First (most relevant) match wins; keep a snippet if a later one has it.
		if prev, ok := out[m.ConversationUUID]; ok {
			if prev.Snippet == "" && m.Snippet != "" {
				out[m.ConversationUUID] = m
			}
			continue
		}
		out[m.ConversationUUID] = m
	}
	return out
}

// Participant is an agent following a conversation.
type Participant struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Role  string    `json:"role"`
}

// Tag is an organization tag.
type Tag struct {
	UUID       uuid.UUID `json:"uuid"`
	Name       string    `json:"name"`
	Color      string    `json:"color"`
	UsageCount int64     `json:"usage_count"`
}

// Detail is one conversation with its tags and participants.
type Detail struct {
	model.Conversation
	Tags         []Tag         `json:"tags"`
	Participants []Participant `json:"participants"`
	// ScreenContext is the page and events the widget sent with the latest
	// inbound message: "hangi ekrandan yazdı" (CF-197). Null when none.
	ScreenContext *screenctx.Screen `json:"screen_context"`
}

// Get returns one conversation for viewer.
func (s *Service) Get(ctx context.Context, orgID, viewerID int64, id uuid.UUID) (Detail, error) {
	q := s.store.Q()
	r, err := q.GetConversationView(ctx, db.GetConversationViewParams{ViewerID: viewerID, OrganizationID: orgID, Uuid: id})
	if err != nil {
		return Detail{}, notFound(err)
	}
	if err := s.mustSee(ctx, orgID, r.Conversation.ID); err != nil {
		return Detail{}, err
	}
	c := s.toConversation(r.Conversation, int(r.UnreadCount), r.TagUuids)
	c.Contact = contactRef(r.ContactUuid, r.ContactName, r.ContactEmail, r.ContactPhone, r.ContactExternalID, r.ContactVerifiedAt)
	c.Company = companyRef(r.CompanyUuid, r.CompanyName.String, r.CompanyExternalID)
	c.Inbox = ref(r.ChannelInstanceUuid, r.ChannelInstanceName.String)
	c.Assignee = ref(r.AssigneeUuid, fullName(r.AssigneeName, r.AssigneeSurname))
	c.Team = ref(r.TeamUuid, r.TeamName.String)
	c.CustomStatus = ref(r.StatusUuid, r.StatusName.String)
	d := Detail{Conversation: c, Tags: []Tag{}, Participants: []Participant{}}
	tags, err := q.ListConversationTags(ctx, db.ListConversationTagsParams{OrganizationID: orgID, ConversationID: r.Conversation.ID})
	if err != nil {
		return Detail{}, err
	}
	for _, t := range tags {
		d.Tags = append(d.Tags, Tag{UUID: t.Uuid, Name: t.Name, Color: t.Color})
	}
	parts, err := q.ListConversationParticipants(ctx, db.ListConversationParticipantsParams{OrganizationID: orgID, ConversationID: r.Conversation.ID})
	if err != nil {
		return Detail{}, err
	}
	for _, p := range parts {
		d.Participants = append(d.Participants, Participant{UUID: p.UserUuid, Name: strings.TrimSpace(p.UserName + " " + p.UserSurname), Email: p.UserEmail, Role: p.Role})
	}
	screen, err := q.ConversationScreenContext(ctx, db.ConversationScreenContextParams{OrganizationID: orgID, ConversationID: r.Conversation.ID})
	switch {
	case err == nil:
		d.ScreenContext = screenctx.Parse(screen.PageUrl, screen.Events, screen.CreatedAt.Time)
	case !errors.Is(err, pgx.ErrNoRows):
		return Detail{}, err
	}
	return d, nil
}

// MessagePage is a page of messages, oldest first; Before pages back.
type MessagePage struct {
	Items   []model.Message `json:"items"`
	HasMore bool            `json:"has_more"`
	Before  *uuid.UUID      `json:"before"`
}

// Messages returns the newest messages of a conversation before the given
// message (uuid.Nil = the newest), oldest first.
func (s *Service) Messages(ctx context.Context, orgID int64, convID uuid.UUID, before uuid.UUID, limit int32) (MessagePage, error) {
	q := s.store.Q()
	conv, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: convID})
	if err != nil {
		return MessagePage{}, notFound(err)
	}
	if err := s.mustSee(ctx, orgID, conv.ID); err != nil {
		return MessagePage{}, err
	}
	if limit <= 0 || limit > MaxMessagesLimit {
		limit = 50
	}
	var beforeID int64
	if before != uuid.Nil {
		m, err := q.GetMessageByUUID(ctx, db.GetMessageByUUIDParams{OrganizationID: orgID, Uuid: before})
		if err != nil || m.ConversationID != conv.ID {
			return MessagePage{}, fmt.Errorf("%w: before is not a message of this conversation", ErrInvalid)
		}
		beforeID = m.ID
	}
	rows, err := q.ListConversationMessages(ctx, db.ListConversationMessagesParams{
		OrganizationID: orgID, ConversationID: conv.ID, BeforeID: beforeID, RowLimit: limit + 1,
	})
	if err != nil {
		return MessagePage{}, err
	}
	page := MessagePage{Items: []model.Message{}}
	if len(rows) > int(limit) {
		page.HasMore = true
		rows = rows[:limit]
	}
	ids := make([]int64, 0, len(rows))
	uuids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
		uuids = append(uuids, r.Uuid)
	}
	files, mentions, err := s.messageExtras(ctx, orgID, ids, uuids)
	if err != nil {
		return MessagePage{}, err
	}
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		m := toMessage(messageOf(r), conv.Uuid, senderOf(r))
		m.ReplyTo = uuidPtr(r.ReplyToUuid)
		if f, ok := files[r.Uuid]; ok {
			m.Attachments = f
		}
		if mm, ok := mentions[r.ID]; ok {
			m.Mentions = mm
		}
		page.Items = append(page.Items, m)
	}
	if page.HasMore && len(page.Items) > 0 {
		oldest := page.Items[0].UUID
		page.Before = &oldest
	}
	return page, nil
}

func (s *Service) messageExtras(ctx context.Context, orgID int64, ids []int64, uuids []uuid.UUID) (map[uuid.UUID][]model.MessageAttachment, map[int64][]model.Ref, error) {
	files := map[uuid.UUID][]model.MessageAttachment{}
	mentions := map[int64][]model.Ref{}
	if len(ids) == 0 {
		return files, mentions, nil
	}
	q := s.store.Q()
	atts, err := q.ListMessageAttachments(ctx, db.ListMessageAttachmentsParams{OrganizationID: orgID, MessageUuids: uuids})
	if err != nil {
		return nil, nil, err
	}
	for _, a := range atts {
		if !a.OwnerID.Valid {
			continue
		}
		owner := uuid.UUID(a.OwnerID.Bytes)
		ma := model.MessageAttachment{
			UUID: a.Uuid, Filename: a.Filename, ContentType: a.ContentType, SizeBytes: a.SizeBytes,
			DownloadURL: fmt.Sprintf(attachmentDownloadPath, a.Uuid),
		}
		if a.HasThumbnail {
			thumb := fmt.Sprintf(attachmentThumbnailPath, a.Uuid)
			ma.ThumbnailURL = &thumb
		}
		files[owner] = append(files[owner], ma)
	}
	ms, err := q.ListMessageMentions(ctx, db.ListMessageMentionsParams{OrganizationID: orgID, MessageIds: ids})
	if err != nil {
		return nil, nil, err
	}
	for _, m := range ms {
		mentions[m.MessageID] = append(mentions[m.MessageID], model.Ref{UUID: m.UserUuid, Name: strings.TrimSpace(m.UserName + " " + m.UserSurname)})
	}
	return files, mentions, nil
}

// MarkRead moves viewer's read marker to the newest message.
func (s *Service) MarkRead(ctx context.Context, orgID, viewerID int64, convID uuid.UUID) error {
	q := s.store.Q()
	conv, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: convID})
	if err != nil {
		return notFound(err)
	}
	if err := s.mustSee(ctx, orgID, conv.ID); err != nil {
		return err
	}
	last, err := q.LatestMessageID(ctx, db.LatestMessageIDParams{OrganizationID: orgID, ConversationID: conv.ID})
	if err != nil {
		return err
	}
	return q.MarkConversationRead(ctx, db.MarkConversationReadParams{OrganizationID: orgID, ConversationID: conv.ID, UserID: viewerID, MessageID: last})
}

// Counts are the inbox badges of viewer.
type Counts struct {
	Open       int64            `json:"open"`
	Mine       int64            `json:"mine"`
	Unassigned int64            `json:"unassigned"`
	Snoozed    int64            `json:"snoozed"`
	Breached   int64            `json:"breached"`
	ByStatus   map[string]int64 `json:"by_status"`
}

// Counts returns the inbox badges.
func (s *Service) Counts(ctx context.Context, orgID, viewerID int64) (Counts, error) {
	q := s.store.Q()
	v := viewerOf(ctx, viewerID)
	c, err := q.ConversationInboxCounts(ctx, db.ConversationInboxCountsParams{
		ViewerID: viewerID, OrganizationID: orgID, SeeAll: v.All, ViewerTeamIds: v.Teams(),
	})
	if err != nil {
		return Counts{}, err
	}
	out := Counts{Open: c.OpenTotal, Mine: c.Mine, Unassigned: c.Unassigned, Snoozed: c.Snoozed, Breached: c.Breached, ByStatus: map[string]int64{}}
	for _, st := range model.Statuses {
		out.ByStatus[st] = 0
	}
	facets, err := q.ConversationStatusFacets(ctx, db.ConversationStatusFacetsParams{
		OrganizationID: orgID, SeeAll: v.All, ViewerID: viewerID, ViewerTeamIds: v.Teams(),
	})
	if err != nil {
		return Counts{}, err
	}
	for _, f := range facets {
		out.ByStatus[f.Status] = f.Count
	}
	return out, nil
}

// Agent is an assignable organization member.
type Agent struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

// Agents lists the organization's assignable members.
func (s *Service) Agents(ctx context.Context, orgID int64) ([]Agent, error) {
	rows, err := s.store.Q().ListOrganizationAgents(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(rows))
	for _, r := range rows {
		out = append(out, Agent{UUID: r.Uuid, Name: strings.TrimSpace(r.Name + " " + r.Surname), Email: r.Email})
	}
	return out, nil
}

// AgentIDs maps agent uuids of the organization to user ids (unknown ones dropped).
func (s *Service) AgentIDs(ctx context.Context, orgID int64, ids []uuid.UUID) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.store.Q().ListAgentsByUUIDs(ctx, db.ListAgentsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out, nil
}

// UpdateInput changes the state of a conversation. Nil fields stay.
type UpdateInput struct {
	OrganizationID   int64
	ConversationUUID uuid.UUID
	Actor            Actor
	Status           *string
	Priority         *string
	Subject          *string
	// Assignee: set to change; a nil inner pointer unassigns.
	Assignee **uuid.UUID
	// Team: set to change; a nil inner pointer removes the team.
	Team         **uuid.UUID
	SnoozedUntil *time.Time
}

// Update applies status, priority, subject and assignment changes. Every
// change is written to the timeline as a system event and published as a
// domain event.
func (s *Service) Update(ctx context.Context, in UpdateInput) (db.Conversation, error) {
	var newAssignee int64
	if in.Assignee != nil && *in.Assignee != nil {
		a, err := s.store.Q().GetOrganizationAgentByUUID(ctx, db.GetOrganizationAgentByUUIDParams{OrganizationID: in.OrganizationID, Uuid: **in.Assignee})
		if err != nil {
			return db.Conversation{}, fmt.Errorf("%w: assignee is not a member of this organization", ErrInvalid)
		}
		newAssignee = a.ID
	}
	var newTeam pgtype.Int8
	var newTeamName string
	if in.Team != nil && *in.Team != nil {
		t, err := s.store.Q().GetTeamRefByUUID(ctx, db.GetTeamRefByUUIDParams{OrganizationID: in.OrganizationID, Uuid: **in.Team})
		if err != nil {
			return db.Conversation{}, fmt.Errorf("%w: team not found", ErrInvalid)
		}
		newTeam, newTeamName = pgInt8(t.ID), t.Name
	}
	if in.Status != nil && !model.Valid(*in.Status, model.Statuses) {
		return db.Conversation{}, fmt.Errorf("%w: unknown status", ErrInvalid)
	}
	if in.Priority != nil && !model.Valid(*in.Priority, model.Priorities) {
		return db.Conversation{}, fmt.Errorf("%w: unknown priority", ErrInvalid)
	}
	if in.Status != nil && *in.Status == model.StatusSnoozed {
		if in.SnoozedUntil == nil || !in.SnoozedUntil.After(s.now()) {
			return db.Conversation{}, fmt.Errorf("%w: snoozed_until must be in the future", ErrInvalid)
		}
	}
	var out db.Conversation
	var timeline []db.Message
	var previous []uuid.UUID
	assignedTo := int64(0)
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		cur, err := tx.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: in.OrganizationID, Uuid: in.ConversationUUID})
		if err != nil {
			return err
		}
		if err := s.mustSee(ctx, in.OrganizationID, cur.ID); err != nil {
			return err
		}
		cur, err = tx.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: in.OrganizationID, ID: cur.ID})
		if err != nil {
			return err
		}
		if in.Assignee != nil {
			// Who sees it now: told about the change even when the new
			// assignee's audience leaves them out (team visibility, CF-44).
			if previous, err = access.Audience(ctx, tx.Q, in.OrganizationID, cur.Uuid); err != nil {
				return err
			}
		}
		next := db.UpdateConversationStateParams{
			Subject: cur.Subject, Status: cur.Status, StatusID: cur.StatusID, Priority: cur.Priority,
			AssigneeUserID: cur.AssigneeUserID, AssigneeTeamID: cur.AssigneeTeamID, CompanyID: cur.CompanyID,
			CustomFields: cur.CustomFields, SnoozedUntil: cur.SnoozedUntil, ResolvedAt: cur.ResolvedAt, ClosedAt: cur.ClosedAt,
			OrganizationID: in.OrganizationID, ID: cur.ID,
		}
		type change struct {
			event, from, to string
			name            string
		}
		var changes []change
		if in.Subject != nil && strings.TrimSpace(*in.Subject) != cur.Subject {
			next.Subject = truncate(*in.Subject, 500)
			changes = append(changes, change{event: "subject_changed", from: cur.Subject, to: next.Subject, name: events.ConversationsUpdated})
		}
		if in.Priority != nil && *in.Priority != cur.Priority {
			next.Priority = *in.Priority
			changes = append(changes, change{event: "priority_changed", from: cur.Priority, to: *in.Priority, name: events.ConversationsUpdated})
		}
		if in.Status != nil && (*in.Status != cur.Status || *in.Status == model.StatusSnoozed) {
			now := s.now()
			next.Status, next.StatusID = *in.Status, pgtype.Int8{}
			next.SnoozedUntil = pgtype.Timestamptz{}
			switch *in.Status {
			case model.StatusSnoozed:
				next.SnoozedUntil = pgTime(*in.SnoozedUntil)
			case model.StatusResolved:
				next.ResolvedAt = pgTime(now)
			case model.StatusClosed:
				next.ClosedAt = pgTime(now)
				if !next.ResolvedAt.Valid {
					next.ResolvedAt = pgTime(now)
				}
			default:
				next.ResolvedAt, next.ClosedAt = pgtype.Timestamptz{}, pgtype.Timestamptz{}
			}
			done := *in.Status == model.StatusResolved || *in.Status == model.StatusClosed
			wasDone := cur.Status == model.StatusResolved || cur.Status == model.StatusClosed
			if done && !wasDone && s.resolveGuard != nil {
				if err := s.resolveGuard.CheckResolve(ctx, cur); err != nil {
					return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
				}
			}
			name := events.ConversationsUpdated
			switch {
			case done:
				name = events.ConversationsResolved
			case cur.Status == model.StatusResolved || cur.Status == model.StatusClosed:
				name = events.ConversationsReopened
			}
			to := *in.Status
			if *in.Status == model.StatusSnoozed {
				to = *in.Status + "|" + in.SnoozedUntil.UTC().Format(time.RFC3339)
			}
			changes = append(changes, change{event: "status_changed", from: cur.Status, to: to, name: name})
		}
		if in.Assignee != nil {
			var target pgtype.Int8
			if newAssignee > 0 {
				target = pgInt8(newAssignee)
			}
			if target != cur.AssigneeUserID {
				next.AssigneeUserID = target
				changes = append(changes, change{event: "assigned", from: s.agentName(ctx, tx, cur.AssigneeUserID), to: s.agentName(ctx, tx, target), name: events.ConversationsAssigned})
				if newAssignee > 0 && newAssignee != in.Actor.ID {
					assignedTo = newAssignee
				}
				if newAssignee > 0 {
					if _, err := tx.Q.AddConversationParticipant(ctx, db.AddConversationParticipantParams{
						OrganizationID: in.OrganizationID, ConversationID: cur.ID, UserID: newAssignee, Role: model.ParticipantCollaborator, AddedBy: pgInt8(in.Actor.ID),
					}); err != nil {
						return err
					}
				}
			}
		}
		if in.Team != nil && newTeam != cur.AssigneeTeamID {
			from := ""
			if cur.AssigneeTeamID.Valid {
				if t, err := tx.Q.GetTeamRefByID(ctx, db.GetTeamRefByIDParams{OrganizationID: in.OrganizationID, ID: cur.AssigneeTeamID.Int64}); err == nil {
					from = t.Name
				}
			}
			next.AssigneeTeamID = newTeam
			changes = append(changes, change{event: "team_assigned", from: from, to: newTeamName, name: events.ConversationsAssigned})
		}
		if len(changes) == 0 {
			out = cur
			return nil
		}
		out, err = tx.Q.UpdateConversationState(ctx, next)
		if err != nil {
			return err
		}
		for _, ch := range changes {
			msg, err := s.timelineEvent(ctx, tx, in.OrganizationID, out.ID, in.Actor, ch.event, ch.from, ch.to)
			if err != nil {
				return err
			}
			timeline = append(timeline, msg)
			if err := s.emit(ctx, tx, ch.name, in.OrganizationID, in.Actor.ID, out, map[string]any{
				"change": ch.event, "from": ch.from, "to": ch.to,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return db.Conversation{}, notFound(err)
	}
	if len(timeline) > 0 {
		s.publishTimeline(ctx, in.OrganizationID, out, timeline, previous...)
	}
	s.indexed(ctx, out.Uuid, uuid.Nil, uuid.Nil)
	if assignedTo > 0 && s.mentions != nil {
		s.mentions.Assigned(ctx, in.OrganizationID, assignedTo, in.Actor, out)
	}
	return out, nil
}

func (s *Service) agentName(ctx context.Context, tx *repository.Tx, id pgtype.Int8) string {
	if !id.Valid {
		return ""
	}
	var name, surname string
	if err := tx.Raw.QueryRow(ctx, "SELECT name, surname FROM users WHERE id = $1", id.Int64).Scan(&name, &surname); err != nil {
		return ""
	}
	return strings.TrimSpace(name + " " + surname)
}

// timelineEvent writes a system event message (never sent, never unread).
func (s *Service) timelineEvent(ctx context.Context, tx *repository.Tx, orgID, convID int64, actor Actor, event, from, to string) (db.Message, error) {
	attrs, err := json.Marshal(map[string]any{"event": event, "from": from, "to": to, "actor_uuid": actor.UUID, "actor_name": actor.Name})
	if err != nil {
		return db.Message{}, err
	}
	app, err := tx.AppendMessage(ctx, repository.NewMessage{
		OrganizationID: orgID, ConversationID: convID, Direction: model.DirectionInternal, SenderType: model.SenderSystem,
		SenderUserID: actor.ID, Kind: model.KindEvent, Attributes: attrs, At: s.now(),
	})
	return app.Message, err
}

func (s *Service) publishTimeline(ctx context.Context, orgID int64, conv db.Conversation, msgs []db.Message, also ...uuid.UUID) {
	s.notify(ctx, orgID, model.LiveEvent{Type: model.EventConversationUpdated, ConversationUUID: conv.Uuid, Also: also})
	for _, m := range msgs {
		msg := toMessage(m, conv.Uuid, model.Sender{Type: model.SenderSystem})
		s.notify(ctx, orgID, model.LiveEvent{Type: model.EventMessageCreated, ConversationUUID: conv.Uuid, Message: &msg})
	}
}

// ---------------------------------------------------------------------------
// Tags

// ErrTagExists: another tag has the name.
var ErrTagExists = errors.New("conversations: tag name already used")

// Tags lists the organization's tags with usage counts.
func (s *Service) Tags(ctx context.Context, orgID int64) ([]Tag, error) {
	rows, err := s.store.Q().ListTags(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]Tag, 0, len(rows))
	for _, r := range rows {
		out = append(out, Tag{UUID: r.Uuid, Name: r.Name, Color: r.Color, UsageCount: r.UsageCount})
	}
	return out, nil
}

func validTag(name, color string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return "", "", fmt.Errorf("%w: tag name must be 1-64 characters", ErrInvalid)
	}
	color = strings.TrimSpace(color)
	if len(color) > 32 {
		return "", "", fmt.Errorf("%w: color is too long", ErrInvalid)
	}
	return name, color, nil
}

// CreateTag creates (or returns the existing) tag of a name.
func (s *Service) CreateTag(ctx context.Context, orgID int64, name, color string) (Tag, error) {
	name, color, err := validTag(name, color)
	if err != nil {
		return Tag{}, err
	}
	t, err := s.store.Q().UpsertTag(ctx, db.UpsertTagParams{OrganizationID: orgID, Name: name, Color: color})
	if err != nil {
		return Tag{}, err
	}
	return Tag{UUID: t.Uuid, Name: t.Name, Color: t.Color}, nil
}

// UpdateTag renames or recolors a tag.
func (s *Service) UpdateTag(ctx context.Context, orgID int64, id uuid.UUID, name, color string) (Tag, error) {
	name, color, err := validTag(name, color)
	if err != nil {
		return Tag{}, err
	}
	cur, err := s.store.Q().GetTagByUUID(ctx, db.GetTagByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return Tag{}, notFound(err)
	}
	t, err := s.store.Q().UpdateTag(ctx, db.UpdateTagParams{Name: name, Color: color, OrganizationID: orgID, ID: cur.ID})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Tag{}, ErrTagExists
	}
	if err != nil {
		return Tag{}, err
	}
	return Tag{UUID: t.Uuid, Name: t.Name, Color: t.Color}, nil
}

// DeleteTag removes a tag from every conversation.
func (s *Service) DeleteTag(ctx context.Context, orgID int64, id uuid.UUID) error {
	cur, err := s.store.Q().GetTagByUUID(ctx, db.GetTagByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return notFound(err)
	}
	_, err = s.store.Q().DeleteTag(ctx, db.DeleteTagParams{OrganizationID: orgID, ID: cur.ID})
	return err
}

// TagConversation adds (add=true) or removes a tag. A new name creates
// the tag. The change lands on the timeline.
func (s *Service) TagConversation(ctx context.Context, orgID int64, convID uuid.UUID, actor Actor, tagID uuid.UUID, name string, add bool) (Tag, error) {
	var tag db.Tag
	var timeline []db.Message
	var conv db.Conversation
	err := s.store.InTx(ctx, func(ctx context.Context, tx *repository.Tx) error {
		var err error
		conv, err = tx.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: convID})
		if err != nil {
			return err
		}
		if err := s.mustSee(ctx, orgID, conv.ID); err != nil {
			return err
		}
		if tagID != uuid.Nil {
			tag, err = tx.Q.GetTagByUUID(ctx, db.GetTagByUUIDParams{OrganizationID: orgID, Uuid: tagID})
		} else {
			var n string
			if n, _, err = validTag(name, ""); err != nil {
				return err
			}
			tag, err = tx.Q.UpsertTag(ctx, db.UpsertTagParams{OrganizationID: orgID, Name: n})
		}
		if err != nil {
			return err
		}
		var n int64
		if add {
			n, err = tx.Q.AddConversationTag(ctx, db.AddConversationTagParams{OrganizationID: orgID, ConversationID: conv.ID, TagID: tag.ID, CreatedBy: pgInt8(actor.ID)})
		} else {
			n, err = tx.Q.RemoveConversationTag(ctx, db.RemoveConversationTagParams{OrganizationID: orgID, ConversationID: conv.ID, TagID: tag.ID})
		}
		if err != nil || n == 0 {
			return err
		}
		event := "tag_added"
		if !add {
			event = "tag_removed"
		}
		msg, err := s.timelineEvent(ctx, tx, orgID, conv.ID, actor, event, "", tag.Name)
		if err != nil {
			return err
		}
		timeline = append(timeline, msg)
		return s.emit(ctx, tx, events.ConversationsUpdated, orgID, actor.ID, conv, map[string]any{"change": event, "tag": tag.Name})
	})
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			return Tag{}, err
		}
		return Tag{}, notFound(err)
	}
	if len(timeline) > 0 {
		s.publishTimeline(ctx, orgID, conv, timeline)
		s.indexed(ctx, conv.Uuid, uuid.Nil, uuid.Nil)
	}
	return Tag{UUID: tag.Uuid, Name: tag.Name, Color: tag.Color}, nil
}

// ---------------------------------------------------------------------------
// mapping

func (s *Service) toConversation(c db.Conversation, unread int, tags []uuid.UUID) model.Conversation {
	out := model.Conversation{
		BrandID: uuidPtr(c.BrandID), UUID: c.Uuid, Number: c.Number, ChannelType: c.ChannelType, Subject: c.Subject, Status: c.Status,
		Priority: c.Priority, Source: c.Source, TagUUIDs: make([]string, 0, len(tags)),
		CustomFields: decodeJSON(c.CustomFields), UnreadCount: unread, MessageCount: int(c.MessageCount),
		LastMessagePreview: c.LastMessagePreview, LastMessageDirection: c.LastMessageDirection,
		LastMessageAt: c.LastMessageAt.Time.UTC(), LastInboundAt: timePtr(c.LastInboundAt), WaitingSince: timePtr(c.WaitingSince),
		FirstResponseAt: timePtr(c.FirstResponseAt), FirstResponseDueAt: timePtr(c.FirstResponseDueAt),
		NextResponseDueAt: timePtr(c.NextResponseDueAt), ResolutionDueAt: timePtr(c.ResolutionDueAt),
		SLABreachedAt: timePtr(c.SlaBreachedAt), SnoozedUntil: timePtr(c.SnoozedUntil), ResolvedAt: timePtr(c.ResolvedAt),
		WindowOpen: true, CreatedAt: c.CreatedAt.Time.UTC(), UpdatedAt: c.UpdatedAt.Time.UTC(),
	}
	for _, t := range tags {
		out.TagUUIDs = append(out.TagUUIDs, t.String())
	}
	if s.registry != nil {
		if a, ok := s.registry.Get(c.ChannelType); ok {
			caps := a.Capabilities()
			if caps.ReplyWindow > 0 {
				out.WindowOpen = channels.WindowOpen(caps, c.LastInboundAt.Time, s.now())
				if c.LastInboundAt.Valid {
					end := c.LastInboundAt.Time.Add(caps.ReplyWindow).UTC()
					out.WindowEndsAt = &end
				}
			}
		}
	}
	return out
}

// contactRef builds the contact of a conversation row (CF-195: with its
// external id and identity state).
func contactRef(id uuid.UUID, name string, email, phone, externalID pgtype.Text, verifiedAt pgtype.Timestamptz) model.ContactRef {
	c := model.ContactRef{UUID: id, Name: name, Email: textPtr(email), Phone: textPtr(phone), ExternalID: textPtr(externalID)}
	if verifiedAt.Valid {
		t := verifiedAt.Time.UTC()
		c.VerifiedAt = &t
	}
	c.IdentityState = model.IdentityState(c.VerifiedAt, c.Name, c.Email, c.Phone)
	return c
}

func companyRef(id pgtype.UUID, name string, externalID pgtype.Text) *model.CompanyRef {
	if !id.Valid {
		return nil
	}
	return &model.CompanyRef{UUID: uuid.UUID(id.Bytes), Name: name, ExternalID: textPtr(externalID)}
}

func ref(id pgtype.UUID, name string) *model.Ref {
	if !id.Valid {
		return nil
	}
	return &model.Ref{UUID: uuid.UUID(id.Bytes), Name: name}
}

func messageOf(r db.ListConversationMessagesRow) db.Message {
	return db.Message{
		ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, ConversationID: r.ConversationID,
		ChannelInstanceID: r.ChannelInstanceID, Direction: r.Direction, SenderType: r.SenderType,
		SenderUserID: r.SenderUserID, SenderContactID: r.SenderContactID, Kind: r.Kind, Content: r.Content,
		ContentAttributes: r.ContentAttributes, Private: r.Private, Status: r.Status, Error: r.Error,
		ExternalID: r.ExternalID, ClientID: r.ClientID, ReplyToID: r.ReplyToID, SentAt: r.SentAt,
		DeliveredAt: r.DeliveredAt, ReadAt: r.ReadAt, CreatedAt: r.CreatedAt, EditedAt: r.EditedAt, DeletedAt: r.DeletedAt,
	}
}

func senderOf(r db.ListConversationMessagesRow) model.Sender {
	switch {
	case r.SenderType == model.SenderContact && r.SenderContactUuid.Valid:
		id := uuid.UUID(r.SenderContactUuid.Bytes)
		return model.Sender{Type: r.SenderType, UUID: &id, Name: r.SenderContactName.String}
	case r.SenderUserUuid.Valid:
		id := uuid.UUID(r.SenderUserUuid.Bytes)
		return model.Sender{Type: r.SenderType, UUID: &id, Name: fullName(r.SenderUserName, r.SenderUserSurname)}
	}
	return model.Sender{Type: r.SenderType}
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func nullableUUID(v *uuid.UUID) pgtype.UUID {
	if v == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *v, Valid: true}
}
