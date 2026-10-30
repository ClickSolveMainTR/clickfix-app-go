// Package videorooms implements LiveKit video rooms (CF-139): policy layers,
// guest join links with lobby, end-to-end encryption keys, recordings and
// participant-minute metering against the billing wallet.
package videorooms

// Policy is the effective rule set for an organization. The platform row is
// complete; a plan override replaces any field it names; the organization
// can only tighten what is left.
type Policy struct {
	Enabled                 bool  `json:"enabled"`
	MaxDurationMinutes      int32 `json:"max_duration_minutes"`
	MaxParticipants         int32 `json:"max_participants"`
	AllowCamera             bool  `json:"allow_camera"`
	AllowScreenShare        bool  `json:"allow_screen_share"`
	AllowChat               bool  `json:"allow_chat"`
	AllowRecording          bool  `json:"allow_recording"`
	AllowGuestJoin          bool  `json:"allow_guest_join"`
	RequirePIN              bool  `json:"require_pin"`
	RequireLobby            bool  `json:"require_lobby"`
	E2EERequired            bool  `json:"e2ee_required"`
	RecordingRetentionHours int32 `json:"recording_retention_hours"`
	MaxConcurrentRecordings int32 `json:"max_concurrent_recordings"`
	MaxRecordingMinutes     int32 `json:"max_recording_minutes"`
	LowBalanceMinutes       int64 `json:"low_balance_minutes"`
}

// Partial names only the fields a plan override or an organization sets.
type Partial struct {
	Enabled                 *bool  `json:"enabled,omitempty"`
	MaxDurationMinutes      *int32 `json:"max_duration_minutes,omitempty"`
	MaxParticipants         *int32 `json:"max_participants,omitempty"`
	AllowCamera             *bool  `json:"allow_camera,omitempty"`
	AllowScreenShare        *bool  `json:"allow_screen_share,omitempty"`
	AllowChat               *bool  `json:"allow_chat,omitempty"`
	AllowRecording          *bool  `json:"allow_recording,omitempty"`
	AllowGuestJoin          *bool  `json:"allow_guest_join,omitempty"`
	RequirePIN              *bool  `json:"require_pin,omitempty"`
	RequireLobby            *bool  `json:"require_lobby,omitempty"`
	E2EERequired            *bool  `json:"e2ee_required,omitempty"`
	RecordingRetentionHours *int32 `json:"recording_retention_hours,omitempty"`
	MaxConcurrentRecordings *int32 `json:"max_concurrent_recordings,omitempty"`
	MaxRecordingMinutes     *int32 `json:"max_recording_minutes,omitempty"`
	LowBalanceMinutes       *int64 `json:"low_balance_minutes,omitempty"`
}

// Hard bounds, whatever the platform admin enters.
const (
	maxDurationCap     = 1440
	minDuration        = 5
	maxParticipantsCap = 500
	minParticipants    = 2
	maxRetentionHours  = 24 * 90
	maxRecordingCap    = 1440
	maxConcurrentCap   = 100
	maxLowBalance      = 1_000_000
)

// Valid checks a complete (platform) policy.
func (p Policy) Valid() bool {
	return p.MaxDurationMinutes >= minDuration && p.MaxDurationMinutes <= maxDurationCap &&
		p.MaxParticipants >= minParticipants && p.MaxParticipants <= maxParticipantsCap &&
		p.RecordingRetentionHours >= 1 && p.RecordingRetentionHours <= maxRetentionHours &&
		p.MaxConcurrentRecordings >= 0 && p.MaxConcurrentRecordings <= maxConcurrentCap &&
		p.MaxRecordingMinutes >= 1 && p.MaxRecordingMinutes <= maxRecordingCap &&
		p.LowBalanceMinutes >= 0 && p.LowBalanceMinutes <= maxLowBalance
}

// Valid checks the numbers a partial sets.
func (p Partial) Valid() bool {
	in := func(v *int32, lo, hi int32) bool { return v == nil || *v >= lo && *v <= hi }
	return in(p.MaxDurationMinutes, minDuration, maxDurationCap) && in(p.MaxParticipants, minParticipants, maxParticipantsCap) &&
		in(p.RecordingRetentionHours, 1, maxRetentionHours) && in(p.MaxConcurrentRecordings, 0, maxConcurrentCap) &&
		in(p.MaxRecordingMinutes, 1, maxRecordingCap) && (p.LowBalanceMinutes == nil || *p.LowBalanceMinutes >= 0 && *p.LowBalanceMinutes <= maxLowBalance)
}

// Override applies a plan override: every named field replaces the base.
func (p Policy) Override(o Partial) Policy {
	setB := func(dst *bool, v *bool) {
		if v != nil {
			*dst = *v
		}
	}
	setI := func(dst *int32, v *int32) {
		if v != nil {
			*dst = *v
		}
	}
	setB(&p.Enabled, o.Enabled)
	setI(&p.MaxDurationMinutes, o.MaxDurationMinutes)
	setI(&p.MaxParticipants, o.MaxParticipants)
	setB(&p.AllowCamera, o.AllowCamera)
	setB(&p.AllowScreenShare, o.AllowScreenShare)
	setB(&p.AllowChat, o.AllowChat)
	setB(&p.AllowRecording, o.AllowRecording)
	setB(&p.AllowGuestJoin, o.AllowGuestJoin)
	setB(&p.RequirePIN, o.RequirePIN)
	setB(&p.RequireLobby, o.RequireLobby)
	setB(&p.E2EERequired, o.E2EERequired)
	setI(&p.RecordingRetentionHours, o.RecordingRetentionHours)
	setI(&p.MaxConcurrentRecordings, o.MaxConcurrentRecordings)
	setI(&p.MaxRecordingMinutes, o.MaxRecordingMinutes)
	if o.LowBalanceMinutes != nil {
		p.LowBalanceMinutes = *o.LowBalanceMinutes
	}
	return p
}

// Tighten applies organization settings: permissions can only be removed,
// requirements only added, limits only lowered. Looser values are ignored.
func (p Policy) Tighten(o Partial) Policy {
	allow := func(dst *bool, v *bool) {
		if v != nil && !*v {
			*dst = false
		}
	}
	require := func(dst *bool, v *bool) {
		if v != nil && *v {
			*dst = true
		}
	}
	lower := func(dst *int32, v *int32) {
		if v != nil && *v < *dst {
			*dst = *v
		}
	}
	allow(&p.Enabled, o.Enabled)
	lower(&p.MaxDurationMinutes, o.MaxDurationMinutes)
	lower(&p.MaxParticipants, o.MaxParticipants)
	allow(&p.AllowCamera, o.AllowCamera)
	allow(&p.AllowScreenShare, o.AllowScreenShare)
	allow(&p.AllowChat, o.AllowChat)
	allow(&p.AllowRecording, o.AllowRecording)
	allow(&p.AllowGuestJoin, o.AllowGuestJoin)
	require(&p.RequirePIN, o.RequirePIN)
	require(&p.RequireLobby, o.RequireLobby)
	require(&p.E2EERequired, o.E2EERequired)
	lower(&p.RecordingRetentionHours, o.RecordingRetentionHours)
	lower(&p.MaxRecordingMinutes, o.MaxRecordingMinutes)
	lower(&p.MaxConcurrentRecordings, o.MaxConcurrentRecordings)
	// The warning threshold is the organization's own choice.
	if o.LowBalanceMinutes != nil {
		p.LowBalanceMinutes = *o.LowBalanceMinutes
	}
	// End-to-end encrypted media cannot be recorded server-side.
	if p.E2EERequired {
		p.AllowRecording = false
	}
	return p
}

// RoomOptions are the per-room choices a host makes when creating a room.
type RoomOptions struct {
	Name               string
	PIN                string
	Lobby              *bool
	GuestJoin          *bool
	E2EE               *bool
	Record             bool
	AllowCamera        *bool
	AllowScreenShare   *bool
	AllowChat          *bool
	MaxParticipants    *int32
	MaxDurationMinutes *int32
}

// RoomSettings are what a room is created with.
type RoomSettings struct {
	Lobby, GuestJoin, E2EE, Recording, AllowCamera, AllowScreenShare, AllowChat bool
	MaxParticipants, MaxDurationMinutes                                         int32
}

// Resolve clamps room options to the policy. Recording a room switches its
// end-to-end encryption off (egress must see the media); a policy that
// requires E2EE therefore refuses recording.
func (p Policy) Resolve(o RoomOptions) (RoomSettings, error) {
	pick := func(v *bool, def bool) bool {
		if v == nil {
			return def
		}
		return *v
	}
	s := RoomSettings{
		Lobby:              pick(o.Lobby, true) || p.RequireLobby,
		GuestJoin:          pick(o.GuestJoin, p.AllowGuestJoin) && p.AllowGuestJoin,
		E2EE:               pick(o.E2EE, true) || p.E2EERequired,
		AllowCamera:        pick(o.AllowCamera, true) && p.AllowCamera,
		AllowScreenShare:   pick(o.AllowScreenShare, true) && p.AllowScreenShare,
		AllowChat:          pick(o.AllowChat, true) && p.AllowChat,
		MaxParticipants:    p.MaxParticipants,
		MaxDurationMinutes: p.MaxDurationMinutes,
	}
	if o.MaxParticipants != nil {
		if *o.MaxParticipants < minParticipants {
			return s, ErrInvalid
		}
		s.MaxParticipants = min(*o.MaxParticipants, p.MaxParticipants)
	}
	if o.MaxDurationMinutes != nil {
		if *o.MaxDurationMinutes < minDuration {
			return s, ErrInvalid
		}
		s.MaxDurationMinutes = min(*o.MaxDurationMinutes, p.MaxDurationMinutes)
	}
	if p.RequirePIN && o.PIN == "" {
		return s, ErrPINRequired
	}
	if o.Record {
		if !p.AllowRecording || p.E2EERequired {
			return s, ErrRecordingNotAllowed
		}
		s.Recording, s.E2EE = true, false
	}
	return s, nil
}
