// Package apichannel connects an organization's custom application with signed JSON webhooks.
package apichannel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Adapter struct{ client channelhttp.Client }

func New(client channelhttp.Client) *Adapter {
	if client == nil {
		client = channelhttp.PublicClient()
	}
	return &Adapter{client: client}
}
func (*Adapter) Type() string { return channels.TypeAPI }
func (*Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{Threading: true, MaxTextLength: 32000}
}
func (*Adapter) Fields() []channels.Field {
	return channelhttp.Fields("signing_secret webhook_url", "", "signing_secret")
}
func (*Adapter) Connect(_ context.Context, inst channels.Instance) (string, error) {
	if err := channelhttp.Required(inst, "signing_secret", "webhook_url"); err != nil {
		return "", err
	}
	if len(inst.Credentials["signing_secret"]) < 32 {
		return "", errors.New("signing secret must have at least 32 characters")
	}
	return "", channelhttp.ValidateURL(inst.Credentials["webhook_url"])
}
func (*Adapter) VerifyWebhook(r *http.Request, b []byte, inst channels.Instance) error {
	return channelhttp.Verify(r, b, inst.Credentials["signing_secret"], time.Now())
}

// Message is the inbound contract. IDs are scoped to the connected channel instance.
type Message struct {
	ID         string    `json:"id"`
	SenderID   string    `json:"sender_id"`
	SenderName string    `json:"sender_name,omitempty"`
	Text       string    `json:"text"`
	ThreadID   string    `json:"thread_id,omitempty"`
	ReplyTo    string    `json:"reply_to,omitempty"`
	At         time.Time `json:"at,omitempty"`
}

func (*Adapter) ParseWebhook(_ context.Context,
	_ *http.Request,
	b []byte,
	_ channels.Instance) (channels.WebhookResult, error) {
	var m Message
	if err := json.Unmarshal(b, &m); err != nil {
		return channels.WebhookResult{}, err
	}
	if m.ID == "" ||
		m.SenderID == "" ||
		m.Text == "" ||
		len(m.ID) > 255 ||
		len(m.SenderID) > 255 ||
		len([]rune(m.Text)) > 32000 ||
		len(m.ThreadID) > 255 {
		return channels.WebhookResult{}, errors.New("id, sender_id and text required; field limit exceeded")
	}
	if m.At.IsZero() {
		m.At = time.Now().UTC()
	}
	return channels.WebhookResult{Messages: []channels.InboundMessage{{ExternalID: m.ID,
			From: channels.Sender{ExternalID: m.SenderID,
				Name:           m.SenderName,
				InstanceScoped: true},
			Kind:              "text",
			Text:              m.Text,
			ThreadID:          m.ThreadID,
			ReplyToExternalID: m.ReplyTo,
			At:                m.At}}},
		nil
}
func (a *Adapter) Send(ctx context.Context,
	inst channels.Instance,
	msg channels.OutboundMessage) (channels.SendResult,
	error) {
	if _, err := a.Connect(ctx, inst); err != nil {
		return channels.SendResult{}, channels.Permanent("configuration", err)
	}
	if len(msg.Attachments) > 0 || msg.Text == "" || len([]rune(msg.Text)) > 32000 {
		return channels.SendResult{}, channels.Permanent("unsupported_media", errors.New("API channel accepts text only"))
	}
	if msg.MessageUUID.String() == "00000000-0000-0000-0000-000000000000" {
		return channels.SendResult{}, channels.Permanent("message_id", errors.New("message UUID required"))
	}
	body,
		err := json.Marshal(map[string]any{"id": msg.MessageUUID,
		"conversation_id": msg.ConversationUUID,
		"recipient_id":    msg.To.ExternalID,
		"text":            msg.Text,
		"thread_id":       msg.ThreadID,
		"reply_to":        msg.ReplyToExternalID})
	if err != nil {
		return channels.SendResult{}, err
	}
	if err := a.post(ctx, inst, "", msg.MessageUUID.String(), body); err != nil {
		return channels.SendResult{}, err
	}
	return channels.SendResult{ExternalID: msg.MessageUUID.String()}, nil
}

// NotifyStatus posts a signed status_changed event (CF-245) when an agent
// or a rule changes the conversation's status. The event id is stable
// across retries and is sent as Idempotency-Key; receivers drop repeats.
// A snoozed conversation has to=snoozed and snoozed_until.
func (a *Adapter) NotifyStatus(ctx context.Context, inst channels.Instance, ch channels.StatusChange) error {
	if _, err := a.Connect(ctx, inst); err != nil {
		return channels.Permanent("configuration", err)
	}
	if ch.EventID == uuid.Nil || ch.ConversationUUID == uuid.Nil || ch.To == "" {
		return channels.Permanent("status_event", errors.New("event id, conversation and target status required"))
	}
	at := ch.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	payload := map[string]any{
		"id": ch.EventID, "type": "status_changed",
		"conversation_id": ch.ConversationUUID, "thread_id": ch.ThreadID,
		"from": ch.From, "to": ch.To, "changed_at": at.UTC(),
	}
	if ch.SnoozedUntil != nil {
		payload["snoozed_until"] = ch.SnoozedUntil.UTC()
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return a.post(ctx, inst, "status_changed", ch.EventID.String(), body)
}

// post sends one signed body to the organization's webhook URL. event sets
// X-ClickFix-Event when not empty (message deliveries carry none, as
// before CF-245).
func (a *Adapter) post(ctx context.Context, inst channels.Instance, event, key string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.Credentials["webhook_url"], bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid webhook URL")
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	if event != "" {
		req.Header.Set("X-ClickFix-Event", event)
	}
	req.Header.Set("X-ClickFix-Timestamp", ts)
	req.Header.Set("X-ClickFix-Signature", channelhttp.Sign(inst.Credentials["signing_secret"], ts, body))
	req.Header.Set("Idempotency-Key", key)
	return channelhttp.Do(a.client, req, nil)
}

// TestDelivery posts a signed test event to the organization's webhook
// URL (event type "test", header X-ClickFix-Event: test) through the
// SSRF-guarded client, so the integration can be verified before real
// traffic. The receiver should answer 2xx and must not create a message.
func (a *Adapter) TestDelivery(ctx context.Context, inst channels.Instance, _ string) (channels.DeliveryTest, error) {
	if _, err := a.Connect(ctx, inst); err != nil {
		return channels.DeliveryTest{}, err
	}
	id := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	body, err := json.Marshal(map[string]any{"id": id, "type": "test", "text": "ClickFix connection test", "sent_at": time.Now().UTC()})
	if err != nil {
		return channels.DeliveryTest{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.Credentials["webhook_url"], bytes.NewReader(body))
	if err != nil {
		return channels.DeliveryTest{}, errors.New("invalid webhook URL")
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ClickFix-Event", "test")
	req.Header.Set("X-ClickFix-Timestamp", ts)
	req.Header.Set("X-ClickFix-Signature", channelhttp.Sign(inst.Credentials["signing_secret"], ts, body))
	req.Header.Set("Idempotency-Key", id)
	out := channels.DeliveryTest{EventID: id}
	res, err := a.client.Do(req)
	if err != nil {
		out.Detail = "the endpoint could not be reached"
		if errors.Is(err, channelhttp.ErrWebhookTarget) {
			out.Detail = channelhttp.ErrWebhookTarget.Error()
		}
		return out, nil
	}
	_ = res.Body.Close()
	out.StatusCode = res.StatusCode
	out.OK = res.StatusCode >= 200 && res.StatusCode < 300
	if !out.OK {
		out.Detail = "the endpoint answered HTTP " + strconv.Itoa(res.StatusCode)
	}
	return out, nil
}

var (
	_ channels.Adapter        = (*Adapter)(nil)
	_ channels.DeliveryTester = (*Adapter)(nil)
	_ channels.StatusNotifier = (*Adapter)(nil)
)
