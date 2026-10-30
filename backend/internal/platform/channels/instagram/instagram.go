// Package instagram is the Instagram channel (CF-58): direct messages and
// comments through the Instagram API with Instagram Login
// (graph.instagram.com), ported from takwim's platform/instagram and
// modules/instagram. A business signs in (LoginURL → ExchangeCode, which
// already returns the 60-day token → SubscribeApp); the instance keeps
// ig_user_id (the /me user_id webhooks name in entry[].id), access_token,
// username and token_expires_at. RefreshToken extends the token; a job
// should call it well before token_expires_at and store the new values.
// Meta calls one app webhook (/v1/webhooks/channels/instagram, object
// "instagram", signed with the app secret); Route returns entry[].id.
//
// Decisions:
//   - DMs: text; image/video/audio/file attachments are downloadable media;
//     story_mention is media too (story CDN url, Attributes story_mention);
//     share / ig_reel / reel / ig_post land in Attributes (attachment_types,
//     shared_urls) and make the message unsupported when it has no text.
//     reply_to.story → Attributes story_reply/story_id/story_url, reply_to.mid
//     → ReplyToExternalID. Echoes, deleted messages and the account's own
//     events are skipped. A reaction becomes a short text message
//     ("reaction:<mid>:<igsid>:<ts>", ReplyToExternalID = reacted mid,
//     Attributes reaction/reaction_action); unreacts are dropped.
//     messaging_seen (read.mid) → read receipt.
//   - Comments: every top-level comment opens its own conversation:
//     ThreadID "comment:<top-level comment id>" (a reply under a comment
//     carries parent_id and joins its thread), ExternalID
//     "comment:<comment id>", Attributes {source: comment, comment_id,
//     media_id, media_product_type, parent_id}. The thread id is what Send
//     reads back, so replying works without the attributes.
//   - Mentions (in a comment on someone else's post): fetched through
//     mentioned_comment; ThreadID "mention:<media_id>:<comment_id>",
//     Attributes source=mention. Caption mentions (no comment) are dropped;
//     a mention whose comment cannot be read is skipped (temporary failures
//     fail the parse so Meta redelivers).
//   - Send: a comment thread is answered publicly (POST /{comment}/replies)
//     unless Attributes reply_mode="dm", which sends the one private reply a
//     comment allows (recipient.comment_id = Attributes comment_id, else the
//     comment of ReplyToExternalID, else the thread's comment). A mention
//     thread replies through POST /{ig_user_id}/mentions. Everything else is
//     a DM to the IGSID (text first, then attachments by URL); Attributes
//     human_agent=true adds MESSAGE_TAG HUMAN_AGENT (7 days). ReplyWindow
//     stays 24h as the inbox's default for DMs; window errors are permanent
//     and wrap channels.ErrWindowClosed. Comment replies take no media.
//   - Health: /me with the token; TokenExpiresAt from token_expires_at
//     (RFC 3339 or unix seconds), which is kept as an optional field so
//     channels.ValidateCredentials does not drop it.
package instagram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/metagraph"
)

// Credential fields.
const (
	FieldUserID      = "ig_user_id"
	FieldAccessToken = "access_token"
	FieldUsername    = "username"
	FieldExpiresAt   = "token_expires_at"
)

// Defaults and limits.
const (
	DefaultBaseURL  = "https://graph.instagram.com"
	DefaultOAuthURL = "https://api.instagram.com"
	AuthorizeURL    = "https://www.instagram.com/oauth/authorize"
	// MaxText is Instagram's text limit (bytes).
	MaxText = 1000
	// SubscribedFields are the webhook fields SubscribeApp asks for.
	SubscribedFields = "messages,messaging_postbacks,messaging_seen,message_reactions,comments,mentions"
	webhookObject    = "instagram"
	// Thread id prefixes of comment conversations.
	ThreadComment = "comment:"
	ThreadMention = "mention:"
)

// Scopes asked at Instagram Login.
var Scopes = []string{"instagram_business_basic", "instagram_business_manage_messages", "instagram_business_manage_comments"}

// Config is the Meta app the channel runs on. A non-empty BaseURL also
// replaces the OAuth host (tests, proxies).
type Config struct {
	AppID, AppSecret, VerifyToken string
	BaseURL, APIVersion           string
	HTTPClient                    *http.Client
	RedirectURL                   string
}

// Adapter is the Instagram channel.
type Adapter struct {
	cfg      Config
	graph    *metagraph.Client
	oauthURL string
	now      func() time.Time
}

// New builds the adapter.
func New(cfg Config) *Adapter {
	oauth := DefaultOAuthURL
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	} else {
		oauth = strings.TrimRight(cfg.BaseURL, "/")
	}
	g := metagraph.NewClient(cfg.BaseURL, cfg.APIVersion, cfg.HTTPClient)
	cfg.HTTPClient = g.HTTP
	return &Adapter{cfg: cfg, graph: g, oauthURL: oauth, now: time.Now}
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
func (a *Adapter) Type() string { return channels.TypeInstagram }

// Capabilities implements channels.Adapter.
func (a *Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{
		Media: true, Typing: true, ReadReceipts: true,
		ReplyWindow: 24 * time.Hour, MaxTextLength: MaxText,
	}
}

// Fields implements channels.Adapter.
func (a *Adapter) Fields() []channels.Field {
	return []channels.Field{
		{Key: FieldUserID, LabelKey: "channels.fields.ig_user_id", Required: true},
		{Key: FieldAccessToken, LabelKey: "channels.fields.access_token", Secret: true, Required: true},
		{Key: FieldUsername, LabelKey: "channels.fields.ig_username"},
		{Key: FieldExpiresAt, LabelKey: "channels.fields.token_expires_at"},
	}
}

func creds(inst channels.Instance) (user, token string) {
	return strings.TrimSpace(inst.Credentials[FieldUserID]), strings.TrimSpace(inst.Credentials[FieldAccessToken])
}

func ownID(inst channels.Instance) string {
	if inst.ExternalID != "" {
		return inst.ExternalID
	}
	user, _ := creds(inst)
	return user
}

// Account is the professional account of a token.
type Account struct {
	UserID   string
	Username string
}

// Me reads the account of a token (user_id is the id webhooks name).
func (a *Adapter) Me(ctx context.Context, token string) (Account, error) {
	var out struct {
		UserID   metagraph.FlexID `json:"user_id"`
		ID       metagraph.FlexID `json:"id"`
		Username string           `json:"username"`
	}
	if err := a.graph.Get(ctx, "me", url.Values{"fields": {"user_id,username"}}, token, &out); err != nil {
		return Account{}, err
	}
	acc := Account{UserID: string(out.UserID), Username: out.Username}
	if acc.UserID == "" {
		acc.UserID = string(out.ID)
	}
	if acc.UserID == "" {
		return Account{}, &metagraph.Error{Status: http.StatusOK, Message: "answer without a user id"}
	}
	return acc, nil
}

// Connect implements channels.Adapter: returns the account's user_id.
func (a *Adapter) Connect(ctx context.Context, inst channels.Instance) (string, error) {
	user, token := creds(inst)
	if token == "" {
		return "", errors.New("instagram: access_token is required")
	}
	acc, err := a.Me(ctx, token)
	if err != nil {
		return "", err
	}
	if user != "" && acc.UserID != user {
		return "", errors.New("instagram: the token belongs to another account")
	}
	return acc.UserID, nil
}

// CheckHealth implements channels.HealthChecker.
func (a *Adapter) CheckHealth(ctx context.Context, inst channels.Instance) (channels.Health, error) {
	if _, err := a.Connect(ctx, inst); err != nil {
		if metagraph.Unhealthy(err) {
			return channels.Health{OK: false, Detail: err.Error()}, nil
		}
		return channels.Health{}, err
	}
	h := channels.Health{OK: true}
	if t, ok := ExpiresAt(inst.Credentials); ok {
		h.TokenExpiresAt = &t
	}
	return h, nil
}

// ExpiresAt reads token_expires_at (RFC 3339 or unix seconds).
func ExpiresAt(c channels.Credentials) (time.Time, bool) {
	v := strings.TrimSpace(c[FieldExpiresAt])
	if v == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), true
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
		return time.Unix(n, 0).UTC(), true
	}
	return time.Time{}, false
}

// VerifyWebhook implements channels.Adapter.
func (a *Adapter) VerifyWebhook(r *http.Request, body []byte, _ channels.Instance) error {
	if !metagraph.VerifySignature(a.cfg.AppSecret, r.Header.Get(metagraph.SignatureHeader), body) {
		return channels.ErrSignature
	}
	return nil
}

// Route implements channels.Router.
func (a *Adapter) Route(_ *http.Request, body []byte) ([]string, error) {
	return metagraph.EntryIDs(body, webhookObject)
}

// ChallengeShared implements channels.SharedChallenger.
func (a *Adapter) ChallengeShared(r *http.Request) (string, bool) {
	return metagraph.Challenge(r, a.cfg.VerifyToken)
}

// Challenge implements channels.Challenger.
func (a *Adapter) Challenge(r *http.Request, _ channels.Instance) (string, bool) {
	return metagraph.Challenge(r, a.cfg.VerifyToken)
}

type wireComment struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	ParentID string `json:"parent_id"`
	From     struct {
		ID       metagraph.FlexID `json:"id"`
		Username string           `json:"username"`
	} `json:"from"`
	Media struct {
		ID               metagraph.FlexID `json:"id"`
		MediaProductType string           `json:"media_product_type"`
	} `json:"media"`
}

type wireMention struct {
	CommentID metagraph.FlexID `json:"comment_id"`
	MediaID   metagraph.FlexID `json:"media_id"`
}

// ParseWebhook implements channels.Adapter. Entries of other accounts are
// ignored.
func (a *Adapter) ParseWebhook(ctx context.Context, _ *http.Request, body []byte, inst channels.Instance) (channels.WebhookResult, error) {
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
			sender := string(ev.Sender.ID)
			if sender == "" || sender == own {
				continue
			}
			at := metagraph.Millis(ev.Timestamp)
			switch {
			case ev.Message != nil:
				m := ev.Message
				if m.IsEcho || m.IsDeleted || m.Mid == "" {
					continue
				}
				out.Messages = append(out.Messages, a.inbound(sender, at, m))
			case ev.Postback != nil:
				id := ev.Postback.Mid
				if id == "" {
					id = "postback:" + sender + ":" + strconv.FormatInt(ev.Timestamp, 10)
				}
				out.Messages = append(out.Messages, channels.InboundMessage{
					ExternalID: id, From: channels.Sender{ExternalID: sender, InstanceScoped: true},
					Kind: "interactive", Text: ev.Postback.Title, At: at,
					Attributes: map[string]any{"postback_payload": ev.Postback.Payload, "postback_title": ev.Postback.Title},
				})
			case ev.Read != nil:
				if ev.Read.Mid != "" {
					out.Statuses = append(out.Statuses, channels.StatusUpdate{ExternalID: ev.Read.Mid, Status: channels.StatusRead, At: at})
				}
			case ev.Reaction != nil:
				r := ev.Reaction
				if r.Action != "react" || r.Mid == "" {
					continue
				}
				text := r.Emoji
				if text == "" {
					text = r.Reaction
				}
				out.Messages = append(out.Messages, channels.InboundMessage{
					ExternalID: "reaction:" + r.Mid + ":" + sender + ":" + strconv.FormatInt(ev.Timestamp, 10),
					From:       channels.Sender{ExternalID: sender, InstanceScoped: true},
					Kind:       "text", Text: text, ReplyToExternalID: r.Mid, At: at,
					Attributes: map[string]any{"reaction": r.Reaction, "reaction_action": r.Action},
				})
			}
		}
		at := metagraph.Millis(e.Time)
		for _, ch := range e.Changes {
			switch ch.Field {
			case "comments":
				var c wireComment
				if json.Unmarshal(ch.Value, &c) != nil || c.ID == "" || string(c.From.ID) == "" || string(c.From.ID) == own {
					continue
				}
				out.Messages = append(out.Messages, commentMessage(c, at))
			case "mentions":
				var m wireMention
				if json.Unmarshal(ch.Value, &m) != nil || m.CommentID == "" {
					continue
				}
				msg, ok, err := a.mention(ctx, inst, string(m.MediaID), string(m.CommentID), at)
				if err != nil {
					return channels.WebhookResult{}, err
				}
				if ok {
					out.Messages = append(out.Messages, msg)
				}
			}
		}
	}
	return out, nil
}

func commentMessage(c wireComment, at time.Time) channels.InboundMessage {
	top := c.ID
	if c.ParentID != "" {
		top = c.ParentID
	}
	attrs := map[string]any{"source": "comment", "comment_id": c.ID, "media_id": string(c.Media.ID)}
	if c.Media.MediaProductType != "" {
		attrs["media_product_type"] = c.Media.MediaProductType
	}
	if c.ParentID != "" {
		attrs["parent_id"] = c.ParentID
	}
	return channels.InboundMessage{
		ExternalID: ThreadComment + c.ID,
		From:       channels.Sender{ExternalID: string(c.From.ID), InstanceScoped: true, Username: c.From.Username},
		Kind:       "text", Text: c.Text, ThreadID: ThreadComment + top, Attributes: attrs, At: at,
	}
}

// mention reads a comment that mentions the account. ok=false skips it;
// err is returned only for temporary failures (Meta redelivers).
func (a *Adapter) mention(ctx context.Context, inst channels.Instance, mediaID, commentID string, at time.Time) (channels.InboundMessage, bool, error) {
	own := ownID(inst)
	_, token := creds(inst)
	if token == "" || !metagraph.ValidID(commentID) || !metagraph.ValidID(own) {
		return channels.InboundMessage{}, false, nil
	}
	var out struct {
		MentionedComment struct {
			ID        string `json:"id"`
			Text      string `json:"text"`
			Timestamp string `json:"timestamp"`
			Username  string `json:"username"`
			From      struct {
				ID       metagraph.FlexID `json:"id"`
				Username string           `json:"username"`
			} `json:"from"`
			Media struct {
				ID        metagraph.FlexID `json:"id"`
				Permalink string           `json:"permalink"`
			} `json:"media"`
		} `json:"mentioned_comment"`
	}
	field := "mentioned_comment.comment_id(" + commentID + "){id,text,timestamp,username,from,media{id,permalink}}"
	if err := a.graph.Get(ctx, own, url.Values{"fields": {field}}, token, &out); err != nil {
		if metagraph.Unhealthy(err) {
			return channels.InboundMessage{}, false, nil
		}
		return channels.InboundMessage{}, false, err
	}
	mc := out.MentionedComment
	username := mc.From.Username
	if username == "" {
		username = mc.Username
	}
	sender := string(mc.From.ID)
	if sender == "" && username != "" {
		sender = "ig_username:" + username
	}
	if sender == "" || sender == own {
		return channels.InboundMessage{}, false, nil
	}
	if mediaID == "" {
		mediaID = string(mc.Media.ID)
	}
	if t, ok := metagraph.ParseTime(mc.Timestamp); ok {
		at = t
	}
	attrs := map[string]any{"source": "mention", "comment_id": commentID, "media_id": mediaID}
	if mc.Media.Permalink != "" {
		attrs["media_permalink"] = mc.Media.Permalink
	}
	return channels.InboundMessage{
		ExternalID: ThreadComment + commentID,
		From:       channels.Sender{ExternalID: sender, InstanceScoped: true, Username: username},
		Kind:       "text", Text: mc.Text, ThreadID: ThreadMention + mediaID + ":" + commentID, Attributes: attrs, At: at,
	}, true, nil
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
		if s := m.ReplyTo.Story; s != nil {
			attrs["story_reply"] = true
			attrs["story_id"] = string(s.ID)
			if s.URL != "" {
				attrs["story_url"] = s.URL
			}
		}
	}
	var other, shared []string
	for _, att := range m.Attachments {
		switch att.Type {
		case "image", "video", "audio", "file", "story_mention":
			if att.Payload.URL == "" {
				continue
			}
			kind := att.Type
			if kind == "story_mention" {
				kind = "image"
				attrs["story_mention"] = true
				attrs["story_url"] = att.Payload.URL
			}
			msg.Attachments = append(msg.Attachments, metagraph.Attachment(a.cfg.HTTPClient, kind, att.Payload.URL, "", ""))
		default: // share, ig_reel, reel, ig_post, fallback …
			other = append(other, att.Type)
			if att.Payload.URL != "" {
				shared = append(shared, att.Payload.URL)
			}
		}
	}
	if len(other) > 0 {
		attrs["attachment_types"] = other
	}
	if len(shared) > 0 {
		attrs["shared_urls"] = shared
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

// --- send -------------------------------------------------------------------

func attr(msg channels.OutboundMessage, key string) string {
	v, _ := msg.Attributes[key].(string)
	return strings.TrimSpace(v)
}

// Send implements channels.Adapter.
func (a *Adapter) Send(ctx context.Context, inst channels.Instance, msg channels.OutboundMessage) (channels.SendResult, error) {
	_, token := creds(inst)
	if token == "" {
		return channels.SendResult{}, channels.Permanent("not_configured", errors.New("instagram: access token is missing"))
	}
	switch {
	case strings.HasPrefix(msg.ThreadID, ThreadMention):
		return a.sendMention(ctx, inst, token, msg)
	case strings.HasPrefix(msg.ThreadID, ThreadComment) || attr(msg, "source") == "comment":
		return a.sendComment(ctx, token, msg)
	}
	if !metagraph.ValidID(msg.To.ExternalID) {
		return channels.SendResult{}, channels.Permanent("invalid_recipient", errors.New("instagram: recipient is not an instagram-scoped id"))
	}
	var parts []map[string]any
	if strings.TrimSpace(msg.Text) != "" {
		parts = append(parts, map[string]any{"text": metagraph.ClipBytes(msg.Text, MaxText)})
	}
	for _, att := range msg.Attachments {
		if att.URL == "" {
			return channels.SendResult{}, channels.Permanent("attachment_url_required", errors.New("instagram: attachments are sent by url"))
		}
		parts = append(parts, map[string]any{"attachment": map[string]any{
			"type": metagraph.OutboundType(att.ContentType), "payload": map[string]any{"url": att.URL},
		}})
	}
	if len(parts) == 0 {
		return channels.SendResult{}, channels.Permanent("empty_message", errors.New("instagram: nothing to send"))
	}
	tagged := metagraph.Truthy(msg.Attributes["human_agent"])
	first := ""
	for i, p := range parts {
		id, err := a.sendMessage(ctx, token, map[string]string{"id": msg.To.ExternalID}, p, tagged)
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

func (a *Adapter) sendMessage(ctx context.Context, token string, recipient map[string]string, message map[string]any, humanAgent bool) (string, error) {
	payload := map[string]any{"recipient": recipient, "message": message}
	if humanAgent {
		payload["messaging_type"], payload["tag"] = "MESSAGE_TAG", "HUMAN_AGENT"
	}
	var out struct {
		MessageID string `json:"message_id"`
	}
	if err := a.graph.Post(ctx, "me/messages", nil, token, payload, &out); err != nil {
		return "", err
	}
	if out.MessageID == "" {
		return "", &metagraph.Error{Status: http.StatusOK, Message: "answer without a message id"}
	}
	return out.MessageID, nil
}

func commentText(msg channels.OutboundMessage) (string, error) {
	if len(msg.Attachments) > 0 {
		return "", channels.Permanent("media_not_supported", errors.New("instagram: comment replies take text only"))
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return "", channels.Permanent("empty_message", errors.New("instagram: nothing to send"))
	}
	return metagraph.ClipBytes(text, MaxText), nil
}

func (a *Adapter) sendComment(ctx context.Context, token string, msg channels.OutboundMessage) (channels.SendResult, error) {
	text, err := commentText(msg)
	if err != nil {
		return channels.SendResult{}, err
	}
	thread := strings.TrimPrefix(msg.ThreadID, ThreadComment)
	if !metagraph.ValidID(thread) {
		thread = attr(msg, "parent_id")
		if thread == "" {
			thread = attr(msg, "comment_id")
		}
	}
	if attr(msg, "reply_mode") == "dm" {
		target := attr(msg, "comment_id")
		if target == "" && strings.HasPrefix(msg.ReplyToExternalID, ThreadComment) {
			target = strings.TrimPrefix(msg.ReplyToExternalID, ThreadComment)
		}
		if target == "" {
			target = thread
		}
		if !metagraph.ValidID(target) {
			return channels.SendResult{}, channels.Permanent("invalid_recipient", errors.New("instagram: no comment to reply to"))
		}
		id, err := a.sendMessage(ctx, token, map[string]string{"comment_id": target}, map[string]any{"text": text}, false)
		if err != nil {
			return channels.SendResult{}, metagraph.SendError(err)
		}
		return channels.SendResult{ExternalID: id}, nil
	}
	if !metagraph.ValidID(thread) {
		return channels.SendResult{}, channels.Permanent("invalid_recipient", errors.New("instagram: no comment to reply to"))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := a.graph.Post(ctx, thread+"/replies", nil, token, url.Values{"message": {text}}, &out); err != nil {
		return channels.SendResult{}, metagraph.SendError(err)
	}
	if out.ID == "" {
		return channels.SendResult{}, metagraph.SendError(&metagraph.Error{Status: http.StatusOK, Message: "answer without a comment id"})
	}
	return channels.SendResult{ExternalID: ThreadComment + out.ID}, nil
}

func (a *Adapter) sendMention(ctx context.Context, inst channels.Instance, token string, msg channels.OutboundMessage) (channels.SendResult, error) {
	text, err := commentText(msg)
	if err != nil {
		return channels.SendResult{}, err
	}
	media, comment, _ := strings.Cut(strings.TrimPrefix(msg.ThreadID, ThreadMention), ":")
	own := ownID(inst)
	if !metagraph.ValidID(media) || !metagraph.ValidID(comment) || !metagraph.ValidID(own) {
		return channels.SendResult{}, channels.Permanent("invalid_recipient", errors.New("instagram: no mention to reply to"))
	}
	var out struct {
		ID string `json:"id"`
	}
	form := url.Values{"media_id": {media}, "comment_id": {comment}, "message": {text}}
	if err := a.graph.Post(ctx, own+"/mentions", nil, token, form, &out); err != nil {
		return channels.SendResult{}, metagraph.SendError(err)
	}
	if out.ID == "" {
		return channels.SendResult{}, metagraph.SendError(&metagraph.Error{Status: http.StatusOK, Message: "answer without a comment id"})
	}
	return channels.SendResult{ExternalID: ThreadComment + out.ID}, nil
}

// SendTyping implements channels.Typer.
func (a *Adapter) SendTyping(ctx context.Context, inst channels.Instance, to channels.Recipient, on bool) error {
	_, token := creds(inst)
	if token == "" || !metagraph.ValidID(to.ExternalID) {
		return channels.Permanent("invalid_recipient", errors.New("instagram: cannot show typing"))
	}
	action := "typing_off"
	if on {
		action = "typing_on"
	}
	return metagraph.SendError(a.graph.Post(ctx, "me/messages", nil, token, map[string]any{
		"recipient": map[string]string{"id": to.ExternalID}, "sender_action": action,
	}, nil))
}

// Reconcile implements channels.Reconciler: DMs of the recent
// conversations created since, the account's own skipped.
func (a *Adapter) Reconcile(ctx context.Context, inst channels.Instance, since time.Time) (channels.WebhookResult, error) {
	user, token := creds(inst)
	if token == "" {
		return channels.WebhookResult{}, errors.New("instagram: access token is missing")
	}
	username := strings.TrimSpace(inst.Credentials[FieldUsername])
	msgs, err := a.graph.RecentMessages(ctx, "me", "instagram", token, since, user, ownID(inst))
	if err != nil {
		return channels.WebhookResult{}, err
	}
	var out channels.WebhookResult
	for _, m := range msgs {
		if username != "" && strings.EqualFold(m.From.Username, username) {
			continue
		}
		in := channels.InboundMessage{
			ExternalID: m.ID, Kind: "text", Text: m.Message, At: m.At,
			From:       channels.Sender{ExternalID: string(m.From.ID), InstanceScoped: true, Username: m.From.Username},
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

// --- Instagram Login ------------------------------------------------------

// LoginURL is the Instagram Login page (state binds the answer to the
// session that asked).
func (a *Adapter) LoginURL(state string) string {
	q := url.Values{
		"client_id": {a.cfg.AppID}, "redirect_uri": {a.cfg.RedirectURL}, "response_type": {"code"},
		"scope": {strings.Join(Scopes, ",")}, "state": {state},
	}
	return AuthorizeURL + "?" + q.Encode()
}

// ExchangeCode turns the login code into a long-lived (60-day) token and
// the account's user id.
func (a *Adapter) ExchangeCode(ctx context.Context, code string) (token, userID string, expiresAt time.Time, err error) {
	if !a.Enabled() {
		return "", "", time.Time{}, errors.New("instagram: the Meta app is not configured")
	}
	code = strings.TrimSuffix(strings.TrimSpace(code), "#_")
	form := url.Values{
		"client_id": {a.cfg.AppID}, "client_secret": {a.cfg.AppSecret}, "grant_type": {"authorization_code"},
		"redirect_uri": {a.cfg.RedirectURL}, "code": {code},
	}
	type shortToken struct {
		AccessToken string           `json:"access_token"`
		UserID      metagraph.FlexID `json:"user_id"`
	}
	var short struct {
		shortToken
		Data []shortToken `json:"data"`
	}
	if err := a.graph.Do(ctx, http.MethodPost, a.oauthURL+"/oauth/access_token", "", form, &short); err != nil {
		return "", "", time.Time{}, err
	}
	st := short.shortToken
	if st.AccessToken == "" && len(short.Data) > 0 {
		st = short.Data[0]
	}
	if st.AccessToken == "" {
		return "", "", time.Time{}, &metagraph.Error{Status: http.StatusOK, Message: "answer without an access token"}
	}
	token, expiresAt, err = a.longToken(ctx, "access_token", url.Values{
		"grant_type": {"ig_exchange_token"}, "client_secret": {a.cfg.AppSecret}, "access_token": {st.AccessToken},
	})
	return token, string(st.UserID), expiresAt, err
}

// RefreshToken extends a long-lived token (at least a day old, not
// expired) by another 60 days.
func (a *Adapter) RefreshToken(ctx context.Context, token string) (string, time.Time, error) {
	return a.longToken(ctx, "refresh_access_token", url.Values{"grant_type": {"ig_refresh_token"}, "access_token": {token}})
}

func (a *Adapter) longToken(ctx context.Context, p string, q url.Values) (string, time.Time, error) {
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := a.graph.Do(ctx, http.MethodGet, a.graph.BaseURL+"/"+p+"?"+q.Encode(), "", nil, &out); err != nil {
		return "", time.Time{}, err
	}
	if out.AccessToken == "" {
		return "", time.Time{}, &metagraph.Error{Status: http.StatusOK, Message: "answer without an access token"}
	}
	var exp time.Time
	if out.ExpiresIn > 0 {
		exp = a.now().Add(time.Duration(out.ExpiresIn) * time.Second).UTC()
	}
	return out.AccessToken, exp, nil
}

// SubscribeApp subscribes the app to the account's messages, comments and
// mentions.
func (a *Adapter) SubscribeApp(ctx context.Context, token string) error {
	var out struct {
		Success bool `json:"success"`
	}
	if err := a.graph.Post(ctx, "me/subscribed_apps", url.Values{"subscribed_fields": {SubscribedFields}}, token, nil, &out); err != nil {
		return err
	}
	if !out.Success {
		return errors.New("instagram: subscribing the account failed")
	}
	return nil
}

// refreshBefore is how long before expiry a long-lived token is renewed
// (they live 60 days and can be refreshed once they are a day old).
const refreshBefore = 10 * 24 * time.Hour

// RefreshCredentials renews the long-lived token when it expires within
// ten days; changed=false leaves the stored credentials alone. The health
// sweep calls it before checking the instance.
func (a *Adapter) RefreshCredentials(ctx context.Context, inst channels.Instance) (channels.Credentials, bool, error) {
	exp, ok := ExpiresAt(inst.Credentials)
	if !ok || time.Until(exp) > refreshBefore {
		return nil, false, nil
	}
	token, newExp, err := a.RefreshToken(ctx, inst.Credentials[FieldAccessToken])
	if err != nil {
		return nil, false, err
	}
	out := channels.Credentials{}
	for k, v := range inst.Credentials {
		out[k] = v
	}
	out[FieldAccessToken] = token
	if !newExp.IsZero() {
		out[FieldExpiresAt] = newExp.UTC().Format(time.RFC3339)
	}
	return out, true, nil
}
