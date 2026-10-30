// Package whatsapp is the WhatsApp Cloud API channel (CF-57), ported from
// Takwim's meta adapter (platform/whatsapp/meta) onto the channels contract.
//
// ClickFix is a Meta Tech Provider: one Meta app (Config, from env) serves
// every organization. A business connects its number with Embedded Signup
// (ExchangeCode → PhoneNumbers → SubscribeApp → RegisterNumber); the
// instance then carries phone_number_id, waba_id and the business token.
// Meta calls one app-wide webhook (/v1/webhooks/channels/whatsapp): Route
// names the phone number ids of a body, VerifyWebhook checks
// X-Hub-Signature-256 with the app secret and ParseWebhook keeps only the
// changes of the instance's number.
//
// Decisions:
//   - Typing is false: the Cloud API typing indicator is tied to an inbound
//     message id (it also marks it read), which channels.Recipient does not
//     carry. MarkRead exposes it for callers that know the message id.
//   - Reactions arrive as Kind "text" with the emoji as text and attributes
//     {wa_type: reaction, reaction, reacted_message_id}; locations and
//     contact cards as text summaries with structured attributes.
//   - Outbound media is sent by link (OutboundAttachment.URL); uploading via
//     Open is not used. Audio cannot carry a caption, so its text goes first
//     as a separate message. With several attachments the last id is
//     returned.
//   - Error mapping: 131047 → ErrWindowClosed (permanent), 190/401 →
//     permanent "token_invalid", 429/130429/131056/80007/4 and 5xx/network →
//     temporary; every other 4xx is permanent with the Meta code as Code.
//   - Template paging follows paging.cursors.after against BaseURL (never
//     the absolute paging.next URL) so the token only goes to Graph.
//   - TemplateVariables returns -1 when placeholders are not exactly 1..n.
//   - Template headers are text only and take no variables.
package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

// Defaults.
const (
	DefaultBaseURL    = "https://graph.facebook.com"
	DefaultAPIVersion = "v21.0"
	DefaultLanguage   = "tr"
)

// Credential fields of an instance.
const (
	FieldPhoneNumberID = "phone_number_id"
	FieldWABAID        = "waba_id"
	FieldAccessToken   = "access_token"
)

// SignatureHeader carries the webhook HMAC.
const SignatureHeader = "X-Hub-Signature-256"

// MaxMediaSize caps one inbound media download.
const MaxMediaSize = 25 << 20

// Config is the Meta app shared by every organization (env).
type Config struct {
	AppID       string
	AppSecret   string
	VerifyToken string
	// ConfigID is the Embedded Signup configuration id (FB.login).
	ConfigID   string
	BaseURL    string
	APIVersion string
	HTTPClient *http.Client
}

// Adapter is the WhatsApp Cloud API channel.
type Adapter struct {
	cfg    Config
	client *http.Client
	media  *http.Client
}

var (
	_ channels.Adapter          = (*Adapter)(nil)
	_ channels.Challenger       = (*Adapter)(nil)
	_ channels.SharedChallenger = (*Adapter)(nil)
	_ channels.Router           = (*Adapter)(nil)
	_ channels.HealthChecker    = (*Adapter)(nil)
)

// New builds the adapter (empty values take the defaults).
func New(cfg Config) *Adapter {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = DefaultAPIVersion
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	cfg.APIVersion = strings.Trim(cfg.APIVersion, "/")
	a := &Adapter{cfg: cfg, client: cfg.HTTPClient, media: cfg.HTTPClient}
	if a.client == nil {
		a.client = &http.Client{Timeout: 20 * time.Second}
		a.media = &http.Client{Timeout: 2 * time.Minute}
	}
	return a
}

// Enabled reports whether the Meta app is configured.
func (a *Adapter) Enabled() bool {
	return a.cfg.AppID != "" && a.cfg.AppSecret != "" && a.cfg.VerifyToken != ""
}

// Type implements channels.Adapter.
func (a *Adapter) Type() string { return channels.TypeWhatsApp }

// Capabilities implements channels.Adapter.
func (a *Adapter) Capabilities() channels.Capabilities {
	return channels.Capabilities{
		Media: true, Templates: true, Typing: false, Reactions: true,
		DeliveryReceipts: true, ReadReceipts: true, ReplyWindow: 24 * time.Hour, MaxTextLength: 4096,
	}
}

// Fields implements channels.Adapter.
func (a *Adapter) Fields() []channels.Field {
	return []channels.Field{
		{Key: FieldPhoneNumberID, LabelKey: "channels.fields.phone_number_id", Required: true},
		{Key: FieldWABAID, LabelKey: "channels.fields.waba_id", Required: true},
		{Key: FieldAccessToken, LabelKey: "channels.fields.access_token", Secret: true, Required: true},
	}
}

var graphIDRE = regexp.MustCompile(`^[0-9]{5,32}$`)

// ValidID reports whether v looks like a Graph id (digits).
func ValidID(v string) bool { return graphIDRE.MatchString(v) }

// Connect implements channels.Adapter: reads the number with the token (no
// message is sent) and answers the phone number id webhooks route by.
func (a *Adapter) Connect(ctx context.Context, inst channels.Instance) (string, error) {
	if _, err := a.phoneNumber(ctx, inst); err != nil {
		return "", fmt.Errorf("whatsapp: connect: %w", err)
	}
	return strings.TrimSpace(inst.Credentials[FieldPhoneNumberID]), nil
}

// PhoneNumber is one number of a WABA.
type PhoneNumber struct {
	ID                 string `json:"id"`
	DisplayPhoneNumber string `json:"display_phone_number"`
	VerifiedName       string `json:"verified_name"`
	QualityRating      string `json:"quality_rating,omitempty"`
}

func (a *Adapter) phoneNumber(ctx context.Context, inst channels.Instance) (PhoneNumber, error) {
	c := inst.Credentials
	id := strings.TrimSpace(c[FieldPhoneNumberID])
	switch {
	case !ValidID(id):
		return PhoneNumber{}, errors.New("phone_number_id must be the numeric id of the number")
	case !ValidID(strings.TrimSpace(c[FieldWABAID])):
		return PhoneNumber{}, errors.New("waba_id must be the numeric id of the WhatsApp Business Account")
	case strings.TrimSpace(c[FieldAccessToken]) == "":
		return PhoneNumber{}, errors.New("access_token is required")
	}
	q := url.Values{"fields": {"display_phone_number,verified_name,quality_rating"}}
	body, err := a.call(ctx, http.MethodGet, id, q, nil, c[FieldAccessToken])
	if err != nil {
		return PhoneNumber{}, err
	}
	var out PhoneNumber
	if err := json.Unmarshal(body, &out); err != nil {
		return PhoneNumber{}, &Error{Status: http.StatusOK, Message: "unreadable phone number"}
	}
	return out, nil
}

// CheckHealth implements channels.HealthChecker: the number must still be
// readable; debug_token (with the app token) tells when the token expires.
func (a *Adapter) CheckHealth(ctx context.Context, inst channels.Instance) (channels.Health, error) {
	if _, err := a.phoneNumber(ctx, inst); err != nil {
		if ctx.Err() != nil {
			return channels.Health{}, ctx.Err()
		}
		return channels.Health{OK: false, Detail: err.Error()}, nil
	}
	h := channels.Health{OK: true}
	if a.cfg.AppID == "" || a.cfg.AppSecret == "" {
		return h, nil
	}
	q := url.Values{
		"input_token":  {inst.Credentials[FieldAccessToken]},
		"access_token": {a.cfg.AppID + "|" + a.cfg.AppSecret},
	}
	body, err := a.call(ctx, http.MethodGet, "debug_token", q, nil, "")
	if err != nil {
		return h, nil // expiry unknown; the number check passed
	}
	var dt struct {
		Data struct {
			IsValid   *bool `json:"is_valid"`
			ExpiresAt int64 `json:"expires_at"`
			Error     struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &dt) != nil {
		return h, nil
	}
	if dt.Data.IsValid != nil && !*dt.Data.IsValid {
		h.OK, h.Detail = false, firstNonEmpty(dt.Data.Error.Message, "access token is not valid")
	}
	if dt.Data.ExpiresAt > 0 {
		t := time.Unix(dt.Data.ExpiresAt, 0).UTC()
		h.TokenExpiresAt = &t
	}
	return h, nil
}

// Sign is the X-Hub-Signature-256 value of body (tests, fakes).
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhook implements channels.Adapter: X-Hub-Signature-256 over the
// raw body with the app secret.
func (a *Adapter) VerifyWebhook(r *http.Request, body []byte, _ channels.Instance) error {
	sig, ok := strings.CutPrefix(strings.TrimSpace(r.Header.Get(SignatureHeader)), "sha256=")
	if a.cfg.AppSecret == "" || !ok {
		return channels.ErrSignature
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return channels.ErrSignature
	}
	mac := hmac.New(sha256.New, []byte(a.cfg.AppSecret))
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), got) {
		return channels.ErrSignature
	}
	return nil
}

// ChallengeShared implements channels.SharedChallenger (hub.mode=subscribe
// with the app's verify token).
func (a *Adapter) ChallengeShared(r *http.Request) (string, bool) {
	q := r.URL.Query()
	want, token, challenge := a.cfg.VerifyToken, q.Get("hub.verify_token"), q.Get("hub.challenge")
	if q.Get("hub.mode") != "subscribe" || want == "" || challenge == "" ||
		subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
		return "", false
	}
	return challenge, true
}

// Challenge implements channels.Challenger for the per-instance URL; the
// verify token is the app's either way.
func (a *Adapter) Challenge(r *http.Request, _ channels.Instance) (string, bool) {
	return a.ChallengeShared(r)
}

// Route implements channels.Router: the phone number ids a body names.
func (a *Adapter) Route(_ *http.Request, body []byte) ([]string, error) {
	w, err := decodeWebhook(body)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, e := range w.Entry {
		for _, c := range e.Changes {
			id := c.Value.Metadata.PhoneNumberID
			if id != "" && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out, nil
}

// Error is an error answer of the Graph API.
type Error struct {
	Status  int
	Code    int
	Subcode int
	Message string
}

func (e *Error) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("whatsapp: %d %s", e.Code, e.Message)
	}
	return fmt.Sprintf("whatsapp: http %d %s", e.Status, e.Message)
}

// call sends one Graph request (path relative to the API version) with an
// optional JSON body and bearer token.
func (a *Adapter) call(ctx context.Context, method, path string, q url.Values, body any, token string) ([]byte, error) {
	u := a.cfg.BaseURL + "/" + a.cfg.APIVersion + "/" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("whatsapp: read answer: %w", err)
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return raw, nil
	}
	var e struct {
		Error struct {
			Message      string `json:"message"`
			Code         int    `json:"code"`
			Subcode      int    `json:"error_subcode"`
			ErrorUserMsg string `json:"error_user_msg"`
			ErrorData    struct {
				Details string `json:"details"`
			} `json:"error_data"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	msg := firstNonEmpty(e.Error.Message, http.StatusText(res.StatusCode))
	if d := firstNonEmpty(e.Error.ErrorData.Details, e.Error.ErrorUserMsg); d != "" && d != msg {
		msg += ": " + d
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return nil, &Error{Status: res.StatusCode, Code: e.Error.Code, Subcode: e.Error.Subcode, Message: msg}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// digits turns a phone number or wa_id into the Cloud API's form.
func digits(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
