// Package model holds the team types (CF-5).
package model

import (
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Resource is the activity / meta slug of teams.
const Resource = "tenant.teams"

// Team member roles.
const (
	RoleLead   = "lead"
	RoleMember = "member"
)

// Weekdays of working hours, Monday first.
var Weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// WorkingDay is the opening time of one weekday ("HH:MM").
type WorkingDay struct {
	Day   string `json:"day"`
	Open  bool   `json:"open"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// WorkingHours are a team's own hours; nil means "use the organization's
// business hours" (SLA and routing read the effective value).
type WorkingHours struct {
	Days []WorkingDay `json:"days"`
}

// MemberUser is the public user part of a team member.
type MemberUser struct {
	UUID    uuid.UUID `json:"uuid"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Status  string    `json:"status"`
}

// TeamMember is a member of a team with its team role and skills.
type TeamMember struct {
	User     MemberUser `json:"user"`
	Role     string     `json:"role"`
	Skills   []string   `json:"skills"`
	JoinedAt time.Time  `json:"joined_at"`
}

// Team is a team of the organization.
type Team struct {
	ID           int64         `json:"-"`
	UUID         uuid.UUID     `json:"uuid"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Color        string        `json:"color"`
	Timezone     *string       `json:"timezone"`
	WorkingHours *WorkingHours `json:"working_hours"`
	Skills       []string      `json:"skills"`
	MemberCount  int64         `json:"member_count"`
	Leads        []MemberUser  `json:"leads"`
	Members      []TeamMember  `json:"members,omitempty"`
	// Department is the business unit the team belongs to (CF-215).
	Department   *DepartmentRef `json:"department"`
	DepartmentID pgtype.Int8    `json:"-"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// DepartmentRef is the department of a team (CF-215).
type DepartmentRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

// TeamInput creates or patches a team. On patch nil fields keep their
// value; ClearWorkingHours / ClearTimezone go back to the organization's.
type TeamInput struct {
	Name         *string       `json:"name"`
	Description  *string       `json:"description"`
	Color        *string       `json:"color"`
	Timezone     *string       `json:"timezone"`
	WorkingHours *WorkingHours `json:"working_hours"`
	// InheritHours drops the team's own hours and timezone (organization
	// defaults apply).
	InheritHours bool     `json:"inherit_hours"`
	Skills       []string `json:"skills"`
	// Members replaces the member list on create (optional).
	Members []MemberInput `json:"members"`
	// Department moves the team into a department (CF-215); null takes it
	// out, omitted keeps it.
	Department deptscope.Optional `json:"department_uuid"`
}

// MemberInput adds a member or changes its team role.
type MemberInput struct {
	UserUUID uuid.UUID `json:"user_uuid"`
	Role     string    `json:"role"`
}

// ListFilter filters GET /v1/tenant/teams.
type ListFilter struct {
	Limit      int32
	Offset     int32
	Q          string
	Skill      string
	MemberUUID uuid.UUID
	SortField  string
	SortDesc   bool
}
