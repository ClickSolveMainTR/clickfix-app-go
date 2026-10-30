package usecase

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/netguard"
)

// pushClient sends Web Push requests: the endpoint URL comes from the browser,
// so only public addresses are dialed and redirects are not followed (CF-153).
var pushClient = netguard.Client(15 * time.Second)

// WebPushContent is the browser push of a notification row (CF-157): the
// same lock-screen-safe text as the mobile push (never the in-app
// title/body), the category, and the notification actions the service
// worker shows (app-push-sw.js): "read" marks the row read, "open" (or a
// click) opens data.action_url.
func WebPushContent(row db.Notification, userLocale string) (string, string, map[string]any) {
	var payload map[string]any
	_ = json.Unmarshal(row.Payload, &payload)
	kind, _ := payload["kind"].(string)
	if kind == "" {
		kind = row.SourceEvent.String
	}
	if kind == "" {
		kind = row.TemplateCode.String
	}
	vars := map[string]string{}
	if pv, ok := payload["push_vars"].(map[string]any); ok {
		for k, v := range pv {
			if sv, ok := v.(string); ok {
				vars[k] = sv
			}
		}
	}
	locale := normalizePushLocale(userLocale)
	if locale == "" {
		locale = "tr"
	}
	txt := lockScreenText(payload, kind, locale, vars)
	category, _ := payload["category"].(string)
	if category == "" {
		category = PushCategoryGeneric
	}
	data := map[string]any{
		"notification_uuid": row.Uuid.String(),
		"category":          category,
		// One visible notification per row: a redelivery replaces it.
		"tag": "notification:" + row.Uuid.String(),
	}
	if kind != "" {
		data["kind"] = kind
	}
	actions := []map[string]string{{"action": "read", "title": webActionTitle(locale, "read")}}
	if row.ActionUrl.Valid && strings.HasPrefix(row.ActionUrl.String, "/") && !strings.HasPrefix(row.ActionUrl.String, "//") {
		data["action_url"] = row.ActionUrl.String
		actions = append([]map[string]string{{"action": "open", "title": webActionTitle(locale, "open")}}, actions...)
	}
	// Browsers show at most two buttons.
	data["actions"] = actions
	return txt.Title, txt.Body, data
}

func webActionTitle(locale, action string) string {
	titles := map[string][2]string{"open": {"Aç", "Open"}, "read": {"Okundu", "Mark read"}}
	if locale == "en" {
		return titles[action][1]
	}
	return titles[action][0]
}

// VAPIDConfig holds Web Push VAPID keys.
type VAPIDConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// WithVAPID enables web push when both keys are set.
func (s *Service) WithVAPID(cfg VAPIDConfig) *Service {
	if cfg.PublicKey != "" && cfg.PrivateKey != "" {
		s.vapid = &cfg
	}
	return s
}

// VAPIDPublicKey returns the configured public key (may be empty).
func (s *Service) VAPIDPublicKey() string {
	if s == nil || s.vapid == nil {
		return ""
	}
	return s.vapid.PublicKey
}

// UpsertPushSubscription stores a browser push subscription for the user.
func (s *Service) UpsertPushSubscription(ctx context.Context, userID int64, endpoint, p256dh, auth string) (db.PushSubscription, error) {
	q, ok := s.q.(interface {
		UpsertPushSubscription(context.Context, db.UpsertPushSubscriptionParams) (db.PushSubscription, error)
	})
	if !ok {
		return db.PushSubscription{}, ErrInvalidRequest
	}
	return q.UpsertPushSubscription(ctx, db.UpsertPushSubscriptionParams{
		UserID:    userID,
		Endpoint:  endpoint,
		KeyP256dh: p256dh,
		KeyAuth:   auth,
	})
}

// DeletePushSubscription removes a subscription endpoint for the user.
func (s *Service) DeletePushSubscription(ctx context.Context, userID int64, endpoint string) error {
	q, ok := s.q.(interface {
		DeletePushSubscription(context.Context, db.DeletePushSubscriptionParams) error
	})
	if !ok {
		return ErrInvalidRequest
	}
	return q.DeletePushSubscription(ctx, db.DeletePushSubscriptionParams{
		UserID:   userID,
		Endpoint: endpoint,
	})
}

func (s *Service) pushBestEffort(ctx context.Context, userID int64, title, body string, data map[string]any) {
	if s.vapid == nil || userID == 0 {
		return
	}
	q, ok := s.q.(interface {
		GetNotificationPreferences(context.Context, int64) (db.NotificationPreference, error)
		ListPushSubscriptionsByUser(context.Context, int64) ([]db.PushSubscription, error)
	})
	if !ok {
		return
	}
	prefs, err := q.GetNotificationPreferences(ctx, userID)
	if err != nil || !prefs.PushEnabled {
		return
	}
	subs, err := q.ListPushSubscriptionsByUser(ctx, userID)
	if err != nil || len(subs) == 0 {
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	payload, _ := json.Marshal(map[string]any{
		"title": title,
		"body":  body,
		"data":  data,
	})
	for _, sub := range subs {
		resp, pushErr := webpush.SendNotification(payload, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.KeyP256dh,
				Auth:   sub.KeyAuth,
			},
		}, &webpush.Options{
			VAPIDPublicKey:  s.vapid.PublicKey,
			VAPIDPrivateKey: s.vapid.PrivateKey,
			Subscriber:      s.vapid.Subject,
			TTL:             60,
			HTTPClient:      pushClient,
		})
		if pushErr != nil {
			// The endpoint path is a bearer capability: log the push service host only.
			host := ""
			if u, err := url.Parse(sub.Endpoint); err == nil {
				host = u.Host
			}
			s.log.Error("webpush_failed", "push_service", host, "error", pushErr)
			continue
		}
		_ = resp.Body.Close()
	}
}
