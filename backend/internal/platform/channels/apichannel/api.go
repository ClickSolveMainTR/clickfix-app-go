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
	"time"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.Credentials["webhook_url"], bytes.NewReader(body))
	if err != nil {
		return channels.SendResult{}, errors.New("invalid webhook URL")
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ClickFix-Timestamp", ts)
	req.Header.Set("X-ClickFix-Signature", channelhttp.Sign(inst.Credentials["signing_secret"], ts, body))
	req.Header.Set("Idempotency-Key", msg.MessageUUID.String())
	if err := channelhttp.Do(a.client, req, nil); err != nil {
		return channels.SendResult{}, err
	}
	return channels.SendResult{ExternalID: msg.MessageUUID.String()}, nil
}

var _ channels.Adapter = (*Adapter)(nil)
