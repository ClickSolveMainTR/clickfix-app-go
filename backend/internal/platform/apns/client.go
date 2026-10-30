// Package apns sends pushes straight to Apple Push Notification service with
// token-based (.p8, ES256) authentication. ClickFix uses it for ActivityKit
// Live Activity pushes (CF-152); ordinary mobile notifications keep going
// through Expo (notifications/usecase/expo.go).
package apns

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/appleauth"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Environment is the APNs host a device token belongs to.
type Environment string

const (
	Production Environment = "production"
	Sandbox    Environment = "sandbox"
)

const (
	productionHost = "https://api.push.apple.com"
	sandboxHost    = "https://api.sandbox.push.apple.com"

	// PushTypeLiveActivity is the apns-push-type of ActivityKit pushes.
	PushTypeLiveActivity = "liveactivity"
	// liveActivityTopicSuffix is appended to the bundle id for ActivityKit.
	liveActivityTopicSuffix = ".push-type.liveactivity"

	// PriorityHigh delivers immediately; PriorityLow lets the system batch
	// (Apple budgets high-priority Live Activity updates).
	PriorityHigh = 10
	PriorityLow  = 5

	// providerTokenTTL: Apple rejects tokens older than one hour and refuses
	// refreshing more often than every 20 minutes.
	providerTokenTTL = 50 * time.Minute
	maxPayloadBytes  = 4096
)

// ErrPayloadTooLarge is returned before sending a payload Apple would reject.
var ErrPayloadTooLarge = errors.New("apns: payload exceeds 4 KB")

// Config is the token-based auth configuration. PrivateKey is the .p8 PEM,
// raw or base64-encoded. Topic is the app bundle id.
type Config struct {
	KeyID      string
	TeamID     string
	PrivateKey string
	Topic      string
}

// Enabled reports whether the key, team and private key are all set.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.KeyID) != "" && strings.TrimSpace(c.TeamID) != "" && strings.TrimSpace(c.PrivateKey) != ""
}

// Client sends pushes over HTTP/2. A nil *Client is a valid disabled sender.
type Client struct {
	keyID  string
	teamID string
	topic  string
	key    *ecdsa.PrivateKey
	http   *http.Client
	hosts  map[Environment]string
	now    func() time.Time

	mu      sync.Mutex
	token   string
	tokenAt time.Time
}

// New builds a client. It returns (nil, nil) when cfg is not Enabled, so
// callers treat Live Activity pushes as off.
func New(cfg Config, httpClient *http.Client) (*Client, error) {
	if !cfg.Enabled() {
		return nil, nil
	}
	key, err := ParsePrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, err
	}
	topic := strings.TrimSpace(cfg.Topic)
	if topic == "" {
		return nil, errors.New("apns: topic (bundle id) is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				Proxy:             http.ProxyFromEnvironment,
				ForceAttemptHTTP2: true, // APNs only speaks HTTP/2
				TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
				IdleConnTimeout:   5 * time.Minute,
			},
		}
	}
	return &Client{
		keyID: strings.TrimSpace(cfg.KeyID), teamID: strings.TrimSpace(cfg.TeamID), topic: topic, key: key,
		http:  httpClient,
		hosts: map[Environment]string{Production: productionHost, Sandbox: sandboxHost},
		now:   time.Now,
	}, nil
}

// ParsePrivateKey accepts the .p8 PEM as is, with literal "\n" escapes, or
// base64-encoded (the form used in env vars).
func ParsePrivateKey(raw string) (*ecdsa.PrivateKey, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "-----BEGIN") {
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, errors.New("apns: private key is neither PEM nor base64 PEM")
		}
		raw = string(decoded)
	}
	key, err := appleauth.ParsePrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("apns: %w", err)
	}
	return key, nil
}

// SetHost overrides the endpoint of one environment (tests).
func (c *Client) SetHost(env Environment, host string) {
	c.hosts[env] = strings.TrimRight(host, "/")
}

// Notification is one push.
type Notification struct {
	Token       string
	Environment Environment
	// Priority is PriorityHigh or PriorityLow.
	Priority int
	// Expiration: Apple stores and retries until then; zero means deliver
	// once, now.
	Expiration time.Time
	Payload    []byte
}

// Result is Apple's answer. Status 200 means accepted.
type Result struct {
	Status int
	Reason string
	APNsID string
}

// OK reports whether Apple accepted the push.
func (r Result) OK() bool { return r.Status == http.StatusOK }

// TokenInvalid reports an answer after which the device token must not be
// used again.
func (r Result) TokenInvalid() bool {
	if r.Status == http.StatusGone {
		return true
	}
	switch r.Reason {
	case "BadDeviceToken", "Unregistered", "DeviceTokenNotForTopic", "ExpiredToken":
		return true
	}
	return false
}

// Retryable reports a transient failure worth retrying later.
func (r Result) Retryable() bool {
	return r.Status == http.StatusTooManyRequests || r.Status >= 500
}

// SendLiveActivity sends an ActivityKit push. The returned error is a
// transport or local failure; Apple's rejections come back in Result.
func (c *Client) SendLiveActivity(ctx context.Context, n Notification) (Result, error) {
	if c == nil {
		return Result{}, errors.New("apns: not configured")
	}
	req, err := c.newRequest(ctx, n, PushTypeLiveActivity, c.topic+liveActivityTopicSuffix)
	if err != nil {
		return Result{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("apns: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	res := Result{Status: resp.StatusCode, APNsID: resp.Header.Get("apns-id")}
	if resp.StatusCode != http.StatusOK {
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&body)
		res.Reason = body.Reason
		if res.Reason == "ExpiredProviderToken" || res.Reason == "InvalidProviderToken" {
			c.resetProviderToken()
		}
	}
	return res, nil
}

func (c *Client) newRequest(ctx context.Context, n Notification, pushType, topic string) (*http.Request, error) {
	token := strings.TrimSpace(n.Token)
	if token == "" {
		return nil, errors.New("apns: device token is required")
	}
	if len(n.Payload) > maxPayloadBytes {
		return nil, ErrPayloadTooLarge
	}
	host, ok := c.hosts[n.Environment]
	if !ok {
		host = c.hosts[Production]
	}
	bearer, err := c.providerToken()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/3/device/"+token, bytes.NewReader(n.Payload))
	if err != nil {
		return nil, fmt.Errorf("apns: request: %w", err)
	}
	priority := n.Priority
	if priority != PriorityLow {
		priority = PriorityHigh
	}
	exp := int64(0)
	if !n.Expiration.IsZero() {
		exp = n.Expiration.Unix()
	}
	req.Header.Set("authorization", "bearer "+bearer)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("apns-push-type", pushType)
	req.Header.Set("apns-topic", topic)
	req.Header.Set("apns-priority", strconv.Itoa(priority))
	req.Header.Set("apns-expiration", strconv.FormatInt(exp, 10))
	return req, nil
}

// providerToken returns the cached ES256 JWT, signing a new one after
// providerTokenTTL.
func (c *Client) providerToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.token != "" && now.Sub(c.tokenAt) < providerTokenTTL {
		return c.token, nil
	}
	tok := jwtlib.NewWithClaims(jwtlib.SigningMethodES256, jwtlib.MapClaims{
		"iss": c.teamID,
		"iat": now.Unix(),
	})
	tok.Header["kid"] = c.keyID
	signed, err := tok.SignedString(c.key)
	if err != nil {
		return "", fmt.Errorf("apns: sign provider token: %w", err)
	}
	c.token, c.tokenAt = signed, now
	return signed, nil
}

func (c *Client) resetProviderToken() {
	c.mu.Lock()
	c.token = ""
	c.mu.Unlock()
}
