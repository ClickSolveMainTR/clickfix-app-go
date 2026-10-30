// Package slack implements workspace installs and threaded Slack Connect messages.
package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
)

type Config struct {
	ClientID      string
	ClientSecret  string
	SigningSecret string
	RedirectURL   string
	BaseURL       string
	Client        channelhttp.Client
}
type Adapter struct {
	cfg   Config
	names NameCache
}

// NameCache keeps channel and user display names between events so a busy
// channel does not call Slack for every message. An empty name is a cached
// miss (failed or unnamed lookup).
type NameCache interface {
	GetName(ctx context.Context, key string) (name string, ok bool)
	SetName(ctx context.Context, key, name string, ttl time.Duration)
}

const (
	nameTTL = 24 * time.Hour
	// missTTL backs off after a failed lookup (missing scope, rate limit).
	missTTL = 5 * time.Minute
	// nameLookupTimeout keeps lookups inside Slack's 3 second event deadline.
	nameLookupTimeout = 2 * time.Second
	maxCachedNames    = 10000
)

// memoryNames is the per-process NameCache used without Redis.
type memoryNames struct {
	mu    sync.Mutex
	names map[string]cachedName
}
type cachedName struct {
	name    string
	expires time.Time
}

func (m *memoryNames) GetName(_ context.Context, key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.names[key]
	if !ok || time.Now().After(c.expires) {
		return "", false
	}
	return c.name, true
}
func (m *memoryNames) SetName(_ context.Context, key, name string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.names) >= maxCachedNames {
		m.names = map[string]cachedName{}
	}
	m.names[key] = cachedName{name: name, expires: time.Now().Add(ttl)}
}

func New(cfg Config) *Adapter {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://slack.com/api"
	}
	if cfg.Client == nil {
		cfg.Client = channelhttp.DefaultClient()
	}
	return &Adapter{cfg: cfg, names: &memoryNames{names: map[string]cachedName{}}}
}

// WithNameCache shares looked-up names across processes (Redis in the API).
func (a *Adapter) WithNameCache(c NameCache) *Adapter {
	if c != nil {
		a.names = c
	}
	return a
}
func (*Adapter) Type() string { return channels.TypeSlack }
func (*Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{Threading: true, MaxTextLength: 4000}
}
func (*Adapter) Fields() []channels.Field {
	return channelhttp.Fields("bot_token team_id signing_secret", "bot_user_id", "bot_token signing_secret")
}
func (a *Adapter) LoginURL(state string) string {
	q := url.Values{"client_id": {a.cfg.ClientID},
		"redirect_uri": {a.cfg.RedirectURL},
		"state":        {state},
		"scope":        {"chat:write,channels:history,groups:history,reactions:read,channels:read,groups:read,users:read"}}
	return "https://slack.com/oauth/v2/authorize?" + q.Encode()
}
func (a *Adapter) OAuthEnabled() bool {
	return a.cfg.ClientID != "" && a.cfg.ClientSecret != "" && a.cfg.SigningSecret != "" && a.cfg.RedirectURL != ""
}
func (a *Adapter) ExchangeCode(ctx context.Context, code string) (channels.Credentials, error) {
	if !a.OAuthEnabled() {
		return nil, errors.New("slack OAuth app is not configured")
	}
	form := url.Values{"code": {code}, "redirect_uri": {a.cfg.RedirectURL}}
	req,
		err := http.NewRequestWithContext(ctx,
		http.MethodPost,
		a.cfg.BaseURL+"/oauth.v2.access",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(a.cfg.ClientID, a.cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var res struct {
		OK        bool   `json:"ok"`
		Token     string `json:"access_token"`
		BotUserID string `json:"bot_user_id"`
		Team      struct {
			ID string `json:"id"`
		} `json:"team"`
	}
	if err := channelhttp.Do(a.cfg.Client, req, &res); err != nil {
		return nil, err
	}
	if !res.OK || res.Token == "" || res.Team.ID == "" {
		return nil, errors.New("slack OAuth exchange refused")
	}
	return channels.Credentials{"bot_token": res.Token,
			"team_id":        res.Team.ID,
			"bot_user_id":    res.BotUserID,
			"signing_secret": a.cfg.SigningSecret},
		nil
}
func (a *Adapter) call(ctx context.Context, inst channels.Instance, method string, payload any, out any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.BaseURL+"/"+method, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	return a.do(req, inst, out)
}
func (a *Adapter) do(req *http.Request, inst channels.Instance, out any) error {
	req.Header.Set("Authorization", "Bearer "+inst.Credentials["bot_token"])
	var raw json.RawMessage
	if err := channelhttp.Do(a.cfg.Client, req, &raw); err != nil {
		return err
	}
	var status struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return err
	}
	if !status.OK {
		err := errors.New("slack API refused request")
		switch status.Error {
		case "ratelimited", "internal_error", "fatal_error", "service_unavailable":
			return &channels.SendError{Code: "slack_unavailable", Err: err}
		}
		return channels.Permanent("slack_rejected", err)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Conversation is a channel the bot is a member of, offered for the
// settings.channel_ids allowlist.
type Conversation struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Private bool   `json:"is_private"`
	Shared  bool   `json:"is_shared"`
}

// maxConversationPages bounds users.conversations pagination (200 per page).
const maxConversationPages = 5

// Conversations lists the unarchived channels the bot was invited to; read
// methods take form parameters, not JSON.
func (a *Adapter) Conversations(ctx context.Context, inst channels.Instance) ([]Conversation, error) {
	out := []Conversation{}
	cursor := ""
	for range maxConversationPages {
		form := url.Values{"types": {"public_channel,private_channel"},
			"exclude_archived": {"true"},
			"limit":            {"200"}}
		if cursor != "" {
			form.Set("cursor", cursor)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.BaseURL+"/users.conversations", strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		var page struct {
			Channels []Conversation `json:"channels"`
			Meta     struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := a.do(req, inst, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Channels...)
		if cursor = page.Meta.NextCursor; cursor == "" {
			break
		}
	}
	return out, nil
}

// lookupName returns a cached or freshly read display name; failures yield ""
// so a missing scope or a Slack outage never blocks message ingestion.
// Keys are scoped by organization and workspace.
func (a *Adapter) lookupName(ctx context.Context, inst channels.Instance, method, param, id string) string {
	if id == "" {
		return ""
	}
	key := strconv.FormatInt(inst.OrganizationID, 10) + ":" + inst.Credentials["team_id"] + ":" + method + ":" + id
	if name, ok := a.names.GetName(ctx, key); ok {
		return name
	}
	name := a.fetchName(ctx, inst, method, param, id)
	ttl := nameTTL
	if name == "" {
		ttl = missTTL
	}
	a.names.SetName(ctx, key, name, ttl)
	return name
}
func (a *Adapter) fetchName(ctx context.Context, inst channels.Instance, method, param, id string) string {
	ctx, cancel := context.WithTimeout(ctx, nameLookupTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.BaseURL+"/"+method,
		strings.NewReader(url.Values{param: {id}}.Encode()))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var res struct {
		Channel struct {
			Name string `json:"name"`
		} `json:"channel"`
		User struct {
			Name    string `json:"name"`
			Profile struct {
				DisplayName string `json:"display_name"`
				RealName    string `json:"real_name"`
			} `json:"profile"`
		} `json:"user"`
	}
	if a.do(req, inst, &res) != nil {
		return ""
	}
	if name := strings.TrimSpace(res.Channel.Name); name != "" {
		return "#" + name
	}
	return firstNonEmpty(res.User.Profile.DisplayName, res.User.Profile.RealName, res.User.Name)
}
func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}
func (a *Adapter) Connect(ctx context.Context, inst channels.Instance) (string, error) {
	if err := channelhttp.Required(inst, "bot_token", "team_id", "signing_secret"); err != nil {
		return "", err
	}
	var res struct {
		TeamID string `json:"team_id"`
	}
	if err := a.call(ctx, inst, "auth.test", map[string]any{}, &res); err != nil {
		return "", err
	}
	if res.TeamID != inst.Credentials["team_id"] {
		return "", errors.New("slack workspace mismatch")
	}
	return res.TeamID, nil
}
func (a *Adapter) CheckHealth(ctx context.Context, inst channels.Instance) (channels.Health, error) {
	_, err := a.Connect(ctx, inst)
	return channels.Health{OK: err == nil}, err
}
func verify(r *http.Request, b []byte, secret string) error {
	ts := r.Header.Get("X-Slack-Request-Timestamp")
	if secret == "" || !channelhttp.Fresh(ts, time.Now()) {
		return channels.ErrSignature
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("v0:" + ts + ":"))
	m.Write(b)
	if !hmac.Equal([]byte("v0="+hex.EncodeToString(m.Sum(nil))), []byte(r.Header.Get("X-Slack-Signature"))) {
		return channels.ErrSignature
	}
	return nil
}
func (*Adapter) VerifyWebhook(r *http.Request, b []byte, inst channels.Instance) error {
	return verify(r, b, inst.Credentials["signing_secret"])
}

// RespondWebhook handles Slack's signed POST subscription challenge, before account routing.
func (a *Adapter) RespondWebhook(r *http.Request, b []byte, inst channels.Instance) (string, string, bool, error) {
	var v struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
	}
	if json.Unmarshal(b, &v) != nil || v.Type != "url_verification" {
		return "", "", false, nil
	}
	secret := inst.Credentials["signing_secret"]
	if secret == "" {
		secret = a.cfg.SigningSecret
	}
	if err := verify(r, b, secret); err != nil {
		return "", "", true, err
	}
	if v.Challenge == "" {
		return "", "", true, errors.New("missing challenge")
	}
	return "text/plain", v.Challenge, true, nil
}

type envelope struct {
	Type    string `json:"type"`
	TeamID  string `json:"team_id"`
	EventID string `json:"event_id"`
	Event   struct {
		Type     string `json:"type"`
		Subtype  string `json:"subtype"`
		Channel  string `json:"channel"`
		User     string `json:"user"`
		BotID    string `json:"bot_id"`
		Text     string `json:"text"`
		TS       string `json:"ts"`
		ThreadTS string `json:"thread_ts"`
		Reaction string `json:"reaction"`
		Item     struct {
			Channel string `json:"channel"`
			TS      string `json:"ts"`
		} `json:"item"`
	} `json:"event"`
}

func (*Adapter) Route(_ *http.Request, b []byte) ([]string, error) {
	var e envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	if e.TeamID == "" {
		return nil, errors.New("missing team_id")
	}
	return []string{e.TeamID}, nil
}

// channel_ids is an explicit allowlist of shared channels; never ingest an unselected channel.
func allowed(inst channels.Instance, id string) bool {
	switch ids := inst.Settings["channel_ids"].(type) {
	case []any:
		for _, v := range ids {
			if v == id {
				return true
			}
		}
	case []string:
		for _, v := range ids {
			if v == id {
				return true
			}
		}
	}
	return false
}
func (a *Adapter) ParseWebhook(ctx context.Context,
	_ *http.Request,
	b []byte,
	inst channels.Instance) (channels.WebhookResult, error) {
	var e envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return channels.WebhookResult{}, err
	}
	if e.TeamID != inst.Credentials["team_id"] {
		return channels.WebhookResult{}, channels.ErrSignature
	}
	v := e.Event
	if e.Type != "event_callback" || v.BotID != "" || v.User == inst.Credentials["bot_user_id"] {
		return channels.WebhookResult{}, nil
	}
	text, kind := v.Text, "text"
	if v.Type == "reaction_added" && v.Reaction == "ticket" {
		v.Channel = v.Item.Channel
		v.TS = v.Item.TS
		text = "🎫 Destek talebi"
		kind = "interactive"
	} else if v.Type != "message" || v.Subtype != "" {
		return channels.WebhookResult{}, nil
	}
	if !allowed(inst, v.Channel) || v.TS == "" || text == "" {
		return channels.WebhookResult{}, nil
	}
	root := v.ThreadTS
	if root == "" {
		root = v.TS
	}
	id := v.Channel + ":" + v.TS
	if kind == "interactive" {
		id = "event:" + e.EventID
		if e.EventID == "" {
			return channels.WebhookResult{}, errors.New("missing event_id")
		}
	}
	m := channels.InboundMessage{ExternalID: id,
		From: channels.Sender{ExternalID: v.Channel,
			InstanceScoped: true,
			Name:           a.lookupName(ctx, inst, "conversations.info", "channel", v.Channel)},
		Kind:     kind,
		Text:     text,
		ThreadID: v.Channel + ":" + root,
		At:       slackTime(v.TS),
		Attributes: map[string]any{"author_id": v.User,
			"slack_channel":    v.Channel,
			"ticket_requested": kind == "interactive"}}
	if author := a.lookupName(ctx, inst, "users.info", "user", v.User); author != "" {
		m.Attributes["author_name"] = author
	}
	if v.ThreadTS != "" {
		m.ReplyToExternalID = v.Channel + ":" + v.ThreadTS
	}
	return channels.WebhookResult{Messages: []channels.InboundMessage{m}}, nil
}
func (a *Adapter) Send(ctx context.Context,
	inst channels.Instance,
	msg channels.OutboundMessage) (channels.SendResult,
	error) {
	ch, root, ok := strings.Cut(msg.ThreadID, ":")
	if !ok || ch == "" || root == "" || !allowed(inst, ch) {
		return channels.SendResult{}, channels.Permanent("thread", errors.New("selected channel and thread required"))
	}
	if len(msg.Attachments) > 0 || msg.Text == "" || len([]rune(msg.Text)) > 4000 {
		return channels.SendResult{},
			channels.Permanent("content",
				errors.New("slack adapter accepts up to 4000 text characters"))
	}
	var res struct {
		TS string `json:"ts"`
	}
	err := a.call(ctx,
		inst,
		"chat.postMessage",
		map[string]any{"channel": ch,
			"thread_ts":     root,
			"text":          msg.Text,
			"client_msg_id": msg.MessageUUID.String(),
			"unfurl_links":  false,
			"unfurl_media":  false},
		&res)
	if err != nil {
		return channels.SendResult{}, err
	}
	if res.TS == "" {
		return channels.SendResult{}, fmt.Errorf("slack: missing message timestamp")
	}
	return channels.SendResult{ExternalID: ch + ":" + res.TS}, nil
}

var _ channels.Adapter = (*Adapter)(nil)
var _ channels.Router = (*Adapter)(nil)

func slackTime(ts string) time.Time {
	sec, fraction, _ := strings.Cut(ts, ".")
	seconds, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		return time.Now().UTC()
	}
	if len(fraction) > 9 {
		fraction = fraction[:9]
	}
	fraction += strings.Repeat("0", 9-len(fraction))
	nanos, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Unix(seconds, nanos).UTC()
}
