package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convaccess "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Contact timeline item kinds (CF-221). Conversation kinds share their
// names with the company timeline.
const (
	ContactTimelineConversationOpened   = TimelineConversationOpened
	ContactTimelineConversationResolved = TimelineConversationResolved
	ContactTimelineCSAT                 = TimelineCSAT
	ContactTimelineSurvey               = "survey"
	ContactTimelineNote                 = TimelineNote
	ContactTimelineAppointment          = "appointment"
	ContactTimelineConsent              = "consent"
	ContactTimelinePrivacyRequest       = "privacy_request"
	ContactTimelineEvent                = TimelineEvent
	ContactTimelineFeedback             = "feedback"
	ContactTimelineForum                = "forum"
	ContactTimelineStatusNotice         = "status_notice"
	ContactTimelineSystem               = "system"
)

// ContactTimelineKinds lists every kind of the contact timeline.
var ContactTimelineKinds = []string{
	ContactTimelineConversationOpened, ContactTimelineConversationResolved, ContactTimelineCSAT, ContactTimelineSurvey,
	ContactTimelineNote, ContactTimelineAppointment, ContactTimelineConsent, ContactTimelinePrivacyRequest,
	ContactTimelineEvent, ContactTimelineFeedback, ContactTimelineForum, ContactTimelineStatusNotice, ContactTimelineSystem,
}

// ProfileAccess is what the member may see on the contact page. The handler
// builds it from the member's permissions; every section and timeline kind
// is filtered by it server side.
type ProfileAccess struct {
	// UserID is the member (team visibility, note authorship, summary audience).
	UserID int64
	// Conversations: tenant.conversations.read. SeeAll: read_all; otherwise
	// only conversations of TeamIDs, assigned to UserID, unrouted or joined.
	Conversations bool
	SeeAll        bool
	TeamIDs       []int64
	// Surveys: NPS / CES results (the survey module needs read_all).
	Surveys bool
	// Appointments: tenant.appointments.read.
	Appointments bool
	// Compliance: consents and KVKK requests (tenant.settings.write, like
	// the compliance module).
	Compliance bool
	// Feedback: roadmap votes and comments, published forum posts
	// (tenant.feedback.read); ForumAll adds pending and rejected posts
	// (tenant.feedback.manage, the moderation permission).
	Feedback bool
	ForumAll bool
	// AI: tenant.ai.use, reads and generates the AI summary.
	AI bool
}

func (a ProfileAccess) teams() []int64 {
	if a.TeamIDs == nil {
		return []int64{}
	}
	return a.TeamIDs
}

// audience keys the cached AI summary: members who see every conversation
// share one, a restricted member has their own.
func (a ProfileAccess) audience() string {
	if a.SeeAll {
		return "all"
	}
	return fmt.Sprintf("user:%d", a.UserID)
}

// Kinds are the timeline kinds the member may see.
func (a ProfileAccess) Kinds() []string {
	out := []string{ContactTimelineNote, ContactTimelineEvent, ContactTimelineSystem}
	if a.Conversations {
		out = append(out, ContactTimelineConversationOpened, ContactTimelineConversationResolved, ContactTimelineCSAT, ContactTimelineStatusNotice)
		if a.Surveys {
			out = append(out, ContactTimelineSurvey)
		}
	}
	if a.Appointments {
		out = append(out, ContactTimelineAppointment)
	}
	if a.Compliance {
		out = append(out, ContactTimelineConsent, ContactTimelinePrivacyRequest)
	}
	if a.Feedback {
		out = append(out, ContactTimelineFeedback, ContactTimelineForum)
	}
	ordered := make([]string, 0, len(out))
	for _, k := range ContactTimelineKinds {
		if contains(out, k) {
			ordered = append(ordered, k)
		}
	}
	return ordered
}

// ContactProfileSections says which sections the member may see; a hidden
// section is returned empty.
type ContactProfileSections struct {
	Conversations bool `json:"conversations"`
	Surveys       bool `json:"surveys"`
	Appointments  bool `json:"appointments"`
	Compliance    bool `json:"compliance"`
	Feedback      bool `json:"feedback"`
	AI            bool `json:"ai"`
}

// ContactProfileStats is the health summary of the contact page.
type ContactProfileStats struct {
	OpenConversations   int64      `json:"open_conversations"`
	ClosedConversations int64      `json:"closed_conversations"`
	SLABreached         int64      `json:"sla_breached"`
	SLAAtRisk           int64      `json:"sla_at_risk"`
	MessageCount        int64      `json:"message_count"`
	FirstConversationAt *time.Time `json:"first_conversation_at"`
	LastConversationAt  *time.Time `json:"last_conversation_at"`
	LastInboundAt       *time.Time `json:"last_inbound_at"`
	CSATAverage         *float64   `json:"csat_average"`
	CSATCount           int64      `json:"csat_count"`
	CSATTotal           int64      `json:"csat_total"`
	NPSLatest           *int32     `json:"nps_latest"`
	CESLatest           *int32     `json:"ces_latest"`
	AttachmentCount     int64      `json:"attachment_count"`
	NoteCount           int64      `json:"note_count"`
}

// ContactConversation is a conversation row on the contact page.
type ContactConversation struct {
	UUID               uuid.UUID  `json:"uuid"`
	Number             int64      `json:"number"`
	Subject            string     `json:"subject"`
	Status             string     `json:"status"`
	ChannelType        string     `json:"channel_type"`
	Priority           string     `json:"priority"`
	LastMessageAt      time.Time  `json:"last_message_at"`
	LastMessagePreview string     `json:"last_message_preview"`
	CreatedAt          time.Time  `json:"created_at"`
	MessageCount       int32      `json:"message_count"`
	SLABreachedAt      *time.Time `json:"sla_breached_at"`
	SLADueAt           *time.Time `json:"sla_due_at"`
	Assignee           string     `json:"assignee"`
	Team               string     `json:"team"`
	Tags               []string   `json:"tags"`
}

// ContactTag is a tag on the contact's conversations.
type ContactTag struct {
	UUID          uuid.UUID `json:"uuid"`
	Name          string    `json:"name"`
	Color         string    `json:"color"`
	Conversations int64     `json:"conversations"`
}

// ContactAttachment is a file of one of the contact's conversations. It is
// opened from its conversation, which owns the download URL.
type ContactAttachment struct {
	UUID               uuid.UUID `json:"uuid"`
	Filename           string    `json:"filename"`
	ContentType        string    `json:"content_type"`
	SizeBytes          int64     `json:"size_bytes"`
	CreatedAt          time.Time `json:"created_at"`
	ConversationUUID   uuid.UUID `json:"conversation_uuid"`
	ConversationNumber int64     `json:"conversation_number"`
}

// ContactAppointment is an appointment booked by or for the contact.
type ContactAppointment struct {
	UUID         uuid.UUID `json:"uuid"`
	Title        string    `json:"title"`
	Status       string    `json:"status"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	Timezone     string    `json:"timezone"`
	LocationKind string    `json:"location_kind"`
	Type         string    `json:"type"`
	Host         string    `json:"host"`
}

// ContactConsent is the latest consent decision for one purpose.
type ContactConsent struct {
	UUID          uuid.UUID `json:"uuid"`
	Purpose       string    `json:"purpose"`
	Granted       bool      `json:"granted"`
	NoticeVersion string    `json:"notice_version"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"created_at"`
}

// ContactPrivacyRequest is a KVKK data subject request (export / erase).
type ContactPrivacyRequest struct {
	UUID        uuid.UUID  `json:"uuid"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	RequestedBy string     `json:"requested_by"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// ContactFeedback is a roadmap vote or comment of the contact.
type ContactFeedback struct {
	Kind        string    `json:"kind"`
	RequestUUID uuid.UUID `json:"request_uuid"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
}

// ContactForumPost is a community forum topic or reply of the contact.
type ContactForumPost struct {
	UUID      uuid.UUID `json:"uuid"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Excerpt   string    `json:"excerpt"`
	Status    string    `json:"status"`
	IsTopic   bool      `json:"is_topic"`
	CreatedAt time.Time `json:"created_at"`
}

// ContactStatusNotice is a status incident the contact was notified about.
type ContactStatusNotice struct {
	UUID      uuid.UUID `json:"uuid"`
	Title     string    `json:"title"`
	Severity  string    `json:"severity"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ContactAISummary is the cached AI summary of a contact.
type ContactAISummary struct {
	Summary     string    `json:"summary"`
	OpenTopics  []string  `json:"open_topics"`
	Sentiment   string    `json:"sentiment"`
	Locale      string    `json:"locale"`
	GeneratedAt time.Time `json:"generated_at"`
	// Stale: something happened after the summary was generated.
	Stale bool `json:"stale"`
}

// ContactProfile is everything the contact page shows above the timeline.
type ContactProfile struct {
	Contact         Contact                 `json:"contact"`
	Company         *Company                `json:"company"`
	Stats           ContactProfileStats     `json:"stats"`
	Conversations   []ContactConversation   `json:"conversations"`
	Tags            []ContactTag            `json:"tags"`
	Attachments     []ContactAttachment     `json:"attachments"`
	Appointments    []ContactAppointment    `json:"appointments"`
	Consents        []ContactConsent        `json:"consents"`
	PrivacyRequests []ContactPrivacyRequest `json:"privacy_requests"`
	Feedback        []ContactFeedback       `json:"feedback"`
	ForumPosts      []ContactForumPost      `json:"forum_posts"`
	StatusNotices   []ContactStatusNotice   `json:"status_notices"`
	AISummary       *ContactAISummary       `json:"ai_summary"`
	Sections        ContactProfileSections  `json:"sections"`
	TimelineKinds   []string                `json:"timeline_kinds"`
}

// Profile loads the contact page: a fixed number of queries, each bound to
// the contact and the organization, none per row.
func (s *Service) Profile(ctx context.Context, orgID int64, contactUUID uuid.UUID, a ProfileAccess) (ContactProfile, error) {
	row, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: orgID, Uuid: contactUUID})
	if err != nil {
		return ContactProfile{}, notFound(err)
	}
	c, err := s.detail(ctx, row)
	if err != nil {
		return ContactProfile{}, err
	}
	out := ContactProfile{
		Contact: c, Conversations: []ContactConversation{}, Tags: []ContactTag{}, Attachments: []ContactAttachment{},
		Appointments: []ContactAppointment{}, Consents: []ContactConsent{}, PrivacyRequests: []ContactPrivacyRequest{},
		Feedback: []ContactFeedback{}, ForumPosts: []ContactForumPost{}, StatusNotices: []ContactStatusNotice{},
		Sections: ContactProfileSections{
			Conversations: a.Conversations, Surveys: a.Conversations && a.Surveys, Appointments: a.Appointments,
			Compliance: a.Compliance, Feedback: a.Feedback, AI: a.AI,
		},
		TimelineKinds: a.Kinds(),
	}
	if c.Company != nil {
		co, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: c.Company.UUID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ContactProfile{}, err
		}
		if err == nil {
			detail, err := s.companyDetail(ctx, co)
			if err != nil {
				return ContactProfile{}, err
			}
			out.Company = &detail
		}
	}
	st, err := s.q.GetContactProfileStats(ctx, db.GetContactProfileStatsParams{
		OrganizationID: orgID, ContactID: row.ID, CanConversations: a.Conversations, CanSurveys: a.Surveys,
		SeeAll: a.SeeAll, ViewerID: a.UserID, ViewerTeamIds: a.teams(),
	})
	if err != nil {
		return ContactProfile{}, err
	}
	out.Stats = ContactProfileStats{
		OpenConversations: st.OpenConversations, ClosedConversations: st.ClosedConversations, SLABreached: st.SlaBreached,
		SLAAtRisk: st.SlaAtRisk, MessageCount: st.MessageCount, FirstConversationAt: timePtr(st.FirstConversationAt),
		LastConversationAt: timePtr(st.LastConversationAt), LastInboundAt: timePtr(st.LastInboundAt),
		CSATCount: st.CsatCount, CSATTotal: st.CsatTotal, AttachmentCount: st.AttachmentCount, NoteCount: st.NoteCount,
	}
	if st.CsatCount > 0 {
		avg := st.CsatAverage
		out.Stats.CSATAverage = &avg
	}
	if st.NpsLatest >= 0 {
		v := st.NpsLatest
		out.Stats.NPSLatest = &v
	}
	if st.CesLatest >= 0 {
		v := st.CesLatest
		out.Stats.CESLatest = &v
	}
	if a.Conversations {
		if err := s.profileConversations(ctx, orgID, row.ID, a, &out); err != nil {
			return ContactProfile{}, err
		}
	}
	if a.Appointments {
		rows, err := s.q.ListContactProfileAppointments(ctx, db.ListContactProfileAppointmentsParams{
			OrganizationID: orgID, ContactID: pgtype.Int8{Int64: row.ID, Valid: true},
		})
		if err != nil {
			return ContactProfile{}, err
		}
		for _, r := range rows {
			out.Appointments = append(out.Appointments, ContactAppointment{
				UUID: r.Uuid, Title: r.Title, Status: r.Status, StartsAt: r.StartsAt.Time, EndsAt: r.EndsAt.Time,
				Timezone: r.Timezone, LocationKind: r.LocationKind, Type: r.TypeName, Host: r.HostName,
			})
		}
	}
	if a.Compliance {
		consents, err := s.q.ListContactProfileConsents(ctx, db.ListContactProfileConsentsParams{OrganizationID: orgID, ContactID: row.ID})
		if err != nil {
			return ContactProfile{}, err
		}
		for _, r := range consents {
			out.Consents = append(out.Consents, ContactConsent{
				UUID: r.Uuid, Purpose: r.Purpose, Granted: r.Granted, NoticeVersion: r.NoticeVersion, Source: r.Source, CreatedAt: r.CreatedAt.Time,
			})
		}
		reqs, err := s.q.ListContactProfileRequests(ctx, db.ListContactProfileRequestsParams{OrganizationID: orgID, ContactID: row.ID})
		if err != nil {
			return ContactProfile{}, err
		}
		for _, r := range reqs {
			out.PrivacyRequests = append(out.PrivacyRequests, ContactPrivacyRequest{
				UUID: r.Uuid, Kind: r.Kind, Status: r.Status, RequestedBy: r.RequestedByName, CreatedAt: r.CreatedAt.Time,
				CompletedAt: timePtr(r.CompletedAt), ExpiresAt: timePtr(r.ExpiresAt),
			})
		}
	}
	if a.Feedback {
		fb, err := s.q.ListContactProfileFeedback(ctx, db.ListContactProfileFeedbackParams{OrganizationID: orgID, ContactID: row.ID})
		if err != nil {
			return ContactProfile{}, err
		}
		for _, r := range fb {
			out.Feedback = append(out.Feedback, ContactFeedback{
				Kind: r.Kind, RequestUUID: r.RequestUuid, Title: r.Title, Status: r.Status, Body: r.Body, CreatedAt: r.CreatedAt.Time,
			})
		}
		posts, err := s.q.ListContactProfileForumPosts(ctx, db.ListContactProfileForumPostsParams{OrganizationID: orgID, ContactID: row.ID, AllStatuses: a.ForumAll})
		if err != nil {
			return ContactProfile{}, err
		}
		for _, r := range posts {
			out.ForumPosts = append(out.ForumPosts, ContactForumPost{
				UUID: r.Uuid, Slug: r.Slug, Title: r.Title, Excerpt: r.Excerpt, Status: r.Status, IsTopic: r.IsTopic, CreatedAt: r.CreatedAt.Time,
			})
		}
	}
	if a.AI {
		if out.AISummary, err = s.cachedSummary(ctx, orgID, row.ID, a); err != nil {
			return ContactProfile{}, err
		}
	}
	return out, nil
}

func (s *Service) profileConversations(ctx context.Context, orgID, contactID int64, a ProfileAccess, out *ContactProfile) error {
	convs, err := s.q.ListContactProfileConversations(ctx, db.ListContactProfileConversationsParams{
		OrganizationID: orgID, ContactID: contactID, SeeAll: a.SeeAll, ViewerID: a.UserID, ViewerTeamIds: a.teams(), RowLimit: 50,
	})
	if err != nil {
		return err
	}
	for _, r := range convs {
		out.Conversations = append(out.Conversations, ContactConversation{
			UUID: r.Uuid, Number: r.Number, Subject: r.Subject, Status: r.Status, ChannelType: r.ChannelType, Priority: r.Priority,
			LastMessageAt: r.LastMessageAt.Time, LastMessagePreview: r.LastMessagePreview, CreatedAt: r.CreatedAt.Time,
			MessageCount: r.MessageCount, SLABreachedAt: timePtr(r.SlaBreachedAt), SLADueAt: timePtr(r.SlaDueAt),
			Assignee: r.AssigneeName, Team: r.TeamName, Tags: nonNil(r.Tags),
		})
	}
	tags, err := s.q.ListContactProfileTags(ctx, db.ListContactProfileTagsParams{
		OrganizationID: orgID, ContactID: contactID, SeeAll: a.SeeAll, ViewerID: a.UserID, ViewerTeamIds: a.teams(),
	})
	if err != nil {
		return err
	}
	for _, r := range tags {
		out.Tags = append(out.Tags, ContactTag{UUID: r.Uuid, Name: r.Name, Color: r.Color, Conversations: r.Conversations})
	}
	files, err := s.q.ListContactProfileAttachments(ctx, db.ListContactProfileAttachmentsParams{
		OrganizationID: orgID, ContactID: contactID, SeeAll: a.SeeAll, ViewerID: a.UserID, ViewerTeamIds: a.teams(),
	})
	if err != nil {
		return err
	}
	for _, r := range files {
		out.Attachments = append(out.Attachments, ContactAttachment{
			UUID: r.Uuid, Filename: r.Filename, ContentType: r.ContentType, SizeBytes: r.SizeBytes, CreatedAt: r.CreatedAt.Time,
			ConversationUUID: r.ConversationUuid, ConversationNumber: r.ConversationNumber,
		})
	}
	notices, err := s.q.ListContactProfileStatusNotices(ctx, db.ListContactProfileStatusNoticesParams{OrganizationID: orgID, ContactID: contactID})
	if err != nil {
		return err
	}
	for _, r := range notices {
		out.StatusNotices = append(out.StatusNotices, ContactStatusNotice{
			UUID: r.Uuid, Title: r.Title, Severity: r.Severity, Status: r.Status, CreatedAt: r.CreatedAt.Time,
		})
	}
	return nil
}

// ContactTimelineQuery filters the contact timeline.
type ContactTimelineQuery struct {
	Kinds  []string
	Q      string
	From   *time.Time
	To     *time.Time
	Before string // cursor from TimelinePage.NextBefore
	Limit  int32
}

// ContactTimeline returns one page of the contact's unified timeline. Kinds
// the member may not see are dropped; asking only for those returns an
// empty page.
func (s *Service) ContactTimeline(ctx context.Context, orgID int64, contactUUID uuid.UUID, a ProfileAccess, q ContactTimelineQuery) (TimelinePage, error) {
	row, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: orgID, Uuid: contactUUID})
	if err != nil {
		return TimelinePage{}, notFound(err)
	}
	for _, k := range q.Kinds {
		if !contains(ContactTimelineKinds, k) {
			return TimelinePage{}, fmt.Errorf("%w: unknown timeline kind %q", ErrInvalid, k)
		}
	}
	allowed := a.Kinds()
	kinds := allowed
	if len(q.Kinds) > 0 {
		kinds = make([]string, 0, len(q.Kinds))
		for _, k := range q.Kinds {
			if contains(allowed, k) {
				kinds = append(kinds, k)
			}
		}
	}
	page := TimelinePage{Items: []TimelineItem{}}
	if len(kinds) == 0 {
		return page, nil
	}
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 30
	}
	if q.From != nil && q.To != nil && !q.To.After(*q.From) {
		return TimelinePage{}, fmt.Errorf("%w: to must be after from", ErrInvalid)
	}
	search := strings.TrimSpace(q.Q)
	if utf8.RuneCountInString(search) > 200 {
		return TimelinePage{}, fmt.Errorf("%w: q is too long", ErrInvalid)
	}
	params := db.ListContactTimelineParams{
		OrganizationID: orgID, ContactID: row.ID, ContactUuid: row.Uuid, Kinds: kinds, RowLimit: q.Limit + 1,
		SeeAll: a.SeeAll, ViewerID: a.UserID, ViewerTeamIds: a.teams(), ForumAll: a.ForumAll,
	}
	if search != "" {
		params.Q = pgtype.Text{String: likeEscape(search), Valid: true}
	}
	if q.From != nil {
		params.FromAt = pgtype.Timestamptz{Time: *q.From, Valid: true}
	}
	if q.To != nil {
		params.ToAt = pgtype.Timestamptz{Time: *q.To, Valid: true}
	}
	if q.Before != "" {
		at, kind, key, ok := decodeTimelineCursor(q.Before)
		if !ok {
			return TimelinePage{}, fmt.Errorf("%w: before is not a valid cursor", ErrInvalid)
		}
		params.Before = pgtype.Timestamptz{Time: at, Valid: true}
		params.BeforeKind, params.BeforeKey = kind, key
	}
	rows, err := s.q.ListContactTimeline(ctx, params)
	if err != nil {
		return TimelinePage{}, err
	}
	if len(rows) > int(q.Limit) {
		rows = rows[:q.Limit]
		last := rows[len(rows)-1]
		next := encodeTimelineCursor(last.OccurredAt.Time, last.Kind, last.SortKey)
		page.NextBefore = &next
	}
	for _, r := range rows {
		it := TimelineItem{
			Kind: r.Kind, OccurredAt: r.OccurredAt.Time, Title: r.Title, Body: r.Body, Actor: r.ActorName, Data: map[string]any{},
		}
		if id, err := uuid.Parse(r.Ref); err == nil {
			it.Ref = &id
		}
		_ = json.Unmarshal(r.Data, &it.Data)
		page.Items = append(page.Items, it)
	}
	return page, nil
}

// likeEscape makes user text literal inside ILIKE '%…%'.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// ContactNote is an agent note on a contact.
type ContactNote struct {
	UUID      uuid.UUID `json:"uuid"`
	Body      string    `json:"body"`
	Author    *Ref      `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// AddContactNote writes a note on a contact.
func (s *Service) AddContactNote(ctx context.Context, orgID int64, contactUUID uuid.UUID, authorID int64, body string) (ContactNote, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > 10000 {
		return ContactNote{}, fmt.Errorf("%w: body must be 1-10000 characters", ErrInvalid)
	}
	c, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: orgID, Uuid: contactUUID})
	if err != nil {
		return ContactNote{}, notFound(err)
	}
	n, err := s.q.CreateContactNote(ctx, db.CreateContactNoteParams{
		OrganizationID: orgID, ContactID: c.ID, AuthorUserID: pgtype.Int8{Int64: authorID, Valid: authorID > 0}, Body: body,
	})
	if err != nil {
		return ContactNote{}, err
	}
	note := ContactNote{UUID: n.Uuid, Body: n.Body, CreatedAt: n.CreatedAt.Time}
	if u, err := s.q.GetUserRefByID(ctx, authorID); err == nil {
		note.Author = &Ref{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}
	}
	return note, nil
}

// DeleteContactNote removes a contact note: its author, or anyone with
// canDeleteAny (the contacts delete permission). It returns the contact.
func (s *Service) DeleteContactNote(ctx context.Context, orgID int64, noteUUID uuid.UUID, actorID int64, canDeleteAny bool) error {
	n, err := s.q.GetContactNoteByUUID(ctx, db.GetContactNoteByUUIDParams{OrganizationID: orgID, Uuid: noteUUID})
	if err != nil {
		return notFound(err)
	}
	if !canDeleteAny && (!n.AuthorUserID.Valid || n.AuthorUserID.Int64 != actorID) {
		return ErrForbidden
	}
	_, err = s.q.DeleteContactNote(ctx, db.DeleteContactNoteParams{OrganizationID: orgID, ID: n.ID})
	return err
}

// ProfileAccessFor builds the member's page access from their permissions.
func (s *Service) ProfileAccessFor(ctx context.Context, scope orgctx.Scope, userID int64) (ProfileAccess, error) {
	a := ProfileAccess{
		UserID:        userID,
		Conversations: convaccess.CanRead(scope),
		SeeAll:        scope.CanSeeAll(convaccess.Resource),
		Appointments:  scope.Has(rbac.PermTenantAppointmentsRead),
		Compliance:    scope.Has(rbac.PermTenantSettingsWrite),
		Feedback:      scope.Has(rbac.PermTenantFeedbackRead) || scope.Has(rbac.PermTenantFeedbackManage),
		ForumAll:      scope.Has(rbac.PermTenantFeedbackManage),
		AI:            scope.Has(rbac.PermTenantAiUse),
		TeamIDs:       []int64{},
	}
	// The survey results screen needs read_all as well.
	a.Surveys = a.Conversations && a.SeeAll
	if a.Conversations && !a.SeeAll {
		v, err := convaccess.For(ctx, s.q, scope, userID)
		if err != nil {
			return ProfileAccess{}, err
		}
		a.TeamIDs = v.Teams()
	}
	return a, nil
}
