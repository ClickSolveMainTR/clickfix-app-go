package events

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type automationTraceKey struct{}

// WithAutomationTrace carries rule ids through in-process API calls into the transactional outbox.
func WithAutomationTrace(ctx context.Context, trace []string) context.Context {
	return context.WithValue(ctx, automationTraceKey{}, append([]string{}, trace...))
}

// AutomationTrace returns a copy so callers cannot mutate another execution's ancestry.
func AutomationTrace(ctx context.Context) []string {
	trace, _ := ctx.Value(automationTraceKey{}).([]string)
	return append([]string{}, trace...)
}

// DomainAliases maps inbox producer names retained for existing consumers onto catalog v1 names.
func DomainAliases() map[string]string {
	return map[string]string{
		"surveys.responded":  CSATSubmitted,
		ConversationsCreated: ConversationCreated, ConversationsUpdated: ConversationUpdated,
		ConversationsAssigned: ConversationAssigned, ConversationsResolved: ConversationResolved,
		ConversationsReopened: ConversationReopened, MessagesReceived: MessageReceived,
		MessagesSent: MessageSent, MessagesStatusChanged: MessageStatusChanged,
	}
}

// ExpandDomainAlias retains the broad inbox event and exposes catalog-specific
// status/priority transitions. Derived events have stable ids across outbox retries.
func ExpandDomainAlias(e Event) []Event {
	if e.Name == "surveys.responded" && e.Payload["kind"] != "csat" {
		return []Event{e}
	}
	canonical, ok := DomainAliases()[e.Name]
	if !ok {
		return []Event{e}
	}
	e.Name = canonical
	out := []Event{e}
	add := func(name string) {
		if name == canonical {
			return
		}
		next := e
		next.Name = name
		next.EventID = uuid.NewSHA1(e.EventID, []byte(name))
		out = append(out, next)
	}
	switch e.Payload["change"] {
	case "status_changed":
		add(ConversationStatusChanged)
		if target, ok := e.Payload["to"].(string); ok && strings.HasPrefix(target, "snoozed|") {
			add(ConversationSnoozed)
		}
	case "tag_added":
		add(ConversationTagAdded)
	case "priority_changed":
		add(ConversationPriorityChanged)
	case "assigned":
		if e.Payload["to"] == "" {
			add(ConversationUnassigned)
		}
	}
	return out
}
