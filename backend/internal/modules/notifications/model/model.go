package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	ChannelInapp    = "inapp"
	ChannelEmail    = "email"
	ChannelRealtime = "realtime"
	ChannelSMS      = "sms"
	ChannelPush     = "push"

	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusSent       = "sent"
	StatusDelivered  = "delivered"
	StatusRead       = "read"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"

	PriorityLow      = "low"
	PriorityNormal   = "normal"
	PriorityHigh     = "high"
	PriorityCritical = "critical"
)

// Notification is the API projection.
type Notification struct {
	UUID            uuid.UUID         `json:"uuid"`
	Channel         string            `json:"channel"`
	Status          string            `json:"status"`
	Priority        string            `json:"priority"`
	Title           string            `json:"title"`
	Body            string            `json:"body"`
	Payload         map[string]any    `json:"payload"`
	ActionURL       *string           `json:"action_url,omitempty"`
	SignedActionURL *string           `json:"signed_action_url,omitempty"`
	Recipient       *string           `json:"recipient,omitempty"`
	TemplateCode    *string           `json:"template_code,omitempty"`
	SourceEvent     *string           `json:"source_event,omitempty"`
	SentAt          *time.Time        `json:"sent_at,omitempty"`
	ReadAt          *time.Time        `json:"read_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	User            *NotificationUser `json:"user,omitempty"`
}

// NotificationUser is the target user on a platform notification row.
type NotificationUser struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// PushDevice is a registered mobile device (Expo push token).
type PushDevice struct {
	UUID       uuid.UUID `json:"uuid"`
	Token      string    `json:"token"`
	Platform   string    `json:"platform"`
	Locale     string    `json:"locale"`
	AppVersion string    `json:"app_version"`
	DeviceName string    `json:"device_name"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// Preferences is the user notification preference set. The *_enabled
// flags are master switches per channel; EventChannels refines them per
// event type (missing entries use the catalog defaults).
type Preferences struct {
	EmailEnabled    bool                       `json:"email_enabled"`
	InappEnabled    bool                       `json:"inapp_enabled"`
	RealtimeEnabled bool                       `json:"realtime_enabled"`
	PushEnabled     bool                       `json:"push_enabled"`
	EventChannels   map[string]map[string]bool `json:"event_channels"`
	QuietHours      QuietHours                 `json:"quiet_hours"`
	EmailDigest     string                     `json:"email_digest"`
	DigestHour      int                        `json:"digest_hour"`
}

// QuietHours mutes push and holds e-mail between Start and End (HH:MM in
// Timezone; may wrap midnight). Critical notifications are not held.
type QuietHours struct {
	Enabled  bool   `json:"enabled"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
}

// PreferencesUpdate is the PUT body; omitted fields keep their value.
// EventChannels entries are merged per event type.
type PreferencesUpdate struct {
	EmailEnabled    *bool                      `json:"email_enabled"`
	InappEnabled    *bool                      `json:"inapp_enabled"`
	RealtimeEnabled *bool                      `json:"realtime_enabled"`
	PushEnabled     *bool                      `json:"push_enabled"`
	EventChannels   map[string]map[string]bool `json:"event_channels"`
	QuietHours      *QuietHours                `json:"quiet_hours"`
	EmailDigest     *string                    `json:"email_digest"`
	DigestHour      *int                       `json:"digest_hour"`
}

// Preference channels of the event matrix. Push covers web push and the
// Expo mobile push mirror.
const (
	PrefChannelInapp = "inapp"
	PrefChannelEmail = "email"
	PrefChannelPush  = "push"
)

// Digest frequencies.
const (
	DigestOff    = "off"
	DigestHourly = "hourly"
	DigestDaily  = "daily"
)

// EventType is one notifiable event in the preference matrix.
type EventType struct {
	Key      string          `json:"key"`
	Group    string          `json:"group"`
	Defaults map[string]bool `json:"defaults"`
	// Locked channels are always on (cannot be turned off per event).
	Locked   []string `json:"locked"`
	Priority string   `json:"priority"`
}

// EnqueueInput creates one or more queued notifications.
type EnqueueInput struct {
	TenantID     *int64
	WorkspaceID  *int64
	UserID       *int64
	UserUUID     *uuid.UUID
	Channels     []string
	Priority     string
	Title        string
	Body         string
	Payload      map[string]any
	ActionURL    *string
	Recipient    *string
	TemplateCode string
	TemplateVars map[string]string
	SourceEvent  string
	Language     string
	// SecurityEmail bypasses email preference (password reset / verification).
	SecurityEmail bool
	// ContentVars are the facts of SourceEvent (content.Vars keys). When the
	// event has a content builder they replace Title/Body and the template
	// on every channel with the enriched text in the recipient's language
	// (CF-157).
	ContentVars map[string]string
	// DedupeKey makes the enqueue idempotent per user and channel (stored as
	// payload.source_event_id; a repeat returns the existing row and is not
	// delivered again). Usually the domain event id.
	DedupeKey string
}
