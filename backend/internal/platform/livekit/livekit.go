// Package livekit is a small LiveKit client: access tokens, webhook
// verification and the Twirp JSON endpoints the video module uses
// (RoomService, Egress). It speaks the wire protocol directly so the backend
// does not pull the protobuf SDK and its dependency tree.
package livekit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrNotConfigured = errors.New("livekit: not configured")
	ErrBadSignature  = errors.New("livekit: invalid webhook signature")
)

// Config holds the server connection. URL is what browsers and phones dial
// (wss://...); APIURL is the server-to-server address (http on the private
// network in production).
type Config struct {
	URL       string
	APIURL    string
	APIKey    string
	APISecret string
}

func (c Config) Enabled() bool {
	return c.URL != "" && c.APIURL != "" && c.APIKey != "" && c.APISecret != ""
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	cfg.APIURL = strings.TrimRight(cfg.APIURL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) Enabled() bool { return c != nil && c.cfg.Enabled() }
func (c *Client) URL() string   { return c.cfg.URL }

// Track sources a participant may publish (LiveKit canPublishSources).
const (
	SourceCamera            = "camera"
	SourceMicrophone        = "microphone"
	SourceScreenShare       = "screen_share"
	SourceScreenShareAudio  = "screen_share_audio"
	defaultTokenTTL         = 6 * time.Hour
	maxWebhookBodyBytes     = 1 << 20
	maxResponseBodyBytes    = 1 << 20
	twirpRoomService        = "/twirp/livekit.RoomService/"
	twirpEgressService      = "/twirp/livekit.Egress/"
	adminTokenTTL           = 5 * time.Minute
	defaultEmptyTimeoutSecs = 300
)

// VideoGrant mirrors LiveKit's claim names.
type VideoGrant struct {
	RoomCreate           bool     `json:"roomCreate,omitempty"`
	RoomList             bool     `json:"roomList,omitempty"`
	RoomRecord           bool     `json:"roomRecord,omitempty"`
	RoomAdmin            bool     `json:"roomAdmin,omitempty"`
	RoomJoin             bool     `json:"roomJoin,omitempty"`
	Room                 string   `json:"room,omitempty"`
	CanPublish           *bool    `json:"canPublish,omitempty"`
	CanSubscribe         *bool    `json:"canSubscribe,omitempty"`
	CanPublishData       *bool    `json:"canPublishData,omitempty"`
	CanPublishSources    []string `json:"canPublishSources,omitempty"`
	CanUpdateOwnMetadata *bool    `json:"canUpdateOwnMetadata,omitempty"`
	Hidden               bool     `json:"hidden,omitempty"`
}

type claims struct {
	jwt.RegisteredClaims
	Name     string      `json:"name,omitempty"`
	Metadata string      `json:"metadata,omitempty"`
	Video    *VideoGrant `json:"video,omitempty"`
	Sha256   string      `json:"sha256,omitempty"`
}

// JoinToken issues a participant token for one room.
func (c *Client) JoinToken(identity, name, metadata string, grant VideoGrant, ttl time.Duration) (string, error) {
	if !c.Enabled() {
		return "", ErrNotConfigured
	}
	if identity == "" || grant.Room == "" {
		return "", fmt.Errorf("livekit: identity and room are required")
	}
	if ttl <= 0 {
		ttl = defaultTokenTTL
	}
	grant.RoomJoin = true
	return c.sign(identity, name, metadata, &grant, ttl)
}

func (c *Client) sign(identity, name, metadata string, grant *VideoGrant, ttl time.Duration) (string, error) {
	now := time.Now()
	cl := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    c.cfg.APIKey,
			Subject:   identity,
			NotBefore: jwt.NewNumericDate(now.Add(-10 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Name: name, Metadata: metadata, Video: grant,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, cl).SignedString([]byte(c.cfg.APISecret))
}

// VerifyWebhook checks the Authorization JWT LiveKit sends with every webhook
// (issuer = API key, sha256 claim = base64 SHA-256 of the body) and returns
// the body.
func (c *Client) VerifyWebhook(r *http.Request) ([]byte, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodyBytes+1))
	if err != nil || len(body) > maxWebhookBodyBytes {
		return nil, ErrBadSignature
	}
	raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if raw == "" {
		return nil, ErrBadSignature
	}
	var cl claims
	tok, err := jwt.ParseWithClaims(raw, &cl, func(*jwt.Token) (any, error) { return []byte(c.cfg.APISecret), nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(c.cfg.APIKey), jwt.WithExpirationRequired(), jwt.WithLeeway(time.Minute))
	if err != nil || !tok.Valid {
		return nil, ErrBadSignature
	}
	sum := sha256.Sum256(body)
	if subtle.ConstantTimeCompare([]byte(cl.Sha256), []byte(base64.StdEncoding.EncodeToString(sum[:]))) != 1 {
		return nil, ErrBadSignature
	}
	return body, nil
}

func (c *Client) call(ctx context.Context, service, method string, grant VideoGrant, in, out any) error {
	if !c.Enabled() {
		return ErrNotConfigured
	}
	token, err := c.sign("clickfix-api", "", "", &grant, adminTokenTTL)
	if err != nil {
		return err
	}
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.APIURL+service+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("livekit: %s: %w", method, err)
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBodyBytes))
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 {
		var twirp struct {
			Code string `json:"code"`
			Msg  string `json:"msg"`
		}
		_ = json.Unmarshal(data, &twirp)
		if twirp.Code == "not_found" {
			return ErrNotFound
		}
		return fmt.Errorf("livekit: %s: http %d %s %s", method, res.StatusCode, twirp.Code, twirp.Msg)
	}
	if out == nil {
		return nil
	}
	return Decode(data, out)
}

var ErrNotFound = errors.New("livekit: not found")

type CreateRoomInput struct {
	Name             string `json:"name"`
	EmptyTimeout     uint32 `json:"emptyTimeout,omitempty"`
	DepartureTimeout uint32 `json:"departureTimeout,omitempty"`
	MaxParticipants  uint32 `json:"maxParticipants,omitempty"`
	Metadata         string `json:"metadata,omitempty"`
}

// CreateRoom pre-creates a room so its limits apply before the first join.
func (c *Client) CreateRoom(ctx context.Context, in CreateRoomInput) error {
	if in.EmptyTimeout == 0 {
		in.EmptyTimeout = defaultEmptyTimeoutSecs
	}
	return c.call(ctx, twirpRoomService, "CreateRoom", VideoGrant{RoomCreate: true}, in, nil)
}

// DeleteRoom disconnects everyone; a missing room is not an error.
func (c *Client) DeleteRoom(ctx context.Context, room string) error {
	err := c.call(ctx, twirpRoomService, "DeleteRoom", VideoGrant{RoomCreate: true}, map[string]string{"room": room}, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (c *Client) RemoveParticipant(ctx context.Context, room, identity string) error {
	err := c.call(ctx, twirpRoomService, "RemoveParticipant", VideoGrant{RoomAdmin: true, Room: room}, map[string]string{"room": room, "identity": identity}, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Permission is a participant's live permission set (RoomService
// UpdateParticipant replaces all of it, so every field is sent).
type Permission struct {
	CanSubscribe   bool `json:"canSubscribe"`
	CanPublish     bool `json:"canPublish"`
	CanPublishData bool `json:"canPublishData"`
	Hidden         bool `json:"hidden"`
}

// UpdatePermission changes a connected participant's permissions without a
// reconnect (e.g. withdrawing data publishing when control is revoked). A
// participant that is not connected is not an error: its next token carries
// the new grant.
func (c *Client) UpdatePermission(ctx context.Context, room, identity string, p Permission) error {
	err := c.call(ctx, twirpRoomService, "UpdateParticipant", VideoGrant{RoomAdmin: true, Room: room},
		map[string]any{"room": room, "identity": identity, "permission": p}, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// S3Output is where egress uploads the recording (per request, so an
// organization's own bucket never touches the egress config).
type S3Output struct {
	AccessKey      string `json:"accessKey"`
	Secret         string `json:"secret"`
	Region         string `json:"region,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
	Bucket         string `json:"bucket"`
	ForcePathStyle bool   `json:"forcePathStyle,omitempty"`
}

// StartRecording starts a composite (grid) MP4 recording of the room.
func (c *Client) StartRecording(ctx context.Context, room, path string, out S3Output) (string, error) {
	req := map[string]any{
		"roomName": room,
		"layout":   "grid",
		"fileOutputs": []map[string]any{{
			"fileType": "MP4",
			"filepath": path,
			"s3":       out,
		}},
	}
	var info EgressInfo
	if err := c.call(ctx, twirpEgressService, "StartRoomCompositeEgress", VideoGrant{RoomRecord: true}, req, &info); err != nil {
		return "", err
	}
	if info.EgressID == "" {
		return "", fmt.Errorf("livekit: egress id missing")
	}
	return info.EgressID, nil
}

func (c *Client) StopRecording(ctx context.Context, egressID string) error {
	err := c.call(ctx, twirpEgressService, "StopEgress", VideoGrant{RoomRecord: true}, map[string]string{"egressId": egressID}, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Webhook payloads. LiveKit encodes them with protojson (lowerCamelCase,
// int64 as strings); Decode also accepts snake_case names.
type Event struct {
	ID          string       `json:"id"`
	Event       string       `json:"event"`
	CreatedAt   Int          `json:"createdAt"`
	Room        *Room        `json:"room"`
	Participant *Participant `json:"participant"`
	EgressInfo  *EgressInfo  `json:"egressInfo"`
}
type Room struct {
	SID          string `json:"sid"`
	Name         string `json:"name"`
	CreationTime Int    `json:"creationTime"`
}
type Participant struct {
	SID      string `json:"sid"`
	Identity string `json:"identity"`
	Name     string `json:"name"`
	JoinedAt Int    `json:"joinedAt"`
}
type EgressInfo struct {
	EgressID    string       `json:"egressId"`
	RoomName    string       `json:"roomName"`
	Status      EgressStatus `json:"status"`
	Error       string       `json:"error"`
	FileResults []FileInfo   `json:"fileResults"`
}
type FileInfo struct {
	Filename string `json:"filename"`
	Size     Int    `json:"size"`
	// Duration is in nanoseconds.
	Duration Int    `json:"duration"`
	Location string `json:"location"`
}

// Int accepts a JSON number or a numeric string (protojson int64).
type Int int64

func (n *Int) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil {
			return err
		}
		v = int64(f)
	}
	*n = Int(v)
	return nil
}

// EgressStatus is the enum name (EGRESS_COMPLETE) whether sent as a name or
// as its number.
type EgressStatus string

var egressStatusNames = []string{"EGRESS_STARTING", "EGRESS_ACTIVE", "EGRESS_ENDING", "EGRESS_COMPLETE", "EGRESS_FAILED", "EGRESS_ABORTED", "EGRESS_LIMIT_REACHED"}

func (s *EgressStatus) UnmarshalJSON(b []byte) error {
	raw := strings.Trim(string(b), `"`)
	if i, err := strconv.Atoi(raw); err == nil {
		if i >= 0 && i < len(egressStatusNames) {
			*s = EgressStatus(egressStatusNames[i])
			return nil
		}
	}
	*s = EgressStatus(raw)
	return nil
}

// Decode unmarshals protojson output into out, normalizing snake_case keys.
func Decode(data []byte, out any) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	norm, err := json.Marshal(camelKeys(v))
	if err != nil {
		return err
	}
	return json.Unmarshal(norm, out)
}

func camelKeys(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[camel(k)] = camelKeys(val)
		}
		return out
	case []any:
		for i := range t {
			t[i] = camelKeys(t[i])
		}
		return t
	default:
		return v
	}
}

func camel(k string) string {
	if !strings.Contains(k, "_") {
		return k
	}
	var b strings.Builder
	up := false
	for _, r := range k {
		if r == '_' {
			up = true
			continue
		}
		if up {
			r = unicode.ToUpper(r)
			up = false
		}
		b.WriteRune(r)
	}
	return b.String()
}
