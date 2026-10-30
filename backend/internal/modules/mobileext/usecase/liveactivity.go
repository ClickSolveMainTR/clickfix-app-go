package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/pushtext"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apns"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ConsumerName is the durable consumer of the Live Activity pushes.
const ConsumerName = "mobile-live-activities"

const (
	// ActivityKit attributes type of the native SwiftUI urgent conversation
	// activity (mobile/targets/widgets).
	attributesType = "UrgentConversationAttributes"

	// Payload events.
	EventStart  = "start"
	EventUpdate = "update"
	EventEnd    = "end"

	maxPreviewRunes = 140
	maxLabelRunes   = 80
	// staleAfterDue: the activity reads as outdated this long after the due time.
	staleAfterDue = 30 * time.Minute
	pushExpiry    = time.Hour
)

// UrgentConversationState is the ActivityKit content-state of
// UrgentConversationAttributes (decoded by the app's Codable type). DueAt is
// unix seconds. The app localizes everything it renders.
type UrgentConversationState struct {
	Contact  string  `json:"contact"`
	Channel  string  `json:"channel"`
	Subject  string  `json:"subject"`
	Preview  string  `json:"preview"`
	DueAt    float64 `json:"dueAt"`
	Breached bool    `json:"breached"`
}

// UrgentConversationActivity is one push about a conversation. Locale only
// picks the language of the start alert.
type UrgentConversationActivity struct {
	Conversation uuid.UUID
	Locale       string
	State        UrgentConversationState
}

type alert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type apsPayload struct {
	Timestamp      int64                   `json:"timestamp"`
	Event          string                  `json:"event"`
	ContentState   UrgentConversationState `json:"content-state"`
	AttributesType string                  `json:"attributes-type,omitempty"`
	Attributes     map[string]string       `json:"attributes,omitempty"`
	Alert          *alert                  `json:"alert,omitempty"`
	StaleDate      int64                   `json:"stale-date,omitempty"`
	DismissalDate  int64                   `json:"dismissal-date,omitempty"`
}

// BuildUrgentConversationPayload renders the ActivityKit push body.
// attributes-type, attributes and alert are only sent with start;
// dismissal-date only with end.
func BuildUrgentConversationPayload(event string, a UrgentConversationActivity, now time.Time) ([]byte, error) {
	aps := apsPayload{Timestamp: now.Unix(), Event: event, ContentState: a.State}
	due := time.Unix(0, int64(a.State.DueAt*float64(time.Second)))
	switch event {
	case EventStart:
		aps.AttributesType = attributesType
		aps.Attributes = map[string]string{"conversation": a.Conversation.String()}
		kind := "sla.warning"
		if a.State.Breached {
			kind = "sla.breached"
		}
		t := pushtext.For(kind, localeOf(a.Locale), nil)
		aps.Alert = &alert{Title: t.Title, Body: t.Body}
		aps.StaleDate = staleDate(due, now)
	case EventUpdate:
		aps.StaleDate = staleDate(due, now)
	case EventEnd:
		aps.DismissalDate = now.Unix()
	default:
		return nil, fmt.Errorf("mobileext: unknown live activity event %q", event)
	}
	return json.Marshal(map[string]any{"aps": aps})
}

func staleDate(due, now time.Time) int64 {
	stale := due.Add(staleAfterDue)
	if floor := now.Add(staleAfterDue); stale.Before(floor) {
		stale = floor
	}
	return stale.Unix()
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// oneLine collapses line breaks and runs of whitespace into single spaces.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func localeOf(raw string) string {
	if raw == "en" {
		return "en"
	}
	return "tr"
}

func conversationActivity(c db.GetLiveActivityConversationRow, due time.Time, breached bool, locale string) UrgentConversationActivity {
	subject := oneLine(c.Subject)
	if subject == "" {
		subject = "#" + strconv.FormatInt(c.Number, 10)
	}
	return UrgentConversationActivity{
		Conversation: c.Uuid,
		Locale:       locale,
		State: UrgentConversationState{
			Contact:  truncateRunes(oneLine(c.ContactName), maxLabelRunes),
			Channel:  c.ChannelType,
			Subject:  truncateRunes(subject, maxLabelRunes),
			Preview:  truncateRunes(oneLine(c.LastMessagePreview), maxPreviewRunes),
			DueAt:    float64(due.UnixMilli()) / 1000,
			Breached: breached,
		},
	}
}

// Consumer delivers Live Activity pushes on SLA and conversation events.
// Register it only when PushConfigured.
func (s *Service) Consumer() eventdispatch.Consumer {
	return eventdispatch.Consumer{
		Name:     ConsumerName,
		Events:   []string{events.SLAWarning, events.SLABreached, events.ConversationResolved, events.MessageSent},
		MaxRetry: 5,
		Timeout:  time.Minute,
		Handle:   s.HandleEvent,
	}
}

// HandleEvent maps one event to Live Activity pushes. Permanent APNs
// rejections are logged and disable the token; only transient failures of
// idempotent pushes (update / end) are returned for a retry.
func (s *Service) HandleEvent(ctx context.Context, ev events.Event) error {
	if !s.PushConfigured() || ev.TenantID == nil {
		return nil
	}
	org := *ev.TenantID
	switch ev.Name {
	case events.SLAWarning:
		p, err := events.DecodePayload[events.SLAPayload](ev)
		if err != nil || p.TargetType != "conversation" {
			return err
		}
		return s.startUrgent(ctx, org, p)
	case events.SLABreached:
		p, err := events.DecodePayload[events.SLAPayload](ev)
		if err != nil || p.TargetType != "conversation" {
			return err
		}
		return s.updateUrgent(ctx, org, p)
	case events.ConversationResolved:
		// The inbox publishes these under an alias with its own payload; the
		// conversation always travels as the event entity.
		p, _ := events.DecodePayload[events.ConversationPayload](ev)
		id, ref := conversationRef(ev, p.ConversationRef)
		return s.endUrgent(ctx, ref, id, org)
	case events.MessageSent:
		p, _ := events.DecodePayload[events.MessagePayload](ev)
		if !agentReply(ev, p) {
			return nil
		}
		id, ref := conversationRef(ev, p.ConversationRef)
		return s.endUrgent(ctx, ref, id, org)
	}
	return nil
}

// conversationRef prefers the typed payload and falls back to the event entity.
func conversationRef(ev events.Event, r events.ConversationRef) (int64, uuid.UUID) {
	id, ref := r.ConversationID, r.ConversationUUID
	if ev.EntityType == "conversation" {
		if id == 0 && ev.EntityID != nil {
			id = *ev.EntityID
		}
		if ref == uuid.Nil && ev.EntityUUID != nil {
			ref = *ev.EntityUUID
		}
	}
	return id, ref
}

// agentReply reports a public reply written by a person. Typed payloads name
// the sender; inbox payloads carry the author as the event actor (AI replies
// have none and are flagged "ai").
func agentReply(ev events.Event, p events.MessagePayload) bool {
	if p.Private || p.Direction == "inbound" {
		return false
	}
	if p.SenderType != "" {
		return p.SenderType == "agent"
	}
	if ai, _ := ev.Payload["ai"].(bool); ai {
		return false
	}
	return ev.ActorUserID != nil && *ev.ActorUserID > 0
}

// recipientAllowed applies the user's push and Live Activity switches.
func (s *Service) recipientAllowed(ctx context.Context, userID int64) (db.GetLiveActivityRecipientRow, bool, error) {
	rcp, err := s.store.Recipient(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return rcp, false, nil
	}
	if err != nil {
		return rcp, false, err
	}
	return rcp, rcp.Status != "disabled" && rcp.PushEnabled && rcp.LiveActivitiesEnabled, nil
}

func (s *Service) loadConversation(ctx context.Context, org, id int64) (db.GetLiveActivityConversationRow, bool, error) {
	c, err := s.store.Conversation(ctx, org, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, false, nil
	}
	return c, err == nil, err
}

func closedStatus(status string) bool {
	return status == "resolved" || status == "closed"
}

func (s *Service) startUrgent(ctx context.Context, org int64, p events.SLAPayload) error {
	c, ok, err := s.loadConversation(ctx, org, p.TargetID)
	if err != nil || !ok || closedStatus(c.Status) {
		return err
	}
	assignee := p.AssigneeUserID
	if assignee == nil && c.AssigneeUserID.Valid {
		assignee = &c.AssigneeUserID.Int64
	}
	if assignee == nil {
		return nil
	}
	rcp, allowed, err := s.recipientAllowed(ctx, *assignee)
	if err != nil || !allowed {
		return err
	}
	// One warning per SLA metric: an activity already running for this
	// conversation on the assignee's device is not started again.
	running, err := s.store.UpdateTokensByRef(ctx, ActivityUrgentConversation, c.Uuid.String())
	if err != nil {
		return err
	}
	for _, t := range running {
		if t.UserID == *assignee {
			return nil
		}
	}
	tokens, err := s.store.PushToStartTokens(ctx, *assignee, ActivityUrgentConversation)
	if err != nil || len(tokens) == 0 {
		return err
	}
	now := s.now()
	body, err := BuildUrgentConversationPayload(EventStart, conversationActivity(c, p.DueAt, false, rcp.Locale), now)
	if err != nil {
		return err
	}
	// A repeated start would show a second activity, so a start is never
	// retried: failures are logged only.
	_ = s.sendAll(ctx, tokens, body, apns.PriorityHigh, now.Add(pushExpiry), "")
	return nil
}

func (s *Service) updateUrgent(ctx context.Context, org int64, p events.SLAPayload) error {
	c, ok, err := s.loadConversation(ctx, org, p.TargetID)
	if err != nil || !ok || closedStatus(c.Status) {
		return err
	}
	tokens, err := s.store.UpdateTokensByRef(ctx, ActivityUrgentConversation, c.Uuid.String())
	if err != nil || len(tokens) == 0 {
		return err
	}
	now := s.now()
	var failed error
	for userID, list := range byUser(tokens) {
		rcp, allowed, err := s.recipientAllowed(ctx, userID)
		if err != nil {
			return err
		}
		if !allowed {
			continue
		}
		body, err := BuildUrgentConversationPayload(EventUpdate, conversationActivity(c, p.DueAt, true, rcp.Locale), now)
		if err != nil {
			return err
		}
		if err := s.sendAll(ctx, list, body, apns.PriorityHigh, now.Add(pushExpiry), ""); err != nil {
			failed = err
		}
	}
	return failed
}

func (s *Service) endUrgent(ctx context.Context, convUUID uuid.UUID, convID, org int64) error {
	if convUUID == uuid.Nil {
		return nil
	}
	tokens, err := s.store.UpdateTokensByRef(ctx, ActivityUrgentConversation, convUUID.String())
	if err != nil || len(tokens) == 0 {
		return err
	}
	c, ok, err := s.loadConversation(ctx, org, convID)
	if err != nil {
		return err
	}
	if !ok {
		c = db.GetLiveActivityConversationRow{Uuid: convUUID}
	}
	now := s.now()
	var failed error
	for userID, list := range byUser(tokens) {
		rcp, allowed, err := s.recipientAllowed(ctx, userID)
		if err != nil {
			return err
		}
		if !allowed {
			continue
		}
		due := now
		if c.SlaBreachedAt.Valid {
			due = c.SlaBreachedAt.Time
		}
		body, err := BuildUrgentConversationPayload(EventEnd, conversationActivity(c, due, c.SlaBreachedAt.Valid, rcp.Locale), now)
		if err != nil {
			return err
		}
		// An ended activity is finished: its tokens are not used again.
		if err := s.sendAll(ctx, list, body, apns.PriorityLow, now.Add(4*pushExpiry), "ended"); err != nil {
			failed = err
		}
	}
	return failed
}

func byUser(tokens []db.LiveActivityToken) map[int64][]db.LiveActivityToken {
	out := map[int64][]db.LiveActivityToken{}
	for _, t := range tokens {
		out[t.UserID] = append(out[t.UserID], t)
	}
	return out
}

// sendAll pushes body to every token. Permanently rejected tokens are
// disabled; accepted ones are disabled with doneReason when it is set. It
// returns an error only for transient failures (transport, 429, 5xx).
func (s *Service) sendAll(ctx context.Context, tokens []db.LiveActivityToken, body []byte, priority int, expires time.Time, doneReason string) error {
	var transient error
	for _, t := range tokens {
		res, err := s.sender.SendLiveActivity(ctx, apns.Notification{
			Token: t.Token, Environment: apns.Environment(t.Environment),
			Priority: priority, Expiration: expires, Payload: body,
		})
		switch {
		case err != nil:
			s.log.Warn("live_activity_push_failed", "token_id", t.ID, "error", err)
			transient = err
		case res.OK():
			if doneReason != "" {
				s.disable(ctx, t.ID, doneReason)
			}
		case res.TokenInvalid():
			s.log.Info("live_activity_token_rejected", "token_id", t.ID, "status", res.Status, "reason", res.Reason)
			s.disable(ctx, t.ID, truncateRunes(res.Reason, 64))
		case res.Retryable():
			s.log.Warn("live_activity_push_deferred", "token_id", t.ID, "status", res.Status, "reason", res.Reason)
			transient = fmt.Errorf("apns: %d %s", res.Status, res.Reason)
		default:
			// Payload or configuration problems are permanent: log only.
			s.log.Error("live_activity_push_rejected", "token_id", t.ID, "status", res.Status, "reason", res.Reason)
		}
	}
	return transient
}

func (s *Service) disable(ctx context.Context, id int64, reason string) {
	if reason == "" {
		reason = "rejected"
	}
	if err := s.store.DisableLiveActivityToken(ctx, id, reason); err != nil {
		s.log.Warn("live_activity_token_disable_failed", "token_id", id, "error", err)
	}
}
