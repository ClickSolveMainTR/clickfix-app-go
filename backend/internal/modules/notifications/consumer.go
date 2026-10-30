package notifications

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	notifusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ConsumerName is the durable consumer of helpdesk events (CF-9).
const ConsumerName = "notifications"

// Lookup resolves what a payload does not carry (conversation number,
// customer, channel, last message, SLA deadline and assignee; author names).
type Lookup interface {
	Conversation(ctx context.Context, organizationID, conversationID int64) (ConversationInfo, error)
	UserName(ctx context.Context, userID int64) string
}

// ConversationInfo is the part of a conversation a notification needs.
type ConversationInfo struct {
	UUID           uuid.UUID
	Number         int64
	Subject        string
	AssigneeUserID *int64
	Customer       string
	ChannelType    string
	LastMessage    string
	// DueAt is the nearest open SLA deadline, if any.
	DueAt    *time.Time
	Timezone string
}

// Vars are the content facts of the conversation (content.Vars keys).
func (c ConversationInfo) Vars() map[string]string {
	v := map[string]string{
		"number": strconv.FormatInt(c.Number, 10), "subject": c.Subject,
		"customer": c.Customer, "channel": c.ChannelType, "preview": c.LastMessage,
		"timezone": c.Timezone, "url": conversationURL(c.UUID),
	}
	if c.DueAt != nil {
		v["due"] = c.DueAt.UTC().Format(time.RFC3339)
	}
	if c.AssigneeUserID == nil {
		v["assignable"] = "1"
	}
	return v
}

// DBLookup implements Lookup with sqlc queries.
type DBLookup struct{ Q *db.Queries }

// Conversation loads one conversation of the organization.
func (l DBLookup) Conversation(ctx context.Context, organizationID, conversationID int64) (ConversationInfo, error) {
	c, err := l.Q.NotificationConversationContext(ctx, db.NotificationConversationContextParams{OrganizationID: organizationID, ID: conversationID})
	if err != nil {
		return ConversationInfo{}, err
	}
	info := ConversationInfo{
		UUID: c.Uuid, Number: c.Number, Subject: c.Subject, Customer: c.ContactName,
		ChannelType: c.ChannelType, LastMessage: c.LastMessagePreview, Timezone: c.OrganizationTimezone,
	}
	if c.AssigneeUserID.Valid {
		id := c.AssigneeUserID.Int64
		info.AssigneeUserID = &id
	}
	info.DueAt = nearestDue(c.FirstResponseAt.Valid, c.FirstResponseDueAt, c.NextResponseDueAt, c.ResolutionDueAt)
	return info, nil
}

// nearestDue is the earliest deadline still open: first response until it
// happened, then next response and resolution.
func nearestDue(responded bool, first, next, resolution pgtype.Timestamptz) *time.Time {
	var best *time.Time
	consider := func(t pgtype.Timestamptz) {
		if t.Valid && (best == nil || t.Time.Before(*best)) {
			at := t.Time
			best = &at
		}
	}
	if !responded {
		consider(first)
	}
	consider(next)
	consider(resolution)
	return best
}

// UserName returns "Name Surname" or "".
func (l DBLookup) UserName(ctx context.Context, userID int64) string {
	u, err := l.Q.GetUserByID(ctx, userID)
	if err != nil {
		return ""
	}
	if u.Surname != "" {
		return u.Name + " " + u.Surname
	}
	return u.Name
}

func conversationURL(id uuid.UUID) string {
	return "/inbox?c=" + id.String()
}

// consumedEvents are the outbox events this consumer turns into
// notifications. The inbox publishes the plural names (messages.received,
// surveys.responded); the singular CF-6 names are handled too.
var consumedEvents = []string{
	events.ConversationAssigned, events.MessageReceived, events.ConversationMentioned,
	events.TicketAssigned, events.TicketCommented, events.SLAWarning, events.SLABreached,
	events.CompanyHealthDropped, events.CSATSubmitted,
	events.MessagesReceived, events.SurveyResponded,
}

// messageWindow coalesces new-message notifications: one per conversation
// and user every few minutes, however many messages the customer sends.
const messageWindow = 5 * time.Minute

// EventConsumer turns helpdesk domain events into per-user notifications
// through the preference matrix. Users never get notified of their own action.
func EventConsumer(svc *notifusecase.Service, lookup Lookup) eventdispatch.Consumer {
	return eventdispatch.Consumer{
		Name:     ConsumerName,
		Events:   consumedEvents,
		MaxRetry: 8,
		Timeout:  time.Minute,
		Handle: func(ctx context.Context, ev events.Event) error {
			in, err := notifyInput(ctx, lookup, ev)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil // the conversation is gone; nothing to tell
				}
				return err
			}
			if in == nil {
				return nil
			}
			if ev.ActorUserID != nil {
				in.UserIDs = without(in.UserIDs, *ev.ActorUserID)
			}
			if len(in.UserIDs) == 0 {
				return nil
			}
			in.OrganizationID = ev.TenantID
			if in.Payload == nil {
				in.Payload = map[string]any{}
			}
			in.Payload["event_id"] = ev.EventID.String()
			if in.DedupeKey == "" {
				in.DedupeKey = ev.EventID.String()
			}
			return svc.NotifyEvent(ctx, *in)
		},
	}
}

func without(ids []int64, drop int64) []int64 {
	out := ids[:0:0]
	for _, id := range ids {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}

func one(id *int64) []int64 {
	if id == nil {
		return nil
	}
	return []int64{*id}
}

// notifyInput maps an event to recipients and content facts; nil means
// nobody is notified.
func notifyInput(ctx context.Context, lookup Lookup, ev events.Event) (*notifusecase.NotifyInput, error) {
	if ev.TenantID == nil {
		return nil, nil
	}
	org := *ev.TenantID
	conv := func(id int64) (ConversationInfo, map[string]string, error) {
		c, err := lookup.Conversation(ctx, org, id)
		if err != nil {
			return c, nil, err
		}
		return c, c.Vars(), nil
	}
	switch ev.Name {
	case events.ConversationAssigned:
		p, err := events.DecodePayload[events.AssignmentPayload](ev)
		if err != nil || p.ConversationID == nil {
			return nil, err
		}
		c, vars, err := conv(*p.ConversationID)
		if err != nil {
			return nil, err
		}
		if ev.ActorUserID != nil {
			vars["actor"] = lookup.UserName(ctx, *ev.ActorUserID)
		}
		return &notifusecase.NotifyInput{Event: ev.Name, UserIDs: one(p.AssigneeUserID), Vars: vars, ActionURL: conversationURL(c.UUID)}, nil
	case events.MessageReceived:
		p, err := events.DecodePayload[events.MessagePayload](ev)
		if err != nil || p.Private || p.Direction == "outbound" {
			return nil, err
		}
		c, vars, err := conv(p.ConversationID)
		if err != nil {
			return nil, err
		}
		if p.Preview != "" {
			vars["preview"] = p.Preview
		}
		return newMessage(c, vars, ev), nil
	case events.MessagesReceived:
		// Inbox event: entity is the conversation, always inbound.
		if ev.EntityID == nil {
			return nil, nil
		}
		c, vars, err := conv(*ev.EntityID)
		if err != nil {
			return nil, err
		}
		return newMessage(c, vars, ev), nil
	case events.ConversationMentioned:
		p, err := events.DecodePayload[events.MentionPayload](ev)
		if err != nil {
			return nil, err
		}
		c, vars, err := conv(p.ConversationID)
		if err != nil {
			return nil, err
		}
		vars["actor"] = lookup.UserName(ctx, p.AuthorUserID)
		vars["preview"] = ""
		return &notifusecase.NotifyInput{Event: ev.Name, UserIDs: without(p.MentionedUserIDs, p.AuthorUserID), Vars: vars, ActionURL: conversationURL(c.UUID)}, nil
	case events.TicketAssigned:
		p, err := events.DecodePayload[events.AssignmentPayload](ev)
		if err != nil || p.TicketID == nil {
			return nil, err
		}
		ticket := "#" + strconv.FormatInt(*p.TicketID, 10)
		vars := map[string]string{"ticket": ticket}
		if ev.ActorUserID != nil {
			vars["actor"] = lookup.UserName(ctx, *ev.ActorUserID)
		}
		return &notifusecase.NotifyInput{Event: ev.Name, UserIDs: one(p.AssigneeUserID), Vars: vars}, nil
	case events.TicketCommented:
		// Ticket followers come with the tickets module; until then nobody
		// but the assignee in ticket.assigned is known.
		return nil, nil
	case events.SLAWarning, events.SLABreached:
		p, err := events.DecodePayload[events.SLAPayload](ev)
		if err != nil {
			return nil, err
		}
		vars := map[string]string{"metric": p.Metric, "due": p.DueAt.UTC().Format(time.RFC3339), "target": fmt.Sprintf("#%d", p.TargetID)}
		action := ""
		recipients := one(p.AssigneeUserID)
		if p.TargetType == "conversation" {
			c, cv, err := conv(p.TargetID)
			if err != nil {
				return nil, err
			}
			for k, v := range cv {
				if k != "due" {
					vars[k] = v
				}
			}
			vars["target"] = "#" + cv["number"]
			action = conversationURL(c.UUID)
			if len(recipients) == 0 {
				recipients = one(c.AssigneeUserID)
			}
		}
		return &notifusecase.NotifyInput{Event: ev.Name, UserIDs: recipients, Vars: vars, ActionURL: action}, nil
	case events.CompanyHealthDropped:
		p, err := events.DecodePayload[events.CompanyHealthPayload](ev)
		if err != nil {
			return nil, err
		}
		prev := "—"
		if p.PreviousScore != nil {
			prev = strconv.Itoa(*p.PreviousScore)
		}
		vars := map[string]string{
			"company": p.CompanyName, "score": strconv.Itoa(p.Score),
			"threshold": strconv.Itoa(p.Threshold), "previous": prev, "previous_score": prev,
		}
		return &notifusecase.NotifyInput{Event: ev.Name, UserIDs: one(p.AccountOwnerUserID), Vars: vars,
			ActionURL: "/companies/" + p.CompanyUUID.String(),
			Payload:   map[string]any{"company_uuid": p.CompanyUUID.String(), "score": p.Score}}, nil
	case events.CSATSubmitted:
		p, err := events.DecodePayload[events.CSATPayload](ev)
		if err != nil || p.ConversationID == nil {
			return nil, err
		}
		return csat(conv, *p.ConversationID, p.Rating, p.Comment, ev.Name)
	case events.SurveyResponded:
		// Inbox event: entity is the conversation; only CSAT notifies.
		if ev.EntityID == nil || ev.Payload["kind"] != "csat" {
			return nil, nil
		}
		score, ok := ev.Payload["score"].(float64)
		if !ok {
			return nil, nil
		}
		comment, _ := ev.Payload["comment"].(string)
		return csat(conv, *ev.EntityID, int(score), comment, events.CSATSubmitted)
	}
	return nil, nil
}

// newMessage notifies the assignee of a customer message, at most once per
// conversation every messageWindow.
func newMessage(c ConversationInfo, vars map[string]string, ev events.Event) *notifusecase.NotifyInput {
	window := ev.OccurredAt
	if window.IsZero() {
		window = time.Now()
	}
	bucket := window.UTC().Truncate(messageWindow).Format(time.RFC3339)
	return &notifusecase.NotifyInput{
		Event: events.MessageReceived, UserIDs: one(c.AssigneeUserID), Vars: vars, ActionURL: conversationURL(c.UUID),
		DedupeKey: "message.received:" + c.UUID.String() + ":" + bucket,
	}
}

func csat(conv func(int64) (ConversationInfo, map[string]string, error), conversationID int64, rating int, comment, event string) (*notifusecase.NotifyInput, error) {
	c, vars, err := conv(conversationID)
	if err != nil {
		return nil, err
	}
	vars["rating"] = strconv.Itoa(rating)
	vars["comment"] = comment
	vars["preview"] = ""
	return &notifusecase.NotifyInput{Event: event, UserIDs: one(c.AssigneeUserID), Vars: vars, ActionURL: conversationURL(c.UUID)}, nil
}
