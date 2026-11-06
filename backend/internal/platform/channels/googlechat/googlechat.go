// Package googlechat implements a Google Chat app (HTTP endpoint mode) with
// per-organization service account credentials (CF-137). Google calls the
// instance webhook with a bearer JWT signed by chat@system.gserviceaccount.com
// whose audience is the Cloud project number; replies go out through the
// Chat API (spaces.messages.create) as the app, authenticated with the
// service account (OAuth 2.0 JWT bearer grant, chat.bot scope).
//
// Mapping: a direct message space is one conversation whose contact is the
// user; in rooms (spaces and group chats) every thread is a conversation
// whose contact is the space, the writer travels as author_name (as Slack).
// Rooms are ingested only when listed in settings.space_ids.
package googlechat

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/format"
	"github.com/golang-jwt/jwt/v5"
)

const (
	// Issuer of the bearer tokens Google Chat sends to HTTP endpoint apps.
	Issuer = "chat@system.gserviceaccount.com"
	// Scope of the app-authenticated Chat API calls.
	Scope = "https://www.googleapis.com/auth/chat.bot"

	defaultCertsURL      = "https://www.googleapis.com/service_accounts/v1/metadata/x509/" + Issuer
	defaultOAuthEndpoint = "https://oauth2.googleapis.com/token"
	defaultBaseURL       = "https://chat.googleapis.com"

	// MaxText bounds one outbound message (Chat accepts 4096 characters).
	MaxText = 4096

	certTTL     = time.Hour
	certBackoff = time.Minute
	// tokenSkew renews an access token this long before it expires.
	tokenSkew = 2 * time.Minute

	nameTTL           = 24 * time.Hour
	missTTL           = 5 * time.Minute
	nameLookupTimeout = 2 * time.Second
	maxCachedNames    = 10000
	maxCachedTokens   = 1000
	maxSpacePages     = 5
)

// Credential and setting keys.
const (
	FieldServiceAccount = "service_account_json"
	FieldProjectNumber  = "project_number"
	SettingSpaceIDs     = "space_ids"
)

var (
	spaceRe         = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]{1,128}$`)
	threadRe        = regexp.MustCompile(`^(spaces/[A-Za-z0-9_-]{1,128})/threads/[A-Za-z0-9_.-]{1,128}$`)
	projectNumberRe = regexp.MustCompile(`^[0-9]{1,20}$`)
)

// Config points the adapter at Google; tests replace every URL.
type Config struct {
	Client   channelhttp.Client
	CertsURL string
	TokenURL string
	BaseURL  string
}

// NameCache keeps space and user display names between events (the Slack
// name fix, CF-119): Google sends names on most events, the cache fills the
// ones it leaves out. An empty name is a cached miss.
type NameCache interface {
	GetName(ctx context.Context, key string) (name string, ok bool)
	SetName(ctx context.Context, key, name string, ttl time.Duration)
}

type accessToken struct {
	token   string
	expires time.Time
}

// Adapter is the Google Chat channel.
type Adapter struct {
	cfg   Config
	names NameCache

	mu        sync.Mutex
	certs     map[string]*rsa.PublicKey
	refresh   time.Time
	lastFetch time.Time

	tokMu  sync.Mutex
	tokens map[string]accessToken
}

// New builds the adapter.
func New(cfg Config) *Adapter {
	if cfg.Client == nil {
		cfg.Client = channelhttp.DefaultClient()
	}
	if cfg.CertsURL == "" {
		cfg.CertsURL = defaultCertsURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = defaultOAuthEndpoint
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return &Adapter{cfg: cfg, names: &memoryNames{names: map[string]cachedName{}},
		certs: map[string]*rsa.PublicKey{}, tokens: map[string]accessToken{}}
}

// WithNameCache shares names across processes (Redis in the API).
func (a *Adapter) WithNameCache(c NameCache) *Adapter {
	if c != nil {
		a.names = c
	}
	return a
}

func (*Adapter) Type() string { return channels.TypeGoogleChat }
func (*Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{Threading: true, MaxTextLength: MaxText}
}
func (*Adapter) Fields() []channels.Field {
	return channelhttp.Fields(FieldServiceAccount+" "+FieldProjectNumber, "", FieldServiceAccount)
}

// serviceAccount is the part of a Google service account key file we use.
// token_uri is ignored: tokens are always minted at Config.TokenURL, so a
// pasted file cannot point the API at another host.
type serviceAccount struct {
	Type         string `json:"type"`
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	PrivateKeyID string `json:"private_key_id"`
	ProjectID    string `json:"project_id"`
}

func parseServiceAccount(inst channels.Instance) (serviceAccount, *rsa.PrivateKey, error) {
	if err := channelhttp.Required(inst, FieldServiceAccount, FieldProjectNumber); err != nil {
		return serviceAccount{}, nil, err
	}
	if !projectNumberRe.MatchString(inst.Credentials[FieldProjectNumber]) {
		return serviceAccount{}, nil, errors.New("google chat project number must be digits")
	}
	var sa serviceAccount
	if json.Unmarshal([]byte(inst.Credentials[FieldServiceAccount]), &sa) != nil {
		return serviceAccount{}, nil, errors.New("google chat service account key is not JSON")
	}
	if sa.Type != "service_account" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return serviceAccount{}, nil, errors.New("google chat service account key is incomplete")
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(sa.PrivateKey))
	if err != nil {
		return serviceAccount{}, nil, errors.New("google chat service account private key is invalid")
	}
	return sa, key, nil
}

// token returns a cached or freshly minted chat.bot access token.
func (a *Adapter) token(ctx context.Context, inst channels.Instance) (string, error) {
	sa, key, err := parseServiceAccount(inst)
	if err != nil {
		return "", err
	}
	// Keyed by the key material, so a rotated key never reuses an old token.
	sum := sha256.Sum256([]byte(sa.ClientEmail + "\n" + sa.PrivateKeyID + "\n" + sa.PrivateKey))
	cacheKey := hex.EncodeToString(sum[:])
	a.tokMu.Lock()
	if t, ok := a.tokens[cacheKey]; ok && time.Now().Add(tokenSkew).Before(t.expires) {
		a.tokMu.Unlock()
		return t.token, nil
	}
	a.tokMu.Unlock()

	now := time.Now()
	assertion := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": sa.ClientEmail, "scope": Scope, "aud": a.cfg.TokenURL,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	if sa.PrivateKeyID != "" {
		assertion.Header["kid"] = sa.PrivateKeyID
	}
	signed, err := assertion.SignedString(key)
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {signed}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		Token     string `json:"access_token"`
		Type      string `json:"token_type"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if err := channelhttp.Do(a.cfg.Client, req, &out); err != nil {
		return "", err
	}
	if out.Token == "" || !strings.EqualFold(out.Type, "Bearer") {
		return "", errors.New("google chat access token missing")
	}
	ttl := time.Duration(out.ExpiresIn) * time.Second
	if ttl <= 0 || ttl > time.Hour {
		ttl = time.Hour
	}
	a.tokMu.Lock()
	if len(a.tokens) >= maxCachedTokens {
		a.tokens = map[string]accessToken{}
	}
	a.tokens[cacheKey] = accessToken{token: out.Token, expires: now.Add(ttl)}
	a.tokMu.Unlock()
	return out.Token, nil
}

// api calls the Chat API as the app. A 409 answers alreadyExists=true with
// no error (a retried create whose first attempt went through).
func (a *Adapter) api(ctx context.Context, inst channels.Instance, method, path string, payload, out any) (alreadyExists bool, err error) {
	token, err := a.token(ctx, inst)
	if err != nil {
		return false, err
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return false, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.cfg.BaseURL, "/")+path, body)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	sc := &statusClient{c: a.cfg.Client}
	err = channelhttp.Do(sc, req, out)
	if sc.status == http.StatusConflict {
		return true, nil
	}
	return false, err
}

// statusClient records the response status channelhttp.Do folds into errors.
type statusClient struct {
	c      channelhttp.Client
	status int
}

func (s *statusClient) Do(r *http.Request) (*http.Response, error) {
	res, err := s.c.Do(r)
	if err == nil {
		s.status = res.StatusCode
	}
	return res, err
}

// Connect mints an access token (proves the key works) and returns the
// project number webhooks are verified against.
func (a *Adapter) Connect(ctx context.Context, inst channels.Instance) (string, error) {
	if _, err := a.token(ctx, inst); err != nil {
		return "", err
	}
	return inst.Credentials[FieldProjectNumber], nil
}

func (a *Adapter) CheckHealth(ctx context.Context, inst channels.Instance) (channels.Health, error) {
	_, err := a.Connect(ctx, inst)
	return channels.Health{OK: err == nil}, err
}

// key returns Google Chat's signing key kid, refreshing the x509 cert set
// hourly (or on an unknown kid) at most once a minute.
func (a *Adapter) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, known := a.certs[kid]
	if time.Now().After(a.refresh) || !known {
		// Bound refresh attempts, including failures, on this public webhook path.
		if time.Since(a.lastFetch) < certBackoff {
			if k, ok := a.certs[kid]; ok {
				return k, nil
			}
			return nil, channels.ErrSignature
		}
		a.lastFetch = time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.CertsURL, nil)
		if err != nil {
			return nil, err
		}
		var doc map[string]string
		if err := channelhttp.Do(a.cfg.Client, req, &doc); err != nil {
			return nil, err
		}
		certs := map[string]*rsa.PublicKey{}
		for id, raw := range doc {
			block, _ := pem.Decode([]byte(raw))
			if block == nil || block.Type != "CERTIFICATE" {
				continue
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				continue
			}
			if pub, ok := cert.PublicKey.(*rsa.PublicKey); ok && pub.N.BitLen() >= 2048 {
				certs[id] = pub
			}
		}
		a.certs = certs
		a.refresh = time.Now().Add(certTTL)
	}
	k, ok := a.certs[kid]
	if !ok {
		return nil, channels.ErrSignature
	}
	return k, nil
}

// VerifyWebhook checks the bearer JWT: RS256 by a current Google Chat key,
// issuer chat@system.gserviceaccount.com, audience the instance's project
// number, unexpired.
func (a *Adapter) VerifyWebhook(r *http.Request, _ []byte, inst channels.Instance) error {
	project := inst.Credentials[FieldProjectNumber]
	if !projectNumberRe.MatchString(project) {
		return channels.ErrSignature
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return channels.ErrSignature
	}
	_, err := jwt.ParseWithClaims(token, jwt.MapClaims{}, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return a.key(r.Context(), kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(Issuer),
		jwt.WithAudience(project),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(time.Minute))
	if err != nil {
		return channels.ErrSignature
	}
	return nil
}

// AcknowledgeWebhook answers an empty JSON object: Google Chat reads the
// body as a synchronous reply, and replies go out asynchronously instead.
func (*Adapter) AcknowledgeWebhook(*http.Request, []byte, channels.Instance) (string, string) {
	return "application/json", "{}"
}

type user struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Type        string `json:"type"`
}

type space struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	SpaceType       string `json:"spaceType"`
	DisplayName     string `json:"displayName"`
	SingleUserBotDM bool   `json:"singleUserBotDm"`
}

// direct reports a one-to-one conversation with the app.
func (s space) direct() bool {
	return s.SpaceType == "DIRECT_MESSAGE" || s.Type == "DM" || s.SingleUserBotDM
}

type event struct {
	Type      string `json:"type"`
	EventTime string `json:"eventTime"`
	Space     space  `json:"space"`
	User      user   `json:"user"`
	Message   *struct {
		Name         string `json:"name"`
		Text         string `json:"text"`
		ArgumentText string `json:"argumentText"`
		CreateTime   string `json:"createTime"`
		Sender       user   `json:"sender"`
		Thread       struct {
			Name string `json:"name"`
		} `json:"thread"`
		Attachment []json.RawMessage `json:"attachment"`
	} `json:"message"`
}

// allowed reports whether a room is in the settings.space_ids allowlist.
// Direct messages are always allowed: a user starts them with the app.
func allowed(inst channels.Instance, name string) bool {
	switch ids := inst.Settings[SettingSpaceIDs].(type) {
	case []any:
		for _, v := range ids {
			if v == name {
				return true
			}
		}
	case []string:
		for _, v := range ids {
			if v == name {
				return true
			}
		}
	}
	return false
}

// ParseWebhook normalizes MESSAGE (and ADDED_TO_SPACE carrying the message
// that added the app) into inbound messages. ADDED_TO_SPACE caches the
// space name; REMOVED_FROM_SPACE and other events carry nothing to store.
func (a *Adapter) ParseWebhook(ctx context.Context, _ *http.Request, b []byte, inst channels.Instance) (channels.WebhookResult, error) {
	var e event
	if err := json.Unmarshal(b, &e); err != nil {
		return channels.WebhookResult{}, err
	}
	if !spaceRe.MatchString(e.Space.Name) {
		return channels.WebhookResult{}, errors.New("invalid Google Chat space")
	}
	switch e.Type {
	case "ADDED_TO_SPACE":
		a.remember(ctx, inst, e.Space.Name, e.Space.DisplayName)
		if e.Message == nil {
			return channels.WebhookResult{}, nil
		}
	case "MESSAGE":
	default:
		return channels.WebhookResult{}, nil
	}
	m := e.Message
	if m == nil || m.Sender.Type == "BOT" || e.User.Type == "BOT" {
		return channels.WebhookResult{}, nil
	}
	direct := e.Space.direct()
	if !direct && !allowed(inst, e.Space.Name) {
		return channels.WebhookResult{}, nil
	}
	if !strings.HasPrefix(m.Name, e.Space.Name+"/messages/") {
		return channels.WebhookResult{}, errors.New("invalid Google Chat message")
	}
	author := m.Sender
	if author.Name == "" {
		author = e.User
	}
	text := strings.TrimSpace(m.ArgumentText)
	if text == "" {
		text = strings.TrimSpace(m.Text)
	}
	kind := "text"
	if text == "" {
		if len(m.Attachment) == 0 {
			return channels.WebhookResult{}, nil
		}
		kind = "unsupported"
	}
	authorName := a.remember(ctx, inst, author.Name, author.DisplayName)
	out := channels.InboundMessage{
		ExternalID: m.Name,
		Kind:       kind,
		Text:       text,
		At:         chatTime(m.CreateTime, e.EventTime),
		Attributes: map[string]any{"author_id": author.Name, "google_chat_space": e.Space.Name},
	}
	if authorName != "" {
		out.Attributes["author_name"] = authorName
	}
	if author.Email != "" {
		out.Attributes["author_email"] = author.Email
	}
	if direct {
		// One conversation per direct message space; the user is the contact.
		out.From = channels.Sender{ExternalID: author.Name, InstanceScoped: true, Name: authorName, Email: author.Email}
		out.ThreadID = e.Space.Name
	} else {
		thread := m.Thread.Name
		if !strings.HasPrefix(thread, e.Space.Name+"/threads/") || !threadRe.MatchString(thread) {
			return channels.WebhookResult{}, errors.New("invalid Google Chat thread")
		}
		spaceName := a.spaceName(ctx, inst, e.Space.Name, e.Space.DisplayName)
		out.From = channels.Sender{ExternalID: e.Space.Name, InstanceScoped: true, Name: spaceName}
		out.ThreadID = thread
	}
	return channels.WebhookResult{Messages: []channels.InboundMessage{out}}, nil
}

// Send replies in the conversation's space; room replies go into the
// conversation's thread. The message id makes a retried send idempotent.
func (a *Adapter) Send(ctx context.Context, inst channels.Instance, msg channels.OutboundMessage) (channels.SendResult, error) {
	spaceName, thread, err := splitThread(msg.ThreadID)
	if err != nil {
		return channels.SendResult{}, channels.Permanent("thread", err)
	}
	if thread != "" && !allowed(inst, spaceName) {
		return channels.SendResult{}, channels.Permanent("thread", errors.New("google chat space is not selected"))
	}
	// Markdown, named explicitly in the request: without markupSyntax Chat
	// uses the app's configured syntax, which differs between old and new
	// apps (CF-177). The limit applies to the text that is sent.
	text := format.GoogleChat(msg.Text)
	if len(msg.Attachments) > 0 || text == "" || len([]rune(text)) > MaxText {
		return channels.SendResult{}, channels.Permanent("content",
			fmt.Errorf("google chat adapter accepts up to %d text characters", MaxText))
	}
	messageID := "client-" + msg.MessageUUID.String()
	q := url.Values{"messageId": {messageID}}
	payload := map[string]any{"text": text, "markupSyntax": "MARKUP_SYNTAX_MARKDOWN"}
	if thread != "" {
		q.Set("messageReplyOption", "REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD")
		payload["thread"] = map[string]string{"name": thread}
	}
	var out struct {
		Name string `json:"name"`
	}
	exists, err := a.api(ctx, inst, http.MethodPost, "/v1/"+spaceName+"/messages?"+q.Encode(), payload, &out)
	if channelhttp.BadRequest(err) {
		// The formatted message was refused (400): the message must still arrive,
		// so send it once more as plain text in the app's default syntax.
		if plain := format.Plain(msg.Text); plain != "" {
			delete(payload, "markupSyntax")
			payload["text"] = plain
			exists, err = a.api(ctx, inst, http.MethodPost, "/v1/"+spaceName+"/messages?"+q.Encode(), payload, &out)
		}
	}
	if err != nil {
		return channels.SendResult{}, err
	}
	if exists {
		// The first attempt went through; read back its resource name.
		if _, err := a.api(ctx, inst, http.MethodGet, "/v1/"+spaceName+"/messages/"+messageID, nil, &out); err != nil {
			return channels.SendResult{}, err
		}
	}
	if !strings.HasPrefix(out.Name, spaceName+"/messages/") {
		return channels.SendResult{}, errors.New("google chat response missing message name")
	}
	return channels.SendResult{ExternalID: out.Name}, nil
}

// splitThread reads a conversation thread id: "spaces/X" (direct message)
// or "spaces/X/threads/Y" (room thread).
func splitThread(id string) (spaceName, thread string, err error) {
	if spaceRe.MatchString(id) {
		return id, "", nil
	}
	if m := threadRe.FindStringSubmatch(id); m != nil {
		return m[1], id, nil
	}
	return "", "", errors.New("valid Google Chat space or thread required")
}

// Space is a room the app is a member of, offered for settings.space_ids.
type Space struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	SpaceType   string `json:"space_type"`
}

// Spaces lists the rooms (not direct messages) the app was added to.
func (a *Adapter) Spaces(ctx context.Context, inst channels.Instance) ([]Space, error) {
	out := []Space{}
	pageToken := ""
	for range maxSpacePages {
		q := url.Values{"pageSize": {"200"}}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		var page struct {
			Spaces        []space `json:"spaces"`
			NextPageToken string  `json:"nextPageToken"`
		}
		if _, err := a.api(ctx, inst, http.MethodGet, "/v1/spaces?"+q.Encode(), nil, &page); err != nil {
			return nil, err
		}
		for _, s := range page.Spaces {
			if s.direct() || !spaceRe.MatchString(s.Name) {
				continue
			}
			a.remember(ctx, inst, s.Name, s.DisplayName)
			out = append(out, Space{Name: s.Name, DisplayName: s.DisplayName, SpaceType: s.SpaceType})
		}
		if pageToken = page.NextPageToken; pageToken == "" {
			break
		}
	}
	return out, nil
}

// nameKey scopes cached names by organization and project.
func (*Adapter) nameKey(inst channels.Instance, id string) string {
	return strconv.FormatInt(inst.OrganizationID, 10) + ":" + inst.Credentials[FieldProjectNumber] + ":" + id
}

// remember caches a name Google sent, or returns the cached one when the
// event left it out.
func (a *Adapter) remember(ctx context.Context, inst channels.Instance, id, name string) string {
	if id == "" {
		return strings.TrimSpace(name)
	}
	key := a.nameKey(inst, id)
	if name = strings.TrimSpace(name); name != "" {
		a.names.SetName(ctx, key, name, nameTTL)
		return name
	}
	cached, _ := a.names.GetName(ctx, key)
	return cached
}

// spaceName returns the room's display name from the event, the cache or
// spaces.get; a failed lookup yields "" and never blocks ingestion.
func (a *Adapter) spaceName(ctx context.Context, inst channels.Instance, id, name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return a.remember(ctx, inst, id, name)
	}
	key := a.nameKey(inst, id)
	if cached, ok := a.names.GetName(ctx, key); ok {
		return cached
	}
	lctx, cancel := context.WithTimeout(ctx, nameLookupTimeout)
	defer cancel()
	var s space
	if _, err := a.api(lctx, inst, http.MethodGet, "/v1/"+id, nil, &s); err != nil {
		a.names.SetName(ctx, key, "", missTTL)
		return ""
	}
	return a.remember(ctx, inst, id, s.DisplayName)
}

func chatTime(values ...string) time.Time {
	for _, v := range values {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

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

var (
	_ channels.Adapter             = (*Adapter)(nil)
	_ channels.HealthChecker       = (*Adapter)(nil)
	_ channels.WebhookAcknowledger = (*Adapter)(nil)
)
