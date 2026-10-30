package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Timeline item kinds (CF-85).
const (
	TimelineConversationOpened   = "conversation_opened"
	TimelineConversationResolved = "conversation_resolved"
	TimelineCSAT                 = "csat"
	TimelineNote                 = "note"
	TimelineEvent                = "event"
	TimelineRevenue              = "revenue"
	TimelineHealth               = "health"
)

// TimelineKinds lists every kind the timeline can filter on.
var TimelineKinds = []string{
	TimelineConversationOpened, TimelineConversationResolved, TimelineCSAT,
	TimelineNote, TimelineEvent, TimelineRevenue, TimelineHealth,
}

// ErrForbidden is returned when the caller may not change a record.
var ErrForbidden = errors.New("contacts: forbidden")

// TimelineItem is one entry of a company's unified timeline.
type TimelineItem struct {
	Kind       string         `json:"kind"`
	Ref        *uuid.UUID     `json:"ref"`
	OccurredAt time.Time      `json:"occurred_at"`
	Title      string         `json:"title"`
	Body       string         `json:"body"`
	Actor      string         `json:"actor"`
	Data       map[string]any `json:"data"`
}

// TimelinePage is a page of timeline items, newest first. NextBefore is the
// opaque cursor for the next (older) page; nil when there is none.
type TimelinePage struct {
	Items      []TimelineItem `json:"items"`
	NextBefore *string        `json:"next_before"`
}

// TimelineQuery filters the timeline.
type TimelineQuery struct {
	Kinds  []string
	Before string // cursor from TimelinePage.NextBefore
	Limit  int32
}

// Timeline returns one page of the company's unified timeline.
func (s *Service) Timeline(ctx context.Context, orgID int64, companyUUID uuid.UUID, q TimelineQuery) (TimelinePage, error) {
	co, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: companyUUID})
	if err != nil {
		return TimelinePage{}, notFound(err)
	}
	for _, k := range q.Kinds {
		if !contains(TimelineKinds, k) {
			return TimelinePage{}, fmt.Errorf("%w: unknown timeline kind %q", ErrInvalid, k)
		}
	}
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 30
	}
	params := db.ListCompanyTimelineParams{
		OrganizationID: orgID, CompanyID: pgtype.Int8{Int64: co.ID, Valid: true}, RowLimit: q.Limit + 1,
	}
	if len(q.Kinds) > 0 {
		params.Kinds = q.Kinds
	}
	if q.Before != "" {
		at, kind, key, ok := decodeTimelineCursor(q.Before)
		if !ok {
			return TimelinePage{}, fmt.Errorf("%w: before is not a valid cursor", ErrInvalid)
		}
		params.Before = pgtype.Timestamptz{Time: at, Valid: true}
		params.BeforeKind, params.BeforeKey = kind, key
	}
	rows, err := s.q.ListCompanyTimeline(ctx, params)
	if err != nil {
		return TimelinePage{}, err
	}
	page := TimelinePage{Items: make([]TimelineItem, 0, len(rows))}
	if len(rows) > int(q.Limit) {
		rows = rows[:q.Limit]
		last := rows[len(rows)-1]
		next := encodeTimelineCursor(last.OccurredAt.Time, last.Kind, last.SortKey)
		page.NextBefore = &next
	}
	for _, r := range rows {
		it := TimelineItem{
			Kind: r.Kind, OccurredAt: r.OccurredAt.Time, Title: r.Title, Body: r.Body, Actor: r.ActorName,
			Data: map[string]any{},
		}
		if id, err := uuid.Parse(r.Ref); err == nil {
			it.Ref = &id
		}
		_ = json.Unmarshal(r.Data, &it.Data)
		page.Items = append(page.Items, it)
	}
	return page, nil
}

// Timeline cursors are (occurred_at, kind, sort key) so items sharing a
// timestamp page correctly; base64url keeps them opaque and URL-safe.
func encodeTimelineCursor(at time.Time, kind, key string) string {
	raw := at.UTC().Format(time.RFC3339Nano) + "|" + kind + "|" + key
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeTimelineCursor(c string) (time.Time, string, string, bool) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, "", "", false
	}
	parts := strings.SplitN(string(b), "|", 3)
	if len(parts) != 3 {
		return time.Time{}, "", "", false
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", "", false
	}
	return at, parts[1], parts[2], true
}

// CompanyConversation is a conversation row on the account screen.
type CompanyConversation struct {
	UUID               uuid.UUID `json:"uuid"`
	Number             int64     `json:"number"`
	Subject            string    `json:"subject"`
	Status             string    `json:"status"`
	ChannelType        string    `json:"channel_type"`
	Priority           string    `json:"priority"`
	LastMessageAt      time.Time `json:"last_message_at"`
	LastMessagePreview string    `json:"last_message_preview"`
	CreatedAt          time.Time `json:"created_at"`
	Contact            Ref       `json:"contact"`
}

// CompanyStats summarises a company's support relationship.
type CompanyStats struct {
	OpenConversations   int64      `json:"open_conversations"`
	ClosedConversations int64      `json:"closed_conversations"`
	LastConversationAt  *time.Time `json:"last_conversation_at"`
	CSATAverage         *float64   `json:"csat_average"`
	CSATCount           int64      `json:"csat_count"`
}

// CompanyOverview is the account screen header: the company, its revenue,
// support stats and the open / recently closed conversations.
type CompanyOverview struct {
	Company           Company               `json:"company"`
	Stats             CompanyStats          `json:"stats"`
	OpenConversations []CompanyConversation `json:"open_conversations"`
	RecentlyClosed    []CompanyConversation `json:"recently_closed"`
}

// Overview loads the account screen of a company.
func (s *Service) Overview(ctx context.Context, orgID int64, companyUUID uuid.UUID) (CompanyOverview, error) {
	row, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: companyUUID})
	if err != nil {
		return CompanyOverview{}, notFound(err)
	}
	co, err := s.companyDetail(ctx, row)
	if err != nil {
		return CompanyOverview{}, err
	}
	list := []Company{co}
	if err := s.attachRevenue(ctx, orgID, []int64{row.ID}, list); err != nil {
		return CompanyOverview{}, err
	}
	out := CompanyOverview{Company: list[0]}
	companyID := pgtype.Int8{Int64: row.ID, Valid: true}
	st, err := s.q.GetCompanyOverviewStats(ctx, db.GetCompanyOverviewStatsParams{OrganizationID: orgID, CompanyID: companyID})
	if err != nil {
		return CompanyOverview{}, err
	}
	out.Stats = CompanyStats{
		OpenConversations: st.OpenConversations, ClosedConversations: st.ClosedConversations, CSATCount: st.CsatCount,
	}
	if st.LastConversationAt.Valid {
		t := st.LastConversationAt.Time
		out.Stats.LastConversationAt = &t
	}
	if st.CsatCount > 0 {
		avg := st.CsatAverage
		out.Stats.CSATAverage = &avg
	}
	if out.OpenConversations, err = s.companyConversations(ctx, orgID, companyID, true, 50); err != nil {
		return CompanyOverview{}, err
	}
	if out.RecentlyClosed, err = s.companyConversations(ctx, orgID, companyID, false, 10); err != nil {
		return CompanyOverview{}, err
	}
	return out, nil
}

func (s *Service) companyConversations(ctx context.Context, orgID int64, companyID pgtype.Int8, open bool, limit int32) ([]CompanyConversation, error) {
	rows, err := s.q.ListCompanyConversations(ctx, db.ListCompanyConversationsParams{
		OrganizationID: orgID, CompanyID: companyID, OpenOnly: pgtype.Bool{Bool: open, Valid: true}, RowLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]CompanyConversation, 0, len(rows))
	for _, r := range rows {
		out = append(out, CompanyConversation{
			UUID: r.Uuid, Number: r.Number, Subject: r.Subject, Status: r.Status, ChannelType: r.ChannelType,
			Priority: r.Priority, LastMessageAt: r.LastMessageAt.Time, LastMessagePreview: r.LastMessagePreview,
			CreatedAt: r.CreatedAt.Time, Contact: Ref{UUID: r.ContactUuid, Name: r.ContactName},
		})
	}
	return out, nil
}

// CompanyNote is an account note.
type CompanyNote struct {
	UUID      uuid.UUID `json:"uuid"`
	Body      string    `json:"body"`
	Author    *Ref      `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// AddNote writes an account note on a company.
func (s *Service) AddNote(ctx context.Context, orgID int64, companyUUID uuid.UUID, authorID int64, body string) (uuid.UUID, CompanyNote, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > 10000 {
		return uuid.Nil, CompanyNote{}, fmt.Errorf("%w: body must be 1-10000 characters", ErrInvalid)
	}
	co, err := s.q.GetCompanyByUUID(ctx, db.GetCompanyByUUIDParams{OrganizationID: orgID, Uuid: companyUUID})
	if err != nil {
		return uuid.Nil, CompanyNote{}, notFound(err)
	}
	n, err := s.q.CreateCompanyNote(ctx, db.CreateCompanyNoteParams{
		OrganizationID: orgID, CompanyID: co.ID, AuthorUserID: pgtype.Int8{Int64: authorID, Valid: authorID > 0}, Body: body,
	})
	if err != nil {
		return uuid.Nil, CompanyNote{}, err
	}
	note := CompanyNote{UUID: n.Uuid, Body: n.Body, CreatedAt: n.CreatedAt.Time}
	if u, err := s.q.GetUserRefByID(ctx, authorID); err == nil {
		note.Author = &Ref{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}
	}
	return co.Uuid, note, nil
}

// DeleteNote removes an account note. Only its author may delete it unless
// canDeleteAny (the contacts delete permission) is set.
func (s *Service) DeleteNote(ctx context.Context, orgID int64, noteUUID uuid.UUID, actorID int64, canDeleteAny bool) error {
	n, err := s.q.GetCompanyNoteByUUID(ctx, db.GetCompanyNoteByUUIDParams{OrganizationID: orgID, Uuid: noteUUID})
	if err != nil {
		return notFound(err)
	}
	if !canDeleteAny && (!n.AuthorUserID.Valid || n.AuthorUserID.Int64 != actorID) {
		return ErrForbidden
	}
	_, err = s.q.DeleteCompanyNote(ctx, db.DeleteCompanyNoteParams{OrganizationID: orgID, ID: n.ID})
	return err
}

// CompanyEventInput is an in-product event sent through the API.
type CompanyEventInput struct {
	Company        CompanyKey
	ContactEmail   string
	Name           string
	Properties     map[string]any
	Source         string
	IdempotencyKey string
	OccurredAt     *time.Time
}

// CompanyEvent is a stored in-product event.
type CompanyEvent struct {
	UUID        uuid.UUID      `json:"uuid"`
	CompanyUUID uuid.UUID      `json:"company_uuid"`
	Name        string         `json:"name"`
	Properties  map[string]any `json:"properties"`
	Source      string         `json:"source"`
	OccurredAt  time.Time      `json:"occurred_at"`
	Duplicate   bool           `json:"duplicate"`
}

// RecordEvent stores an in-product event on the timeline. A repeated
// idempotency key returns the stored event with Duplicate set.
func (s *Service) RecordEvent(ctx context.Context, orgID int64, in CompanyEventInput) (CompanyEvent, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return CompanyEvent{}, fmt.Errorf("%w: name must be 1-100 characters", ErrInvalid)
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "api"
	}
	if len(source) > 32 {
		return CompanyEvent{}, fmt.Errorf("%w: source is too long", ErrInvalid)
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	if len(key) > 200 {
		return CompanyEvent{}, fmt.Errorf("%w: idempotency_key is too long", ErrInvalid)
	}
	props := in.Properties
	if props == nil {
		props = map[string]any{}
	}
	pb, err := json.Marshal(props)
	if err != nil || len(pb) > 16<<10 {
		return CompanyEvent{}, fmt.Errorf("%w: properties must be a JSON object under 16 KB", ErrInvalid)
	}
	occurred := time.Now().UTC()
	if in.OccurredAt != nil {
		occurred = in.OccurredAt.UTC()
		if occurred.After(time.Now().Add(5 * time.Minute)) {
			return CompanyEvent{}, fmt.Errorf("%w: occurred_at is in the future", ErrInvalid)
		}
	}
	co, err := s.companyByKey(ctx, orgID, in.Company)
	if err != nil {
		return CompanyEvent{}, err
	}
	var contactID pgtype.Int8
	if email := strings.TrimSpace(in.ContactEmail); email != "" {
		ct, err := s.q.FindContactByEmail(ctx, db.FindContactByEmailParams{OrganizationID: orgID, Email: email})
		if err == nil && ct.CompanyID.Valid && ct.CompanyID.Int64 == co.ID {
			// Only a contact of this company is named on its timeline.
			contactID = pgtype.Int8{Int64: ct.ID, Valid: true}
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return CompanyEvent{}, err
		}
	}
	keyText := pgtype.Text{String: key, Valid: key != ""}
	row, err := s.q.CreateCompanyEvent(ctx, db.CreateCompanyEventParams{
		OrganizationID: orgID, CompanyID: co.ID, ContactID: contactID, Name: name, Properties: pb,
		Source: source, IdempotencyKey: keyText, OccurredAt: pgtype.Timestamptz{Time: occurred, Valid: true},
	})
	duplicate := false
	if errors.Is(err, pgx.ErrNoRows) && keyText.Valid {
		row, err = s.q.GetCompanyEventByIdempotencyKey(ctx, db.GetCompanyEventByIdempotencyKeyParams{OrganizationID: orgID, IdempotencyKey: keyText})
		duplicate = true
	}
	if err != nil {
		return CompanyEvent{}, err
	}
	if row.CompanyID != co.ID {
		return CompanyEvent{}, fmt.Errorf("%w: idempotency_key was already used for another company", ErrInvalid)
	}
	ev := CompanyEvent{
		UUID: row.Uuid, CompanyUUID: co.Uuid, Name: row.Name, Source: row.Source, OccurredAt: row.OccurredAt.Time,
		Properties: map[string]any{}, Duplicate: duplicate,
	}
	_ = json.Unmarshal(row.Properties, &ev.Properties)
	return ev, nil
}
