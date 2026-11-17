package support

import (
	"time"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/api"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/remote"
)

// Phase, state.phase değeridir (IPC.md "Durum modeli").
type Phase string

const (
	PhaseIdle         Phase = "idle"
	PhaseResolved     Phase = "resolved"
	PhaseReady        Phase = "ready"
	PhaseStarting     Phase = "starting"
	PhaseActive       Phase = "active"
	PhaseReconnecting Phase = "reconnecting"
	PhaseEnding       Phase = "ending"
	PhaseEnded        Phase = "ended"
)

// Control, state.control değeridir.
type Control string

const (
	ControlViewOnly          Control = "view_only"
	ControlRequested         Control = "requested"
	ControlPendingPermission Control = "pending_permission"
	ControlActive            Control = "active"
	ControlRevoked           Control = "revoked"
)

// Kontrol olayının by değerleri.
const (
	ByCustomer = "customer"
	ByAgent    = "agent"
	BySystem   = "system"
)

// Permissions, permissions.status yanıtıdır.
type Permissions struct {
	Platform        string             `json:"platform"`
	ScreenRecording capture.Permission `json:"screen_recording"`
	Accessibility   capture.Permission `json:"accessibility"`
	RestartRequired bool               `json:"restart_required"`
}

// SessionView, state.session nesnesidir: çözülen oturum, sır olmadan.
type SessionView struct {
	UUID               string           `json:"uuid"`
	Organization       api.Organization `json:"organization"`
	Agent              api.Person       `json:"agent"`
	ModeRequested      string           `json:"mode_requested"`
	RecordingRequested bool             `json:"recording_requested"`
	MaxDurationMinutes int              `json:"max_duration_minutes"`
	ConsentDeadline    time.Time        `json:"consent_deadline"`
	Warning            string           `json:"warning"`
	Locale             string           `json:"locale"`
}

func sessionView(r api.Resolved) *SessionView {
	return &SessionView{
		UUID: r.SessionUUID, Organization: r.Organization, Agent: r.Agent,
		ModeRequested: r.ModeRequested, RecordingRequested: r.RecordingRequested,
		MaxDurationMinutes: r.MaxDurationMinutes, ConsentDeadline: r.ConsentDeadline,
		Warning: r.Warning, Locale: r.Locale,
	}
}

// Consent, state.consent nesnesidir.
type Consent struct {
	View      bool  `json:"view"`
	Control   bool  `json:"control"`
	Recording *bool `json:"recording"`
}

// ControlSummary, summary.control nesnesidir.
type ControlSummary struct {
	Granted   bool    `json:"granted"`
	Seconds   int64   `json:"seconds"`
	RevokedBy *string `json:"revoked_by"`
}

// Summary, oturum bitiş özetidir.
type Summary struct {
	AgentName        string         `json:"agent_name"`
	OrganizationName string         `json:"organization_name"`
	StartedAt        *time.Time     `json:"started_at"`
	EndedAt          time.Time      `json:"ended_at"`
	DurationSeconds  int64          `json:"duration_seconds"`
	Control          ControlSummary `json:"control"`
	Recorded         bool           `json:"recorded"`
	EndedReason      string         `json:"ended_reason"`
	EndedBy          string         `json:"ended_by"`
	Reported         bool           `json:"reported"`
}

// ReasonConnectionLost, sunucuya ulaşılamadan biten oturumun nedenidir.
const ReasonConnectionLost = "connection_lost"

// EndedBy, ended_reason'ı tasarımdaki "Bitiren" satırına indirger.
func EndedBy(reason string) string {
	switch reason {
	case "customer", "declined", "reported":
		return "customer"
	case "agent", "conversation_closed":
		return "agent"
	case "timeout", "max_duration", "code_expired", "attempts":
		return "time"
	case "platform":
		return "platform"
	}
	return "connection"
}

// State, state olayının ve yöntem yanıtlarının tam anlık görüntüsüdür.
type State struct {
	Phase        Phase             `json:"phase"`
	Session      *SessionView      `json:"session"`
	Consent      Consent           `json:"consent"`
	Control      Control           `json:"control"`
	Connection   remote.Connection `json:"connection"`
	AgentPresent bool              `json:"agent_present"`
	StartedAt    *time.Time        `json:"started_at"`
	EndsAt       *time.Time        `json:"ends_at"`
	Permissions  Permissions       `json:"permissions"`
	Summary      *Summary          `json:"summary"`
}

// StateResult, yöntem yanıtıdır: {"state": ...}.
type StateResult struct {
	State State `json:"state"`
}

// ControlEvent, control olayıdır.
type ControlEvent struct {
	State    Control   `json:"state"`
	Previous Control   `json:"previous"`
	By       string    `json:"by,omitempty"`
	Notice   string    `json:"notice,omitempty"`
	At       time.Time `json:"at"`
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	t = t.UTC()
	return &t
}
