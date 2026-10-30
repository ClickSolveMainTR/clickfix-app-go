// Package metagraph is the Meta Graph API plumbing shared by the Instagram
// (CF-58) and Messenger (CF-59) channel adapters, ported from takwim's
// platform/instagram and platform/messenger clients: a small JSON client,
// the Graph error type and its mapping onto channels.SendError, the
// X-Hub-Signature-256 and hub.challenge helpers, the common webhook
// envelope (object / entry[] / messaging[] / changes[]), attachment
// downloads capped at 25 MB and the conversations listing the Reconcilers
// use (garanti-kulucka apps/worker/src/meta-reconcile.ts).
//
// Decisions:
//   - Graph ids arrive as numbers or strings; FlexID reads both.
//   - Error classification looks at the Graph code before the HTTP status,
//     because Meta answers rate limits and window errors with 400.
//   - Reply window errors (code 10, subcodes 2018278 Messenger / 2534022
//     Instagram) are permanent and wrap channels.ErrWindowClosed.
//   - Downloads only accept http(s) URLs taken from signed webhooks or Graph
//     answers; no token is sent (Meta CDN URLs are pre-signed).
//   - Conversation paging only follows paging.next on the client's own host
//     so a token never leaves Graph.
package metagraph

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
	"mime"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

// Defaults.
const (
	DefaultVersion = "v21.0"
	// SignatureHeader carries the app-secret HMAC of a webhook body.
	SignatureHeader = "X-Hub-Signature-256"
	// MaxAttachmentBytes caps an inbound attachment download.
	MaxAttachmentBytes int64 = 25 << 20
	maxAnswerBytes           = 1 << 20
)

// Client calls one Graph host. Versioned paths are BaseURL/Version/path.
type Client struct {
	BaseURL string
	Version string
	HTTP    *http.Client
}

// NewClient builds a client (empty version → DefaultVersion, nil client →
// 20s timeout).
func NewClient(baseURL, version string, hc *http.Client) *Client {
	if version == "" {
		version = DefaultVersion
	}
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Version: strings.Trim(version, "/"), HTTP: hc}
}

// URL is the versioned URL of a Graph path.
func (c *Client) URL(p string, q url.Values) string {
	u := c.BaseURL + "/" + c.Version + "/" + strings.TrimLeft(p, "/")
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// Get calls GET path and decodes the answer into out (nil = discard).
func (c *Client) Get(ctx context.Context, p string, q url.Values, token string, out any) error {
	return c.Do(ctx, http.MethodGet, c.URL(p, q), token, nil, out)
}

// Post calls POST path with a JSON body (nil = no body).
func (c *Client) Post(ctx context.Context, p string, q url.Values, token string, body any, out any) error {
	return c.Do(ctx, http.MethodPost, c.URL(p, q), token, body, out)
}

// Do sends one request. body is nil, url.Values (form) or JSON-encoded;
// a non-empty token goes in the Authorization header.
func (c *Client) Do(ctx context.Context, method, rawURL, token string, body any, out any) error {
	var rd io.Reader
	ctype := ""
	switch b := body.(type) {
	case nil:
	case url.Values:
		rd, ctype = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			return err
		}
		rd, ctype = bytes.NewReader(raw), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rd)
	if err != nil {
		return err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("metagraph: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxAnswerBytes))
	if err != nil {
		return fmt.Errorf("metagraph: read answer: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return parseError(res.StatusCode, raw)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &Error{Status: res.StatusCode, Message: "unreadable answer"}
	}
	return nil
}

// Error is an error answer of Graph (or of the Instagram OAuth host, which
// answers {error_type, code, error_message}).
type Error struct {
	Status  int
	Code    int
	Subcode int
	Type    string
	Message string
	TraceID string
}

func (e *Error) Error() string {
	if e.Code != 0 {
		if e.Subcode != 0 {
			return fmt.Sprintf("metagraph: graph error %d/%d (%s): %s", e.Code, e.Subcode, e.Type, e.Message)
		}
		return fmt.Sprintf("metagraph: graph error %d (%s): %s", e.Code, e.Type, e.Message)
	}
	return fmt.Sprintf("metagraph: http %d: %s", e.Status, e.Message)
}

func parseError(status int, raw []byte) *Error {
	var w struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
			Subcode int    `json:"error_subcode"`
			TraceID string `json:"fbtrace_id"`
		} `json:"error"`
		ErrorType    string `json:"error_type"`
		Code         int    `json:"code"`
		ErrorMessage string `json:"error_message"`
	}
	_ = json.Unmarshal(raw, &w)
	e := &Error{Status: status}
	if w.Error != nil {
		e.Code, e.Subcode, e.Type, e.Message, e.TraceID = w.Error.Code, w.Error.Subcode, w.Error.Type, w.Error.Message, w.Error.TraceID
	} else if w.ErrorMessage != "" {
		e.Type, e.Message = w.ErrorType, w.ErrorMessage
	}
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	if len(e.Message) > 300 {
		e.Message = e.Message[:300]
	}
	return e
}

// AsError unwraps a Graph error.
func AsError(err error) (*Error, bool) {
	var ge *Error
	ok := errors.As(err, &ge)
	return ge, ok
}

// IsTokenError reports an expired, revoked or invalid access token.
func IsTokenError(err error) bool {
	ge, ok := AsError(err)
	return ok && (ge.Code == 190 || ge.Code == 102)
}

// IsRateLimit reports an application, user or page rate limit.
func IsRateLimit(err error) bool {
	ge, ok := AsError(err)
	if !ok {
		return false
	}
	switch ge.Code {
	case 4, 17, 32, 613:
		return true
	}
	return ge.Code >= 80001 && ge.Code <= 80014 // business use case limits
}

// IsWindowError reports a send outside the messaging window.
func IsWindowError(err error) bool {
	ge, ok := AsError(err)
	if !ok || ge.Code != 10 {
		return false
	}
	return ge.Subcode == 2018278 || ge.Subcode == 2534022 || strings.Contains(strings.ToLower(ge.Message), "window")
}

// Unhealthy reports whether a failed health call means the connection is
// broken (token, permission, wrong account) rather than a passing outage
// (network, 5xx, rate limit) worth checking again later.
func Unhealthy(err error) bool {
	if err == nil {
		return false
	}
	var ue *url.Error
	if errors.As(err, &ue) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	ge, ok := AsError(err)
	if !ok {
		return true
	}
	return IsTokenError(err) || (ge.Status < 500 && !IsRateLimit(err) && ge.Code != 1 && ge.Code != 2)
}

// SendError maps a send failure onto the outbound pipeline's retry rules:
// window → permanent ErrWindowClosed, token → permanent token_invalid, rate
// limits, 5xx, Graph "temporary" codes and network errors → temporary,
// every other Graph error → permanent with its code.
func SendError(err error) error {
	if err == nil {
		return nil
	}
	var se *channels.SendError
	if errors.As(err, &se) {
		return err
	}
	ge, ok := AsError(err)
	switch {
	case !ok:
		return &channels.SendError{Code: "network", Err: err}
	case IsWindowError(err):
		return channels.Permanent("window_closed", fmt.Errorf("%w: %w", channels.ErrWindowClosed, err))
	case IsTokenError(err):
		return channels.Permanent("token_invalid", err)
	case IsRateLimit(err):
		return &channels.SendError{Code: "rate_limited", Err: err}
	case ge.Status >= 500 || ge.Code == 1 || ge.Code == 2:
		return &channels.SendError{Code: "unavailable", Err: err}
	case ge.Code != 0:
		return channels.Permanent(strconv.Itoa(ge.Code), err)
	default:
		return channels.Permanent("http_"+strconv.Itoa(ge.Status), err)
	}
}

// Sign returns the X-Hub-Signature-256 value of body.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks an X-Hub-Signature-256 header in constant time.
func VerifySignature(secret, header string, body []byte) bool {
	if secret == "" || header == "" {
		return false
	}
	return hmac.Equal([]byte(strings.TrimSpace(header)), []byte(Sign(secret, body)))
}

// Challenge answers Meta's subscription GET (hub.mode=subscribe,
// hub.verify_token compared in constant time, hub.challenge echoed).
func Challenge(r *http.Request, verifyToken string) (string, bool) {
	q := r.URL.Query()
	got, challenge := q.Get("hub.verify_token"), q.Get("hub.challenge")
	if verifyToken == "" || q.Get("hub.mode") != "subscribe" || challenge == "" {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(verifyToken)) != 1 {
		return "", false
	}
	return challenge, true
}

// ErrTooLarge: an attachment is over the download cap.
var ErrTooLarge = errors.New("metagraph: attachment too large")

// Download opens an attachment URL, failing past max bytes.
func Download(ctx context.Context, hc *http.Client, rawURL string, max int64) (io.ReadCloser, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("metagraph: invalid attachment url")
	}
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("metagraph: download: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		_ = res.Body.Close()
		return nil, &Error{Status: res.StatusCode, Message: "attachment download failed"}
	}
	if res.ContentLength > max {
		_ = res.Body.Close()
		return nil, ErrTooLarge
	}
	return &capReader{rc: res.Body, left: max}, nil
}

type capReader struct {
	rc   io.ReadCloser
	left int64
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.left < 0 {
		return 0, ErrTooLarge
	}
	if int64(len(p)) > c.left+1 {
		p = p[:c.left+1]
	}
	n, err := c.rc.Read(p)
	c.left -= int64(n)
	if c.left < 0 {
		return n, ErrTooLarge
	}
	return n, err
}

func (c *capReader) Close() error { return c.rc.Close() }

// Attachment builds an inbound attachment whose Open downloads rawURL.
// kind is the Meta attachment type (image, video, audio, file …).
func Attachment(hc *http.Client, kind, rawURL, name, contentType string) channels.InboundAttachment {
	if name == "" {
		if u, err := url.Parse(rawURL); err == nil {
			if b := path.Base(u.Path); b != "." && b != "/" && strings.Contains(b, ".") && len(b) <= 120 {
				name = b
			}
		}
	}
	if contentType == "" && name != "" {
		contentType, _, _ = strings.Cut(mime.TypeByExtension(path.Ext(name)), ";")
	}
	if contentType == "" {
		contentType = map[string]string{"image": "image/jpeg", "video": "video/mp4", "audio": "audio/mp4"}[kind]
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}
	if name == "" {
		name = kind
		if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
			name += exts[0]
		}
	}
	return channels.InboundAttachment{
		Filename: name, ContentType: contentType,
		Open: func(ctx context.Context) (io.ReadCloser, error) {
			return Download(ctx, hc, rawURL, MaxAttachmentBytes)
		},
	}
}

// OutboundType is the Send API attachment type of a content type.
func OutboundType(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	case strings.HasPrefix(contentType, "audio/"):
		return "audio"
	}
	return "file"
}

// Truthy reads a boolean attribute (true, "true", "1").
func Truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(t)
		return b
	}
	return false
}

// ClipRunes keeps text within n characters.
func ClipRunes(text string, n int) string {
	if n <= 0 || utf8.RuneCountInString(text) <= n {
		return text
	}
	return string([]rune(text)[:n])
}

// ClipBytes keeps text within n bytes on a rune boundary.
func ClipBytes(text string, n int) string {
	if n <= 0 || len(text) <= n {
		return text
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

// FlexID is a Graph id sent as a JSON number or string.
type FlexID string

// UnmarshalJSON implements json.Unmarshaler.
func (f *FlexID) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "null" {
		s = ""
	}
	*f = FlexID(s)
	return nil
}

// ValidID reports whether v is a numeric Graph id (never a path).
func ValidID(v string) bool {
	if v == "" || len(v) > 40 {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Millis reads a webhook timestamp (milliseconds, or seconds when small).
func Millis(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	if v < 1e12 {
		return time.Unix(v, 0).UTC()
	}
	return time.UnixMilli(v).UTC()
}

// ParseTime reads a Graph time (2026-10-23T22:15:30+0000 or RFC 3339).
func ParseTime(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02T15:04:05-0700", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// --- webhook envelope -------------------------------------------------------

// ErrMalformed: the body is not a Meta webhook of the expected object.
var ErrMalformed = errors.New("metagraph: malformed webhook")

// Webhook is one Meta webhook body.
type Webhook struct {
	Object string  `json:"object"`
	Entry  []Entry `json:"entry"`
}

// Entry is the part of a webhook addressed to one account (entry.id).
type Entry struct {
	ID        FlexID           `json:"id"`
	Time      int64            `json:"time"`
	Messaging []MessagingEvent `json:"messaging"`
	Changes   []Change         `json:"changes"`
}

// Change is one changes[] item (comments, mentions, feed).
type Change struct {
	Field string          `json:"field"`
	Value json.RawMessage `json:"value"`
}

// MessagingEvent is one messaging[] item of Messenger or Instagram.
type MessagingEvent struct {
	Sender    struct{ ID FlexID } `json:"sender"`
	Recipient struct{ ID FlexID } `json:"recipient"`
	Timestamp int64               `json:"timestamp"`
	Message   *WireMessage        `json:"message"`
	Postback  *struct {
		Mid     string `json:"mid"`
		Title   string `json:"title"`
		Payload string `json:"payload"`
	} `json:"postback"`
	Delivery *struct {
		Mids      []string `json:"mids"`
		Watermark int64    `json:"watermark"`
	} `json:"delivery"`
	Read *struct {
		Mid       string `json:"mid"`
		Watermark int64  `json:"watermark"`
	} `json:"read"`
	Reaction *struct {
		Mid      string `json:"mid"`
		Action   string `json:"action"`
		Reaction string `json:"reaction"`
		Emoji    string `json:"emoji"`
	} `json:"reaction"`
}

// WireMessage is messaging[].message.
type WireMessage struct {
	Mid           string `json:"mid"`
	Text          string `json:"text"`
	IsEcho        bool   `json:"is_echo"`
	IsDeleted     bool   `json:"is_deleted"`
	IsUnsupported bool   `json:"is_unsupported"`
	QuickReply    *struct {
		Payload string `json:"payload"`
	} `json:"quick_reply"`
	ReplyTo *struct {
		Mid   string `json:"mid"`
		Story *struct {
			ID  FlexID `json:"id"`
			URL string `json:"url"`
		} `json:"story"`
	} `json:"reply_to"`
	Attachments []WireAttachment `json:"attachments"`
}

// WireAttachment is one message attachment.
type WireAttachment struct {
	Type    string `json:"type"`
	Payload struct {
		URL         string `json:"url"`
		Title       string `json:"title"`
		StickerID   FlexID `json:"sticker_id"`
		ReelVideoID FlexID `json:"reel_video_id"`
	} `json:"payload"`
}

// DecodeWebhook reads a body of the given object ("page", "instagram").
func DecodeWebhook(body []byte, object string) (Webhook, error) {
	var w Webhook
	if err := json.Unmarshal(body, &w); err != nil || w.Object != object {
		return Webhook{}, ErrMalformed
	}
	return w, nil
}

// EntryIDs lists the distinct account ids (entry[].id) of a body.
func EntryIDs(body []byte, object string) ([]string, error) {
	w, err := DecodeWebhook(body, object)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, e := range w.Entry {
		id := string(e.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

// --- conversations (reconcile) ----------------------------------------------

// ConvMessage is one message of the conversations API.
type ConvMessage struct {
	ID          string `json:"id"`
	CreatedTime string `json:"created_time"`
	From        struct {
		ID       FlexID `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"from"`
	Message     string `json:"message"`
	Attachments struct {
		Data []ConvAttachment `json:"data"`
	} `json:"attachments"`
	At time.Time `json:"-"`
}

// ConvAttachment is one attachment of a conversations API message.
type ConvAttachment struct {
	ID        string `json:"id"`
	MimeType  string `json:"mime_type"`
	Name      string `json:"name"`
	FileURL   string `json:"file_url"`
	ImageData *struct {
		URL string `json:"url"`
	} `json:"image_data"`
	VideoData *struct {
		URL string `json:"url"`
	} `json:"video_data"`
}

// Inbound turns a conversations API attachment into an inbound one (ok
// false when it has no downloadable URL).
func (a ConvAttachment) Inbound(hc *http.Client) (channels.InboundAttachment, bool) {
	switch {
	case a.ImageData != nil && a.ImageData.URL != "":
		return Attachment(hc, "image", a.ImageData.URL, a.Name, a.MimeType), true
	case a.VideoData != nil && a.VideoData.URL != "":
		return Attachment(hc, "video", a.VideoData.URL, a.Name, a.MimeType), true
	case a.FileURL != "":
		kind := "file"
		if strings.HasPrefix(a.MimeType, "audio/") {
			kind = "audio"
		}
		return Attachment(hc, kind, a.FileURL, a.Name, a.MimeType), true
	}
	return channels.InboundAttachment{}, false
}

// Reconcile limits.
const (
	ReconcileMessagesPerConversation = 20
	ReconcileConversationsPerPage    = 25
	ReconcileMaxPages                = 4
)

// RecentMessages reads the conversations of owner (a page id or "me")
// updated since, newest first, and returns the messages created at or after
// since that were not sent by one of ownIDs, oldest first and de-duplicated.
func (c *Client) RecentMessages(ctx context.Context, owner, platform, token string, since time.Time, ownIDs ...string) ([]ConvMessage, error) {
	own := map[string]bool{}
	for _, id := range ownIDs {
		if id != "" {
			own[id] = true
		}
	}
	fields := fmt.Sprintf("id,updated_time,messages.limit(%d){id,created_time,from,message,attachments{id,mime_type,name,file_url,image_data,video_data}}",
		ReconcileMessagesPerConversation)
	next := c.URL(owner+"/conversations", url.Values{
		"platform": {platform}, "fields": {fields}, "limit": {strconv.Itoa(ReconcileConversationsPerPage)},
	})
	seen := map[string]bool{}
	var out []ConvMessage
	for page := 0; next != "" && page < ReconcileMaxPages; page++ {
		var body struct {
			Data []struct {
				UpdatedTime string `json:"updated_time"`
				Messages    struct {
					Data []ConvMessage `json:"data"`
				} `json:"messages"`
			} `json:"data"`
			Paging struct {
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := c.Do(ctx, http.MethodGet, next, token, nil, &body); err != nil {
			return nil, err
		}
		older := false
		for _, conv := range body.Data {
			if t, ok := ParseTime(conv.UpdatedTime); ok && t.Before(since) {
				older = true
				break
			}
			for _, m := range conv.Messages.Data {
				at, ok := ParseTime(m.CreatedTime)
				if m.ID == "" || !ok || at.Before(since) || m.From.ID == "" || own[string(m.From.ID)] || seen[m.ID] {
					continue
				}
				seen[m.ID] = true
				m.At = at
				out = append(out, m)
			}
		}
		next = ""
		if !older && strings.HasPrefix(body.Paging.Next, c.BaseURL+"/") {
			next = body.Paging.Next
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}
