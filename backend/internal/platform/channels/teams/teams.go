// Package teams implements Microsoft Bot Framework text activities with per-organization app credentials.
package teams

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/format"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const issuer = "https://api.botframework.com"

type Config struct {
	Client   channelhttp.Client
	KeysURL  string
	LoginURL string
}
type signingKey struct {
	key          *rsa.PublicKey
	endorsements []string
}
type Adapter struct {
	cfg       Config
	mu        sync.Mutex
	keys      map[string]signingKey
	refresh   time.Time
	lastFetch time.Time
}

func New(cfg Config) *Adapter {
	if cfg.Client == nil {
		cfg.Client = channelhttp.DefaultClient()
	}
	if cfg.KeysURL == "" {
		cfg.KeysURL = "https://login.botframework.com/v1/.well-known/keys"
	}
	if cfg.LoginURL == "" {
		cfg.LoginURL = "https://login.microsoftonline.com"
	}
	return &Adapter{cfg: cfg, keys: map[string]signingKey{}}
}
func (*Adapter) Type() string { return channels.TypeTeams }
func (*Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{Threading: true, MaxTextLength: 4000}
}
func (*Adapter) Fields() []channels.Field {
	return channelhttp.Fields("app_id app_password tenant_id", "", "app_password")
}
func (a *Adapter) accessToken(ctx context.Context, inst channels.Instance) (string, error) {
	if err := channelhttp.Required(inst, "app_id", "app_password", "tenant_id"); err != nil {
		return "", err
	}
	if _, err := uuid.Parse(inst.Credentials["app_id"]); err != nil {
		return "", errors.New("invalid Teams app ID")
	}
	tenant := inst.Credentials["tenant_id"]
	if _, err := uuid.Parse(tenant); err != nil {
		return "", errors.New("a single-tenant Teams registration requires its tenant UUID")
	}
	form := url.Values{"grant_type": {"client_credentials"},
		"client_id":     {inst.Credentials["app_id"]},
		"client_secret": {inst.Credentials["app_password"]},
		"scope":         {"https://api.botframework.com/.default"}}
	req,
		err := http.NewRequestWithContext(ctx,
		http.MethodPost,
		a.cfg.LoginURL+"/"+tenant+"/oauth2/v2.0/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		Token string `json:"access_token"`
		Type  string `json:"token_type"`
	}
	if err := channelhttp.Do(a.cfg.Client, req, &out); err != nil {
		return "", err
	}
	if out.Token == "" || !strings.EqualFold(out.Type, "Bearer") {
		return "", errors.New("teams access token missing")
	}
	return out.Token, nil
}
func (a *Adapter) Connect(ctx context.Context, inst channels.Instance) (string, error) {
	_, err := a.accessToken(ctx, inst)
	if err != nil {
		return "", err
	}
	return inst.Credentials["app_id"], nil
}
func (a *Adapter) CheckHealth(ctx context.Context, inst channels.Instance) (channels.Health, error) {
	_, err := a.Connect(ctx, inst)
	return channels.Health{OK: err == nil}, err
}
func (a *Adapter) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, known := a.keys[kid]
	if time.Now().After(a.refresh) || !known {
		// Bound refresh attempts, including failures, on this public webhook path.
		if time.Since(a.lastFetch) < time.Minute {
			return nil, channels.ErrSignature
		}
		a.lastFetch = time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.KeysURL, nil)
		if err != nil {
			return nil, err
		}
		var doc struct {
			Keys []struct {
				KID          string   `json:"kid"`
				N            string   `json:"n"`
				E            string   `json:"e"`
				KTY          string   `json:"kty"`
				Use          string   `json:"use"`
				Endorsements []string `json:"endorsements"`
			} `json:"keys"`
		}
		if err := channelhttp.Do(a.cfg.Client, req, &doc); err != nil {
			return nil, err
		}
		keys := map[string]signingKey{}
		for _, k := range doc.Keys {
			n, en := base64.RawURLEncoding.DecodeString(k.N)
			e, ee := base64.RawURLEncoding.DecodeString(k.E)
			if en != nil || ee != nil || len(n) < 256 || len(e) > 4 || len(e) == 0 || k.KTY != "RSA" || k.Use != "sig" {
				continue
			}
			exp := 0
			for _, v := range e {
				exp = exp*256 + int(v)
			}
			if exp < 3 {
				continue
			}
			keys[k.KID] = signingKey{key: &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp}, endorsements: k.Endorsements}
		}
		a.keys = keys
		a.refresh = time.Now().Add(time.Hour)
		a.lastFetch = time.Now()
	}
	k, ok := a.keys[kid]
	if !ok {
		return nil, channels.ErrSignature
	}
	if len(k.endorsements) > 0 && !slices.Contains(k.endorsements, "msteams") {
		return nil, channels.ErrSignature
	}
	return k.key, nil
}

type activity struct {
	Type       string    `json:"type"`
	ID         string    `json:"id"`
	ChannelID  string    `json:"channelId"`
	ServiceURL string    `json:"serviceUrl"`
	Text       string    `json:"text"`
	ReplyToID  string    `json:"replyToId"`
	At         time.Time `json:"timestamp"`
	From       struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"from"`
	Recipient struct {
		ID string `json:"id"`
	} `json:"recipient"`
	Conversation struct {
		ID       string `json:"id"`
		Type     string `json:"conversationType"`
		TenantID string `json:"tenantId"`
	} `json:"conversation"`
	ChannelData struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	} `json:"channelData"`
}

func serviceURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.HasSuffix(host,
		".trafficmanager.net") ||
		strings.HasSuffix(host,
			".botframework.com") ||
		strings.HasSuffix(host,
			".skype.com")
}
func (a *Adapter) VerifyWebhook(r *http.Request, b []byte, inst channels.Instance) error {
	var v activity
	if json.Unmarshal(b, &v) != nil || v.ChannelID != "msteams" || !serviceURL(v.ServiceURL) {
		return channels.ErrSignature
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return channels.ErrSignature
	}
	claims := jwt.MapClaims{}
	_,
		err := jwt.ParseWithClaims(token,
		claims,
		func(t *jwt.Token) (any,
			error) {
			kid,
				_ := t.Header["kid"].(string)
			return a.key(r.Context(),
				kid)
		},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(inst.Credentials["app_id"]),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(5*time.Minute))
	if err != nil || claims["serviceurl"] != v.ServiceURL && claims["serviceUrl"] != v.ServiceURL {
		return channels.ErrSignature
	}
	// Single-tenant app registrations cannot ingest another customer's tenant.
	tenant := inst.Credentials["tenant_id"]
	if v.Conversation.TenantID != tenant && v.ChannelData.Tenant.ID != tenant {
		return channels.ErrSignature
	}
	return nil
}

type thread struct {
	Service      string `json:"service"`
	Conversation string `json:"conversation"`
	Root         string `json:"root"`
}

func (*Adapter) ParseWebhook(_ context.Context,
	_ *http.Request,
	b []byte,
	inst channels.Instance) (channels.WebhookResult, error) {
	var v activity
	if err := json.Unmarshal(b, &v); err != nil {
		return channels.WebhookResult{}, err
	}
	if v.Type != "message" || v.From.ID == v.Recipient.ID {
		return channels.WebhookResult{}, nil
	}
	if v.ID == "" || v.Conversation.ID == "" || v.From.ID == "" || v.Text == "" || !serviceURL(v.ServiceURL) {
		return channels.WebhookResult{}, errors.New("invalid Teams message")
	}
	root := v.ReplyToID
	if root == "" && v.Conversation.Type != "personal" {
		root = v.ID
	}
	replyTo := ""
	if v.ReplyToID != "" {
		replyTo = v.Conversation.ID + ":" + v.ReplyToID
	}
	ref, _ := json.Marshal(thread{Service: v.ServiceURL, Conversation: v.Conversation.ID, Root: root})
	if v.At.IsZero() {
		v.At = time.Now().UTC()
	}
	return channels.WebhookResult{Messages: []channels.InboundMessage{{ExternalID: v.Conversation.ID + ":" + v.ID,
			From: channels.Sender{ExternalID: v.Conversation.ID,
				InstanceScoped: true},
			Kind:              "text",
			Text:              v.Text,
			ThreadID:          base64.RawURLEncoding.EncodeToString(ref),
			ReplyToExternalID: replyTo,
			At:                v.At,
			Attributes: map[string]any{"author_id": v.From.ID,
				"author_name":  v.From.Name,
				"teams_app_id": inst.Credentials["app_id"]}}}},
		nil
}
func (a *Adapter) Send(ctx context.Context,
	inst channels.Instance,
	m channels.OutboundMessage) (channels.SendResult,
	error) {
	var ref thread
	b, err := base64.RawURLEncoding.DecodeString(m.ThreadID)
	if err != nil || json.Unmarshal(b, &ref) != nil || !serviceURL(ref.Service) || ref.Conversation == "" {
		return channels.SendResult{}, channels.Permanent("thread", errors.New("valid Teams conversation reference required"))
	}
	// CommonMark for textFormat "extendedmarkdown" (CF-177); the limit
	// applies to the text that is sent.
	text := format.Teams(m.Text)
	if len(m.Attachments) > 0 || text == "" || len([]rune(text)) > 4000 {
		return channels.SendResult{},
			channels.Permanent("content",
				errors.New("teams adapter accepts up to 4000 text characters"))
	}
	token, err := a.accessToken(ctx, inst)
	if err != nil {
		return channels.SendResult{}, err
	}
	path := strings.TrimRight(ref.Service, "/") + "/v3/conversations/" + url.PathEscape(ref.Conversation) + "/activities"
	if ref.Root != "" {
		path += "/" + url.PathEscape(ref.Root)
	}
	var out struct {
		ID string `json:"id"`
	}
	post := func(text, textFormat string) error {
		payload, _ := json.Marshal(map[string]any{"type": "message",
			"text":       text,
			"textFormat": textFormat,
			"replyToId":  ref.Root})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return channelhttp.Do(a.cfg.Client, req, &out)
	}
	err = post(text, "extendedmarkdown")
	if channelhttp.Rejected(err) {
		// The formatted activity was refused: the message must still arrive,
		// so send it once more as plain text.
		if plain := format.Plain(m.Text); plain != "" {
			err = post(plain, "plain")
		}
	}
	if err != nil {
		return channels.SendResult{}, err
	}
	if out.ID == "" {
		return channels.SendResult{}, errors.New("teams response missing activity ID")
	}
	return channels.SendResult{ExternalID: ref.Conversation + ":" + out.ID}, nil
}

var _ channels.Adapter = (*Adapter)(nil)
