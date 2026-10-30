package adapters

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Tenant spec ids (CF-45). Every document carries organization_id and is
// served only through /v1/tenant/search with the caller's organization filter.
const (
	SpecConversations = "conversations"
	SpecMessages      = "messages"
	SpecContacts      = "contacts"
	SpecCompanies     = "companies"
)

// MaxIndexedContent caps the message text stored in the index (characters).
// Hits link to the conversation, so the start of a long e-mail is enough.
const MaxIndexedContent = 4000

// subtitleLength caps display subtitles (runes).
const subtitleLength = 140

// Inbox deep links (frontend /inbox reads ?c, ?contact and ?company).
func conversationHref(id uuid.UUID) string { return "/inbox?status=all&c=" + id.String() }
func contactHref(id uuid.UUID) string      { return "/inbox?status=all&contact=" + id.String() }
func companyHref(id uuid.UUID) string      { return "/inbox?status=all&company=" + id.String() }

// TenantQuerier is the slice of db.Queries the tenant adapters read.
type TenantQuerier interface {
	SearchConversationDocs(ctx context.Context, arg db.SearchConversationDocsParams) ([]db.SearchConversationDocsRow, error)
	SearchMessageDocs(ctx context.Context, arg db.SearchMessageDocsParams) ([]db.SearchMessageDocsRow, error)
	SearchContactDocs(ctx context.Context, arg db.SearchContactDocsParams) ([]db.SearchContactDocsRow, error)
	SearchCompanyDocs(ctx context.Context, arg db.SearchCompanyDocsParams) ([]db.SearchCompanyDocsRow, error)
}

// All returns every adapter of this build (platform + tenant), in palette order.
func All(q *db.Queries) []searchengine.Adapter {
	return []searchengine.Adapter{
		NewUsers(q),
		NewRoles(q),
		NewConversations(q),
		NewMessages(q),
		NewContacts(q),
		NewCompanies(q),
	}
}

// ---------------------------------------------------------------------------
// shared keyset walker

// walk pages a table by id: fetch returns the rows after afterID as
// documents plus the last id. It stops on a short page.
func walk(ctx context.Context, size int, fetch func(ctx context.Context, afterID int64, limit int32) ([]searchengine.Document, int64, error), fn func([]searchengine.Document) error) error {
	if size <= 0 {
		size = 200
	}
	size = min(size, 1000)
	after := int64(0)
	for {
		docs, last, err := fetch(ctx, after, safeconv.ToInt32(size))
		if err != nil {
			return err
		}
		if len(docs) > 0 {
			if err := fn(docs); err != nil {
				return err
			}
		}
		if len(docs) < size {
			return nil
		}
		after = last
	}
}

func listAll(ctx context.Context, bl searchengine.BatchLister) ([]searchengine.Document, error) {
	var out []searchengine.Document
	err := bl.ListBatches(ctx, 500, func(docs []searchengine.Document) error {
		out = append(out, docs...)
		return nil
	})
	return out, err
}

func parseID(spec, id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("%s search: invalid uuid: %w", spec, searchengine.ErrNotFound)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func one[T any](spec string, rows []T, err error) (T, error) {
	var zero T
	if err != nil {
		return zero, err
	}
	if len(rows) == 0 {
		return zero, fmt.Errorf("%s search: %w", spec, searchengine.ErrNotFound)
	}
	return rows[0], nil
}

func text(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return strings.TrimSpace(t.String)
}

// joinNonEmpty joins the non-empty parts with sep.
func joinNonEmpty(sep string, parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

func keywords(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// clip collapses whitespace and cuts s to max runes with an ellipsis.
func clip(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

func number(n int64) string { return "#" + strconv.FormatInt(n, 10) }

// ---------------------------------------------------------------------------
// conversations

// ConversationsAdapter indexes inbox conversations.
type ConversationsAdapter struct{ q TenantQuerier }

func NewConversations(q TenantQuerier) *ConversationsAdapter { return &ConversationsAdapter{q: q} }

func (a *ConversationsAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SpecConversations,
		LabelKey:   "search.specs_conversations",
		Permission: rbac.PermTenantConversationsRead,
		Icon:       "inbox",
		Searchable: []string{"title", "subtitle", "keywords"},
		Tenant:     true,
	}
}

func (a *ConversationsAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	return listAll(ctx, a)
}

func (a *ConversationsAdapter) ListBatches(ctx context.Context, size int, fn func([]searchengine.Document) error) error {
	return walk(ctx, size, func(ctx context.Context, after int64, limit int32) ([]searchengine.Document, int64, error) {
		rows, err := a.q.SearchConversationDocs(ctx, db.SearchConversationDocsParams{AfterID: after, RowLimit: limit})
		if err != nil || len(rows) == 0 {
			return nil, 0, err
		}
		docs := make([]searchengine.Document, 0, len(rows))
		for _, r := range rows {
			docs = append(docs, ConversationDocument(r))
		}
		return docs, rows[len(rows)-1].ID, nil
	}, fn)
}

func (a *ConversationsAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	key, err := parseID(SpecConversations, id)
	if err != nil {
		return searchengine.Document{}, err
	}
	rows, err := a.q.SearchConversationDocs(ctx, db.SearchConversationDocsParams{Uuid: key, RowLimit: 1})
	r, err := one(SpecConversations, rows, err)
	if err != nil {
		return searchengine.Document{}, err
	}
	return ConversationDocument(r), nil
}

// ConversationDocument maps one conversation row.
func ConversationDocument(r db.SearchConversationDocsRow) searchengine.Document {
	title := number(r.Number)
	if s := strings.TrimSpace(r.Subject); s != "" {
		title += " · " + s
	} else if n := strings.TrimSpace(r.ContactName); n != "" {
		title += " · " + n
	}
	return searchengine.Document{
		ID:       r.Uuid.String(),
		Spec:     SpecConversations,
		Title:    title,
		Subtitle: clip(joinNonEmpty(" · ", r.ContactName, r.LastMessagePreview), subtitleLength),
		Keywords: keywords(
			number(r.Number), strconv.FormatInt(r.Number, 10), r.ContactName, text(r.ContactEmail), text(r.ContactPhone),
			r.CompanyName, r.InboxName, r.TagNames, r.Status, r.Priority, r.ChannelType,
		),
		Href:           conversationHref(r.Uuid),
		Icon:           "inbox",
		OrganizationID: r.OrganizationID,
		ConversationID: r.Uuid.String(),
	}
}

// ---------------------------------------------------------------------------
// messages

// MessagesAdapter indexes message texts, internal notes included (every
// agent of the organization reads notes). Hits link to the conversation.
type MessagesAdapter struct{ q TenantQuerier }

func NewMessages(q TenantQuerier) *MessagesAdapter { return &MessagesAdapter{q: q} }

func (a *MessagesAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SpecMessages,
		LabelKey:   "search.specs_messages",
		Permission: rbac.PermTenantConversationsRead,
		Icon:       "message",
		Searchable: []string{"body", "title", "keywords"},
		Filterable: []string{"conversation_id", "private"},
		Tenant:     true,
	}
}

func (a *MessagesAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	return listAll(ctx, a)
}

func (a *MessagesAdapter) ListBatches(ctx context.Context, size int, fn func([]searchengine.Document) error) error {
	return walk(ctx, size, func(ctx context.Context, after int64, limit int32) ([]searchengine.Document, int64, error) {
		rows, err := a.q.SearchMessageDocs(ctx, db.SearchMessageDocsParams{MaxContent: MaxIndexedContent, AfterID: after, RowLimit: limit})
		if err != nil || len(rows) == 0 {
			return nil, 0, err
		}
		docs := make([]searchengine.Document, 0, len(rows))
		for _, r := range rows {
			docs = append(docs, MessageDocument(r))
		}
		return docs, rows[len(rows)-1].ID, nil
	}, fn)
}

func (a *MessagesAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	key, err := parseID(SpecMessages, id)
	if err != nil {
		return searchengine.Document{}, err
	}
	rows, err := a.q.SearchMessageDocs(ctx, db.SearchMessageDocsParams{MaxContent: MaxIndexedContent, Uuid: key, RowLimit: 1})
	r, err := one(SpecMessages, rows, err)
	if err != nil {
		return searchengine.Document{}, err
	}
	return MessageDocument(r), nil
}

// MessageDocument maps one message row. Body holds the (already truncated)
// content; the subtitle is a short preview.
func MessageDocument(r db.SearchMessageDocsRow) searchengine.Document {
	body := r.Content
	if utf8.RuneCountInString(body) > MaxIndexedContent {
		body = string([]rune(body)[:MaxIndexedContent])
	}
	icon := "message"
	if r.Private {
		icon = "note"
	}
	return searchengine.Document{
		ID:             r.Uuid.String(),
		Spec:           SpecMessages,
		Title:          joinNonEmpty(" · ", number(r.ConversationNumber), r.ConversationSubject, r.ContactName),
		Subtitle:       clip(body, subtitleLength),
		Keywords:       keywords(r.SenderUserName, r.Direction),
		Href:           conversationHref(r.ConversationUuid),
		Icon:           icon,
		OrganizationID: r.OrganizationID,
		Body:           body,
		ConversationID: r.ConversationUuid.String(),
		Private:        r.Private,
	}
}

// ---------------------------------------------------------------------------
// contacts

// ContactsAdapter indexes contacts (merged and deleted ones are skipped).
type ContactsAdapter struct{ q TenantQuerier }

func NewContacts(q TenantQuerier) *ContactsAdapter { return &ContactsAdapter{q: q} }

func (a *ContactsAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SpecContacts,
		LabelKey:   "search.specs_contacts",
		Permission: rbac.PermTenantContactsRead,
		Icon:       "contact",
		Searchable: []string{"title", "subtitle", "keywords"},
		Tenant:     true,
	}
}

func (a *ContactsAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	return listAll(ctx, a)
}

func (a *ContactsAdapter) ListBatches(ctx context.Context, size int, fn func([]searchengine.Document) error) error {
	return walk(ctx, size, func(ctx context.Context, after int64, limit int32) ([]searchengine.Document, int64, error) {
		rows, err := a.q.SearchContactDocs(ctx, db.SearchContactDocsParams{AfterID: after, RowLimit: limit})
		if err != nil || len(rows) == 0 {
			return nil, 0, err
		}
		docs := make([]searchengine.Document, 0, len(rows))
		for _, r := range rows {
			docs = append(docs, ContactDocument(r))
		}
		return docs, rows[len(rows)-1].ID, nil
	}, fn)
}

func (a *ContactsAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	key, err := parseID(SpecContacts, id)
	if err != nil {
		return searchengine.Document{}, err
	}
	rows, err := a.q.SearchContactDocs(ctx, db.SearchContactDocsParams{Uuid: key, RowLimit: 1})
	r, err := one(SpecContacts, rows, err)
	if err != nil {
		return searchengine.Document{}, err
	}
	return ContactDocument(r), nil
}

// ContactDocument maps one contact row.
func ContactDocument(r db.SearchContactDocsRow) searchengine.Document {
	email, phone := text(r.Email), text(r.Phone)
	title := strings.TrimSpace(r.Name)
	if title == "" {
		title = joinNonEmpty(" · ", email, phone)
	}
	return searchengine.Document{
		ID:             r.Uuid.String(),
		Spec:           SpecContacts,
		Title:          title,
		Subtitle:       joinNonEmpty(" · ", email, phone, r.CompanyName),
		Keywords:       keywords(email, phone, r.CompanyName),
		Href:           contactHref(r.Uuid),
		Icon:           "contact",
		OrganizationID: r.OrganizationID,
	}
}

// ---------------------------------------------------------------------------
// companies

// CompaniesAdapter indexes companies.
type CompaniesAdapter struct{ q TenantQuerier }

func NewCompanies(q TenantQuerier) *CompaniesAdapter { return &CompaniesAdapter{q: q} }

func (a *CompaniesAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SpecCompanies,
		LabelKey:   "search.specs_companies",
		Permission: rbac.PermTenantContactsRead,
		Icon:       "building",
		Searchable: []string{"title", "subtitle", "keywords"},
		Tenant:     true,
	}
}

func (a *CompaniesAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	return listAll(ctx, a)
}

func (a *CompaniesAdapter) ListBatches(ctx context.Context, size int, fn func([]searchengine.Document) error) error {
	return walk(ctx, size, func(ctx context.Context, after int64, limit int32) ([]searchengine.Document, int64, error) {
		rows, err := a.q.SearchCompanyDocs(ctx, db.SearchCompanyDocsParams{AfterID: after, RowLimit: limit})
		if err != nil || len(rows) == 0 {
			return nil, 0, err
		}
		docs := make([]searchengine.Document, 0, len(rows))
		for _, r := range rows {
			docs = append(docs, CompanyDocument(r))
		}
		return docs, rows[len(rows)-1].ID, nil
	}, fn)
}

func (a *CompaniesAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	key, err := parseID(SpecCompanies, id)
	if err != nil {
		return searchengine.Document{}, err
	}
	rows, err := a.q.SearchCompanyDocs(ctx, db.SearchCompanyDocsParams{Uuid: key, RowLimit: 1})
	r, err := one(SpecCompanies, rows, err)
	if err != nil {
		return searchengine.Document{}, err
	}
	return CompanyDocument(r), nil
}

// CompanyDocument maps one company row.
func CompanyDocument(r db.SearchCompanyDocsRow) searchengine.Document {
	return searchengine.Document{
		ID:             r.Uuid.String(),
		Spec:           SpecCompanies,
		Title:          strings.TrimSpace(r.Name),
		Subtitle:       text(r.Domain),
		Keywords:       keywords(text(r.Domain)),
		Href:           companyHref(r.Uuid),
		Icon:           "building",
		OrganizationID: r.OrganizationID,
	}
}

var (
	_ searchengine.Adapter     = (*ConversationsAdapter)(nil)
	_ searchengine.Adapter     = (*MessagesAdapter)(nil)
	_ searchengine.Adapter     = (*ContactsAdapter)(nil)
	_ searchengine.Adapter     = (*CompaniesAdapter)(nil)
	_ searchengine.BatchLister = (*ConversationsAdapter)(nil)
	_ searchengine.BatchLister = (*MessagesAdapter)(nil)
	_ searchengine.BatchLister = (*ContactsAdapter)(nil)
	_ searchengine.BatchLister = (*CompaniesAdapter)(nil)
	_ TenantQuerier            = (*db.Queries)(nil)
)
