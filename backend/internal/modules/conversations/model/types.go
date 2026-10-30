// Package model holds the vocabulary of the conversation data model (CF-25,
// docs/design/conversations.md): statuses, priorities, message directions,
// senders and kinds shared by the repository, usecases and handlers.
package model

import "slices"

// Resource is the activity / realtime slug of the inbox.
const Resource = "tenant.conversations"

// Conversation statuses (base categories; custom statuses map onto them).
const (
	StatusOpen     = "open"
	StatusPending  = "pending"
	StatusSnoozed  = "snoozed"
	StatusResolved = "resolved"
	StatusClosed   = "closed"
)

// Statuses lists every base status.
var Statuses = []string{StatusOpen, StatusPending, StatusSnoozed, StatusResolved, StatusClosed}

// Priorities.
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

// Priorities lists every priority, lowest first.
var Priorities = []string{PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent}

// Conversation sources.
const (
	SourceInbound  = "inbound"
	SourceOutbound = "outbound"
)

// Message directions.
const (
	DirectionIn       = "in"
	DirectionOut      = "out"
	DirectionInternal = "internal"
)

// Message sender types.
const (
	SenderContact = "contact"
	SenderAgent   = "agent"
	SenderBot     = "bot"
	SenderSystem  = "system"
)

// Message kinds.
const (
	KindText        = "text"
	KindMedia       = "media"
	KindTemplate    = "template"
	KindInteractive = "interactive"
	KindEmail       = "email"
	KindNote        = "note"
	KindEvent       = "event"
	KindUnsupported = "unsupported"
)

// Message delivery statuses. Inbound messages are "received", private notes
// and system events "recorded"; outbound ones move pending → queued → sent →
// delivered → read, or end in failed.
const (
	MessageReceived  = "received"
	MessagePending   = "pending"
	MessageQueued    = "queued"
	MessageSent      = "sent"
	MessageDelivered = "delivered"
	MessageRead      = "read"
	MessageFailed    = "failed"
	MessageRecorded  = "recorded"
)

// Participant roles.
const (
	ParticipantWatcher      = "watcher"
	ParticipantCollaborator = "collaborator"
)

// PreviewLength is how many characters of the last message the inbox list keeps.
const PreviewLength = 160

// Valid reports whether v is one of allowed.
func Valid(v string, allowed []string) bool {
	return slices.Contains(allowed, v)
}

// Preview shortens a message text for the inbox list (rune safe, single line).
func Preview(text string) string {
	out := make([]rune, 0, PreviewLength)
	space := false
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			if !space && len(out) > 0 {
				out = append(out, ' ')
			}
			space = true
			continue
		}
		space = false
		if len(out) >= PreviewLength {
			out = append(out[:PreviewLength-1], '…')
			return string(out)
		}
		out = append(out, r)
	}
	if len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
	}
	return string(out)
}
