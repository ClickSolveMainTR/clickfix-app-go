// Package usecase is the inbox core (CF-8 / CF-25): the inbound pipeline
// (webhook → contact → conversation → message → events → realtime), the
// outbound delivery of agent messages through the channel adapters and the
// delivery receipts. Channel specifics stay behind platform/channels.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Errors returned by the service.
var (
	ErrNotFound      = errors.New("conversations: not found")
	ErrInvalid       = errors.New("conversations: invalid request")
	ErrNoChannel     = errors.New("conversations: conversation has no channel to reply on")
	ErrNoRecipient   = errors.New("conversations: contact has no identity on this channel")
	ErrChannelClosed = errors.New("conversations: channel is not active")
)

// InstanceSource opens channel instances with their decrypted credentials
// and adapter (implemented by the channels module).
type InstanceSource interface {
	Instance(ctx context.Context, orgID, instanceID int64) (channels.Instance, channels.Adapter, error)
}

// SendQueue schedules the delivery of an outbound message.
type SendQueue interface {
	EnqueueSend(ctx context.Context, orgID, messageID int64, delay time.Duration) error
}

// Outbox writes domain events into the open transaction.
type Outbox interface {
	Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error
}

// Notifier pushes realtime inbox events to the organization's agents.
type Notifier interface {
	Notify(ctx context.Context, orgID int64, ev model.LiveEvent)
}

// AttachmentStore stores inbound files as message attachments.
type AttachmentStore interface {
	StoreInbound(ctx context.Context, orgID int64, messageUUID uuid.UUID, filename, contentType string, body io.Reader) error
}

// SendLimiter throttles sends per channel instance.
type SendLimiter interface {
	AllowSend(ctx context.Context, instance channels.Instance) (bool, time.Duration)
}

// SearchIndexer receives the records a change touched, after the commit
// (CF-45). Implementations only enqueue: they never block or fail the change.
type SearchIndexer interface {
	ConversationChanged(ctx context.Context, id uuid.UUID)
	MessageChanged(ctx context.Context, id uuid.UUID)
	ContactChanged(ctx context.Context, id uuid.UUID)
}

// SearchFinder matches inbox search text against the search index (message
// texts, notes, contact fields). Matches are ordered by relevance.
type SearchFinder interface {
	MatchConversations(ctx context.Context, orgID int64, q string) ([]SearchMatch, error)
}

// SearchMatch is one conversation the index matched; Snippet is the matched
// message text when the hit came from a message.
type SearchMatch struct {
	ConversationUUID uuid.UUID
	Snippet          string
	Private          bool
}

// Settings tune the pipeline.
type Settings struct {
	// ReopenWindow: an inbound message reopens a resolved conversation
	// resolved less than this long ago; older ones start a new conversation.
	ReopenWindow time.Duration
	// MaxSendAttempts: after this many failed attempts a message is failed.
	MaxSendAttempts int
}

// Service is the inbox core.
type Service struct {
	store     *repository.Store
	instances InstanceSource
	queue     SendQueue
	outbox    Outbox
	notifier  Notifier
	files     AttachmentStore
	limiter   SendLimiter
	registry  *channels.Registry
	presence  PresenceStore
	mentions  MentionNotifier
	// resolveGuard may refuse resolving (CF-52).
	resolveGuard ResolveGuard
	indexer      SearchIndexer
	finder       SearchFinder
	settings     Settings
	log          *slog.Logger
	now          func() time.Time
}

// New builds the service.
func New(store *repository.Store, instances InstanceSource, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store:     store,
		instances: instances,
		settings:  Settings{ReopenWindow: 7 * 24 * time.Hour, MaxSendAttempts: 8},
		log:       log,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// WithQueue sets the outbound send queue.
func (s *Service) WithQueue(q SendQueue) *Service { s.queue = q; return s }

// WithOutbox sets the transactional outbox for domain events.
func (s *Service) WithOutbox(o Outbox) *Service { s.outbox = o; return s }

// WithNotifier sets the realtime notifier.
func (s *Service) WithNotifier(n Notifier) *Service { s.notifier = n; return s }

// WithAttachments sets the store for inbound files.
func (s *Service) WithAttachments(a AttachmentStore) *Service { s.files = a; return s }

// WithLimiter sets the per-instance send limiter.
func (s *Service) WithLimiter(l SendLimiter) *Service { s.limiter = l; return s }

// WithSearch sets the search index hooks (CF-45). Either may be nil; without
// a finder the inbox search stays on the database (ILIKE) only.
func (s *Service) WithSearch(ix SearchIndexer, f SearchFinder) *Service {
	s.indexer, s.finder = ix, f
	return s
}

// indexed tells the search indexer what a committed change touched. Zero
// uuids are skipped.
func (s *Service) indexed(ctx context.Context, conv, msg, contact uuid.UUID) {
	if s.indexer == nil {
		return
	}
	if conv != uuid.Nil {
		s.indexer.ConversationChanged(ctx, conv)
	}
	if msg != uuid.Nil {
		s.indexer.MessageChanged(ctx, msg)
	}
	if contact != uuid.Nil {
		s.indexer.ContactChanged(ctx, contact)
	}
}

// WithSettings overrides the pipeline settings (zero values keep defaults).
func (s *Service) WithSettings(set Settings) *Service {
	if set.ReopenWindow > 0 {
		s.settings.ReopenWindow = set.ReopenWindow
	}
	if set.MaxSendAttempts > 0 {
		s.settings.MaxSendAttempts = set.MaxSendAttempts
	}
	return s
}

// WithClock replaces the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// emit writes a domain event into the transaction of ctx (no-op without an outbox).
func (s *Service) emit(ctx context.Context, tx *repository.Tx, name string, orgID int64, actor int64, conv db.Conversation, payload map[string]any) error {
	if s.outbox == nil {
		return nil
	}
	if trace := events.AutomationTrace(ctx); len(trace) > 0 {
		payload["_automation_trace"] = trace
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if err := addExternalRefs(ctx, tx, orgID, conv, payload); err != nil {
		return err
	}
	ev := events.New(name).WithTenant(orgID).WithEntity("conversation", &conv.ID, &conv.Uuid).WithPayload(payload)
	if actor > 0 {
		ev = ev.WithActor(actor)
	}
	if err := s.outbox.Enqueue(ctx, tx.Raw, ev); err != nil {
		return fmt.Errorf("conversations: outbox %s: %w", name, err)
	}
	return nil
}

// addExternalRefs puts the contact's and the company's external ids on a
// conversation event (CF-195), so webhook receivers can match their own
// records without another call. Keys already set by the caller win.
func addExternalRefs(ctx context.Context, tx *repository.Tx, orgID int64, conv db.Conversation, payload map[string]any) error {
	if conv.ContactID == 0 {
		return nil
	}
	r, err := tx.Q.GetConversationExternalRefs(ctx, db.GetConversationExternalRefsParams{
		OrganizationID: orgID, ContactID: conv.ContactID, CompanyID: conv.CompanyID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("conversations: external refs: %w", err)
	}
	set := func(k string, v any) {
		if _, ok := payload[k]; !ok {
			payload[k] = v
		}
	}
	set("contact_uuid", r.ContactUuid.String())
	set("contact_external_id", textPtr(r.ContactExternalID))
	set("contact_verified", r.ContactVerifiedAt.Valid)
	if r.CompanyUuid.Valid {
		set("company_uuid", uuid.UUID(r.CompanyUuid.Bytes).String())
	}
	set("company_external_id", textPtr(r.CompanyExternalID))
	return nil
}

func (s *Service) notify(ctx context.Context, orgID int64, ev model.LiveEvent) {
	if s.notifier != nil {
		s.notifier.Notify(ctx, orgID, ev)
	}
}

// Viewer builds the inbox viewer of a member (team visibility, CF-44).
func (s *Service) Viewer(ctx context.Context, scope orgctx.Scope, userID int64) (access.Viewer, error) {
	return access.For(ctx, s.store.Q(), scope, userID)
}

// viewerOf returns the request's viewer; calls without one (workers,
// webhooks) see every conversation.
func viewerOf(ctx context.Context, userID int64) access.Viewer {
	if v, ok := access.From(ctx); ok {
		return v
	}
	return access.Viewer{UserID: userID, All: true, TeamIDs: []int64{}}
}

// mustSee answers ErrNotFound when the request's viewer may not see the
// conversation (a hidden conversation looks like a missing one).
func (s *Service) mustSee(ctx context.Context, orgID, conversationID int64) error {
	v, ok := access.From(ctx)
	if !ok || v.All {
		return nil
	}
	visible, err := v.CanSee(ctx, s.store.Q(), orgID, conversationID)
	if err != nil {
		return err
	}
	if !visible {
		return ErrNotFound
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
