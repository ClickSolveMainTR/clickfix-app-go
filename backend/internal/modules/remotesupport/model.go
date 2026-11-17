// Package remotesupport is the server side of remote support (CF-233,
// docs/design/masaustu.md §5 and §7): an agent starts a short-lived session
// from a conversation, the customer's desktop app (ClickFix Destek) resolves
// it with a 9-digit code or a one-time join link and gives separate consents
// for viewing, control and recording. Media flows through LiveKit (room
// rs_<uuid>): the customer only publishes the screen, the agent only
// subscribes, and the agent may publish data (input events) only while a
// control consent is in force. Every step is written to the organization's
// audit log.
//
//	POST /v1/tenant/conversations/{uuid}/remote-support   create (agent)
//	GET  /v1/tenant/conversations/{uuid}/remote-support   sessions of the conversation
//	GET  /v1/tenant/remote-support/{uuid}                 status
//	POST /v1/tenant/remote-support/{uuid}/token           agent LiveKit token
//	POST /v1/tenant/remote-support/{uuid}/end             end
//	GET|PUT /v1/tenant/remote-support/settings            organization settings
//	POST /v1/public/remote-support/resolve                code or join token
//	POST /v1/public/remote-support/{uuid}/consent         view / control / recording, revoke
//	POST /v1/public/remote-support/{uuid}/token           customer LiveKit token
//	POST /v1/public/remote-support/{uuid}/end             customer stops
//	POST /v1/public/remote-support/{uuid}/report          "this is a scam"
//	GET  /v1/platform/remote-support/organizations        platform overview
//	PUT  /v1/platform/remote-support/organizations/{uuid}/state  kill switch
//	GET  /v1/platform/remote-support/reports              reported sessions
package remotesupport

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalid       = errors.New("remote support: invalid input")
	ErrNotFound      = errors.New("remote support: not found")
	ErrDenied        = errors.New("remote support: forbidden")
	ErrConflict      = errors.New("remote support: conflicting state")
	ErrDisabled      = errors.New("remote support: disabled for the organization")
	ErrSuspended     = errors.New("remote support: suspended by the platform")
	ErrNotConfigured = errors.New("remote support: livekit not configured")
	ErrNoMinutes     = errors.New("remote support: no minutes left")
	ErrCodeInvalid   = errors.New("remote support: invalid or expired code")
	ErrUnauthorized  = errors.New("remote support: invalid client secret")
	ErrConsent       = errors.New("remote support: consent missing")
	ErrEnded         = errors.New("remote support: session ended")
	ErrRateLimited   = errors.New("remote support: too many attempts")
)

const (
	// CodeTTL is how long the code and the join link stay valid, and how
	// long a resolved session may wait for the viewing consent.
	CodeTTL = 15 * time.Minute
	// MaxCodeAttempts wrong codes charged to one session invalidate it.
	MaxCodeAttempts = 5
	// DefaultMaxDuration bounds a session unless the organization sets less
	// or more (5..240 minutes).
	DefaultMaxDuration = 60
	// Reports within ReportWindow reaching ReportThreshold suspend the
	// organization's remote support automatically.
	ReportWindow    = 24 * time.Hour
	ReportThreshold = 3

	FeatureEnabled = "remote_support.enabled"
	FeatureMinutes = "remote_support.minutes"

	// RoomPrefix marks LiveKit rooms (and their webhooks) of this module.
	RoomPrefix = "rs_"
	// ChannelPrefix is the Centrifugo namespace of session updates.
	ChannelPrefix = "remote_support:"

	identityAgent    = "rs:agent:"
	identityCustomer = "rs:customer:"
)

// Session states.
const (
	StatusPending   = "pending"
	StatusJoined    = "joined"
	StatusActive    = "active"
	StatusEnded     = "ended"
	StatusExpired   = "expired"
	StatusCancelled = "cancelled"
)

// Requested modes.
const (
	ModeView    = "view"
	ModeControl = "control"
)

// Who ended a session.
const (
	ByAgent    = "agent"
	ByCustomer = "customer"
	BySystem   = "system"
	ByPlatform = "platform"
)

// Platform states of an organization.
const (
	PlatformActive    = "active"
	PlatformDisabled  = "disabled"
	PlatformSuspended = "suspended"
)

// Channel is the Centrifugo channel of one session (agent side only).
func Channel(id uuid.UUID) string { return ChannelPrefix + id.String() }

// Person is the agent as the customer and the inbox see them.
type Person struct {
	UUID      uuid.UUID `json:"uuid"`
	Name      string    `json:"name"`
	AvatarURL *string   `json:"avatar_url"`
}

// SessionView is the agent-side projection (no secrets).
type SessionView struct {
	UUID               uuid.UUID  `json:"uuid"`
	ConversationUUID   uuid.UUID  `json:"conversation_uuid"`
	Status             string     `json:"status"`
	ModeRequested      string     `json:"mode_requested"`
	Agent              *Person    `json:"agent"`
	CodeExpiresAt      time.Time  `json:"code_expires_at"`
	ResolvedAt         *time.Time `json:"resolved_at"`
	ViewConsentAt      *time.Time `json:"view_consent_at"`
	ControlConsentAt   *time.Time `json:"control_consent_at"`
	ControlRevokedAt   *time.Time `json:"control_revoked_at"`
	ControlActive      bool       `json:"control_active"`
	RecordingConsent   *bool      `json:"recording_consent"`
	RecordingStatus    string     `json:"recording_status"`
	ClientOS           string     `json:"client_os"`
	ClientVersion      string     `json:"client_version"`
	MaxDurationMinutes int32      `json:"max_duration_minutes"`
	StartedAt          *time.Time `json:"started_at"`
	EndsAt             *time.Time `json:"ends_at"`
	EndedAt            *time.Time `json:"ended_at"`
	EndedReason        string     `json:"ended_reason"`
	EndedBy            string     `json:"ended_by"`
	ReportedAt         *time.Time `json:"reported_at"`
	BilledMinutes      int64      `json:"billed_minutes"`
	Channel            string     `json:"channel"`
	CreatedAt          time.Time  `json:"created_at"`
}

// Created is returned once, to the agent who created the session: the code
// and join link are never shown again.
type Created struct {
	Session       SessionView `json:"session"`
	Code          string      `json:"code"`
	JoinURL       string      `json:"join_url"`
	DownloadURL   string      `json:"download_url"`
	MessagePosted bool        `json:"message_posted"`
}

type CreateInput struct {
	Mode string `json:"mode"`
}

// Grant is a LiveKit connection for one side.
type Grant struct {
	ServerURL      string      `json:"server_url"`
	Token          string      `json:"token"`
	Identity       string      `json:"identity"`
	Room           string      `json:"room"`
	CanPublishData bool        `json:"can_publish_data"`
	ExpiresAt      time.Time   `json:"expires_at"`
	Session        SessionView `json:"session"`
}

// ResolveInput: exactly one of code / token.
type ResolveInput struct {
	Code          string `json:"code"`
	Token         string `json:"token"`
	ClientOS      string `json:"client_os"`
	ClientVersion string `json:"client_version"`
}

// Organization is how the customer app shows the business.
type Organization struct {
	UUID     uuid.UUID `json:"uuid"`
	Name     string    `json:"name"`
	LogoURL  *string   `json:"logo_url"`
	Verified bool      `json:"verified"`
}

// Resolved is what the customer app shows before asking for consent. The
// client secret authenticates its later calls and is returned only here.
type Resolved struct {
	SessionUUID        uuid.UUID    `json:"session_uuid"`
	ClientSecret       string       `json:"client_secret"`
	Organization       Organization `json:"organization"`
	Agent              Person       `json:"agent"`
	ModeRequested      string       `json:"mode_requested"`
	RecordingRequested bool         `json:"recording_requested"`
	MaxDurationMinutes int32        `json:"max_duration_minutes"`
	ConsentDeadline    time.Time    `json:"consent_deadline"`
	Warning            string       `json:"warning"`
	Locale             string       `json:"locale"`
}

// ClientInput authenticates a customer call.
type ClientInput struct {
	ClientSecret string `json:"client_secret"`
}

// ConsentInput: view can only be given (stopping is /end); control and
// recording can be given (true) or withdrawn (false).
type ConsentInput struct {
	ClientSecret string `json:"client_secret"`
	View         *bool  `json:"view"`
	Control      *bool  `json:"control"`
	Recording    *bool  `json:"recording"`
}

type ReportInput struct {
	ClientSecret string `json:"client_secret"`
	Reason       string `json:"reason"`
}

// CustomerView is the customer-side projection.
type CustomerView struct {
	SessionUUID      uuid.UUID  `json:"session_uuid"`
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

// Settings is the organization configuration plus the platform state.
type Settings struct {
	Enabled                bool       `json:"enabled"`
	AllowControl           bool       `json:"allow_control"`
	Recording              string     `json:"recording"`
	MaxDurationMinutes     int32      `json:"max_duration_minutes"`
	RecordingRetentionDays int32      `json:"recording_retention_days"`
	PlatformState          string     `json:"platform_state"`
	PlatformReason         string     `json:"platform_reason"`
	PlatformChangedAt      *time.Time `json:"platform_changed_at"`
	Configured             bool       `json:"configured"`
	Entitled               bool       `json:"entitled"`
	Ready                  bool       `json:"ready"`
}

type SettingsInput struct {
	Enabled                bool   `json:"enabled"`
	AllowControl           bool   `json:"allow_control"`
	Recording              string `json:"recording"`
	MaxDurationMinutes     int32  `json:"max_duration_minutes"`
	RecordingRetentionDays int32  `json:"recording_retention_days"`
}

type PlatformStateInput struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

type PlatformOrganization struct {
	UUID              uuid.UUID  `json:"uuid"`
	Name              string     `json:"name"`
	Slug              string     `json:"slug"`
	Enabled           bool       `json:"enabled"`
	PlatformState     string     `json:"platform_state"`
	PlatformReason    string     `json:"platform_reason"`
	PlatformChangedAt *time.Time `json:"platform_changed_at"`
	Reports30d        int64      `json:"reports_30d"`
	Sessions30d       int64      `json:"sessions_30d"`
}

type PlatformReport struct {
	SessionUUID      uuid.UUID `json:"session_uuid"`
	Status           string    `json:"status"`
	ModeRequested    string    `json:"mode_requested"`
	ReportedAt       time.Time `json:"reported_at"`
	ReportReason     string    `json:"report_reason"`
	EndedReason      string    `json:"ended_reason"`
	CreatedAt        time.Time `json:"created_at"`
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
	OrganizationName string    `json:"organization_name"`
	AgentName        string    `json:"agent_name"`
}
