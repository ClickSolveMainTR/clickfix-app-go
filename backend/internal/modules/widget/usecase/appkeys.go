package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Mobile app keys (CF-201). A native app has no page origin, so a widget
// may list the apps that embed it: platform, package name (Android
// application id or iOS bundle id) and a key. The visitor API accepts
// `X-ClickFix-App-Key` + `X-ClickFix-App-Package` in place of the origin
// allowlist; everything after that (visitor token, user_hash, rate limits)
// is the same as in a browser. Only the key's sha256 is stored: the key is
// returned once, on creation, and can be revoked but never read again.

const (
	// AppKeyPrefix starts every app key so logs and support can tell it from
	// an identity secret or an API key.
	AppKeyPrefix = "cf_app_"
	// MaxAppKeys bounds the active keys of one widget.
	MaxAppKeys  = 20
	appKeyBytes = 20
	appKeyShown = 12
	// appKeyTouchInterval bounds last_used_at writes (the query repeats the
	// check, this skips the round trip).
	appKeyTouchInterval = 5 * time.Minute
)

// ErrAppKeyInvalid: the app key is unknown, revoked, belongs to another
// widget or does not match the package name. One error for all four so a
// caller learns nothing about which part was wrong.
var ErrAppKeyInvalid = errors.New("widget: app key or package name is not valid")

// AppPlatforms are the accepted platforms.
var AppPlatforms = []string{"ios", "android"}

// packageNameRe accepts Android application ids and iOS bundle ids: two or
// more dot-separated labels of letters, digits, '_' and '-'.
var packageNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)+$`)

// AppKey is one mobile app identity of a widget (never the key itself).
type AppKey struct {
	UUID        uuid.UUID  `json:"uuid"`
	Platform    string     `json:"platform"`
	PackageName string     `json:"package_name"`
	Label       string     `json:"label"`
	KeyPrefix   string     `json:"key_prefix"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
}

// CreatedAppKey is the create response: the key is in it exactly once.
type CreatedAppKey struct {
	AppKey
	Key string `json:"key"`
}

// CreateAppKeyInput names the app.
type CreateAppKeyInput struct {
	Platform    string `json:"platform"`
	PackageName string `json:"package_name"`
	Label       string `json:"label"`
}

// AppIdentity is the app a visitor request came from (Widget.App).
type AppIdentity struct {
	Platform    string
	PackageName string
}

// InstallOrigin is what the install checker records for an app in place
// of a page origin: "app://android/com.example.shop".
func (a AppIdentity) InstallOrigin() string {
	return "app://" + a.Platform + "/" + strings.ToLower(a.PackageName)
}

func appKeyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func appKeyOf(r db.WidgetAppKey) AppKey {
	return AppKey{
		UUID: r.Uuid, Platform: r.Platform, PackageName: r.PackageName, Label: r.Label, KeyPrefix: r.KeyPrefix,
		CreatedAt: r.CreatedAt.Time.UTC(), LastUsedAt: timePtr(r.LastUsedAt), RevokedAt: timePtr(r.RevokedAt),
	}
}

// ValidateAppKeyInput normalizes and checks the platform and package name.
func ValidateAppKeyInput(in CreateAppKeyInput) (CreateAppKeyInput, error) {
	in.Platform = strings.ToLower(strings.TrimSpace(in.Platform))
	in.PackageName = strings.TrimSpace(in.PackageName)
	in.Label = strings.TrimSpace(in.Label)
	ok := false
	for _, p := range AppPlatforms {
		ok = ok || p == in.Platform
	}
	if !ok {
		return in, fmt.Errorf("%w: platform must be ios or android", ErrInvalid)
	}
	if len(in.PackageName) < 3 || len(in.PackageName) > 200 || !packageNameRe.MatchString(in.PackageName) {
		return in, fmt.Errorf("%w: package_name must be a bundle id or application id like com.example.app", ErrInvalid)
	}
	if len(in.Label) > 100 {
		return in, fmt.Errorf("%w: label must be at most 100 characters", ErrInvalid)
	}
	return in, nil
}

// ListAppKeys lists the widget's app keys, active first.
func (c *Configs) ListAppKeys(ctx context.Context, org int64, id uuid.UUID) ([]AppKey, error) {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return nil, err
	}
	rows, err := c.q.ListWidgetAppKeys(ctx, db.ListWidgetAppKeysParams{OrganizationID: org, ChannelInstanceID: row.ID})
	if err != nil {
		return nil, err
	}
	out := make([]AppKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, appKeyOf(r))
	}
	return out, nil
}

// CreateAppKey mints a key for an app and returns it once.
func (c *Configs) CreateAppKey(ctx context.Context, org, actor int64, id uuid.UUID, in CreateAppKeyInput) (CreatedAppKey, error) {
	in, err := ValidateAppKeyInput(in)
	if err != nil {
		return CreatedAppKey{}, err
	}
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return CreatedAppKey{}, err
	}
	existing, err := c.q.ListWidgetAppKeys(ctx, db.ListWidgetAppKeysParams{OrganizationID: org, ChannelInstanceID: row.ID})
	if err != nil {
		return CreatedAppKey{}, err
	}
	active := 0
	for _, k := range existing {
		if !k.RevokedAt.Valid {
			active++
		}
	}
	if active >= MaxAppKeys {
		return CreatedAppKey{}, fmt.Errorf("%w: a widget may have at most %d active app keys", ErrInvalid, MaxAppKeys)
	}
	raw := make([]byte, appKeyBytes)
	if _, err := rand.Read(raw); err != nil {
		return CreatedAppKey{}, err
	}
	key := AppKeyPrefix + hex.EncodeToString(raw)
	created, err := c.q.CreateWidgetAppKey(ctx, db.CreateWidgetAppKeyParams{
		OrganizationID: org, ChannelInstanceID: row.ID, Platform: in.Platform, PackageName: in.PackageName, Label: in.Label,
		KeyPrefix: key[:appKeyShown], KeyHash: appKeyHash(key), CreatedBy: optID(actor),
	})
	if err != nil {
		return CreatedAppKey{}, err
	}
	return CreatedAppKey{AppKey: appKeyOf(created), Key: key}, nil
}

// RevokeAppKey stops a key; revoking twice is not found.
func (c *Configs) RevokeAppKey(ctx context.Context, org int64, id, key uuid.UUID) (AppKey, error) {
	row, err := c.instance(ctx, org, id)
	if err != nil {
		return AppKey{}, err
	}
	r, err := c.q.RevokeWidgetAppKey(ctx, db.RevokeWidgetAppKeyParams{OrganizationID: org, ChannelInstanceID: row.ID, Uuid: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return AppKey{}, ErrNotFound
	}
	if err != nil {
		return AppKey{}, err
	}
	return appKeyOf(r), nil
}

// OpenApp loads a live widget for a native app: the key must hash to an
// active key of this widget whose package name matches. The origin
// allowlist is not consulted (an app has no page origin; the handler
// rejects a key that arrives together with one).
func (s *Service) OpenApp(ctx context.Context, publicID uuid.UUID, key, packageName string) (Widget, error) {
	w, err := s.load(ctx, publicID)
	if err != nil {
		return Widget{}, err
	}
	key = strings.TrimSpace(key)
	packageName = strings.TrimSpace(packageName)
	if !strings.HasPrefix(key, AppKeyPrefix) || len(key) > 200 || packageName == "" || len(packageName) > 200 {
		return Widget{}, ErrAppKeyInvalid
	}
	row, err := s.q.GetWidgetAppKeyByHash(ctx, appKeyHash(key))
	if errors.Is(err, pgx.ErrNoRows) {
		return Widget{}, ErrAppKeyInvalid
	}
	if err != nil {
		return Widget{}, err
	}
	// The hash lookup already proved the key; the remaining checks are
	// constant-time out of habit, not need.
	if row.ChannelInstanceID != w.Instance.ID || row.RevokedAt.Valid ||
		subtle.ConstantTimeCompare([]byte(strings.ToLower(row.PackageName)), []byte(strings.ToLower(packageName))) != 1 {
		return Widget{}, ErrAppKeyInvalid
	}
	if !row.LastUsedAt.Valid || s.now().Sub(row.LastUsedAt.Time) >= appKeyTouchInterval {
		if err := s.q.TouchWidgetAppKey(ctx, row.ID); err != nil {
			s.log.Warn("widget_app_key_touch_failed", "instance", w.Instance.UUID, "error", err)
		}
	}
	w.App = &AppIdentity{Platform: row.Platform, PackageName: row.PackageName}
	return w, nil
}
