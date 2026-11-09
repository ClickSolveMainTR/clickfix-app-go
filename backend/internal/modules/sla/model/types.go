// Package model holds the API shapes of the SLA module (CF-46).
package model

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/businesshours"
	"github.com/google/uuid"
)

// Priorities a policy has targets for.
var Priorities = []string{"low", "normal", "high", "urgent"}

// BusinessHours is a weekly schedule.
type BusinessHours struct {
	UUID              uuid.UUID          `json:"uuid"`
	Name              string             `json:"name"`
	Timezone          string             `json:"timezone"`
	WorkingHours      businesshours.Week `json:"working_hours"`
	UsePublicHolidays bool               `json:"use_public_holidays"`
	HolidayCountry    string             `json:"holiday_country"`
	TeamID            *int64             `json:"team_id"`
	IsDefault         bool               `json:"is_default"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

// BusinessHoursInput creates or replaces a schedule.
type BusinessHoursInput struct {
	Name              string             `json:"name"`
	Timezone          string             `json:"timezone"`
	WorkingHours      businesshours.Week `json:"working_hours"`
	UsePublicHolidays *bool              `json:"use_public_holidays"`
	HolidayCountry    string             `json:"holiday_country"`
	TeamID            *int64             `json:"team_id"`
	IsDefault         bool               `json:"is_default"`
}

// Holiday is an organization holiday or short day.
type Holiday struct {
	UUID              uuid.UUID  `json:"uuid"`
	BusinessHoursUUID *uuid.UUID `json:"business_hours_uuid"`
	Date              string     `json:"date"`
	Name              string     `json:"name"`
	Closed            bool       `json:"closed"`
	Open              *string    `json:"open"`
	Close             *string    `json:"close"`
}

// HolidayInput adds an organization holiday.
type HolidayInput struct {
	BusinessHoursUUID *uuid.UUID `json:"business_hours_uuid"`
	Date              string     `json:"date"`
	Name              string     `json:"name"`
	Closed            bool       `json:"closed"`
	Open              string     `json:"open"`
	Close             string     `json:"close"`
}

// PublicHoliday is a national holiday.
type PublicHoliday struct {
	Date    string `json:"date"`
	Name    string `json:"name"`
	NameEN  string `json:"name_en"`
	HalfDay bool   `json:"half_day"`
}

// Conditions select the conversations a policy applies to. An empty list
// matches everything.
type Conditions struct {
	Priorities   []string    `json:"priorities"`
	ChannelTypes []string    `json:"channel_types"`
	TagUUIDs     []uuid.UUID `json:"tag_uuids"`
	CompanyUUIDs []uuid.UUID `json:"company_uuids"`
	// CompanyTiers match the company's tier (plan / segment, CF-84).
	CompanyTiers []string `json:"company_tiers"`
	// CompanyPlans match the customer's revenue plan (CF-86).
	CompanyPlans []string `json:"company_plans"`
	// MinCompanyMRRMinor matches companies whose MRR (minor units, any
	// currency) is at least this amount (CF-86).
	MinCompanyMRRMinor *int64 `json:"min_company_mrr_minor"`
	// CustomFields match list custom fields of the contact, company or
	// conversation by option id (CF-196 catalog): every entry must hold,
	// any of its options matches. Labels may be renamed without breaking it.
	CustomFields []customfields.FieldMatch `json:"custom_fields"`
}

// Target is the goal of one priority, in working minutes (nil: not measured).
type Target struct {
	Priority             string `json:"priority"`
	FirstResponseMinutes *int32 `json:"first_response_minutes"`
	NextResponseMinutes  *int32 `json:"next_response_minutes"`
	ResolutionMinutes    *int32 `json:"resolution_minutes"`
}

// Policy is an SLA policy.
type Policy struct {
	UUID              uuid.UUID  `json:"uuid"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	IsActive          bool       `json:"is_active"`
	Position          int32      `json:"position"`
	BusinessHoursUUID *uuid.UUID `json:"business_hours_uuid"`
	Conditions        Conditions `json:"conditions"`
	WarningPercent    int32      `json:"warning_percent"`
	Targets           []Target   `json:"targets"`
	// ContractedCompanies counts companies whose contracted SLA this is.
	ContractedCompanies int       `json:"contracted_companies"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// PolicyInput creates or replaces a policy.
type PolicyInput struct {
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	IsActive          *bool      `json:"is_active"`
	Position          *int32     `json:"position"`
	BusinessHoursUUID *uuid.UUID `json:"business_hours_uuid"`
	Conditions        Conditions `json:"conditions"`
	WarningPercent    *int32     `json:"warning_percent"`
	Targets           []Target   `json:"targets"`
}

// PolicyRef names a policy.
type PolicyRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Metric is one SLA clock of a conversation.
type Metric struct {
	Metric           string     `json:"metric"`
	Status           string     `json:"status"` // running, paused, met, breached
	Cycle            int32      `json:"cycle"`
	TargetSeconds    int64      `json:"target_seconds"`
	StartedAt        time.Time  `json:"started_at"`
	DueAt            *time.Time `json:"due_at"`
	WarnAt           *time.Time `json:"warn_at"`
	RemainingSeconds int64      `json:"remaining_seconds"`
	PausedAt         *time.Time `json:"paused_at"`
	BreachedAt       *time.Time `json:"breached_at"`
	CompletedAt      *time.Time `json:"completed_at"`
}

// ConversationSLA is the SLA state of one conversation.
type ConversationSLA struct {
	ConversationUUID uuid.UUID  `json:"conversation_uuid"`
	Policy           *PolicyRef `json:"policy"`
	Metrics          []Metric   `json:"metrics"`
}

// CompanyPolicyInput sets a company's contracted SLA (null clears it).
type CompanyPolicyInput struct {
	PolicyUUID *uuid.UUID `json:"policy_uuid"`
}

// CompanyPolicy is a company's contracted SLA.
type CompanyPolicy struct {
	CompanyUUID uuid.UUID  `json:"company_uuid"`
	PolicyUUID  *uuid.UUID `json:"policy_uuid"`
}

// ContractedCompany is a company with a contracted SLA.
type ContractedCompany struct {
	CompanyUUID uuid.UUID `json:"company_uuid"`
	Name        string    `json:"name"`
	Tier        string    `json:"tier"`
	Policy      PolicyRef `json:"policy"`
}
