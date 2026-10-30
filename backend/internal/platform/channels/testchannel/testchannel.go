// Package testchannel is the fake "test" channel (CF-8 acceptance): a
// provider-free adapter that receives signed JSON webhooks and records what
// it sends, so the whole inbound → inbox → outbound path runs end to end in
// tests, local development and demos without a Meta or mail account.
//
// Webhook body (POST /v1/webhooks/channels/test/{instance}):
//
//	{"messages":[{"id":"m1","from":{"id":"visitor-1","name":"Ada","email":"ada@example.com"},
//	              "text":"Merhaba","thread_id":"","subject":"","at":"2026-10-28T10:00:00Z"}],
//	 "statuses":[{"id":"test-…","status":"delivered","at":"…"}]}
//
// signed with X-Test-Signature: sha256=<hex HMAC-SHA256 of the body with the
// instance's webhook_secret>. Outbound texts containing "#fail" fail
// temporarily, "#reject" permanently (to exercise the retry path).
package testchannel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

// SignatureHeader carries the webhook HMAC.
const SignatureHeader = "X-Test-Signature"

// SecretField is the credential key of the webhook secret.
const SecretField = "webhook_secret"

// Adapter is the test channel. It keeps the last sent messages in memory.
type Adapter struct {
	mu   sync.Mutex
	sent []Sent
	max  int
	now  func() time.Time
}

// Sent is one recorded outbound message.
type Sent struct {
	InstanceID int64
	ExternalID string
	Message    channels.OutboundMessage
	At         time.Time
}

// New builds the adapter.
func New() *Adapter {
	return &Adapter{max: 500, now: time.Now}
}

// Type implements channels.Adapter.
func (a *Adapter) Type() string { return channels.TypeTest }

// Capabilities implements channels.Adapter.
func (a *Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{
		Media: true, Typing: false, DeliveryReceipts: true, ReadReceipts: true,
		Threading: true, MaxTextLength: 4096,
	}
}

// Fields implements channels.Adapter.
func (a *Adapter) Fields() []channels.Field {
	return []channels.Field{{Key: SecretField, LabelKey: "channels.fields.webhook_secret", Secret: true, Required: true}}
}

// Connect implements channels.Adapter: the test channel routes by instance
// uuid, so there is no provider account id.
func (a *Adapter) Connect(_ context.Context, inst channels.Instance) (string, error) {
	if len(inst.Credentials[SecretField]) < 16 {
		return "", fmt.Errorf("testchannel: %s must be at least 16 characters", SecretField)
	}
	return "", nil
}

// Sign returns the signature header value of body.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhook implements channels.Adapter.
func (a *Adapter) VerifyWebhook(r *http.Request, body []byte, inst channels.Instance) error {
	secret := inst.Credentials[SecretField]
	got := r.Header.Get(SignatureHeader)
	if secret == "" || got == "" || !hmac.Equal([]byte(got), []byte(Sign(secret, body))) {
		return channels.ErrSignature
	}
	return nil
}

type wire struct {
	Messages []struct {
		ID   string `json:"id"`
		From struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
			Phone string `json:"phone"`
		} `json:"from"`
		Text     string    `json:"text"`
		ThreadID string    `json:"thread_id"`
		Subject  string    `json:"subject"`
		ReplyTo  string    `json:"reply_to"`
		At       time.Time `json:"at"`
	} `json:"messages"`
	Statuses []struct {
		ID     string    `json:"id"`
		Status string    `json:"status"`
		Error  string    `json:"error"`
		At     time.Time `json:"at"`
	} `json:"statuses"`
}

// ParseWebhook implements channels.Adapter.
func (a *Adapter) ParseWebhook(_ context.Context, _ *http.Request, body []byte, _ channels.Instance) (channels.WebhookResult, error) {
	var w wire
	if err := json.Unmarshal(body, &w); err != nil {
		return channels.WebhookResult{}, fmt.Errorf("testchannel: decode: %w", err)
	}
	var out channels.WebhookResult
	for _, m := range w.Messages {
		if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.From.ID) == "" {
			return channels.WebhookResult{}, errors.New("testchannel: message id and from.id are required")
		}
		kind := "text"
		if m.Subject != "" || m.ThreadID != "" {
			kind = "email"
		}
		out.Messages = append(out.Messages, channels.InboundMessage{
			ExternalID: m.ID,
			From: channels.Sender{
				ExternalID: m.From.ID, InstanceScoped: true, Name: m.From.Name,
				Email: m.From.Email, Phone: m.From.Phone,
			},
			Kind: kind, Text: m.Text, ThreadID: m.ThreadID, Subject: m.Subject,
			ReplyToExternalID: m.ReplyTo, At: m.At,
		})
	}
	for _, s := range w.Statuses {
		out.Statuses = append(out.Statuses, channels.StatusUpdate{ExternalID: s.ID, Status: s.Status, Error: s.Error, At: s.At})
	}
	return out, nil
}

// Send implements channels.Adapter: records the message.
func (a *Adapter) Send(_ context.Context, inst channels.Instance, msg channels.OutboundMessage) (channels.SendResult, error) {
	switch {
	case strings.Contains(msg.Text, "#reject"):
		return channels.SendResult{}, channels.Permanent("rejected", errors.New("test channel rejected the message"))
	case strings.Contains(msg.Text, "#fail"):
		return channels.SendResult{}, &channels.SendError{Code: "unavailable", Err: errors.New("test channel temporarily unavailable")}
	}
	id := "test-" + uuid.NewString()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sent = append(a.sent, Sent{InstanceID: inst.ID, ExternalID: id, Message: msg, At: a.now()})
	if len(a.sent) > a.max {
		a.sent = a.sent[len(a.sent)-a.max:]
	}
	return channels.SendResult{ExternalID: id}, nil
}

// Sent returns a copy of the recorded messages of an instance (0 = all).
func (a *Adapter) Sent(instanceID int64) []Sent {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Sent, 0, len(a.sent))
	for _, s := range a.sent {
		if instanceID == 0 || s.InstanceID == instanceID {
			out = append(out, s)
		}
	}
	return out
}
