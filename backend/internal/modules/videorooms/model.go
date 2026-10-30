package videorooms

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalid             = errors.New("video: invalid input")
	ErrNotFound            = errors.New("video: not found")
	ErrDenied              = errors.New("video: forbidden")
	ErrConflict            = errors.New("video: conflicting state")
	ErrDisabled            = errors.New("video: module disabled")
	ErrNotConfigured       = errors.New("video: livekit not configured")
	ErrNoMinutes           = errors.New("video: no video minutes left")
	ErrPINRequired         = errors.New("video: pin required")
	ErrPINInvalid          = errors.New("video: wrong pin")
	ErrRecordingNotAllowed = errors.New("video: recording not allowed")
	ErrRoomFull            = errors.New("video: room full")
	ErrRoomEnded           = errors.New("video: room ended")
	ErrLinkInvalid         = errors.New("video: invalid link")
	ErrLinkExpired         = errors.New("video: link expired")
	ErrLoginRequired       = errors.New("video: login required")
	ErrRateLimited         = errors.New("video: too many attempts")
	ErrRecorderUnavailable = errors.New("video: recording service unavailable")
)

// Participant roles inside a room.
const (
	RoleHost  = "host"
	RoleAgent = "agent"
	RoleGuest = "guest"
)

// Participant lobby states.
const (
	StatusWaiting  = "waiting"
	StatusAdmitted = "admitted"
	StatusDenied   = "denied"
	StatusRemoved  = "removed"
)

// UnitVideoMinutes is the wallet unit rooms consume (participant-minutes).
const UnitVideoMinutes = "video_minutes"

// CreateRoomInput is the tenant API body for a new room.
type CreateRoomInput struct {
	Name               string     `json:"name"`
	PIN                string     `json:"pin"`
	Lobby              *bool      `json:"lobby"`
	GuestJoin          *bool      `json:"guest_join"`
	E2EE               *bool      `json:"e2ee"`
	Record             bool       `json:"record"`
	AllowCamera        *bool      `json:"allow_camera"`
	AllowScreenShare   *bool      `json:"allow_screen_share"`
	AllowChat          *bool      `json:"allow_chat"`
	MaxParticipants    *int32     `json:"max_participants"`
	MaxDurationMinutes *int32     `json:"max_duration_minutes"`
	ScheduledStartAt   *time.Time `json:"scheduled_start_at"`
	ScheduledEndAt     *time.Time `json:"scheduled_end_at"`
	ConversationUUID   *uuid.UUID `json:"conversation_uuid"`
	AppointmentUUID    *uuid.UUID `json:"appointment_uuid"`
	HostUUID           *uuid.UUID `json:"host_uuid"`
}

// AppointmentRoomInput is the small contract CF-138 appointments call to get
// (or reuse) the room of an appointment.
type AppointmentRoomInput struct {
	Name       string     `json:"name"`
	StartsAt   time.Time  `json:"starts_at"`
	EndsAt     time.Time  `json:"ends_at"`
	HostUUID   *uuid.UUID `json:"host_uuid"`
	GuestEmail string     `json:"guest_email"`
	GuestName  string     `json:"guest_name"`
	Locale     string     `json:"locale"`
	PIN        string     `json:"pin"`
}

type LinkInput struct {
	Role       string `json:"role"`
	TTLMinutes int32  `json:"ttl_minutes"`
}

type InviteInput struct {
	Email  string `json:"email"`
	Name   string `json:"name"`
	Locale string `json:"locale"`
}

type GuestJoinInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	PIN   string `json:"pin"`
}

type TicketInput struct {
	Ticket string `json:"ticket"`
}

// JoinGrant is what an admitted participant receives: the LiveKit token and,
// for an encrypted room, the shared E2EE key. Nothing secret is returned
// while the participant waits in the lobby.
type JoinGrant struct {
	Status          string    `json:"status"`
	Ticket          string    `json:"ticket,omitempty"`
	ParticipantUUID uuid.UUID `json:"participant_uuid"`
	Role            string    `json:"role"`
	ServerURL       string    `json:"server_url,omitempty"`
	Token           string    `json:"token,omitempty"`
	E2EEKey         string    `json:"e2ee_key,omitempty"`
	Room            RoomView  `json:"room"`
}

// RoomView is the public projection of a room (no secrets).
type RoomView struct {
	UUID               uuid.UUID  `json:"uuid"`
	Name               string     `json:"name"`
	OrganizationName   string     `json:"organization_name"`
	Status             string     `json:"status"`
	PINRequired        bool       `json:"pin_required"`
	Lobby              bool       `json:"lobby"`
	GuestJoin          bool       `json:"guest_join"`
	E2EE               bool       `json:"e2ee"`
	Recording          bool       `json:"recording"`
	AllowCamera        bool       `json:"allow_camera"`
	AllowScreenShare   bool       `json:"allow_screen_share"`
	AllowChat          bool       `json:"allow_chat"`
	MaxParticipants    int32      `json:"max_participants"`
	MaxDurationMinutes int32      `json:"max_duration_minutes"`
	ScheduledStartAt   *time.Time `json:"scheduled_start_at"`
	ScheduledEndAt     *time.Time `json:"scheduled_end_at"`
	LinkRole           string     `json:"link_role,omitempty"`
}

// StorageInput configures where recordings go. Keys are write-only: empty
// keeps the stored value.
type StorageInput struct {
	Mode      string `json:"mode"`
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	PathStyle bool   `json:"path_style"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

type OrgSettingsInput struct {
	Enabled bool         `json:"enabled"`
	Policy  Partial      `json:"policy"`
	Storage StorageInput `json:"storage"`
}

type PlatformSettingsInput struct {
	Policy Policy `json:"policy"`
}
