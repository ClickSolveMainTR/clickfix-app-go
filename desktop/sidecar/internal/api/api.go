// Package api, ClickFix uzaktan destek public uçlarının (CF-233,
// backend/docs/openapi.yaml "RemoteSupport") müşteri tarafı istemcisidir.
//
// Uçlar: resolve, consent, token, end, report. Hepsi POST'tur ve JSON zarfı
// {success, data | error, meta} döner. İstemci zaman aşımı ve geçici
// hatalarda (ağ, 502/503/504) geri çekilmeli yeniden deneme uygular; 429 ve
// diğer 4xx yeniden denenmez. Kullanıcı ajanı "ClickFixSupport/<sürüm> (<os>)".
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Varsayılanlar.
const (
	DefaultAttemptTimeout = 6 * time.Second
	DefaultTotalTimeout   = 15 * time.Second
	DefaultRetries        = 2
	maxBody               = 1 << 20
)

// Organization, çözülen oturumun organizasyonudur.
type Organization struct {
	UUID     string  `json:"uuid"`
	Name     string  `json:"name"`
	LogoURL  *string `json:"logo_url"`
	Verified bool    `json:"verified"`
}

// Person, temsilcidir.
type Person struct {
	UUID      string  `json:"uuid"`
	Name      string  `json:"name"`
	AvatarURL *string `json:"avatar_url"`
}

// Resolved, RemoteSupportResolved şemasıdır.
type Resolved struct {
	SessionUUID        string       `json:"session_uuid"`
	ClientSecret       string       `json:"client_secret"`
	Organization       Organization `json:"organization"`
	Agent              Person       `json:"agent"`
	ModeRequested      string       `json:"mode_requested"`
	RecordingRequested bool         `json:"recording_requested"`
	MaxDurationMinutes int          `json:"max_duration_minutes"`
	ConsentDeadline    time.Time    `json:"consent_deadline"`
	Warning            string       `json:"warning"`
	Locale             string       `json:"locale"`
}

// CustomerView, RemoteSupportCustomerView şemasıdır.
type CustomerView struct {
	SessionUUID      string     `json:"session_uuid"`
	Status           string     `json:"status"`
	ModeRequested    string     `json:"mode_requested"`
	ViewConsentAt    *time.Time `json:"view_consent_at"`
	ControlActive    bool       `json:"control_active"`
	RecordingConsent *bool      `json:"recording_consent"`
	EndsAt           *time.Time `json:"ends_at"`
	EndedAt          *time.Time `json:"ended_at"`
	EndedReason      string     `json:"ended_reason"`
	Reported         bool       `json:"reported"`
}

// Grant, RemoteSupportGrant şemasıdır (müşterinin LiveKit token'ı).
type Grant struct {
	ServerURL      string    `json:"server_url"`
	Token          string    `json:"token"`
	Identity       string    `json:"identity"`
	Room           string    `json:"room"`
	CanPublishData bool      `json:"can_publish_data"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// ResolveInput, RemoteSupportResolveInput şemasıdır.
type ResolveInput struct {
	Code          string `json:"code,omitempty"`
	Token         string `json:"token,omitempty"`
	ClientOS      string `json:"client_os,omitempty"`
	ClientVersion string `json:"client_version,omitempty"`
}

// ConsentInput, RemoteSupportConsentInput şemasıdır (client_secret istemci ekler).
type ConsentInput struct {
	View      *bool `json:"view,omitempty"`
	Control   *bool `json:"control,omitempty"`
	Recording *bool `json:"recording,omitempty"`
}

// Error, API'nin döndürdüğü ya da istemcide oluşan hatadır.
type Error struct {
	Status  int    // HTTP durumu; ağ hatasında 0
	Code    string // zarftaki error.code (ör. REMOTE_SUPPORT_CODE_INVALID)
	Message string
	Network bool // sunucuya ulaşılamadı ya da zaman aşımı
	Err     error

	notSent bool // bağlantı kurulamadı; istek sunucuya hiç gitmedi
}

func (e *Error) Error() string {
	switch {
	case e.Network:
		return fmt.Sprintf("api: ağ hatası: %v", e.Err)
	case e.Code != "":
		return fmt.Sprintf("api: %d %s: %s", e.Status, e.Code, e.Message)
	default:
		return fmt.Sprintf("api: %d %s", e.Status, e.Message)
	}
}

func (e *Error) Unwrap() error { return e.Err }

// AsError, err bir *Error ise onu döndürür.
func AsError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// Client, public uç istemcisidir.
type Client struct {
	base      *url.URL // .../api/v1
	http      *http.Client
	userAgent string
	attempt   time.Duration
	total     time.Duration
	retries   int
	sleep     func(context.Context, time.Duration) error
}

// Option, istemci ayarıdır.
type Option func(*Client)

// WithHTTPClient, http.Client'ı değiştirir (testler için).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithTimeouts, deneme ve toplam süreyi değiştirir.
func WithTimeouts(attempt, total time.Duration) Option {
	return func(c *Client) { c.attempt, c.total = attempt, total }
}

// WithRetries, ek deneme sayısını değiştirir.
func WithRetries(n int) Option { return func(c *Client) { c.retries = n } }

// WithSleep, denemeler arası beklemeyi değiştirir (testler için).
func WithSleep(f func(context.Context, time.Duration) error) Option {
	return func(c *Client) { c.sleep = f }
}

// UserAgent, "ClickFixSupport/<sürüm> (<os>)" metnidir.
func UserAgent(version, os string) string {
	return fmt.Sprintf("ClickFixSupport/%s (%s)", version, os)
}

// New, uygulama kökünden (ör. https://getclickfix.com) bir istemci kurar.
// Uçlar <kök>/api/v1/public/remote-support/... altındadır. Kök https olmalı;
// yalnız localhost için http kabul edilir (kabuktaki CLICKFIX_APP_URL kuralı).
func New(appURL, userAgent string, opts ...Option) (*Client, error) {
	u, err := ParseAppURL(appURL)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("api", "v1")
	c := &Client{
		base:      u,
		http:      &http.Client{},
		userAgent: userAgent,
		attempt:   DefaultAttemptTimeout,
		total:     DefaultTotalTimeout,
		retries:   DefaultRetries,
		sleep:     sleepCtx,
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// ParseAppURL, uygulama kökünü doğrular: mutlak, https (localhost'ta http),
// yolsuz, sorgusuz.
func ParseAppURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("uygulama kökü geçerli bir URL değil: %q", raw)
	}
	host := u.Hostname()
	local := host == "localhost" || host == "127.0.0.1" || host == "::1"
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && local:
	default:
		return nil, fmt.Errorf("uygulama kökü https olmalı (yalnız localhost için http): %q", raw)
	}
	if u.User != nil {
		return nil, fmt.Errorf("uygulama kökü kullanıcı bilgisi içeremez")
	}
	return &url.URL{Scheme: u.Scheme, Host: u.Host}, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

const prefix = "public/remote-support"

// Resolve kodu ya da katılma token'ını çözer.
func (c *Client) Resolve(ctx context.Context, in ResolveInput) (Resolved, error) {
	var out Resolved
	// Çözme tek kullanımlıktır: istek sunucuya ulaşmış olabilecekse
	// (gönderimden sonra zaman aşımı) yeniden denenmez.
	err := c.post(ctx, prefix+"/resolve", in, &out, false)
	return out, err
}

// Consent onay verir ya da kaldırır.
func (c *Client) Consent(ctx context.Context, session, secret string, in ConsentInput) (CustomerView, error) {
	body := struct {
		ClientSecret string `json:"client_secret"`
		ConsentInput
	}{secret, in}
	var out CustomerView
	err := c.post(ctx, sessionPath(session, "consent"), body, &out, true)
	return out, err
}

// Token müşterinin LiveKit token'ını alır.
func (c *Client) Token(ctx context.Context, session, secret string) (Grant, error) {
	var out Grant
	err := c.post(ctx, sessionPath(session, "token"), clientInput{secret}, &out, true)
	return out, err
}

// End oturumu bitirir; zaten bitmişse saklanan durumu döner (idempotent).
func (c *Client) End(ctx context.Context, session, secret string) (CustomerView, error) {
	var out CustomerView
	err := c.post(ctx, sessionPath(session, "end"), clientInput{secret}, &out, true)
	return out, err
}

// Report dolandırıcılık bildirir.
func (c *Client) Report(ctx context.Context, session, secret, reason string) (CustomerView, error) {
	body := struct {
		ClientSecret string `json:"client_secret"`
		Reason       string `json:"reason,omitempty"`
	}{secret, reason}
	var out CustomerView
	err := c.post(ctx, sessionPath(session, "report"), body, &out, true)
	return out, err
}

type clientInput struct {
	ClientSecret string `json:"client_secret"`
}

func sessionPath(session, action string) string {
	return prefix + "/" + url.PathEscape(session) + "/" + action
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func retryable(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func (c *Client) post(ctx context.Context, path string, in, out any, idempotent bool) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, c.total)
	defer cancel()
	target := c.base.JoinPath(strings.Split(path, "/")...).String()

	var last error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			// 300 ms, 900 ms, ...
			if err := c.sleep(ctx, time.Duration(300*(1<<(2*(attempt-1))))*time.Millisecond); err != nil {
				break
			}
		}
		status, err := c.once(ctx, target, body, out)
		if err == nil {
			return nil
		}
		last = err
		e, _ := AsError(err)
		if e == nil || !retryable(status) && (!e.Network || !idempotent && !e.notSent) {
			return err
		}
		if ctx.Err() != nil {
			break
		}
	}
	return last
}

func (c *Client) once(ctx context.Context, target string, body []byte, out any) (int, error) {
	actx, cancel := context.WithTimeout(ctx, c.attempt)
	defer cancel()
	req, err := http.NewRequestWithContext(actx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error mesajı tam adresi içerir; sır yolda değil gövdededir,
		// yine de yalnız iç hatayı taşı.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		var oe *net.OpError
		notSent := errors.As(err, &oe) && oe.Op == "dial"
		var de *net.DNSError
		notSent = notSent || errors.As(err, &de)
		return 0, &Error{Network: true, Err: err, notSent: notSent}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, &Error{Network: true, Status: resp.StatusCode, Err: err}
	}
	var env envelope
	if jerr := json.Unmarshal(raw, &env); jerr != nil {
		return resp.StatusCode, &Error{Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
	}
	if resp.StatusCode/100 != 2 || !env.Success {
		e := &Error{Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
		if env.Error != nil {
			e.Code, e.Message = env.Error.Code, env.Error.Message
		}
		return resp.StatusCode, e
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return resp.StatusCode, &Error{Status: resp.StatusCode, Message: "beklenmeyen yanıt: " + err.Error()}
		}
	}
	return resp.StatusCode, nil
}
