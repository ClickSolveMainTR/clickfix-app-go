package providers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/mail"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
	"log/slog"
)

// DeliveryResult is the outcome of a provider send.
type DeliveryResult struct {
	Status            string
	Provider          string
	ProviderReference string
}

// Provider delivers a notification on a channel.
type Provider interface {
	Channel() string
	Deliver(ctx context.Context, n db.Notification, userUUID *uuid.UUID) (DeliveryResult, error)
}

// InappProvider marks in-app rows as delivered (row itself is the inbox).
type InappProvider struct{}

func (InappProvider) Channel() string { return model.ChannelInapp }

func (InappProvider) Deliver(_ context.Context, _ db.Notification, _ *uuid.UUID) (DeliveryResult, error) {
	return DeliveryResult{Status: model.StatusDelivered, Provider: "inapp"}, nil
}

// EmailProvider sends via SMTP in the shared branded layout (CF-155).
type EmailProvider struct {
	Mail mail.Sender
	// AppURL is the public frontend origin: relative action links resolve
	// against it and the footer links to its notification preferences.
	AppURL string
}

func (EmailProvider) Channel() string { return model.ChannelEmail }

func (p EmailProvider) Deliver(ctx context.Context, n db.Notification, _ *uuid.UUID) (DeliveryResult, error) {
	to := ""
	if n.Recipient.Valid {
		to = n.Recipient.String
	}
	if to == "" {
		return DeliveryResult{}, ErrNoRecipient
	}
	if p.Mail == nil {
		p.Mail = mail.NoopSender{}
	}
	msg, err := mail.Branded([]string{to}, p.layout(n))
	if err != nil {
		return DeliveryResult{}, err
	}
	if err := p.Mail.Send(ctx, msg); err != nil {
		return DeliveryResult{}, err
	}
	return DeliveryResult{Status: model.StatusSent, Provider: "smtp"}, nil
}

func (p EmailProvider) layout(n db.Notification) mail.Layout {
	lang := "tr"
	var payload map[string]any
	if len(n.Payload) > 0 && json.Unmarshal(n.Payload, &payload) == nil {
		if l, ok := payload["locale"].(string); ok && strings.HasPrefix(l, "en") {
			lang = "en"
		}
	}
	event := n.SourceEvent.String
	if event == "" {
		event = n.TemplateCode.String
	}
	l := mail.Layout{Lang: lang, Title: n.Title, Body: n.Body, Eyebrow: eyebrow(event, lang)}
	if n.ActionUrl.Valid {
		l.ActionURL = p.absolute(n.ActionUrl.String)
	}
	if label, ok := actionLabels[event]; ok {
		l.ActionLabel = label[lang]
	}
	// Enriched notifications (CF-157) name their own primary action.
	if label, ok := payload["action_label"].(string); ok && strings.TrimSpace(label) != "" {
		l.ActionLabel = label
	}
	if n.UserID.Valid {
		l.PreferencesURL = p.absolute("/profile")
	}
	return l
}

func (p EmailProvider) absolute(link string) string {
	link = strings.TrimSpace(link)
	if strings.HasPrefix(link, "/") && !strings.HasPrefix(link, "//") && p.AppURL != "" {
		return strings.TrimRight(p.AppURL, "/") + link
	}
	return link
}

// eyebrows label an email by the module that sent it (first segment of the
// source event or template code).
var eyebrows = map[string]map[string]string{
	"appointment":   {"tr": "Randevu", "en": "Appointment"},
	"auth":          {"tr": "Hesap", "en": "Account"},
	"automation":    {"tr": "Otomasyon", "en": "Automation"},
	"billing":       {"tr": "Faturalama", "en": "Billing"},
	"bulk":          {"tr": "Toplu işlem", "en": "Bulk action"},
	"calls":         {"tr": "Arama", "en": "Call"},
	"company":       {"tr": "Müşteri", "en": "Customer"},
	"conversation":  {"tr": "Konuşma", "en": "Conversation"},
	"csat":          {"tr": "Memnuniyet", "en": "Satisfaction"},
	"exports":       {"tr": "Dışa aktarım", "en": "Export"},
	"imports":       {"tr": "İçe aktarım", "en": "Import"},
	"message":       {"tr": "Mesaj", "en": "Message"},
	"notifications": {"tr": "Bildirim", "en": "Notification"},
	"oncall":        {"tr": "Nöbet", "en": "On-call"},
	"organizations": {"tr": "Ekip daveti", "en": "Team invitation"},
	"reports":       {"tr": "Rapor", "en": "Report"},
	"sla":           {"tr": "SLA", "en": "SLA"},
	"status":        {"tr": "Durum sayfası", "en": "Status page"},
	"teamchat":      {"tr": "Ekip sohbeti", "en": "Team chat"},
	"ticket":        {"tr": "Talep", "en": "Ticket"},
	"video":         {"tr": "Görüntülü görüşme", "en": "Video call"},
}

var actionLabels = map[string]map[string]string{
	"organizations.invitation": {"tr": "Daveti kabul et", "en": "Accept invitation"},
}

func eyebrow(event, lang string) string {
	prefix, _, _ := strings.Cut(event, ".")
	if m, ok := eyebrows[prefix]; ok {
		return m[lang]
	}
	return ""
}

// RealtimeProvider publishes to Centrifugo user channel.
type RealtimeProvider struct {
	Pub realtime.Publisher
}

func (RealtimeProvider) Channel() string { return model.ChannelRealtime }

func (p RealtimeProvider) Deliver(ctx context.Context, n db.Notification, userUUID *uuid.UUID) (DeliveryResult, error) {
	if p.Pub == nil {
		p.Pub = realtime.NoopPublisher{}
	}
	channel := ""
	if userUUID != nil {
		channel = "user:" + userUUID.String()
	}
	if channel == "" {
		return DeliveryResult{Status: model.StatusSent, Provider: "centrifugo"}, nil
	}
	notification := map[string]any{
		"uuid": n.Uuid, "channel": n.Channel, "title": n.Title, "body": n.Body,
		"status": n.Status, "priority": n.Priority,
	}
	if n.ActionUrl.Valid && n.ActionUrl.String != "" {
		notification["action_url"] = n.ActionUrl.String
	}
	if n.TemplateCode.Valid && n.TemplateCode.String != "" {
		notification["template_code"] = n.TemplateCode.String
	}
	if len(n.Payload) > 0 {
		var extra map[string]any
		if err := json.Unmarshal(n.Payload, &extra); err == nil && len(extra) > 0 {
			notification["payload"] = extra
		}
	}
	payload := map[string]any{
		"type":         "notification.created",
		"notification": notification,
	}
	if err := p.Pub.Publish(ctx, channel, payload); err != nil {
		return DeliveryResult{}, err
	}
	return DeliveryResult{Status: model.StatusSent, Provider: "centrifugo"}, nil
}

// NoopProvider logs and marks sms/push as sent.
type NoopProvider struct {
	Name string
	Log  *slog.Logger
}

func (p NoopProvider) Channel() string { return p.Name }

func (p NoopProvider) Deliver(_ context.Context, n db.Notification, _ *uuid.UUID) (DeliveryResult, error) {
	if p.Log != nil {
		p.Log.Info("notification_noop_provider", "channel", p.Name, "uuid", n.Uuid)
	}
	return DeliveryResult{Status: model.StatusSent, Provider: "noop"}, nil
}

// ErrNoRecipient is returned when email has no recipient.
var ErrNoRecipient = errString("notification recipient required")

type errString string

func (e errString) Error() string { return string(e) }
