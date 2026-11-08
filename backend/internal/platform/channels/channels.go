// Package channels is the contract every messaging channel plugs into (CF-8,
// docs/design/channels.md). A channel type (WhatsApp, Instagram, e-mail, web
// widget, Telegram, the test channel) is one Adapter registered in a
// Registry; an organization connects accounts of a type as channel instances
// (the inboxes, table channel_instances) whose credentials are sealed by the
// Vault. The inbox core never talks to a provider: it hands an Adapter the
// decrypted Instance and gets back normalized inbound messages, delivery
// receipts and send results.
//
// Adding a channel = writing one Adapter (plus optional interfaces below)
// and registering it in httpserver; nothing in conversations changes.
package channels

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Channel types shipped with ClickFix. Adapters live under
// internal/platform/channels/<type>.
const (
	TypeTest       = "test"
	TypeWidget     = "widget"
	TypeEmail      = "email"
	TypeWhatsApp   = "whatsapp"
	TypeInstagram  = "instagram"
	TypeMessenger  = "messenger"
	TypeTelegram   = "telegram"
	TypeSlack      = "slack"
	TypeTeams      = "teams"
	TypeGoogleChat = "google_chat"
	TypeSMS        = "sms"
	TypeAPI        = "api"
	TypeVoice      = "voice"
)

// Capabilities describe what a channel supports; the inbox UI and the
// outbound pipeline read them instead of switching on the type.
type Capabilities struct {
	// Media: images, files, audio and video can be sent.
	Media bool `json:"media"`
	// Templates: pre-approved templates can be sent (WhatsApp).
	Templates bool `json:"templates"`
	// Typing: a typing indicator can be shown to the contact.
	Typing bool `json:"typing"`
	// Reactions: emoji reactions on messages.
	Reactions bool `json:"reactions"`
	// DeliveryReceipts / ReadReceipts: the provider reports them.
	DeliveryReceipts bool `json:"delivery_receipts"`
	ReadReceipts     bool `json:"read_receipts"`
	// ReplyWindow is how long after the contact's last message a free-form
	// reply is allowed (24h on Meta channels); 0 means no window.
	ReplyWindow time.Duration `json:"reply_window"`
	// Threading: messages carry a thread id and subject (e-mail).
	Threading bool `json:"threading"`
	// MaxTextLength bounds one outbound text (0 = no limit).
	MaxTextLength int `json:"max_text_length"`
}

// Field describes one credential or setting field of a channel type, for
// the connect form. Secret fields are never sent back to a client.
type Field struct {
	Key      string `json:"key"`
	LabelKey string `json:"label_key"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
}

// Instance is an organization's connected account with its decrypted
// credentials, as an adapter sees it. Never serialized to a client.
type Instance struct {
	BrandID        *uuid.UUID
	ID             int64
	UUID           uuid.UUID
	OrganizationID int64
	Type           string
	Name           string
	Status         string
	// ExternalID is the provider account id webhooks are routed by.
	ExternalID  string
	Credentials Credentials
	Settings    map[string]any
}

// Credentials are the secret values of an instance (field key → value).
type Credentials map[string]string

// Sender is the contact who wrote, as the channel knows them.
type Sender struct {
	// ExternalID is the contact's id on the channel (phone, IGSID, e-mail
	// address, chat id, widget visitor id).
	ExternalID string
	// InstanceScoped: the id only means something inside this instance
	// (page scoped ids, bot chat ids). Global ids (e-mail, phone) are false
	// so the same person is recognized across inboxes.
	InstanceScoped bool
	Name           string
	Username       string
	Email          string
	Phone          string
	Meta           map[string]any
}

// InboundAttachment is a file of an inbound message. Open streams it from
// the provider (adapters close over their credentials); it is called after
// the message is stored, so a slow download never blocks the webhook.
type InboundAttachment struct {
	Filename    string
	ContentType string
	Size        int64
	Open        func(ctx context.Context) (io.ReadCloser, error)
}

// InboundMessage is one normalized message from a contact.
type InboundMessage struct {
	// ExternalID is the provider message id, unique within the instance
	// (adapters prefix ids that are only unique per chat).
	ExternalID string
	From       Sender
	// Kind: text, media, interactive, email, unsupported (model.Kind*).
	Kind        string
	Text        string
	Attachments []InboundAttachment
	// ThreadID groups messages into one conversation when the channel has
	// threads (e-mail Message-ID root); empty = one conversation per contact.
	ThreadID          string
	Subject           string
	ReplyToExternalID string
	Attributes        map[string]any
	At                time.Time
}

// Delivery statuses an adapter may report.
const (
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusRead      = "read"
	StatusFailed    = "failed"
)

// StatusUpdate is a delivery or read receipt of an outbound message.
type StatusUpdate struct {
	ExternalID string
	Status     string
	Error      string
	At         time.Time
	// Attributes carry provider extras (WhatsApp pricing: billable,
	// category) for usage metering.
	Attributes map[string]any
}

// WebhookResult is what one webhook delivery carried.
type WebhookResult struct {
	Messages []InboundMessage
	Statuses []StatusUpdate
	Calls    []CallEvent
}

// CallEvent is a verified voice lifecycle update. Call and recording IDs
// come from the provider; the organization always comes from Instance.
type CallEvent struct {
	FromReport                 bool
	InitiatedAt                time.Time
	ProviderFinal              bool
	InternalNumber             string
	RecordingURL               string
	AnsweredAt                 time.Time
	EndedAt                    time.Time
	ExternalID                 string
	Provider                   string
	Direction                  string
	Status                     string
	From                       string
	To                         string
	DurationSeconds            int32
	RecordingMessageExternalID string
	At                         time.Time
}

// SubscriptionResponder handles signed POST subscription challenges before
// shared routing (Slack has no workspace installed during URL verification).
// It must authenticate the request itself and return handled=false for normal events.
type SubscriptionResponder interface {
	RespondWebhook(*http.Request, []byte, Instance) (contentType, body string, handled bool, err error)
}

// WebhookAcknowledger supplies a provider-specific response after verification
// and successful inbox persistence (voice instructions or an empty SMS TwiML response).
type WebhookAcknowledger interface {
	AcknowledgeWebhook(*http.Request, []byte, Instance) (contentType, body string)
}

// OutboundAttachment is a file to send. URL is a short-lived signed API URL
// providers can fetch; Open streams the bytes for providers that upload.
type OutboundAttachment struct {
	Filename    string
	ContentType string
	Size        int64
	URL         string
	Open        func(ctx context.Context) (io.ReadCloser, error)
}

// Template is a pre-approved template message (Capabilities.Templates).
type Template struct {
	Name       string
	Language   string
	Parameters []string
}

// Recipient is the contact a message goes to.
type Recipient struct {
	ExternalID string
	Name       string
	Email      string
	Phone      string
}

// OutboundMessage is one message to deliver.
type OutboundMessage struct {
	MessageUUID       uuid.UUID
	ConversationUUID  uuid.UUID
	To                Recipient
	Kind              string
	Text              string
	Template          *Template
	Attachments       []OutboundAttachment
	ThreadID          string
	Subject           string
	ReplyToExternalID string
	Attributes        map[string]any
}

// SendResult is the provider's answer to a send.
// Usage is the billable unit of a provider-accepted send (not delivery receipts).
// The outbound transaction writes it once alongside the sent message.
type Usage struct {
	Feature  string
	Quantity int64
}

type SendResult struct {
	Usage *Usage
	// ExternalID is the provider message id receipts will refer to.
	ExternalID string
}

// Adapter is one channel type.
type Adapter interface {
	// Type is the channel type code (TypeWhatsApp …).
	Type() string
	// Capabilities lists what the channel supports.
	Capabilities() Capabilities
	// Fields lists the credentials and settings the connect form asks for.
	Fields() []Field
	// Connect validates an instance's credentials with the provider (a
	// cheap authenticated call) and returns the provider account id
	// webhooks are routed by ("" when the type routes by instance uuid).
	Connect(ctx context.Context, inst Instance) (externalID string, err error)
	// VerifyWebhook authenticates a webhook request (signature header,
	// shared secret). body is the raw request body.
	VerifyWebhook(r *http.Request, body []byte, inst Instance) error
	// ParseWebhook normalizes a verified webhook body.
	ParseWebhook(ctx context.Context, r *http.Request, body []byte, inst Instance) (WebhookResult, error)
	// Send delivers one outbound message.
	Send(ctx context.Context, inst Instance, msg OutboundMessage) (SendResult, error)
}

// Challenger is implemented by channels whose webhook subscription is
// confirmed with a GET challenge (Meta hub.challenge). ok=false rejects.
type Challenger interface {
	Challenge(r *http.Request, inst Instance) (response string, ok bool)
}

// Router is implemented by channels whose provider calls one webhook URL for
// every account (a Meta app, a Telegram bot per org is not one). It returns
// the provider account ids a body is addressed to; the webhook handler
// resolves each to its instance (channel_instances.external_id) and then
// verifies and parses the body per instance.
type Router interface {
	Route(r *http.Request, body []byte) ([]string, error)
}

// Typer is implemented by channels that can show a typing indicator.
type Typer interface {
	SendTyping(ctx context.Context, inst Instance, to Recipient, on bool) error
}

// SharedChallenger is implemented by Router channels whose app-wide webhook
// URL (/v1/webhooks/channels/{type}) is confirmed with a GET challenge
// signed by an app-level verify token, not an instance secret.
type SharedChallenger interface {
	ChallengeShared(r *http.Request) (response string, ok bool)
}

// Reconciler is implemented by channels whose provider can list recent
// messages (Meta conversations API). A periodic job stores what webhooks
// missed; messages are idempotent on their external id, so overlapping
// windows are fine.
type Reconciler interface {
	Reconcile(ctx context.Context, inst Instance, since time.Time) (WebhookResult, error)
}

// Health is what a provider says about an instance's connection.
type Health struct {
	// OK: the credentials still work.
	OK bool `json:"ok"`
	// Detail explains a failed check (provider error message).
	Detail string `json:"detail,omitempty"`
	// TokenExpiresAt is when the access token stops working (nil = never
	// or unknown).
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
}

// HealthChecker is implemented by channels that can report token expiry or
// connection state beyond Connect; others are checked by calling Connect.
type HealthChecker interface {
	CheckHealth(ctx context.Context, inst Instance) (Health, error)
}

// DeliveryTester is implemented by channels that can send a harmless test
// event to the organization's own endpoint (the REST API channel, CF-185).
// It runs only on an explicit request, never from the health monitor.
type DeliveryTester interface {
	TestDelivery(ctx context.Context, inst Instance) (DeliveryTest, error)
}

// DeliveryTest is the outcome of a test delivery. Detail never carries a
// credential.
type DeliveryTest struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	Detail     string `json:"detail,omitempty"`
	EventID    string `json:"event_id"`
}

// Errors.
var (
	// ErrUnknownType: no adapter registered for the type.
	ErrUnknownType = errors.New("channels: unknown channel type")
	// ErrSignature: the webhook failed authentication.
	ErrSignature = errors.New("channels: webhook signature invalid")
	// ErrWindowClosed: a free-form reply outside the reply window.
	ErrWindowClosed = errors.New("channels: reply window closed")
)

// SendError is a failed send. Permanent errors are not retried (invalid
// recipient, window closed, rejected content); others are retried with
// backoff by the outbound queue.
type SendError struct {
	Permanent bool
	Code      string
	Err       error
}

func (e *SendError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("channels: send failed (%s): %v", e.Code, e.Err)
	}
	return fmt.Sprintf("channels: send failed: %v", e.Err)
}

func (e *SendError) Unwrap() error { return e.Err }

// Permanent wraps err as a permanent send failure.
func Permanent(code string, err error) error {
	return &SendError{Permanent: true, Code: code, Err: err}
}

// IsPermanent reports whether a send error must not be retried.
func IsPermanent(err error) bool {
	if errors.Is(err, ErrWindowClosed) {
		return true
	}
	var se *SendError
	return errors.As(err, &se) && se.Permanent
}

// Registry holds the adapters by type.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry registers adapters (nil entries are skipped; a later adapter of
// the same type replaces an earlier one).
func NewRegistry(adapters ...Adapter) *Registry {
	r := &Registry{adapters: map[string]Adapter{}}
	for _, a := range adapters {
		r.Register(a)
	}
	return r
}

// Register adds an adapter.
func (r *Registry) Register(a Adapter) {
	if r == nil || a == nil {
		return
	}
	r.adapters[a.Type()] = a
}

// Get returns the adapter of a type.
func (r *Registry) Get(channelType string) (Adapter, bool) {
	if r == nil {
		return nil, false
	}
	a, ok := r.adapters[channelType]
	return a, ok
}

// Types lists the registered types in a stable order.
func (r *Registry) Types() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.adapters))
	for t := range r.adapters {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// ValidateCredentials keeps the declared fields of an adapter, reports
// missing required ones and drops unknown keys.
func ValidateCredentials(a Adapter, in Credentials) (Credentials, []string) {
	out := Credentials{}
	missing := []string{}
	for _, f := range a.Fields() {
		v := strings.TrimSpace(in[f.Key])
		if v == "" {
			if f.Required {
				missing = append(missing, f.Key)
			}
			continue
		}
		out[f.Key] = v
	}
	return out, missing
}

// Mask shows the first and last four characters of a secret of at least
// 12 characters, else only bullets.
func Mask(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) < 12 {
		return "••••"
	}
	return v[:4] + "••••" + v[len(v)-4:]
}

// Hint is the displayable form of credentials: secret fields masked,
// others shown.
func Hint(a Adapter, c Credentials) map[string]string {
	out := map[string]string{}
	for _, f := range a.Fields() {
		v := c[f.Key]
		if v == "" {
			continue
		}
		if f.Secret {
			out[f.Key] = Mask(v)
		} else {
			out[f.Key] = v
		}
	}
	return out
}

// WindowOpen reports whether a free-form reply is allowed at now given the
// contact's last inbound message time.
func WindowOpen(c Capabilities, lastInbound time.Time, now time.Time) bool {
	if c.ReplyWindow <= 0 {
		return true
	}
	if lastInbound.IsZero() {
		return false
	}
	return now.Sub(lastInbound) < c.ReplyWindow
}
