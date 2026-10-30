// Package usecase holds the mobile OS surface logic (CF-152): widget tokens,
// Live Activity push tokens and settings, and the Live Activity pushes sent
// on SLA and conversation events.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apns"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// WidgetTokenTTL is the lifetime of a widget token; the app renews it on
	// foreground when it is older than 12 hours.
	WidgetTokenTTL = 72 * time.Hour
	// widgetTokenBytes of randomness, base64url without padding (43 chars).
	widgetTokenBytes = 32
	widgetTokenLen   = 43

	maxDeviceIDLen   = 128
	maxActivityIDLen = 128
	maxRefLen        = 64
	minAPNsTokenLen  = 32
	maxAPNsTokenLen  = 255
)

// Live Activity token enums (CHECK constraints in 000210).
const (
	KindPushToStart = "push_to_start"
	KindUpdate      = "update"

	ActivityCall               = "call"
	ActivityUrgentConversation = "urgent_conversation"
)

var (
	// ErrInvalidWidgetToken: unknown, revoked, expired, or its session ended.
	ErrInvalidWidgetToken = errors.New("mobileext: invalid widget token")
	// ErrSessionRequired: the access token is not bound to a refresh session.
	ErrSessionRequired = errors.New("mobileext: a signed-in session is required")
)

// ValidationError lists field -> message.
type ValidationError map[string]string

func (v ValidationError) Error() string { return "mobileext: validation failed" }

// Sender is the APNs client (nil = Live Activity push off).
type Sender interface {
	SendLiveActivity(ctx context.Context, n apns.Notification) (apns.Result, error)
}

// Store is the persistence the service needs (repository.Postgres).
type Store interface {
	IssueWidgetToken(ctx context.Context, p db.CreateWidgetTokenParams) (db.WidgetToken, error)
	RevokeWidgetTokens(ctx context.Context, userID int64, deviceID string) error
	ValidWidgetToken(ctx context.Context, hash string) (db.GetValidWidgetTokenRow, bool, error)
	TouchWidgetToken(ctx context.Context, id int64) error
	RebindWidgetTokens(ctx context.Context, userID int64, from, to uuid.UUID) error

	UpsertLiveActivityToken(ctx context.Context, p db.UpsertLiveActivityTokenParams) error
	DeleteLiveActivityToken(ctx context.Context, userID int64, token string) error
	PushToStartTokens(ctx context.Context, userID int64, activityType string) ([]db.LiveActivityToken, error)
	UpdateTokensByRef(ctx context.Context, activityType, ref string) ([]db.LiveActivityToken, error)
	DisableLiveActivityToken(ctx context.Context, id int64, reason string) error
	LiveActivitiesEnabled(ctx context.Context, userID int64) (bool, error)
	SetLiveActivitiesEnabled(ctx context.Context, userID int64, enabled bool) error

	Recipient(ctx context.Context, userID int64) (db.GetLiveActivityRecipientRow, error)
	Conversation(ctx context.Context, organizationID, id int64) (db.GetLiveActivityConversationRow, error)
}

// Service implements the mobile surface use cases.
type Service struct {
	store  Store
	sender Sender
	log    *slog.Logger
	now    func() time.Time
	random func([]byte) (int, error)
}

// New builds the service. sender may be nil (Live Activity push off).
func New(store Store, sender Sender, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{store: store, log: log, now: time.Now, random: rand.Read}
	// A typed nil (*apns.Client)(nil) must read as "off".
	if c, ok := sender.(*apns.Client); !ok || c != nil {
		s.sender = sender
	}
	return s
}

// PushConfigured reports whether Live Activity pushes can be sent.
func (s *Service) PushConfigured() bool { return s != nil && s.sender != nil }

// HashToken is the stored form of a widget token (sha256 hex).
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// wellFormedToken rejects values that cannot be a widget token before any
// database lookup.
func wellFormedToken(raw string) bool {
	if len(raw) != widgetTokenLen {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(raw)
	return err == nil
}

// NormalizeDeviceID trims and validates an installation id.
func NormalizeDeviceID(raw string) (string, bool) {
	id := strings.TrimSpace(raw)
	if id == "" || len(id) > maxDeviceIDLen || !utf8.ValidString(id) {
		return "", false
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return id, true
}

// IssueInput is the caller of POST /v1/mobile/widget-token.
type IssueInput struct {
	UserID         int64
	OrganizationID int64
	SessionID      uuid.UUID
	DeviceID       string
}

// IssuedWidgetToken is returned once; only its hash is stored.
type IssuedWidgetToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IssueWidgetToken creates a token for (user, organization, device) bound to
// the caller's refresh session and revokes the device's previous tokens.
func (s *Service) IssueWidgetToken(ctx context.Context, in IssueInput) (IssuedWidgetToken, error) {
	device, ok := NormalizeDeviceID(in.DeviceID)
	if !ok {
		return IssuedWidgetToken{}, ValidationError{"device_id": "must be 1-128 characters"}
	}
	if in.SessionID == uuid.Nil {
		return IssuedWidgetToken{}, ErrSessionRequired
	}
	buf := make([]byte, widgetTokenBytes)
	if _, err := s.random(buf); err != nil {
		return IssuedWidgetToken{}, fmt.Errorf("mobileext: token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	expires := s.now().UTC().Add(WidgetTokenTTL).Truncate(time.Second)
	_, err := s.store.IssueWidgetToken(ctx, db.CreateWidgetTokenParams{
		UserID: in.UserID, OrganizationID: in.OrganizationID, DeviceID: device,
		TokenHash:   HashToken(raw),
		SessionUuid: pgtype.UUID{Bytes: in.SessionID, Valid: true},
		ExpiresAt:   pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err != nil {
		return IssuedWidgetToken{}, err
	}
	return IssuedWidgetToken{Token: raw, ExpiresAt: expires}, nil
}

// RevokeWidgetToken revokes the caller's tokens on a device (logout).
func (s *Service) RevokeWidgetToken(ctx context.Context, userID int64, deviceID string) error {
	device, ok := NormalizeDeviceID(deviceID)
	if !ok {
		return ValidationError{"device_id": "must be 1-128 characters"}
	}
	return s.store.RevokeWidgetTokens(ctx, userID, device)
}

// WidgetIdentity is who a valid widget token speaks for.
type WidgetIdentity struct {
	TokenID        int64
	UserID         int64
	UserUUID       uuid.UUID
	OrganizationID int64
	SessionID      uuid.UUID
}

// AuthenticateWidget resolves a raw widget token.
func (s *Service) AuthenticateWidget(ctx context.Context, raw string) (WidgetIdentity, error) {
	raw = strings.TrimSpace(raw)
	if !wellFormedToken(raw) {
		return WidgetIdentity{}, ErrInvalidWidgetToken
	}
	row, ok, err := s.store.ValidWidgetToken(ctx, HashToken(raw))
	if err != nil {
		return WidgetIdentity{}, err
	}
	if !ok {
		return WidgetIdentity{}, ErrInvalidWidgetToken
	}
	id := WidgetIdentity{TokenID: row.ID, UserID: row.UserID, UserUUID: row.UserUuid, OrganizationID: row.OrganizationID}
	if row.SessionUuid.Valid {
		id.SessionID = row.SessionUuid.Bytes
	}
	// Best effort, at most once a minute (the query skips fresh rows).
	if !row.LastUsedAt.Valid || s.now().Sub(row.LastUsedAt.Time) >= time.Minute {
		if err := s.store.TouchWidgetToken(ctx, row.ID); err != nil {
			s.log.Warn("widget_token_touch_failed", "error", err)
		}
	}
	return id, nil
}

// SessionRotated implements auth usecase.SessionRotationHook.
func (s *Service) SessionRotated(ctx context.Context, userID int64, from, to uuid.UUID) error {
	if from == uuid.Nil || to == uuid.Nil || from == to {
		return nil
	}
	return s.store.RebindWidgetTokens(ctx, userID, from, to)
}

// LiveActivityTokenInput is the body of POST /v1/mobile/live-activity-tokens.
type LiveActivityTokenInput struct {
	DeviceID     string `json:"device_id"`
	Kind         string `json:"kind"`
	ActivityType string `json:"activity_type"`
	ActivityID   string `json:"activity_id"`
	Ref          string `json:"ref"`
	Token        string `json:"token"`
	Environment  string `json:"environment"`
}

// NormalizeAPNsToken lower-cases a hex APNs token and validates it.
func NormalizeAPNsToken(raw string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(raw))
	if len(t) < minAPNsTokenLen || len(t) > maxAPNsTokenLen {
		return "", false
	}
	if _, err := hex.DecodeString(t); err != nil {
		return "", false
	}
	return t, true
}

// ValidateLiveActivityToken normalizes the input or lists field errors.
func ValidateLiveActivityToken(in LiveActivityTokenInput) (db.UpsertLiveActivityTokenParams, ValidationError) {
	errs := ValidationError{}
	out := db.UpsertLiveActivityTokenParams{
		Kind: in.Kind, ActivityType: in.ActivityType, Environment: in.Environment,
		ActivityID: strings.TrimSpace(in.ActivityID), Ref: strings.TrimSpace(in.Ref),
	}
	if d, ok := NormalizeDeviceID(in.DeviceID); ok {
		out.DeviceID = d
	} else {
		errs["device_id"] = "must be 1-128 characters"
	}
	if in.Kind != KindPushToStart && in.Kind != KindUpdate {
		errs["kind"] = "must be push_to_start or update"
	}
	if in.ActivityType != ActivityCall && in.ActivityType != ActivityUrgentConversation {
		errs["activity_type"] = "must be call or urgent_conversation"
	}
	if in.Environment != string(apns.Production) && in.Environment != string(apns.Sandbox) {
		errs["environment"] = "must be production or sandbox"
	}
	if len(out.ActivityID) > maxActivityIDLen || !utf8.ValidString(out.ActivityID) {
		errs["activity_id"] = "must be at most 128 characters"
	}
	if len(out.Ref) > maxRefLen || !utf8.ValidString(out.Ref) {
		errs["ref"] = "must be at most 64 characters"
	}
	if t, ok := NormalizeAPNsToken(in.Token); ok {
		out.Token = t
	} else {
		errs["token"] = "must be a hex APNs token (32-255 characters)"
	}
	if len(errs) > 0 {
		return out, errs
	}
	return out, nil
}

// RegisterLiveActivityToken stores (or moves to the caller) a push token.
func (s *Service) RegisterLiveActivityToken(ctx context.Context, userID int64, in LiveActivityTokenInput) error {
	p, errs := ValidateLiveActivityToken(in)
	if errs != nil {
		return errs
	}
	p.UserID = userID
	return s.store.UpsertLiveActivityToken(ctx, p)
}

// DeleteLiveActivityToken removes one of the caller's tokens. Idempotent.
func (s *Service) DeleteLiveActivityToken(ctx context.Context, userID int64, token string) error {
	t, ok := NormalizeAPNsToken(token)
	if !ok {
		return nil // cannot be stored, so there is nothing to delete
	}
	return s.store.DeleteLiveActivityToken(ctx, userID, t)
}

// LiveActivitySettings is GET/PUT /v1/mobile/live-activity-settings.
type LiveActivitySettings struct {
	Enabled bool `json:"enabled"`
}

// GetLiveActivitySettings returns the caller's switch.
func (s *Service) GetLiveActivitySettings(ctx context.Context, userID int64) (LiveActivitySettings, error) {
	on, err := s.store.LiveActivitiesEnabled(ctx, userID)
	return LiveActivitySettings{Enabled: on}, err
}

// SetLiveActivitySettings stores the switch; off deletes the caller's tokens.
func (s *Service) SetLiveActivitySettings(ctx context.Context, userID int64, in LiveActivitySettings) (LiveActivitySettings, error) {
	if err := s.store.SetLiveActivitiesEnabled(ctx, userID, in.Enabled); err != nil {
		return LiveActivitySettings{}, err
	}
	return in, nil
}
