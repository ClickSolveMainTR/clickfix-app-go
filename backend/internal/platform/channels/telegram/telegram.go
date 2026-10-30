// Package telegram is the Telegram Bot API channel adapter (CF-60). Every
// organization connects its own bot by entering the bot token (@BotFather);
// one channel instance is one bot.
//
// Webhooks: Connect validates the token with getMe and registers
// PublicBaseURL + /v1/webhooks/channels/telegram/{instance uuid} with
// setWebhook. The webhook secret is not stored: SecretToken derives it from
// the bot token and the instance uuid (HMAC-SHA256), Telegram echoes it in
// X-Telegram-Bot-Api-Secret-Token and VerifyWebhook compares it in constant
// time. Rotating the token therefore requires Connect again (which a token
// change does anyway). The external id is the bot id, so a bot can be
// connected to only one instance. Without PublicBaseURL (local dev) Connect
// only validates the token.
//
// Decisions:
//   - Private chats: every message opens/continues the contact's
//     conversation; the contact is the chat id (instance scoped).
//   - Groups and supergroups: only messages addressed to the bot (@username
//     mention, text_mention of the bot, /command@botname, reply to a bot
//     message) are taken; the contact is the group itself and the human
//     author goes to Attributes (author_id, author_name, author_username,
//     chat_type). The text is passed through unchanged. Plain /commands
//     without @botname in groups are ignored (ambiguous with several bots).
//   - The bot username for mention detection comes from a getMe call cached
//     per token for an hour (Connect refreshes it); if getMe fails,
//     inst.Settings["bot_username"] is used. The bot id is read from the
//     token prefix, so private chats never need a network call.
//   - Message ids are only unique per chat: external ids are
//     "<chat_id>:<message_id>"; button presses are "cb:<callback_query_id>".
//   - edited_message and my_chat_member updates are subscribed (so a later
//     version can surface them) but ignored: stored messages are immutable.
//   - callback_query is answered (answerCallbackQuery) best effort in the
//     background so the user's button stops spinning.
//   - Send is plain text (no parse_mode, nothing to escape). Texts over 4096
//     UTF-16 units are split on rune boundaries (preferring newlines/spaces).
//     Attachments go one message each (no sendMediaGroup: albums cannot carry
//     an inline keyboard and partial failures are clearer); the text becomes
//     the caption of the first one when it fits 1024 units, else it is sent
//     first as separate messages. Attachments go by URL when one is given
//     (images over 5 MB as documents, Telegram's URL photo limit), else they
//     are uploaded with multipart. Buttons go on the last message, the reply
//     reference on the first. The result id is the last message sent.
//   - Inbound files are fetched lazily through getFile; files over 20 MB
//     (Bot API download limit) fail with ErrFileTooLarge.
package telegram

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

const (
	// DefaultBaseURL is the public Bot API server.
	DefaultBaseURL = "https://api.telegram.org"
	// TokenField is the credential key of the bot token.
	TokenField = "bot_token"
	// SettingBotUsername is the optional instance setting used when getMe
	// is unavailable for group mention detection.
	SettingBotUsername = "bot_username"
	// SecretHeader carries the webhook secret token.
	SecretHeader = "X-Telegram-Bot-Api-Secret-Token" // #nosec G101 -- Telegram protocol header name; its secret value is loaded from encrypted credentials
	// WebhookPath is the API path prefix webhooks are delivered to.
	WebhookPath = "/v1/webhooks/channels/telegram/"
	// MaxDownloadSize is the Bot API getFile limit.
	MaxDownloadSize = 20 << 20

	maxText         = 4096
	maxCaption      = 1024
	maxCallbackData = 64
	maxPhotoURLSize = 5 << 20
	botInfoTTL      = time.Hour
	healthWindow    = time.Hour
)

// AllowedUpdates are the update types setWebhook subscribes to.
var AllowedUpdates = []string{"message", "edited_message", "callback_query", "my_chat_member"}

// ErrFileTooLarge: an inbound file exceeds the Bot API download limit.
var ErrFileTooLarge = errors.New("telegram: file exceeds the 20 MB Bot API download limit")

// Config configures the adapter.
type Config struct {
	// BaseURL is the Bot API origin (default DefaultBaseURL).
	BaseURL string
	// PublicBaseURL is the API origin Telegram reaches webhooks on
	// (https://api.getclickfix.com); empty skips setWebhook.
	PublicBaseURL string
	// HTTPClient is used for every call (default: a client without a global
	// timeout; API calls carry their own deadline).
	HTTPClient *http.Client
}

// Adapter is the Telegram channel.
type Adapter struct {
	baseURL       string
	publicBaseURL string
	hc            *http.Client
	bots          sync.Map // sha256(token) → cachedBot
	now           func() time.Time
}

// Bot is the identity of a connected bot.
type Bot struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type cachedBot struct {
	bot Bot
	at  time.Time
}

var (
	_ channels.Adapter       = (*Adapter)(nil)
	_ channels.Typer         = (*Adapter)(nil)
	_ channels.HealthChecker = (*Adapter)(nil)
)

// New builds the adapter.
func New(cfg Config) *Adapter {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{}
	}
	return &Adapter{
		baseURL:       base,
		publicBaseURL: strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/"),
		hc:            hc,
		now:           time.Now,
	}
}

// Type implements channels.Adapter.
func (a *Adapter) Type() string { return channels.TypeTelegram }

// Capabilities implements channels.Adapter.
func (a *Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{Media: true, Typing: true, MaxTextLength: maxText}
}

// Fields implements channels.Adapter.
func (a *Adapter) Fields() []channels.Field {
	return []channels.Field{{Key: TokenField, LabelKey: "channels.fields.bot_token", Secret: true, Required: true}}
}

// SecretToken is the webhook secret of an instance: the first 48 hex chars
// of HMAC-SHA256(bot_token, "clickfix-telegram-webhook:"+uuid).
func SecretToken(inst channels.Instance) string {
	mac := hmac.New(sha256.New, []byte(inst.Credentials[TokenField]))
	mac.Write([]byte("clickfix-telegram-webhook:" + inst.UUID.String()))
	return hex.EncodeToString(mac.Sum(nil))[:48]
}

// WebhookURL is the URL Telegram delivers an instance's updates to ("" when
// PublicBaseURL is not configured).
func (a *Adapter) WebhookURL(inst channels.Instance) string {
	if a.publicBaseURL == "" {
		return ""
	}
	return a.publicBaseURL + WebhookPath + inst.UUID.String()
}

func tokenOf(inst channels.Instance) (string, error) {
	t := strings.TrimSpace(inst.Credentials[TokenField])
	if t == "" || !strings.Contains(t, ":") {
		return "", errors.New("telegram: bot_token is missing or malformed")
	}
	return t, nil
}

// botIDOf reads the bot id from the token prefix ("123456:ABC…"), falling
// back to the instance external id.
func botIDOf(token string, inst channels.Instance) int64 {
	if i := strings.IndexByte(token, ':'); i > 0 {
		if id, err := strconv.ParseInt(token[:i], 10, 64); err == nil {
			return id
		}
	}
	id, _ := strconv.ParseInt(inst.ExternalID, 10, 64)
	return id
}

// Connect implements channels.Adapter: getMe, then setWebhook. It returns
// the bot id.
func (a *Adapter) Connect(ctx context.Context, inst channels.Instance) (string, error) {
	token, err := tokenOf(inst)
	if err != nil {
		return "", err
	}
	bot, err := a.fetchBot(ctx, token)
	if err != nil {
		return "", err
	}
	if url := a.WebhookURL(inst); url != "" {
		params := map[string]any{
			"url":                  url,
			"secret_token":         SecretToken(inst),
			"allowed_updates":      AllowedUpdates,
			"drop_pending_updates": false,
		}
		if err := a.call(ctx, token, "setWebhook", params, nil); err != nil {
			return "", err
		}
	}
	return strconv.FormatInt(bot.ID, 10), nil
}

// Disconnect removes the bot's webhook (pending updates are kept).
func (a *Adapter) Disconnect(ctx context.Context, inst channels.Instance) error {
	token, err := tokenOf(inst)
	if err != nil {
		return err
	}
	return a.call(ctx, token, "deleteWebhook", map[string]any{"drop_pending_updates": false}, nil)
}

// BotInfo calls getMe and refreshes the cached identity.
func (a *Adapter) BotInfo(ctx context.Context, inst channels.Instance) (Bot, error) {
	token, err := tokenOf(inst)
	if err != nil {
		return Bot{}, err
	}
	return a.fetchBot(ctx, token)
}

func (a *Adapter) fetchBot(ctx context.Context, token string) (Bot, error) {
	var u user
	if err := a.call(ctx, token, "getMe", nil, &u); err != nil {
		return Bot{}, err
	}
	if !u.IsBot || u.ID == 0 {
		return Bot{}, errors.New("telegram: the token does not belong to a bot")
	}
	b := Bot{ID: u.ID, Username: u.Username, Name: u.FullName()}
	a.bots.Store(tokenKey(token), cachedBot{bot: b, at: a.now()})
	return b, nil
}

func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// botUsername returns the bot's username for mention detection: cached
// getMe, then a fresh getMe, then inst.Settings["bot_username"].
func (a *Adapter) botUsername(ctx context.Context, inst channels.Instance, token string) string {
	if v, ok := a.bots.Load(tokenKey(token)); ok {
		if c := v.(cachedBot); a.now().Sub(c.at) < botInfoTTL && c.bot.Username != "" {
			return c.bot.Username
		}
	}
	if b, err := a.fetchBot(ctx, token); err == nil && b.Username != "" {
		return b.Username
	}
	s, _ := inst.Settings[SettingBotUsername].(string)
	return strings.TrimPrefix(strings.TrimSpace(s), "@")
}

// VerifyWebhook implements channels.Adapter.
func (a *Adapter) VerifyWebhook(r *http.Request, _ []byte, inst channels.Instance) error {
	if _, err := tokenOf(inst); err != nil {
		return channels.ErrSignature
	}
	got := r.Header.Get(SecretHeader)
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(SecretToken(inst))) != 1 {
		return channels.ErrSignature
	}
	return nil
}

// SendTyping implements channels.Typer. Telegram shows "typing…" for ~5s
// and has no way to stop it, so on=false is a no-op.
func (a *Adapter) SendTyping(ctx context.Context, inst channels.Instance, to channels.Recipient, on bool) error {
	if !on {
		return nil
	}
	token, err := tokenOf(inst)
	if err != nil {
		return err
	}
	chat := chatParam(to.ExternalID)
	if chat == nil {
		return errors.New("telegram: recipient chat id is empty")
	}
	return a.call(ctx, token, "sendChatAction", map[string]any{"chat_id": chat, "action": "typing"}, nil)
}

// CheckHealth implements channels.HealthChecker via getWebhookInfo.
func (a *Adapter) CheckHealth(ctx context.Context, inst channels.Instance) (channels.Health, error) {
	token, err := tokenOf(inst)
	if err != nil {
		return channels.Health{OK: false, Detail: err.Error()}, nil
	}
	var info struct {
		URL              string `json:"url"`
		LastErrorDate    int64  `json:"last_error_date"`
		LastErrorMessage string `json:"last_error_message"`
	}
	if err := a.call(ctx, token, "getWebhookInfo", nil, &info); err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.Code < 500 && ae.Code != http.StatusTooManyRequests {
			return channels.Health{OK: false, Detail: ae.Description}, nil
		}
		return channels.Health{}, err
	}
	if want := a.WebhookURL(inst); want != "" && info.URL != want {
		if info.URL == "" {
			return channels.Health{OK: false, Detail: "webhook is not set"}, nil
		}
		return channels.Health{OK: false, Detail: "webhook points to " + info.URL}, nil
	}
	if info.LastErrorDate > 0 && a.now().Sub(time.Unix(info.LastErrorDate, 0)) < healthWindow {
		detail := info.LastErrorMessage
		if detail == "" {
			detail = "webhook delivery failed"
		}
		return channels.Health{OK: false, Detail: fmt.Sprintf("webhook: %s", detail)}, nil
	}
	return channels.Health{OK: true}, nil
}
