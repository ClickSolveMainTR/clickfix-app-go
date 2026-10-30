// Package model holds the API shapes of routing (CF-53) and agent
// capacity (CF-54).
package model

import (
	"time"

	"github.com/google/uuid"
)

// Settings are an organization's routing settings.
type Settings struct {
	Enabled           bool   `json:"enabled"`
	Strategy          string `json:"strategy"`
	RespectCapacity   bool   `json:"respect_capacity"`
	VIPAccountOwner   bool   `json:"vip_account_owner"`
	ReassignOnOffline bool   `json:"reassign_on_offline"`
	// UnansweredMinutes reassigns a routed conversation still waiting on
	// its agent after this long (null: never).
	UnansweredMinutes *int32 `json:"unanswered_minutes"`
}

// Conditions select the conversations a rule routes (empty: all).
type Conditions struct {
	ChannelTypes []string    `json:"channel_types"`
	Priorities   []string    `json:"priorities"`
	TagUUIDs     []uuid.UUID `json:"tag_uuids"`
}

// Rule sends matching conversations to a team queue and/or requires skills.
type Rule struct {
	UUID       uuid.UUID  `json:"uuid"`
	Name       string     `json:"name"`
	Position   int32      `json:"position"`
	IsActive   bool       `json:"is_active"`
	Conditions Conditions `json:"conditions"`
	Team       *TeamRef   `json:"team"`
	Skills     []string   `json:"skills"`
}

// RuleInput creates or replaces a rule.
type RuleInput struct {
	Name       string     `json:"name"`
	Position   *int32     `json:"position"`
	IsActive   *bool      `json:"is_active"`
	Conditions Conditions `json:"conditions"`
	TeamUUID   *uuid.UUID `json:"team_uuid"`
	Skills     []string   `json:"skills"`
}

// TeamRef names a team.
type TeamRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Limits are capacity limits by channel type ('*' = all channels).
type Limits map[string]int32

// CapacityLimits are the organization defaults and per-agent overrides.
type CapacityLimits struct {
	Defaults Limits               `json:"defaults"`
	Agents   map[uuid.UUID]Limits `json:"agents"`
}

// CapacityInput replaces the limits of one agent (or the defaults when
// user_uuid is null).
type CapacityInput struct {
	UserUUID *uuid.UUID `json:"user_uuid"`
	Limits   Limits     `json:"limits"`
}

// AgentLoad is one agent in the supervisor capacity view.
type AgentLoad struct {
	UserUUID   uuid.UUID      `json:"user_uuid"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	LastSeenAt *time.Time     `json:"last_seen_at"`
	Skills     []string       `json:"skills"`
	Teams      []TeamRef      `json:"teams"`
	Open       map[string]int `json:"open"`
	OpenTotal  int            `json:"open_total"`
	// Limits are the effective limits (own or default) by channel type.
	Limits Limits `json:"limits"`
	// Utilization is open_total / '*' limit (null when unlimited).
	Utilization *float64 `json:"utilization"`
	// Available: online with room for at least one more conversation.
	Available bool `json:"available"`
}

// Capacity is the supervisor view.
type Capacity struct {
	Agents   []AgentLoad `json:"agents"`
	Queue    int         `json:"queue"`
	Online   int         `json:"online"`
	Settings Settings    `json:"settings"`
}

// Event is one routing audit row.
type Event struct {
	ConversationUUID uuid.UUID `json:"conversation_uuid"`
	Number           int64     `json:"number"`
	Reason           string    `json:"reason"`
	Strategy         string    `json:"strategy"`
	Note             string    `json:"note"`
	From             *AgentRef `json:"from"`
	To               *AgentRef `json:"to"`
	Team             string    `json:"team"`
	CreatedAt        time.Time `json:"created_at"`
}

// AgentRef names an agent.
type AgentRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}
