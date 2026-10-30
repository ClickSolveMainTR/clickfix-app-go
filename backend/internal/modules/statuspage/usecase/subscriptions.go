package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	notif "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"strconv"
	"strings"
	"time"
)

func fmtInt(n int64) string { return strconv.FormatInt(n, 10) }

type subClaims struct {
	Org     int64     `json:"o"`
	ID      uuid.UUID `json:"i"`
	Expires int64     `json:"e"`
	Action  string    `json:"a"`
}

func (s *Service) signSubscription(c subClaims) string {
	raw, _ := json.Marshal(c)
	enc := base64.RawURLEncoding.EncodeToString(raw)
	return enc + "." + s.subMAC(enc)
}
func (s *Service) subMAC(enc string) string {
	h := hmac.New(sha256.New, []byte(s.secret))
	_, _ = h.Write([]byte("clickfix/status-subscription/v1:" + enc))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// SubscriptionPreview says what a subscription link does (confirm |
// unsubscribe) and on which page, without doing it. E-mail links open a
// confirmation page that shows this; only its button (POST) acts, so link
// scanners and previews cannot confirm or cancel a subscription (CF-157).
type SubscriptionPreview struct {
	Action    string `json:"action"`
	PageTitle string `json:"page_title"`
}

func (s *Service) subscriptionClaims(ctx context.Context, slug, token string) (subClaims, db.StatusPage, error) {
	enc, sig, ok := strings.Cut(token, ".")
	if !ok || len(token) > 1024 || !hmac.Equal([]byte(sig), []byte(s.subMAC(enc))) {
		return subClaims{}, db.StatusPage{}, model.ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	var c subClaims
	if err != nil || json.Unmarshal(raw, &c) != nil || c.ID == uuid.Nil {
		return subClaims{}, db.StatusPage{}, model.ErrInvalid
	}
	p, err := s.repo.Q.StatusPublicPage(ctx, slug)
	if err != nil {
		return subClaims{}, db.StatusPage{}, err
	}
	if c.Org != p.OrganizationID {
		return subClaims{}, db.StatusPage{}, model.ErrInvalid
	}
	if c.Action != "unsubscribe" && (c.Action != "confirm" || time.Now().Unix() >= c.Expires) {
		return subClaims{}, db.StatusPage{}, model.ErrInvalid
	}
	return c, p, nil
}

// PreviewSubscription validates a link token and returns what it would do.
func (s *Service) PreviewSubscription(ctx context.Context, slug, token string) (SubscriptionPreview, error) {
	c, p, err := s.subscriptionClaims(ctx, slug, token)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	return SubscriptionPreview{Action: c.Action, PageTitle: p.Title}, nil
}

// SubscriptionLink is the confirmation page of a token (web app).
func (s *Service) SubscriptionLink(slug, token string) string {
	return s.baseURL + "/status/" + slug + "/subscription/" + token
}

// SubscriptionAction confirms or cancels the subscription of a token.
func (s *Service) SubscriptionAction(ctx context.Context, slug, token string) error {
	c, _, err := s.subscriptionClaims(ctx, slug, token)
	if err != nil {
		return err
	}
	if c.Action == "unsubscribe" {
		return s.repo.Q.StatusUnsubscribe(ctx, db.StatusUnsubscribeParams{OrganizationID: c.Org, Uuid: c.ID})
	}
	if c.Action != "confirm" || time.Now().Unix() >= c.Expires {
		return model.ErrInvalid
	}
	n, err := s.repo.Q.StatusConfirmSubscription(ctx, db.StatusConfirmSubscriptionParams{OrganizationID: c.Org, Uuid: c.ID})
	if err != nil {
		return err
	}
	if n == 0 {
		return model.ErrInvalid
	}
	return nil
}
func (s *Service) SendConfirmation(ctx context.Context, org int64, id uuid.UUID) error {
	row, err := s.repo.Q.StatusSubscription(ctx, db.StatusSubscriptionParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // removed before the event ran
	}
	if err != nil {
		return err
	}
	if row.ConfirmedAt.Valid && !row.UnsubscribedAt.Valid {
		return nil
	}
	if !row.ExpiresAt.Time.After(time.Now()) {
		return nil
	}
	p, err := s.Page(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // no status page set up: nobody to notify
	}
	if err != nil {
		return err
	}
	if !p.Published {
		return nil
	}
	if s.notifier == nil {
		return model.ErrInvalid
	}
	token := s.signSubscription(subClaims{Org: org, ID: id, Expires: row.ExpiresAt.Time.Unix(), Action: "confirm"})
	link := s.SubscriptionLink(p.Slug, token)
	body := p.Title + " durum sayfasının e-posta bildirimlerine abone olmak için bu adres kullanıldı. " +
		"Aboneliği onaylamak için aşağıdaki bağlantıyı açıp onay düğmesine basın. Bu isteği siz yapmadıysanız e-postayı yok sayabilirsiniz; onaylanmayan abonelik 24 saat içinde düşer.\n\n" +
		"Aboneliği onayla: " + link
	_, err = s.notifier.Enqueue(ctx, notif.EnqueueInput{TenantID: &org, Channels: []string{notif.ChannelEmail}, Recipient: &row.Email, Title: p.Title + " — Abonelik onayı", Body: body, SourceEvent: "status.subscription_requested"})
	return err
}
func (s *Service) NotifySubscribers(ctx context.Context, org int64, incident uuid.UUID, body string) error {
	p, err := s.Page(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // no status page set up: nobody to notify
	}
	if err != nil {
		return err
	}
	if !p.Published {
		return nil
	}
	if s.notifier == nil {
		return model.ErrInvalid
	}
	var after int64
	for {
		rows, err := s.repo.Q.StatusSubscribers(ctx, db.StatusSubscribersParams{OrganizationID: org, AfterID: after})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, r := range rows {
			token := s.signSubscription(subClaims{Org: org, ID: r.Uuid, Action: "unsubscribe"})
			link := s.SubscriptionLink(p.Slug, token)
			_, err = s.notifier.Enqueue(ctx, notif.EnqueueInput{TenantID: &org, Channels: []string{notif.ChannelEmail}, Recipient: &r.Email, Title: p.Title + " — Durum güncellemesi",
				Body: body + "\n\nDurum sayfası: " + s.PublicURL(p.Slug) + "\nAbonelikten ayrıl: " + link, SourceEvent: "status.changed", Payload: map[string]any{"incident_uuid": incident.String()}})
			if err != nil {
				return err
			}
			after = r.ID
		}
	}
}
