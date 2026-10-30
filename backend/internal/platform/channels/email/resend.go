package email

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Resend is the client of the Resend API ClickFix uses for the inbound
// routing domain (Receiving), outbound mail and the sender domain wizard.
// Ported from 216detay-mail-system (receiving + raw download); sending and
// domains follow the same request style.
type Resend struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// APIError is a non-2xx Resend answer.
type APIError struct {
	Status  int
	Name    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("resend: %d %s: %s", e.Status, e.Name, e.Message)
}

// Temporary reports whether a retry may succeed.
func (e *APIError) Temporary() bool { return e.Status == 429 || e.Status >= 500 }

func (c *Resend) do(ctx context.Context, method, path string, body any, idempotencyKey string, out any) error {
	if c == nil || c.APIKey == "" {
		return errors.New("resend: not configured")
	}
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("resend: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("resend: read: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Message == "" {
			e.Message = strings.TrimSpace(string(raw))
		}
		return &APIError{Status: resp.StatusCode, Name: e.Name, Message: e.Message}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// RawReceived downloads the raw MIME of a received e-mail: the receiving
// API gives a short-lived https download URL that is fetched without the
// API key.
func (c *Resend) RawReceived(ctx context.Context, emailID string) ([]byte, error) {
	var email struct {
		Raw struct {
			DownloadURL string `json:"download_url"`
		} `json:"raw"`
	}
	if err := c.do(ctx, http.MethodGet, "/emails/receiving/"+url.PathEscape(emailID), nil, "", &email); err != nil {
		return nil, err
	}
	u, err := url.Parse(email.Raw.DownloadURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !strings.HasPrefix(c.BaseURL, "http://")) {
		return nil, errors.New("resend: receiving api did not provide an https raw download url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resend: download raw email: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{Status: resp.StatusCode, Message: "raw email download failed"}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxMessageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("resend: read raw email: %w", err)
	}
	if len(raw) > MaxMessageBytes {
		return nil, errors.New("resend: email exceeds the size limit")
	}
	return raw, nil
}

// SendRequest is POST /emails.
type SendRequest struct {
	From        string             `json:"from"`
	To          []string           `json:"to"`
	Cc          []string           `json:"cc,omitempty"`
	Bcc         []string           `json:"bcc,omitempty"`
	ReplyTo     []string           `json:"reply_to,omitempty"`
	Subject     string             `json:"subject"`
	Text        string             `json:"text"`
	HTML        string             `json:"html,omitempty"`
	Headers     map[string]string  `json:"headers,omitempty"`
	Attachments []ResendAttachment `json:"attachments,omitempty"`
	Tags        []ResendTag        `json:"tags,omitempty"`
}

// ResendAttachment is a file Resend fetches from Path.
type ResendAttachment struct {
	Filename string `json:"filename"`
	Path     string `json:"path,omitempty"`
	Content  string `json:"content,omitempty"`
}

// ResendTag labels a sent e-mail; tags come back on its webhook events.
type ResendTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Send sends one e-mail; key makes a retried send idempotent (24h).
func (c *Resend) Send(ctx context.Context, req SendRequest, key string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/emails", req, key, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// Domain is a sender domain in Resend with the DNS records to publish.
type Domain struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Status  string      `json:"status"`
	Region  string      `json:"region,omitempty"`
	Records []DNSRecord `json:"records"`
}

// DNSRecord is one record of the domain wizard (SPF, DKIM, MX for bounces).
type DNSRecord struct {
	Record   string `json:"record"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      string `json:"ttl,omitempty"`
	Status   string `json:"status"`
	Priority *int   `json:"priority,omitempty"`
}

// CreateDomain registers a sender domain.
func (c *Resend) CreateDomain(ctx context.Context, name string) (Domain, error) {
	var d Domain
	err := c.do(ctx, http.MethodPost, "/domains", map[string]any{"name": name, "region": "eu-west-1"}, "", &d)
	return d, err
}

// GetDomain reads a domain with its records and verification state.
func (c *Resend) GetDomain(ctx context.Context, id string) (Domain, error) {
	var d Domain
	err := c.do(ctx, http.MethodGet, "/domains/"+url.PathEscape(id), nil, "", &d)
	return d, err
}

// VerifyDomain asks Resend to check the DNS records again.
func (c *Resend) VerifyDomain(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/domains/"+url.PathEscape(id)+"/verify", nil, "", nil)
}

// FindDomain returns the Resend domain of a name, if registered.
func (c *Resend) FindDomain(ctx context.Context, name string) (Domain, bool, error) {
	var list struct {
		Data []Domain `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/domains", nil, "", &list); err != nil {
		return Domain{}, false, err
	}
	for _, d := range list.Data {
		if strings.EqualFold(d.Name, name) {
			return d, true, nil
		}
	}
	return Domain{}, false, nil
}

// Svix webhook signature (Resend signs webhooks with Svix).
const (
	HeaderSvixID        = "svix-id"
	HeaderSvixTimestamp = "svix-timestamp"
	HeaderSvixSignature = "svix-signature"
	svixTolerance       = 5 * time.Minute
)

// VerifySvix checks a Svix signature: base64(HMAC-SHA256(secret,
// id.timestamp.body)) in a space separated "v1,<sig>" list, with the
// timestamp within five minutes of now.
func VerifySvix(secret string, h http.Header, body []byte, now time.Time) bool {
	id, ts, sigs := h.Get(HeaderSvixID), h.Get(HeaderSvixTimestamp), h.Get(HeaderSvixSignature)
	if secret == "" || id == "" || ts == "" || sigs == "" {
		return false
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	if d := now.Sub(time.Unix(sec, 0)); d > svixTolerance || d < -svixTolerance {
		return false
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return false
	}
	want := SignSvix(key, id, ts, body)
	for _, s := range strings.Fields(sigs) {
		version, sig, ok := strings.Cut(s, ",")
		if ok && version == "v1" && hmac.Equal([]byte(sig), []byte(want)) {
			return true
		}
	}
	return false
}

// SignSvix computes the v1 signature (tests and fixtures).
func SignSvix(key []byte, id, ts string, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
