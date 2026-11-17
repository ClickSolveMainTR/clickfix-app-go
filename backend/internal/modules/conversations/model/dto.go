package model

import (
	"time"

	"github.com/google/uuid"
)

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// ContactRef is the contact of a conversation as the inbox list shows it.
type ContactRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Email *string   `json:"email"`
	Phone *string   `json:"phone"`
	// ExternalID is the customer's own user id (CF-195); nil when unknown.
	ExternalID *string `json:"external_id"`
	// IdentityState: verified, anonymous or self_named (IdentityState).
	IdentityState string     `json:"identity_state"`
	VerifiedAt    *time.Time `json:"verified_at"`
}

// CompanyRef is the company of a conversation with its external id
// (CF-195), kept separate from the contact's.
type CompanyRef struct {
	UUID       uuid.UUID `json:"uuid"`
	Name       string    `json:"name"`
	ExternalID *string   `json:"external_id"`
}

// Conversation is one inbox row / conversation header.
type Conversation struct {
	BrandID              *uuid.UUID  `json:"brand_id"`
	UUID                 uuid.UUID   `json:"uuid"`
	Number               int64       `json:"number"`
	ChannelType          string      `json:"channel_type"`
	Inbox                *Ref        `json:"inbox"`
	Contact              ContactRef  `json:"contact"`
	Company              *CompanyRef `json:"company"`
	Subject              string      `json:"subject"`
	Status               string      `json:"status"`
	CustomStatus         *Ref        `json:"custom_status"`
	Priority             string      `json:"priority"`
	Source               string      `json:"source"`
	Assignee             *Ref        `json:"assignee"`
	Team                 *Ref        `json:"team"`
	TagUUIDs             []string    `json:"tag_uuids"`
	CustomFields         any         `json:"custom_fields"`
	UnreadCount          int         `json:"unread_count"`
	MessageCount         int         `json:"message_count"`
	LastMessagePreview   string      `json:"last_message_preview"`
	LastMessageDirection string      `json:"last_message_direction"`
	LastMessageAt        time.Time   `json:"last_message_at"`
	LastInboundAt        *time.Time  `json:"last_inbound_at"`
	WaitingSince         *time.Time  `json:"waiting_since"`
	FirstResponseAt      *time.Time  `json:"first_response_at"`
	FirstResponseDueAt   *time.Time  `json:"first_response_due_at"`
	NextResponseDueAt    *time.Time  `json:"next_response_due_at"`
	ResolutionDueAt      *time.Time  `json:"resolution_due_at"`
	SLABreachedAt        *time.Time  `json:"sla_breached_at"`
	SnoozedUntil         *time.Time  `json:"snoozed_until"`
	ResolvedAt           *time.Time  `json:"resolved_at"`
	// ExternalThreadID is the channel's thread id (the REST API channel's
	// thread_id, CF-245); null when the channel has none.
	ExternalThreadID *string `json:"external_thread_id"`
	// WindowOpen: a free-form reply is allowed now (channels with a reply
	// window, e.g. 24 hours on WhatsApp); WindowEndsAt when it closes.
	WindowOpen   bool       `json:"window_open"`
	WindowEndsAt *time.Time `json:"window_ends_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// SearchMatch is set on list rows a ?q search matched through a message
	// (CF-45): the matched text, so the agent sees why the row is there.
	SearchMatch *SearchMatch `json:"search_match,omitempty"`
}

// SearchMatch is the message text a search matched in a conversation.
type SearchMatch struct {
	Snippet string `json:"snippet"`
	Private bool   `json:"private"`
}

// Sender is who wrote a message.
type Sender struct {
	Type string     `json:"type"`
	UUID *uuid.UUID `json:"uuid"`
	Name string     `json:"name"`
}

// MessageAttachment is a file of a message (API-owned URLs).
type MessageAttachment struct {
	UUID         uuid.UUID `json:"uuid"`
	Filename     string    `json:"filename"`
	ContentType  string    `json:"content_type"`
	SizeBytes    int64     `json:"size_bytes"`
	DownloadURL  string    `json:"download_url"`
	ThumbnailURL *string   `json:"thumbnail_url"`
}

// Message is one message of a conversation.
type Message struct {
	UUID             uuid.UUID           `json:"uuid"`
	ConversationUUID uuid.UUID           `json:"conversation_uuid"`
	Direction        string              `json:"direction"`
	Sender           Sender              `json:"sender"`
	Kind             string              `json:"kind"`
	Content          string              `json:"content"`
	Attributes       any                 `json:"attributes"`
	Private          bool                `json:"private"`
	Status           string              `json:"status"`
	Error            string              `json:"error"`
	ClientID         *uuid.UUID          `json:"client_id"`
	ReplyTo          *uuid.UUID          `json:"reply_to"`
	Mentions         []Ref               `json:"mentions"`
	Attachments      []MessageAttachment `json:"attachments"`
	SentAt           *time.Time          `json:"sent_at"`
	DeliveredAt      *time.Time          `json:"delivered_at"`
	ReadAt           *time.Time          `json:"read_at"`
	CreatedAt        time.Time           `json:"created_at"`
}

// Realtime event types published on the organization inbox channel.
const (
	EventConversationCreated = "conversation.created"
	EventConversationUpdated = "conversation.updated"
	EventMessageCreated      = "message.created"
	EventMessageUpdated      = "message.updated"
	// EventConversationViewers: who views / types in a conversation (CF-49).
	EventConversationViewers = "conversation.viewers"
	// EventAIJobFinished: an AI agent job of the conversation finished
	// (draft, handoff or automatic reply); the composer refetches the job
	// list to show a draft (CF-246).
	EventAIJobFinished = "ai.job_finished"
)

// LiveEvent is one realtime inbox event. Message is set for message events
// so an open conversation renders it without a refetch.
type LiveEvent struct {
	Type             string     `json:"type"`
	ConversationUUID uuid.UUID  `json:"conversation_uuid"`
	Message          *Message   `json:"message,omitempty"`
	ActorUUID        *uuid.UUID `json:"actor_uuid,omitempty"`
	// Also lists members (user uuids) who must get the event even though the
	// conversation's current audience no longer includes them: after a
	// reassignment the previous audience learns that it left their inbox.
	Also    []uuid.UUID `json:"-"`
	Viewers []Viewer    `json:"viewers,omitempty"`
}

// Viewer is an agent who has a conversation open (CF-49).
type Viewer struct {
	UUID   uuid.UUID `json:"uuid"`
	Name   string    `json:"name"`
	Typing bool      `json:"typing"`
	At     time.Time `json:"at"`
}
