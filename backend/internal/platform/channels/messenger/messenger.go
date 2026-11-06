// Package messenger is the Facebook Messenger channel (CF-59), ported from
// takwim's platform/messenger and modules/messenger: a business signs in
// with Facebook (LoginURL → ExchangeCode → Pages → SubscribePage) and picks
// a Page; the instance keeps page_id and the Page access token (taken from a
// long-lived user token, it does not expire). Meta calls one app webhook
// (/v1/webhooks/channels/messenger, object "page", signed with the app
// secret); Route returns entry[].id (the Page id = channel_instances
// .external_id).
//
// Decisions:
//   - Inbound: text, image/video/audio/file attachments (Open downloads
//     payload.url), quick replies and postbacks (Kind interactive, payload in
//     Attributes), reply_to.mid → ReplyToExternalID. Echoes (is_echo) are
//     the Page's own sends and are skipped. Postbacks without a mid get
//     "postback:<psid>:<timestamp>".
//   - Receipts: delivery mids → delivered. Read events only carry a
//     watermark (no message ids), which the contract cannot express, so they
//     are dropped and ReadReceipts is false.
//   - Window: Capabilities.ReplyWindow is 7 days because the HUMAN_AGENT tag
//     allows replies up to 7 days after the contact wrote. The adapter does
//     not know the last inbound time, so it sends messaging_type RESPONSE
//     and, when Meta rejects it as outside the 24h window (code 10 /
//     2018278), resends once with MESSAGE_TAG HUMAN_AGENT. Attributes
//     human_agent=true tags up front; human_agent=false or automated=true
//     disables the fallback (bots must not use the tag). A window error left
//     after that is permanent and wraps channels.ErrWindowClosed.
//   - Text is sent first, then each attachment by URL (one Send API call
//     each); the first message id is the result. A failure after a part was
//     delivered is permanent ("partial_send") so a retry never duplicates.
//   - Page comments (feed) are not part of this channel.
package messenger

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/format"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/metagraph"
)

// Credential fields.
const (
	FieldPageID          = "page_id"
	FieldPageAccessToken = "page_access_token"
)

// Defaults and limits.
const (
	DefaultBaseURL = "https://graph.facebook.com"
	DialogURL      = "https://www.facebook.com"
	// MaxText is Messenger's text limit (characters).
	MaxText = 2000
	// SubscribedFields are the Page webhook fields SubscribePage asks for.
	SubscribedFields = "messages,messaging_postbacks,message_deliveries,message_reads,message_echoes"
	webhookObject    = "page"
)

// Scopes asked at Facebook Login.
var Scopes = []string{"pages_messaging", "pages_show_list", "pages_manage_metadata", "pages_read_engagement"}

// Config is the Meta app the channel runs on.
type Config struct {
	AppID, AppSecret, VerifyToken string
	BaseURL, APIVersion           string
	HTTPClient                    *http.Client
	RedirectURL                   string
}

// Adapter is the Messenger channel.
type Adapter struct {
	cfg   Config
	graph *metagraph.Client
}

// New builds the adapter (empty BaseURL/APIVersion take the defaults).
func New(cfg Config) *Adapter {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	g := metagraph.NewClient(cfg.BaseURL, cfg.APIVersion, cfg.HTTPClient)
	cfg.HTTPClient = g.HTTP
	return &Adapter{cfg: cfg, graph: g}
}

var (
	_ channels.Adapter          = (*Adapter)(nil)
	_ channels.Router           = (*Adapter)(nil)
	_ channels.Challenger       = (*Adapter)(nil)
	_ channels.SharedChallenger = (*Adapter)(nil)
	_ channels.Typer            = (*Adapter)(nil)
	_ channels.Reconciler       = (*Adapter)(nil)
	_ channels.HealthChecker    = (*Adapter)(nil)
)

// Enabled reports whether the Meta app is configured.
func (a *Adapter) Enabled() bool { return a.cfg.AppID != "" && a.cfg.AppSecret != "" }

// Type implements channels.Adapter.
func (a *Adapter) Type() string { return channels.TypeMessenger }

// Capabilities implements channels.Adapter.
func (a *Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{
		Media: true, Typing: true, DeliveryReceipts: true,
		ReplyWindow: 7 * 24 * time.Hour, MaxTextLength: MaxText,
	}
}

// Fields implements channels.Adapter.
func (a *Adapter) Fields() []channels.Field {
	return []channels.Field{
		{Key: FieldPageID, LabelKey: "channels.fields.page_id", Required: true},
		{Key: FieldPageAccessToken, LabelKey: "channels.fields.page_access_token", Secret: true, Required: true},
	}
}

func creds(inst channels.Instance) (page, token string) {
	return strings.TrimSpace(inst.Credentials[FieldPageID]), strings.TrimSpace(inst.Credentials[FieldPageAccessToken])
}

func ownID(inst channels.Instance) string {
	if inst.ExternalID != "" {
		return inst.ExternalID
	}
	page, _ := creds(inst)
	return page
}

// Connect implements channels.Adapter: reads the Page with its token and
// returns the Page id.
func (a *Adapter) Connect(ctx context.Context, inst channels.Instance) (string, error) {
	page, token := creds(inst)
	if !metagraph.ValidID(page) || token == "" {
		return "", errors.New("messenger: page_id and page_access_token are required")
	}
	var out struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := a.graph.Get(ctx, page, url.Values{"fields": {"id,name"}}, token, &out); err != nil {
		return "", err
	}
	if out.ID != page {
		return "", errors.New("messenger: the token belongs to another Page")
	}
	return page, nil
}

// CheckHealth implements channels.HealthChecker. Token problems are an
// unhealthy result, other failures an error (check again later). With the
// app configured, debug_token reports the token's expiry.
func (a *Adapter) CheckHealth(ctx context.Context, inst channels.Instance) (channels.Health, error) {
	if _, err := a.Connect(ctx, inst); err != nil {
		if metagraph.Unhealthy(err) {
			return channels.Health{OK: false, Detail: err.Error()}, nil
		}
		return channels.Health{}, err
	}
	h := channels.Health{OK: true}
	if !a.Enabled() {
		return h, nil
	}
	_, token := creds(inst)
	var dbg struct {
		Data struct {
			IsValid   bool  `json:"is_valid"`
			ExpiresAt int64 `json:"expires_at"`
		} `json:"data"`
	}
	if err := a.graph.Get(ctx, "debug_token", url.Values{"input_token": {token}}, a.cfg.AppID+"|"+a.cfg.AppSecret, &dbg); err != nil {
		return h, nil // best effort: the token itself works
	}
	if !dbg.Data.IsValid {
		return channels.Health{OK: false, Detail: "messenger: token is not valid"}, nil
	}
	if dbg.Data.ExpiresAt > 0 {
		t := time.Unix(dbg.Data.ExpiresAt, 0).UTC()
		h.TokenExpiresAt = &t
	}
	return h, nil
}

// VerifyWebhook implements channels.Adapter (X-Hub-Signature-256 with the
// app secret).
func (a *Adapter) VerifyWebhook(r *http.Request, body []byte, _ channels.Instance) error {
	if !metagraph.VerifySignature(a.cfg.AppSecret, r.Header.Get(metagraph.SignatureHeader), body) {
		return channels.ErrSignature
	}
	return nil
}

// Route implements channels.Router: the Page ids of a body.
func (a *Adapter) Route(_ *http.Request, body []byte) ([]string, error) {
	return metagraph.EntryIDs(body, webhookObject)
}

// ChallengeShared implements channels.SharedChallenger.
func (a *Adapter) ChallengeShared(r *http.Request) (string, bool) {
	return metagraph.Challenge(r, a.cfg.VerifyToken)
}

// Challenge implements channels.Challenger (same app verify token).
func (a *Adapter) Challenge(r *http.Request, _ channels.Instance) (string, bool) {
	return metagraph.Challenge(r, a.cfg.VerifyToken)
}

// ParseWebhook implements channels.Adapter. Entries of other Pages are
// ignored.
func (a *Adapter) ParseWebhook(_ context.Context, _ *http.Request, body []byte, inst channels.Instance) (channels.WebhookResult, error) {
	w, err := metagraph.DecodeWebhook(body, webhookObject)
	if err != nil {
		return channels.WebhookResult{}, err
	}
	own := ownID(inst)
	var out channels.WebhookResult
	for _, e := range w.Entry {
		if own == "" || string(e.ID) != own {
			continue
		}
		for _, ev := range e.Messaging {
			at := metagraph.Millis(ev.Timestamp)
			sender := string(ev.Sender.ID)
			switch {
			case ev.Message != nil:
				m := ev.Message
				if m.IsEcho || m.Mid == "" || sender == "" || sender == own {
					continue
				}
				out.Messages = append(out.Messages, a.inbound(sender, at, m))
			case ev.Postback != nil:
				if sender == "" || sender == own {
					continue
				}
				id := ev.Postback.Mid
				if id == "" {
					id = "postback:" + sender + ":" + strconv.FormatInt(ev.Timestamp, 10)
				}
				out.Messages = append(out.Messages, channels.InboundMessage{
					ExternalID: id, From: channels.Sender{ExternalID: sender, InstanceScoped: true},
					Kind: "interactive", Text: ev.Postback.Title, At: at,
					Attributes: map[string]any{"postback_payload": ev.Postback.Payload, "postback_title": ev.Postback.Title},
				})
			case ev.Delivery != nil:
				dat := metagraph.Millis(ev.Delivery.Watermark)
				if dat.IsZero() {
					dat = at
				}
				for _, mid := range ev.Delivery.Mids {
					if mid != "" {
						out.Statuses = append(out.Statuses, channels.StatusUpdate{ExternalID: mid, Status: channels.StatusDelivered, At: dat})
					}
				}
			}
		}
	}
	return out, nil
}

func (a *Adapter) inbound(sender string, at time.Time, m *metagraph.WireMessage) channels.InboundMessage {
	msg := channels.InboundMessage{
		ExternalID: m.Mid, From: channels.Sender{ExternalID: sender, InstanceScoped: true},
		Kind: "text", Text: m.Text, At: at,
	}
	attrs := map[string]any{}
	if m.QuickReply != nil {
		msg.Kind = "interactive"
		attrs["quick_reply_payload"] = m.QuickReply.Payload
	}
	if m.ReplyTo != nil {
		msg.ReplyToExternalID = m.ReplyTo.Mid
	}
	var other []string
	for _, att := range m.Attachments {
		switch att.Type {
		case "image", "video", "audio", "file":
			if att.Payload.URL == "" {
				continue
			}
			msg.Attachments = append(msg.Attachments, metagraph.Attachment(a.cfg.HTTPClient, att.Type, att.Payload.URL, "", ""))
			if att.Payload.StickerID != "" {
				attrs["sticker_id"] = string(att.Payload.StickerID)
			}
		default:
			other = append(other, att.Type)
		}
	}
	if len(other) > 0 {
		attrs["attachment_types"] = other
	}
	switch {
	case len(msg.Attachments) > 0 && msg.Kind == "text":
		msg.Kind = "media"
	case m.IsUnsupported || (strings.TrimSpace(m.Text) == "" && len(msg.Attachments) == 0 && m.QuickReply == nil):
		msg.Kind = "unsupported"
	}
	if len(attrs) > 0 {
		msg.Attributes = attrs
	}
	return msg
}

// Send implements channels.Adapter.
func (a *Adapter) Send(ctx context.Context, inst channels.Instance, msg channels.OutboundMessage) (channels.SendResult, error) {
	page, token := creds(inst)
	if !metagraph.ValidID(page) || token == "" {
		return channels.SendResult{}, channels.Permanent("not_configured", errors.New("messenger: page credentials are missing"))
	}
	if !metagraph.ValidID(msg.To.ExternalID) {
		return channels.SendResult{}, channels.Permanent("invalid_recipient", errors.New("messenger: recipient is not a page-scoped id"))
	}
	// Messenger documents no text formatting: send without marks (CF-177).
	msg.Text = format.Plain(msg.Text)
	var parts []map[string]any
	if strings.TrimSpace(msg.Text) != "" {
		parts = append(parts, map[string]any{"text": metagraph.ClipRunes(msg.Text, MaxText)})
	}
	for _, att := range msg.Attachments {
		if att.URL == "" {
			return channels.SendResult{}, channels.Permanent("attachment_url_required", errors.New("messenger: attachments are sent by url"))
		}
		parts = append(parts, map[string]any{"attachment": map[string]any{
			"type": metagraph.OutboundType(att.ContentType), "payload": map[string]any{"url": att.URL, "is_reusable": false},
		}})
	}
	if len(parts) == 0 {
		return channels.SendResult{}, channels.Permanent("empty_message", errors.New("messenger: nothing to send"))
	}
	tagged := metagraph.Truthy(msg.Attributes["human_agent"])
	fallback := !tagged && !metagraph.Truthy(msg.Attributes["automated"]) && !explicitFalse(msg.Attributes["human_agent"])
	first := ""
	for i, p := range parts {
		id, err := a.sendOne(ctx, page, token, msg.To.ExternalID, p, tagged)
		if err != nil && !tagged && fallback && metagraph.IsWindowError(err) {
			tagged = true
			id, err = a.sendOne(ctx, page, token, msg.To.ExternalID, p, true)
		}
		if err != nil {
			mapped := metagraph.SendError(err)
			if i > 0 {
				return channels.SendResult{ExternalID: first}, channels.Permanent("partial_send", mapped)
			}
			return channels.SendResult{}, mapped
		}
		if first == "" {
			first = id
		}
	}
	return channels.SendResult{ExternalID: first}, nil
}

func explicitFalse(v any) bool {
	switch t := v.(type) {
	case bool:
		return !t
	case string:
		b, err := strconv.ParseBool(t)
		return err == nil && !b
	}
	return false
}

func (a *Adapter) sendOne(ctx context.Context, page, token, to string, message map[string]any, humanAgent bool) (string, error) {
	payload := map[string]any{"recipient": map[string]string{"id": to}, "message": message, "messaging_type": "RESPONSE"}
	if humanAgent {
		payload["messaging_type"], payload["tag"] = "MESSAGE_TAG", "HUMAN_AGENT"
	}
	var out struct {
		MessageID string `json:"message_id"`
	}
	if err := a.graph.Post(ctx, page+"/messages", nil, token, payload, &out); err != nil {
		return "", err
	}
	if out.MessageID == "" {
		return "", &metagraph.Error{Status: http.StatusOK, Message: "answer without a message id"}
	}
	return out.MessageID, nil
}

// SendTyping implements channels.Typer.
func (a *Adapter) SendTyping(ctx context.Context, inst channels.Instance, to channels.Recipient, on bool) error {
	page, token := creds(inst)
	if !metagraph.ValidID(page) || token == "" || !metagraph.ValidID(to.ExternalID) {
		return channels.Permanent("invalid_recipient", errors.New("messenger: cannot show typing"))
	}
	action := "typing_off"
	if on {
		action = "typing_on"
	}
	err := a.graph.Post(ctx, page+"/messages", nil, token, map[string]any{
		"recipient": map[string]string{"id": to.ExternalID}, "sender_action": action,
	}, nil)
	return metagraph.SendError(err)
}

// Reconcile implements channels.Reconciler: customer messages of the
// Page's recent conversations created since, the Page's own skipped.
func (a *Adapter) Reconcile(ctx context.Context, inst channels.Instance, since time.Time) (channels.WebhookResult, error) {
	page, token := creds(inst)
	if !metagraph.ValidID(page) || token == "" {
		return channels.WebhookResult{}, errors.New("messenger: page credentials are missing")
	}
	msgs, err := a.graph.RecentMessages(ctx, page, "messenger", token, since, page, ownID(inst))
	if err != nil {
		return channels.WebhookResult{}, err
	}
	var out channels.WebhookResult
	for _, m := range msgs {
		in := channels.InboundMessage{
			ExternalID: m.ID, Kind: "text", Text: m.Message, At: m.At,
			From:       channels.Sender{ExternalID: string(m.From.ID), InstanceScoped: true, Name: m.From.Name},
			Attributes: map[string]any{"reconciled": true},
		}
		for _, att := range m.Attachments.Data {
			if ia, ok := att.Inbound(a.cfg.HTTPClient); ok {
				in.Attachments = append(in.Attachments, ia)
			}
		}
		if len(in.Attachments) > 0 {
			in.Kind = "media"
		} else if strings.TrimSpace(in.Text) == "" {
			in.Kind = "unsupported"
		}
		out.Messages = append(out.Messages, in)
	}
	return out, nil
}

// --- Facebook Login --------------------------------------------------------

// LoginURL is the Facebook Login dialog (state binds the answer to the
// session that asked).
func (a *Adapter) LoginURL(state string) string {
	q := url.Values{
		"client_id": {a.cfg.AppID}, "redirect_uri": {a.cfg.RedirectURL}, "state": {state},
		"response_type": {"code"}, "scope": {strings.Join(Scopes, ",")},
	}
	return DialogURL + "/" + a.graph.Version + "/dialog/oauth?" + q.Encode()
}

func (a *Adapter) userToken(ctx context.Context, q url.Values) (string, error) {
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := a.graph.Get(ctx, "oauth/access_token", q, "", &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", &metagraph.Error{Status: http.StatusOK, Message: "answer without an access token"}
	}
	return out.AccessToken, nil
}

// ExchangeCode turns the login code into a long-lived user token (its Page
// tokens do not expire).
func (a *Adapter) ExchangeCode(ctx context.Context, code string) (string, error) {
	if !a.Enabled() {
		return "", errors.New("messenger: the Meta app is not configured")
	}
	short, err := a.userToken(ctx, url.Values{
		"client_id": {a.cfg.AppID}, "client_secret": {a.cfg.AppSecret}, "redirect_uri": {a.cfg.RedirectURL}, "code": {code},
	})
	if err != nil {
		return "", err
	}
	return a.userToken(ctx, url.Values{
		"grant_type": {"fb_exchange_token"}, "client_id": {a.cfg.AppID}, "client_secret": {a.cfg.AppSecret}, "fb_exchange_token": {short},
	})
}

// Page is a Facebook Page the signed-in person may message for.
type Page struct {
	ID          string
	Name        string
	AccessToken string
}

// Pages lists the Pages of a user token that carry a Page token and allow
// messaging.
func (a *Adapter) Pages(ctx context.Context, userToken string) ([]Page, error) {
	var out struct {
		Data []struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			AccessToken string   `json:"access_token"`
			Tasks       []string `json:"tasks"`
		} `json:"data"`
	}
	if err := a.graph.Get(ctx, "me/accounts", url.Values{"limit": {"100"}, "fields": {"id,name,access_token,tasks"}}, userToken, &out); err != nil {
		return nil, err
	}
	pages := make([]Page, 0, len(out.Data))
	for _, d := range out.Data {
		if metagraph.ValidID(d.ID) && d.AccessToken != "" && canMessage(d.Tasks) {
			pages = append(pages, Page{ID: d.ID, Name: d.Name, AccessToken: d.AccessToken})
		}
	}
	return pages, nil
}

// canMessage: no tasks listed → assume yes (Meta decides on send).
func canMessage(tasks []string) bool {
	if len(tasks) == 0 {
		return true
	}
	for _, t := range tasks {
		if t == "MESSAGING" || t == "MODERATE" || t == "MANAGE" {
			return true
		}
	}
	return false
}

// SubscribePage subscribes the app to the Page's messaging webhooks.
func (a *Adapter) SubscribePage(ctx context.Context, pageID, pageToken string) error {
	if !metagraph.ValidID(pageID) {
		return errors.New("messenger: invalid page id")
	}
	var out struct {
		Success bool `json:"success"`
	}
	if err := a.graph.Post(ctx, pageID+"/subscribed_apps", url.Values{"subscribed_fields": {SubscribedFields}}, pageToken, nil, &out); err != nil {
		return err
	}
	if !out.Success {
		return fmt.Errorf("messenger: subscribing page %s failed", pageID)
	}
	return nil
}

// UserID resolves the authenticated app-scoped subject for deauthorization callbacks.
func (a *Adapter) UserID(ctx context.Context, token string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := a.graph.Get(ctx, "me", url.Values{"fields": {"id"}}, token, &out); err != nil {
		return "", err
	}
	if !metagraph.ValidID(out.ID) {
		return "", errors.New("messenger: missing user id")
	}
	return out.ID, nil
}
