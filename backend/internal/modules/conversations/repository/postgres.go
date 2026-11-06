// Package repository persists the conversation data model (CF-25): contacts
// and their channel identities, conversations, messages and the per-agent
// state around them. Writes that must stay consistent (a new conversation and
// its number, a message and the conversation counters, a contact merge) run
// in one transaction through Store.InTx; the transaction also rides on the
// context (platform/dbtx) so the outbox writes its events into it.
package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/dbtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by the repository.
var (
	ErrNotFound      = errors.New("conversations: not found")
	ErrInvalidMerge  = errors.New("conversations: invalid contact merge")
	ErrClosedThread  = errors.New("conversations: conversation is closed")
	ErrMissingParent = errors.New("conversations: conversation required")
)

// Store is the sqlc backed repository.
type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New builds a store on pool.
func New(pool *pgxpool.Pool, q *db.Queries) *Store {
	return &Store{pool: pool, q: q}
}

// Q returns the queries for reads outside a transaction.
func (s *Store) Q() *db.Queries { return s.q }

// Tx is one open transaction of the store.
type Tx struct {
	Q   *db.Queries
	Raw pgx.Tx
}

// InTx runs fn in a transaction. The context handed to fn carries the
// transaction (dbtx) so outbox.Enqueue(ctx, nil, ev) joins it.
func (s *Store) InTx(ctx context.Context, fn func(ctx context.Context, tx *Tx) error) error {
	raw, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("conversations: begin tx: %w", err)
	}
	defer func() { _ = raw.Rollback(ctx) }()
	if err := fn(dbtx.WithTx(ctx, raw), &Tx{Q: s.q.WithTx(raw), Raw: raw}); err != nil {
		return err
	}
	if err := raw.Commit(ctx); err != nil {
		return fmt.Errorf("conversations: commit: %w", err)
	}
	return nil
}

// NotFound maps pgx.ErrNoRows to ErrNotFound.
func NotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ---------------------------------------------------------------------------
// Contacts

// IdentityInput is how a channel knows the person who wrote.
type IdentityInput struct {
	OrganizationID int64
	ChannelType    string
	// InstanceID scopes ids that only mean something inside one account
	// (page scoped ids, bot chat ids); 0 for global ids (e-mail, phone).
	InstanceID  int64
	ExternalID  string
	DisplayName string
	Username    string
	Email       string
	Phone       string
	Meta        []byte
	SeenAt      time.Time
}

// ResolvedContact is the contact behind an identity.
type ResolvedContact struct {
	Contact  db.Contact
	Identity db.ContactIdentity
	Created  bool
}

// identityLockNamespace keeps the identity advisory locks apart from other
// transaction-scoped locks.
const identityLockNamespace = "contact_identity"

// ResolveContact finds or creates the contact of an identity. The identity is
// matched first; a new identity with an e-mail address or phone number joins
// an existing contact with the same address before a new contact is made.
// Concurrent deliveries for the same new identity are serialized by an
// advisory lock so exactly one contact is created.
func (t *Tx) ResolveContact(ctx context.Context, in IdentityInput) (ResolvedContact, error) {
	in.ExternalID = strings.TrimSpace(in.ExternalID)
	if in.ExternalID == "" || in.ChannelType == "" {
		return ResolvedContact{}, fmt.Errorf("conversations: identity requires channel type and external id")
	}
	if in.SeenAt.IsZero() {
		in.SeenAt = time.Now().UTC()
	}
	if len(in.Meta) == 0 {
		in.Meta = []byte("{}")
	}
	lockKey := fmt.Sprintf("%s:%d:%s:%d:%s", identityLockNamespace, in.OrganizationID, in.ChannelType, in.InstanceID, in.ExternalID)
	if _, err := t.Raw.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return ResolvedContact{}, fmt.Errorf("conversations: lock identity: %w", err)
	}
	instance := optInt8(in.InstanceID)
	existing, err := t.Q.FindContactIdentity(ctx, db.FindContactIdentityParams{
		OrganizationID: in.OrganizationID, ChannelType: in.ChannelType,
		ChannelInstanceID: instance, ExternalID: in.ExternalID,
	})
	var contact db.Contact
	created := false
	switch {
	case err == nil:
		contact, err = t.Q.GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: in.OrganizationID, ID: existing.ContactID})
		if err != nil {
			return ResolvedContact{}, fmt.Errorf("conversations: load contact: %w", err)
		}
		// A merged contact forwards to the survivor.
		for hops := 0; contact.MergedIntoID.Valid && hops < 8; hops++ {
			contact, err = t.Q.GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: in.OrganizationID, ID: contact.MergedIntoID.Int64})
			if err != nil {
				return ResolvedContact{}, fmt.Errorf("conversations: load merged contact: %w", err)
			}
		}
	case errors.Is(err, pgx.ErrNoRows):
		contact, created, err = t.matchOrCreateContact(ctx, in)
		if err != nil {
			return ResolvedContact{}, err
		}
	default:
		return ResolvedContact{}, fmt.Errorf("conversations: find identity: %w", err)
	}
	if !created {
		if err := t.Q.FillContactDetails(ctx, db.FillContactDetailsParams{
			Name: in.DisplayName, Email: optText(in.Email), Phone: optText(in.Phone),
			SeenAt: ts(in.SeenAt), OrganizationID: in.OrganizationID, ID: contact.ID,
		}); err != nil {
			return ResolvedContact{}, fmt.Errorf("conversations: fill contact: %w", err)
		}
	}
	identity, err := t.Q.UpsertContactIdentity(ctx, db.UpsertContactIdentityParams{
		OrganizationID: in.OrganizationID, ContactID: contact.ID, ChannelType: in.ChannelType,
		ChannelInstanceID: instance, ExternalID: in.ExternalID, DisplayName: in.DisplayName,
		Username: in.Username, Meta: in.Meta, LastSeenAt: ts(in.SeenAt),
	})
	if err != nil {
		return ResolvedContact{}, fmt.Errorf("conversations: upsert identity: %w", err)
	}
	return ResolvedContact{Contact: contact, Identity: identity, Created: created}, nil
}

func (t *Tx) matchOrCreateContact(ctx context.Context, in IdentityInput) (db.Contact, bool, error) {
	if email := strings.TrimSpace(in.Email); email != "" {
		c, err := t.Q.FindContactByEmail(ctx, db.FindContactByEmailParams{OrganizationID: in.OrganizationID, Email: email})
		if err == nil {
			return c, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.Contact{}, false, fmt.Errorf("conversations: match email: %w", err)
		}
	}
	if phone := strings.TrimSpace(in.Phone); phone != "" {
		c, err := t.Q.FindContactByPhone(ctx, db.FindContactByPhoneParams{OrganizationID: in.OrganizationID, Phone: phone})
		if errors.Is(err, pgx.ErrNoRows) && in.ChannelType == "voice" {
			c, err = t.Q.FindContactByPhoneLast10(ctx, db.FindContactByPhoneLast10Params{OrganizationID: in.OrganizationID, Phone: phone})
		}
		if err == nil {
			return c, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.Contact{}, false, fmt.Errorf("conversations: match phone: %w", err)
		}
	}
	var company pgtype.Int8
	if domain := CompanyDomain(in.Email); domain != "" {
		co, err := t.Q.GetCompanyByDomain(ctx, db.GetCompanyByDomainParams{OrganizationID: in.OrganizationID, Domain: domain})
		if err == nil {
			company = optInt8(co.ID)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.Contact{}, false, fmt.Errorf("conversations: match company: %w", err)
		}
	}
	c, err := t.Q.InsertContact(ctx, db.InsertContactParams{
		OrganizationID: in.OrganizationID, CompanyID: company, Name: truncate(in.DisplayName, 255),
		Email: optText(in.Email), Phone: optText(in.Phone), CustomFields: []byte("{}"),
	})
	if err != nil {
		return db.Contact{}, false, fmt.Errorf("conversations: insert contact: %w", err)
	}
	return c, true, nil
}

// freeMailDomains never identify a company.
var freeMailDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "hotmail.com": true, "outlook.com": true, "live.com": true,
	"yahoo.com": true, "icloud.com": true, "me.com": true, "yandex.com": true, "yandex.com.tr": true,
	"mail.ru": true, "proton.me": true, "protonmail.com": true, "aol.com": true, "msn.com": true, "hotmail.com.tr": true,
}

// CompanyDomain returns the company domain of an e-mail address ("" for
// free mail providers and malformed addresses).
func CompanyDomain(email string) string {
	at := strings.LastIndexByte(email, '@')
	if at < 0 || at == len(email)-1 {
		return ""
	}
	d := strings.ToLower(strings.TrimSpace(email[at+1:]))
	if freeMailDomains[d] || !strings.Contains(d, ".") {
		return ""
	}
	return d
}

// MergeResult counts what moved to the surviving contact.
type MergeResult struct {
	Identities    int64
	Conversations int64
	Messages      int64
}

// MergeContacts moves identities, conversations and authored messages of
// from into into, then soft deletes from with merged_into_id set. Empty
// fields of the survivor are filled from the merged contact.
func (t *Tx) MergeContacts(ctx context.Context, orgID, fromID, intoID int64) (MergeResult, error) {
	if fromID == intoID {
		return MergeResult{}, ErrInvalidMerge
	}
	from, err := t.Q.GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: orgID, ID: fromID})
	if err != nil {
		return MergeResult{}, NotFound(err)
	}
	into, err := t.Q.GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: orgID, ID: intoID})
	if err != nil {
		return MergeResult{}, NotFound(err)
	}
	if from.DeletedAt.Valid || into.DeletedAt.Valid {
		return MergeResult{}, ErrInvalidMerge
	}
	var res MergeResult
	if res.Identities, err = t.Q.MoveContactIdentities(ctx, db.MoveContactIdentitiesParams{
		IntoID: intoID, OrganizationID: orgID, FromID: fromID,
	}); err != nil {
		return MergeResult{}, fmt.Errorf("conversations: move identities: %w", err)
	}
	if err := t.Q.DeleteContactIdentities(ctx, db.DeleteContactIdentitiesParams{OrganizationID: orgID, ContactID: fromID}); err != nil {
		return MergeResult{}, fmt.Errorf("conversations: drop duplicate identities: %w", err)
	}
	if res.Conversations, err = t.Q.MoveContactConversations(ctx, db.MoveContactConversationsParams{
		IntoID: intoID, OrganizationID: orgID, FromID: fromID,
	}); err != nil {
		return MergeResult{}, fmt.Errorf("conversations: move conversations: %w", err)
	}
	if res.Messages, err = t.Q.MoveContactMessages(ctx, db.MoveContactMessagesParams{
		IntoID: optInt8(intoID), OrganizationID: orgID, FromID: optInt8(fromID),
	}); err != nil {
		return MergeResult{}, fmt.Errorf("conversations: move messages: %w", err)
	}
	if err := t.Q.FillContactDetails(ctx, db.FillContactDetailsParams{
		Name: from.Name, Email: from.Email, Phone: from.Phone,
		SeenAt: latest(from.LastSeenAt, into.LastSeenAt), OrganizationID: orgID, ID: intoID,
	}); err != nil {
		return MergeResult{}, fmt.Errorf("conversations: fill survivor: %w", err)
	}
	if !into.CompanyID.Valid && from.CompanyID.Valid {
		into.CompanyID = from.CompanyID
		if _, err := t.Raw.Exec(ctx, "UPDATE contacts SET company_id = $3 WHERE organization_id = $1 AND id = $2",
			orgID, intoID, from.CompanyID); err != nil {
			return MergeResult{}, fmt.Errorf("conversations: copy company: %w", err)
		}
	}
	if err := t.Q.MarkContactMerged(ctx, db.MarkContactMergedParams{IntoID: optInt8(intoID), OrganizationID: orgID, ID: fromID}); err != nil {
		return MergeResult{}, fmt.Errorf("conversations: mark merged: %w", err)
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// Conversations

// NewConversation is a conversation to open.
type NewConversation struct {
	OrganizationID   int64
	ChannelType      string
	InstanceID       int64
	ContactID        int64
	CompanyID        int64
	Subject          string
	Status           string
	Priority         string
	Source           string
	AssigneeUserID   int64
	AssigneeTeamID   int64
	CustomFields     []byte
	ExternalThreadID string
	At               time.Time
}

// CreateConversation hands out the next per-organization number and inserts
// the conversation.
func (t *Tx) CreateConversation(ctx context.Context, in NewConversation) (db.Conversation, error) {
	number, err := t.Q.NextConversationNumber(ctx, in.OrganizationID)
	if err != nil {
		return db.Conversation{}, fmt.Errorf("conversations: next number: %w", err)
	}
	if in.Status == "" {
		in.Status = model.StatusOpen
	}
	if in.Priority == "" {
		in.Priority = model.PriorityNormal
	}
	if in.Source == "" {
		in.Source = model.SourceInbound
	}
	if len(in.CustomFields) == 0 {
		in.CustomFields = []byte("{}")
	}
	if in.At.IsZero() {
		in.At = time.Now().UTC()
	}
	row, err := t.Q.InsertConversation(ctx, db.InsertConversationParams{
		OrganizationID: in.OrganizationID, Number: number, ChannelType: in.ChannelType,
		ChannelInstanceID: optInt8(in.InstanceID), ContactID: in.ContactID, CompanyID: optInt8(in.CompanyID),
		Subject: truncate(in.Subject, 500), Status: in.Status, Priority: in.Priority, Source: in.Source,
		AssigneeUserID: optInt8(in.AssigneeUserID), AssigneeTeamID: optInt8(in.AssigneeTeamID),
		CustomFields: in.CustomFields, ExternalThreadID: optText(in.ExternalThreadID), LastMessageAt: ts(in.At),
	})
	if err != nil {
		return db.Conversation{}, fmt.Errorf("conversations: insert: %w", err)
	}
	return row, nil
}

// NewMessage is a message to append to a conversation.
type NewMessage struct {
	OrganizationID   int64
	ConversationID   int64
	InstanceID       int64
	Direction        string
	SenderType       string
	SenderUserID     int64
	SenderContactID  int64
	Kind             string
	Content          string
	Attributes       []byte
	Private          bool
	Status           string
	ExternalID       string
	ClientID         pgtype.UUID
	ReplyToID        int64
	MentionedUserIDs []int64
	At               time.Time
}

// Appended is the outcome of AppendMessage.
type Appended struct {
	Message      db.Message
	Conversation db.Conversation
	// Duplicate: the message was already stored (same external or client
	// id); nothing changed and Conversation is the current row.
	Duplicate bool
	// Reopened: an inbound message woke a snoozed or resolved conversation.
	Reopened bool
}

// AppendMessage stores a message and moves the conversation counters
// (activity, waiting time, first response, unread) in the same transaction.
// A message whose external id or client id is already stored is returned as
// a duplicate without touching the conversation.
func (t *Tx) AppendMessage(ctx context.Context, in NewMessage) (Appended, error) {
	conv, err := t.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: in.OrganizationID, ID: in.ConversationID})
	if err != nil {
		return Appended{}, NotFound(err)
	}
	if in.At.IsZero() {
		in.At = time.Now().UTC()
	}
	if len(in.Attributes) == 0 {
		in.Attributes = []byte("{}")
	}
	if in.Kind == "" {
		in.Kind = model.KindText
	}
	if in.Status == "" {
		switch in.Direction {
		case model.DirectionIn:
			in.Status = model.MessageReceived
		case model.DirectionInternal:
			in.Status = model.MessageRecorded
		default:
			in.Status = model.MessagePending
		}
	}
	var sentAt pgtype.Timestamptz
	if in.Direction == model.DirectionOut && in.Status == model.MessageSent {
		sentAt = ts(in.At)
	}
	msg, err := t.Q.InsertMessage(ctx, db.InsertMessageParams{
		OrganizationID: in.OrganizationID, ConversationID: conv.ID, ChannelInstanceID: optInt8(in.InstanceID),
		Direction: in.Direction, SenderType: in.SenderType, SenderUserID: optInt8(in.SenderUserID),
		SenderContactID: optInt8(in.SenderContactID), Kind: in.Kind, Content: in.Content,
		ContentAttributes: in.Attributes, Private: in.Private, Status: in.Status,
		ExternalID: optText(in.ExternalID), ClientID: in.ClientID, ReplyToID: optInt8(in.ReplyToID),
		SentAt: sentAt, CreatedAt: ts(in.At),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		dup, derr := t.findDuplicate(ctx, in)
		if derr != nil {
			return Appended{}, derr
		}
		return Appended{Message: dup, Conversation: conv, Duplicate: true}, nil
	}
	if err != nil {
		return Appended{}, fmt.Errorf("conversations: insert message: %w", err)
	}
	out := Appended{Message: msg}
	preview := model.Preview(in.Content)
	if in.Direction == model.DirectionOut {
		preview = model.PreviewOutbound(in.Content)
	}
	if preview == "" && in.Kind == model.KindMedia {
		preview = "📎"
	}
	switch in.Direction {
	case model.DirectionIn:
		out.Reopened = conv.Status == model.StatusSnoozed || conv.Status == model.StatusResolved
		out.Conversation, err = t.Q.ApplyInboundToConversation(ctx, db.ApplyInboundToConversationParams{
			At: ts(in.At), Preview: preview, OrganizationID: in.OrganizationID, ID: conv.ID,
		})
	case model.DirectionOut:
		out.Conversation, err = t.Q.ApplyOutboundToConversation(ctx, db.ApplyOutboundToConversationParams{
			At: ts(in.At), Preview: preview, ByAgent: in.SenderType == model.SenderAgent,
			OrganizationID: in.OrganizationID, ID: conv.ID,
		})
	default:
		err = t.Q.ApplyInternalToConversation(ctx, db.ApplyInternalToConversationParams{OrganizationID: in.OrganizationID, ID: conv.ID})
		conv.MessageCount++
		out.Conversation = conv
	}
	if err != nil {
		return Appended{}, fmt.Errorf("conversations: update conversation: %w", err)
	}
	for _, uid := range in.MentionedUserIDs {
		if _, err := t.Q.InsertMessageMention(ctx, db.InsertMessageMentionParams{OrganizationID: in.OrganizationID, MessageID: msg.ID, UserID: uid}); err != nil {
			return Appended{}, fmt.Errorf("conversations: mention: %w", err)
		}
		if _, err := t.Q.AddConversationParticipant(ctx, db.AddConversationParticipantParams{
			OrganizationID: in.OrganizationID, ConversationID: conv.ID, UserID: uid,
			Role: model.ParticipantWatcher, AddedBy: optInt8(in.SenderUserID),
		}); err != nil {
			return Appended{}, fmt.Errorf("conversations: mention participant: %w", err)
		}
	}
	return out, nil
}

func (t *Tx) findDuplicate(ctx context.Context, in NewMessage) (db.Message, error) {
	if in.ExternalID != "" && in.InstanceID != 0 {
		m, err := t.Q.GetMessageByExternal(ctx, db.GetMessageByExternalParams{
			OrganizationID: in.OrganizationID, ChannelInstanceID: optInt8(in.InstanceID),
			ExternalID: optText(in.ExternalID),
		})
		if err == nil {
			return m, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.Message{}, err
		}
	}
	if in.ClientID.Valid {
		m, err := t.Q.GetMessageByClientID(ctx, db.GetMessageByClientIDParams{
			OrganizationID: in.OrganizationID, ConversationID: in.ConversationID, ClientID: in.ClientID,
		})
		if err == nil {
			return m, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.Message{}, err
		}
	}
	return db.Message{}, fmt.Errorf("conversations: message conflict without a stored duplicate")
}

// ---------------------------------------------------------------------------
// helpers

func optInt8(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func optText(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func ts(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func latest(a, b pgtype.Timestamptz) pgtype.Timestamptz {
	switch {
	case !a.Valid && !b.Valid:
		return ts(time.Now().UTC())
	case !a.Valid:
		return b
	case !b.Valid:
		return a
	case a.Time.After(b.Time):
		return a
	default:
		return b
	}
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}
