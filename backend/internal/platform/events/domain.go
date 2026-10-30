package events

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/google/uuid"
)

// ClickFix helpdesk domain events (CF-6 catalog v1). Names are singular
// `{module}.{action}`. Billing lifecycle events (subscription.*) live in
// catalog.go next to the billing module that owns them. Each name
// has a typed payload registered below: build events with NewDomain and read
// them with DecodePayload so producers and consumers share one schema.
const (
	ConversationTagAdded        = "conversation.tag_added"
	ConversationCreated         = "conversation.created"
	ConversationUpdated         = "conversation.updated"
	ConversationAssigned        = "conversation.assigned"
	ConversationUnassigned      = "conversation.unassigned"
	ConversationStatusChanged   = "conversation.status_changed"
	ConversationPriorityChanged = "conversation.priority_changed"
	ConversationResolved        = "conversation.resolved"
	ConversationReopened        = "conversation.reopened"
	ConversationSnoozed         = "conversation.snoozed"
	ConversationMentioned       = "conversation.mentioned"

	MessageReceived      = "message.received"
	MessageSent          = "message.sent"
	MessageStatusChanged = "message.status_changed"
	MessageFailed        = "message.failed"

	ContactCreated = "contact.created"
	ContactUpdated = "contact.updated"
	ContactMerged  = "contact.merged"
	CompanyCreated = "company.created"
	CompanyUpdated = "company.updated"
	// CompanyHealthDropped: a company's health score fell below the
	// organization's churn-risk threshold (CF-87); once per crossing.
	CompanyHealthDropped = "company.health_dropped"

	TicketCreated       = "ticket.created"
	TicketUpdated       = "ticket.updated"
	TicketAssigned      = "ticket.assigned"
	TicketStatusChanged = "ticket.status_changed"
	TicketCommented     = "ticket.commented"
	TicketResolved      = "ticket.resolved"
	TicketClosed        = "ticket.closed"
	TicketReopened      = "ticket.reopened"

	SLAApplied  = "sla.applied"
	SLAWarning  = "sla.warning"
	SLABreached = "sla.breached"
	SLAMet      = "sla.met"

	CSATSubmitted = "csat.submitted"

	AgentPresenceChanged = "agent.presence_changed"
)

// ConversationRef identifies a conversation in every conversation/message payload.
type ConversationRef struct {
	ConversationID   int64     `json:"conversation_id"`
	ConversationUUID uuid.UUID `json:"conversation_uuid"`
	InboxID          *int64    `json:"inbox_id,omitempty"`
	ChannelType      string    `json:"channel_type,omitempty"`
}

// ConversationPayload: conversation.created / updated / resolved / reopened / snoozed.
type ConversationPayload struct {
	ConversationRef
	ContactID      *int64     `json:"contact_id,omitempty"`
	AssigneeUserID *int64     `json:"assignee_user_id,omitempty"`
	TeamID         *int64     `json:"team_id,omitempty"`
	Status         string     `json:"status"`
	Priority       string     `json:"priority,omitempty"`
	Subject        string     `json:"subject,omitempty"`
	SnoozedUntil   *time.Time `json:"snoozed_until,omitempty"`
	ChangedFields  []string   `json:"changed_fields,omitempty"`
}

// AssignmentPayload: conversation.assigned / unassigned and ticket.assigned.
// Source tells who assigned: user, rule, round_robin, ai or system.
type AssignmentPayload struct {
	ConversationID         *int64 `json:"conversation_id,omitempty"`
	TicketID               *int64 `json:"ticket_id,omitempty"`
	AssigneeUserID         *int64 `json:"assignee_user_id,omitempty"`
	PreviousAssigneeUserID *int64 `json:"previous_assignee_user_id,omitempty"`
	TeamID                 *int64 `json:"team_id,omitempty"`
	Source                 string `json:"source"`
}

// StatusChangePayload: conversation.status_changed / priority_changed and ticket.status_changed.
type StatusChangePayload struct {
	ConversationID *int64 `json:"conversation_id,omitempty"`
	TicketID       *int64 `json:"ticket_id,omitempty"`
	From           string `json:"from"`
	To             string `json:"to"`
}

// MessagePayload: message.received / sent. Direction is inbound or outbound;
// SenderType is contact, agent, bot or system.
type MessagePayload struct {
	ConversationRef
	MessageID   int64     `json:"message_id"`
	MessageUUID uuid.UUID `json:"message_uuid"`
	Direction   string    `json:"direction"`
	SenderType  string    `json:"sender_type"`
	SenderID    *int64    `json:"sender_id,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
	Private     bool      `json:"private,omitempty"`
	ExternalID  string    `json:"external_id,omitempty"`
	Preview     string    `json:"preview,omitempty"`
}

// MessageStatusPayload: message.status_changed / failed (delivery receipts).
type MessageStatusPayload struct {
	ConversationRef
	MessageID int64  `json:"message_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

// MentionPayload: conversation.mentioned (an agent @mentioned teammates in a
// private note).
type MentionPayload struct {
	ConversationRef
	NoteID           int64   `json:"note_id"`
	AuthorUserID     int64   `json:"author_user_id"`
	MentionedUserIDs []int64 `json:"mentioned_user_ids"`
}

// ContactPayload: contact.created / updated and company.created / updated.
type ContactPayload struct {
	ContactID     *int64   `json:"contact_id,omitempty"`
	CompanyID     *int64   `json:"company_id,omitempty"`
	UUID          string   `json:"uuid,omitempty"`
	ChangedFields []string `json:"changed_fields,omitempty"`
}

// CompanyHealthPayload: company.health_dropped.
type CompanyHealthPayload struct {
	CompanyID          int64     `json:"company_id"`
	CompanyUUID        uuid.UUID `json:"company_uuid"`
	CompanyName        string    `json:"company_name"`
	Score              int       `json:"score"`
	PreviousScore      *int      `json:"previous_score,omitempty"`
	Threshold          int       `json:"threshold"`
	AccountOwnerUserID *int64    `json:"account_owner_user_id,omitempty"`
}

// ContactMergedPayload: contact.merged; MergedContactID no longer exists.
type ContactMergedPayload struct {
	ContactID       int64 `json:"contact_id"`
	MergedContactID int64 `json:"merged_contact_id"`
}

// TicketPayload: ticket.created / updated / resolved / closed / reopened.
type TicketPayload struct {
	TicketID       int64     `json:"ticket_id"`
	TicketUUID     uuid.UUID `json:"ticket_uuid"`
	Number         string    `json:"number,omitempty"`
	Status         string    `json:"status"`
	Priority       string    `json:"priority,omitempty"`
	AssigneeUserID *int64    `json:"assignee_user_id,omitempty"`
	TeamID         *int64    `json:"team_id,omitempty"`
	ContactID      *int64    `json:"contact_id,omitempty"`
	ConversationID *int64    `json:"conversation_id,omitempty"`
	ChangedFields  []string  `json:"changed_fields,omitempty"`
}

// TicketCommentPayload: ticket.commented.
type TicketCommentPayload struct {
	TicketID     int64  `json:"ticket_id"`
	CommentID    int64  `json:"comment_id"`
	AuthorUserID *int64 `json:"author_user_id,omitempty"`
	Private      bool   `json:"private"`
}

// SLAPayload: sla.*. TargetType is conversation or ticket; Metric is
// first_response, next_response or resolution.
type SLAPayload struct {
	PolicyID       int64      `json:"policy_id"`
	TargetType     string     `json:"target_type"`
	TargetID       int64      `json:"target_id"`
	Metric         string     `json:"metric"`
	DueAt          time.Time  `json:"due_at"`
	BreachedAt     *time.Time `json:"breached_at,omitempty"`
	AssigneeUserID *int64     `json:"assignee_user_id,omitempty"`
}

// CSATPayload: csat.submitted. Rating is 1-5.
type CSATPayload struct {
	ConversationID *int64 `json:"conversation_id,omitempty"`
	TicketID       *int64 `json:"ticket_id,omitempty"`
	ContactID      *int64 `json:"contact_id,omitempty"`
	Rating         int    `json:"rating"`
	Comment        string `json:"comment,omitempty"`
}

// PresencePayload: agent.presence_changed (routing reacts to agents coming
// online or leaving). Reason is manual, heartbeat or idle.
type PresencePayload struct {
	UserID   int64     `json:"user_id"`
	UserUUID uuid.UUID `json:"user_uuid"`
	Status   string    `json:"status"`
	Previous string    `json:"previous"`
	Reason   string    `json:"reason"`
}

// Definition describes one catalog event: its current payload version, the
// Go type of its payload and whether organizations may subscribe to it
// through outgoing webhooks.
type Definition struct {
	Name    string
	Version int
	Webhook bool
	payload reflect.Type
}

func def[T any](name string, webhook bool) Definition {
	return Definition{Name: name, Version: 1, Webhook: webhook, payload: reflect.TypeFor[T]()}
}

var domainDefinitions = func() map[string]Definition {
	list := []Definition{
		def[ConversationPayload](ConversationCreated, true),
		def[ConversationPayload](ConversationUpdated, true),
		def[ConversationPayload](ConversationTagAdded, true),
		def[AssignmentPayload](ConversationAssigned, true),
		def[AssignmentPayload](ConversationUnassigned, true),
		def[StatusChangePayload](ConversationStatusChanged, true),
		def[StatusChangePayload](ConversationPriorityChanged, true),
		def[ConversationPayload](ConversationResolved, true),
		def[ConversationPayload](ConversationReopened, true),
		def[ConversationPayload](ConversationSnoozed, true),
		def[MentionPayload](ConversationMentioned, false),
		def[MessagePayload](MessageReceived, true),
		def[MessagePayload](MessageSent, true),
		def[MessageStatusPayload](MessageStatusChanged, false),
		def[MessageStatusPayload](MessageFailed, true),
		def[ContactPayload](ContactCreated, true),
		def[ContactPayload](ContactUpdated, true),
		def[ContactMergedPayload](ContactMerged, true),
		def[ContactPayload](CompanyCreated, true),
		def[ContactPayload](CompanyUpdated, true),
		def[CompanyHealthPayload](CompanyHealthDropped, false),
		def[TicketPayload](TicketCreated, true),
		def[TicketPayload](TicketUpdated, true),
		def[AssignmentPayload](TicketAssigned, true),
		def[StatusChangePayload](TicketStatusChanged, true),
		def[TicketCommentPayload](TicketCommented, true),
		def[TicketPayload](TicketResolved, true),
		def[TicketPayload](TicketClosed, true),
		def[TicketPayload](TicketReopened, true),
		def[SLAPayload](SLAApplied, false),
		def[SLAPayload](SLAWarning, true),
		def[SLAPayload](SLABreached, true),
		def[SLAPayload](SLAMet, false),
		def[CSATPayload](CSATSubmitted, true),
		def[PresencePayload](AgentPresenceChanged, false),
	}
	out := make(map[string]Definition, len(list))
	for _, d := range list {
		out[d.Name] = d
	}
	return out
}()

// LookupDefinition returns the typed definition of a domain event.
func LookupDefinition(name string) (Definition, bool) {
	d, ok := domainDefinitions[name]
	return d, ok
}

// DomainDefinitions returns every typed definition sorted by name.
func DomainDefinitions() []Definition {
	out := make([]Definition, 0, len(domainDefinitions))
	for _, d := range domainDefinitions {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// WebhookEvents returns the event names organizations can subscribe to.
func WebhookEvents() []string {
	out := []string{}
	for _, d := range DomainDefinitions() {
		if d.Webhook {
			out = append(out, d.Name)
		}
	}
	return out
}

// NewDomain builds an organization-scoped catalog event. payload must be the
// registered payload type for name (value, not pointer).
func NewDomain(name string, organizationID int64, payload any) (Event, error) {
	d, ok := domainDefinitions[name]
	if !ok {
		return Event{}, fmt.Errorf("events: %q is not a typed domain event", name)
	}
	if reflect.TypeOf(payload) != d.payload {
		return Event{}, fmt.Errorf("events: %s expects %s, got %T", name, d.payload, payload)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("events: marshal %s: %w", name, err)
	}
	data := map[string]any{}
	if err := json.Unmarshal(body, &data); err != nil {
		return Event{}, fmt.Errorf("events: unmarshal %s: %w", name, err)
	}
	ev := New(name).WithOrganization(organizationID).WithPayload(NormalizeNumbers(data).(map[string]any))
	ev.Version = d.Version
	return ev, nil
}

// DecodePayload reads a typed payload. It fails when T is not the payload
// type registered for the event or the event is newer than this build knows.
func DecodePayload[T any](ev Event) (T, error) {
	var out T
	d, ok := domainDefinitions[ev.Name]
	if !ok {
		return out, fmt.Errorf("events: %q is not a typed domain event", ev.Name)
	}
	if want := reflect.TypeFor[T](); want != d.payload {
		return out, fmt.Errorf("events: %s carries %s, not %s", ev.Name, d.payload, want)
	}
	if ev.Version > d.Version {
		return out, fmt.Errorf("events: %s version %d is newer than supported %d", ev.Name, ev.Version, d.Version)
	}
	body, err := json.Marshal(ev.Payload)
	if err != nil {
		return out, fmt.Errorf("events: marshal %s payload: %w", ev.Name, err)
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("events: decode %s payload: %w", ev.Name, err)
	}
	return out, nil
}
