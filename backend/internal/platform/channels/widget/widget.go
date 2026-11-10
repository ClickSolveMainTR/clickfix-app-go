// Package widget is the web widget channel (CF-55): the chat bubble an
// organization embeds on its own site with one script tag. Unlike provider
// channels it has no webhook: the public visitor API (modules/widget) turns a
// visitor's message into a normalized channels.InboundMessage and runs the
// conversations pipeline, and agent replies are "sent" by publishing them on
// the visitor's private realtime channel (the visitor also polls, so a
// realtime outage never loses a reply: the message is already stored).
//
// An organization may create several widget instances (one per site or
// brand); the instance uuid is the public widget id of the embed snippet.
package widget

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

// IdentitySecretField is the credential key of the identity verification
// secret: the customer's backend signs its user ids with it (user_hash).
const IdentitySecretField = "identity_secret"

// ChannelPrefix is the Centrifugo namespace of visitor channels. The
// namespace keeps allow_subscribe_for_client=false: a visitor joins only
// with a subscription token the widget API issues for its own channel.
const ChannelPrefix = "widget:"

// MaxTextLength bounds one widget message.
const MaxTextLength = 10000

// Publisher publishes realtime events (realtime.Publisher).
type Publisher interface {
	Publish(ctx context.Context, channel string, data any) error
}

// Event types published on a visitor channel.
const (
	EventMessageCreated = "message.created"
	EventTyping         = "typing"
	EventConversation   = "conversation.updated"
)

// Event is one visitor channel event (docs/widget/04: one envelope).
type Event struct {
	Type             string     `json:"type"`
	ConversationUUID *uuid.UUID `json:"conversation_uuid,omitempty"`
	MessageUUID      *uuid.UUID `json:"message_uuid,omitempty"`
	Text             string     `json:"text,omitempty"`
	Typing           *bool      `json:"typing,omitempty"`
	Status           string     `json:"status,omitempty"`
	OccurredAt       time.Time  `json:"occurred_at"`
}

// Adapter is the widget channel.
type Adapter struct {
	pub Publisher
	log *slog.Logger
	now func() time.Time
}

// New builds the adapter. A nil publisher keeps sends working (visitors
// then only poll).
func New(pub Publisher, log *slog.Logger) *Adapter {
	if log == nil {
		log = slog.Default()
	}
	return &Adapter{pub: pub, log: log, now: func() time.Time { return time.Now().UTC() }}
}

// Type implements channels.Adapter.
func (a *Adapter) Type() string { return channels.TypeWidget }

// Capabilities implements channels.Adapter.
func (a *Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{
		Media: true, Typing: true, DeliveryReceipts: true, ReadReceipts: true, MaxTextLength: MaxTextLength,
	}
}

// Fields implements channels.Adapter. The identity secret is optional:
// without it the widget only knows anonymous visitors.
func (a *Adapter) Fields() []channels.Field {
	return []channels.Field{{Key: IdentitySecretField, LabelKey: "channels.fields.identity_secret", Secret: true}}
}

// Connect implements channels.Adapter: widgets route by instance uuid. A set
// identity secret must be long enough to resist guessing.
func (a *Adapter) Connect(_ context.Context, inst channels.Instance) (string, error) {
	if s := inst.Credentials[IdentitySecretField]; s != "" && len(s) < 24 {
		return "", errors.New("widget: identity_secret must be at least 24 characters")
	}
	return "", nil
}

// VerifyWebhook implements channels.Adapter: the widget has no provider
// webhook, so every webhook request is rejected.
func (a *Adapter) VerifyWebhook(*http.Request, []byte, channels.Instance) error {
	return channels.ErrSignature
}

// ParseWebhook implements channels.Adapter (never reached, see VerifyWebhook).
func (a *Adapter) ParseWebhook(context.Context, *http.Request, []byte, channels.Instance) (channels.WebhookResult, error) {
	return channels.WebhookResult{}, errors.New("widget: messages arrive through the visitor API")
}

// Send implements channels.Adapter: the reply is stored already, so sending
// is telling the visitor's open widgets about it. A failed publish is only
// logged; the widget picks the reply up on its next poll.
func (a *Adapter) Send(ctx context.Context, inst channels.Instance, msg channels.OutboundMessage) (channels.SendResult, error) {
	if strings.TrimSpace(msg.To.ExternalID) == "" {
		return channels.SendResult{}, channels.Permanent("no_visitor", errors.New("widget: recipient has no visitor id"))
	}
	conv, id := msg.ConversationUUID, msg.MessageUUID
	a.publish(ctx, inst, msg.To.ExternalID, Event{
		Type: EventMessageCreated, ConversationUUID: &conv, MessageUUID: &id, Text: msg.Text, OccurredAt: a.now(),
	})
	return channels.SendResult{ExternalID: ExternalMessageID(id)}, nil
}

// SendTyping implements channels.Typer.
func (a *Adapter) SendTyping(ctx context.Context, inst channels.Instance, to channels.Recipient, on bool) error {
	if strings.TrimSpace(to.ExternalID) == "" {
		return errors.New("widget: recipient has no visitor id")
	}
	a.publish(ctx, inst, to.ExternalID, Event{Type: EventTyping, Typing: &on, OccurredAt: a.now()})
	return nil
}

// Notify publishes an event on a visitor channel (conversation status
// changes and the like).
func (a *Adapter) Notify(ctx context.Context, inst channels.Instance, visitorExternalID string, ev Event) {
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = a.now()
	}
	a.publish(ctx, inst, visitorExternalID, ev)
}

func (a *Adapter) publish(ctx context.Context, inst channels.Instance, externalID string, ev Event) {
	if a.pub == nil {
		return
	}
	if err := a.pub.Publish(ctx, VisitorChannel(inst.UUID, externalID), ev); err != nil {
		a.log.Warn("widget_publish_failed", "instance", inst.UUID, "type", ev.Type, "error", err)
	}
}

// ExternalMessageID is the provider id of a widget reply (read receipts of
// the visitor refer to it).
func ExternalMessageID(messageUUID uuid.UUID) string {
	return "widget:" + messageUUID.String()
}

// VisitorChannel is the private realtime channel of one visitor (or
// identified user) of a widget. The contact id is hashed so the channel
// name never carries a customer's user id.
func VisitorChannel(instance uuid.UUID, externalID string) string {
	sum := sha256.Sum256([]byte(instance.String() + "\x00" + externalID))
	return ChannelPrefix + instance.String() + "." + hex.EncodeToString(sum[:16])
}

// UserHash is the identity verification signature of a user id:
// hex(HMAC-SHA256(identity secret, user id)). The customer's backend
// computes it and passes it to ClickFix('identify', {user_id, user_hash}).
func UserHash(secret, userID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID))
	return hex.EncodeToString(mac.Sum(nil))
}

// UserCompanyHash is the identity verification signature of a user id
// together with the user's company id (CF-206):
// hex(HMAC-SHA256(identity secret, user id + "\n" + company id)). The
// newline keeps the two ids apart, so "1" + "12" and "11" + "2" never sign
// the same bytes; neither id may contain a newline (identify rejects one).
// A company id is sent in identify as company.id (the company's external
// id); when it is signed, the user cannot claim another company.
func UserCompanyHash(secret, userID, companyID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID))
	mac.Write([]byte("\n"))
	mac.Write([]byte(companyID))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyUserHash reports whether hash signs userID with secret (constant
// time; an empty secret never verifies).
func VerifyUserHash(secret, userID, hash string) bool {
	if secret == "" || userID == "" || hash == "" {
		return false
	}
	return hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(hash))), []byte(UserHash(secret, userID)))
}

// VerifyUserCompanyHash reports whether hash signs userID and companyID
// together (UserCompanyHash) with secret, in constant time.
func VerifyUserCompanyHash(secret, userID, companyID, hash string) bool {
	if secret == "" || userID == "" || companyID == "" || hash == "" {
		return false
	}
	return hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(hash))), []byte(UserCompanyHash(secret, userID, companyID)))
}
