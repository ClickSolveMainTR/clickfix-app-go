// Package model holds the department types (CF-215).
package model

import (
	"errors"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/deptscope"
	"github.com/google/uuid"
)

// Resource is the activity / meta slug of departments.
const Resource = "tenant.departments"

var (
	// ErrNotFound is an unknown department (or one of another organization).
	ErrNotFound = errors.New("department not found")
	// ErrNameTaken is a duplicate department name in the organization.
	ErrNameTaken = errors.New("department name is already used")
	// ErrInUse is a department that still owns knowledge.
	ErrInUse = errors.New("department still owns knowledge")
	// ErrForbidden is a department action the caller may not take (not a
	// lead of the department, not a member of it).
	ErrForbidden = errors.New("not allowed for this department")
	// ErrConversationNotFound is an unknown conversation.
	ErrConversationNotFound = errors.New("conversation not found")
	// ErrResourceNotFound is an unknown knowledge item.
	ErrResourceNotFound = errors.New("knowledge item not found")
	// ErrInvalid is a request the department use case rejects.
	ErrInvalid = errors.New("invalid department request")
)

// Department sources of a conversation.
const (
	SourceCustomer = "customer"
	SourceAI       = "ai"
	SourceAgent    = "agent"
	SourceRule     = "rule"
)

// Knowledge kinds a department lead can share.
const (
	KindArticle = "article"
	KindSource  = "source"
	KindContext = "context"
)

// Kinds are the shareable knowledge kinds.
var Kinds = []string{KindArticle, KindSource, KindContext}

// WorkingDay is the opening time of one weekday ("HH:MM"), the teams shape.
type WorkingDay struct {
	Day   string `json:"day"`
	Open  bool   `json:"open"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// WorkingHours are a department's own hours; nil means the organization's.
type WorkingHours struct {
	Days []WorkingDay `json:"days"`
}

// Ref is the public reference of a department in other resources.
type Ref struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

// TeamRef is a team of a department.
type TeamRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

// UserRef is a lead of a department.
type UserRef struct {
	UUID    uuid.UUID `json:"uuid"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
}

// AISettings are the AI settings of a department.
type AISettings struct {
	// Enabled lets the customer AI answer conversations of the department.
	Enabled bool `json:"enabled"`
	// Instructions are added to the AI's reply instructions.
	Instructions string `json:"instructions"`
	// RoutingHint tells the AI which first messages belong here.
	RoutingHint string `json:"routing_hint"`
}

// Department is a business unit of the organization.
type Department struct {
	ID            int64         `json:"-"`
	UUID          uuid.UUID     `json:"uuid"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Color         string        `json:"color"`
	BrandUUID     *uuid.UUID    `json:"brand_uuid"`
	WidgetVisible bool          `json:"widget_visible"`
	SortOrder     int32         `json:"sort_order"`
	SLAPolicyUUID *uuid.UUID    `json:"sla_policy_uuid"`
	Timezone      *string       `json:"timezone"`
	WorkingHours  *WorkingHours `json:"working_hours"`
	AI            AISettings    `json:"ai"`
	TeamCount     int64         `json:"team_count"`
	Teams         []TeamRef     `json:"teams"`
	Leads         []UserRef     `json:"leads"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// AIInput patches the AI settings; nil fields keep their value.
type AIInput struct {
	Enabled      *bool   `json:"enabled"`
	Instructions *string `json:"instructions"`
	RoutingHint  *string `json:"routing_hint"`
}

// Input creates or patches a department. On patch nil fields keep their
// value; BrandUUID / SLAPolicyUUID take null to clear.
type Input struct {
	Name          *string            `json:"name"`
	Description   *string            `json:"description"`
	Color         *string            `json:"color"`
	BrandUUID     deptscope.Optional `json:"brand_uuid"`
	WidgetVisible *bool              `json:"widget_visible"`
	SortOrder     *int32             `json:"sort_order"`
	SLAPolicyUUID deptscope.Optional `json:"sla_policy_uuid"`
	Timezone      *string            `json:"timezone"`
	WorkingHours  *WorkingHours      `json:"working_hours"`
	// InheritHours drops the department's own hours and timezone.
	InheritHours bool     `json:"inherit_hours"`
	AI           *AIInput `json:"ai"`
	// TeamUUIDs, when set, is the complete team list of the department.
	TeamUUIDs *[]uuid.UUID `json:"team_uuids"`
}

// ListFilter filters GET /v1/tenant/departments.
type ListFilter struct {
	Limit, Offset int32
	Q             string
	BrandUUID     *uuid.UUID
	// Mine lists only the caller's departments.
	Mine bool
}

// Usage is the knowledge a department owns.
type Usage struct {
	Articles        int64 `json:"articles"`
	Sources         int64 `json:"sources"`
	ContextSections int64 `json:"context_sections"`
	Suggestions     int64 `json:"suggestions"`
}

// Total is the number of owned items.
func (u Usage) Total() int64 { return u.Articles + u.Sources + u.ContextSections + u.Suggestions }

// InUseError reports the knowledge that blocks a delete.
type InUseError struct{ Usage Usage }

func (e *InUseError) Error() string { return ErrInUse.Error() }

// Unwrap matches ErrInUse.
func (e *InUseError) Unwrap() error { return ErrInUse }

// Share is one knowledge item shared with a department.
type Share struct {
	UUID          uuid.UUID `json:"uuid"`
	ResourceKind  string    `json:"resource_kind"`
	ResourceUUID  uuid.UUID `json:"resource_uuid"`
	ResourceTitle string    `json:"resource_title"`
	// Department receives the item; Owner is the department it belongs to.
	Department Ref       `json:"department"`
	Owner      *Ref      `json:"owner"`
	CreatedAt  time.Time `json:"created_at"`
}

// ShareInput shares one item with the department of the path.
type ShareInput struct {
	ResourceKind string    `json:"resource_kind"`
	ResourceUUID uuid.UUID `json:"resource_uuid"`
}

// Transfer is one department change of a conversation.
type Transfer struct {
	UUID           uuid.UUID  `json:"uuid"`
	From           *Ref       `json:"from"`
	To             *Ref       `json:"to"`
	ActorKind      string     `json:"actor_kind"`
	ActorUUID      *uuid.UUID `json:"actor_uuid"`
	ActorName      string     `json:"actor_name"`
	Confidence     *float32   `json:"confidence"`
	Reason         string     `json:"reason"`
	ContextSummary string     `json:"context_summary"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ConversationDepartment is the department state of a conversation.
type ConversationDepartment struct {
	Department *Ref       `json:"department"`
	Source     *string    `json:"source"`
	Confidence *float32   `json:"confidence"`
	RoutedAt   *time.Time `json:"routed_at"`
	Transfers  []Transfer `json:"transfers"`
}

// TransferInput moves a conversation to another department with the context
// the receiving department needs. DepartmentUUID nil makes it
// organization-wide.
type TransferInput struct {
	DepartmentUUID *uuid.UUID `json:"department_uuid"`
	Reason         string     `json:"reason"`
	ContextSummary string     `json:"context_summary"`
	// ReleaseAssignee unassigns the current agent too (default keeps them
	// only when they stay in the department's teams).
	ReleaseAssignee bool `json:"release_assignee"`
}

// WidgetDepartment is a department the widget offers.
type WidgetDepartment struct {
	UUID        uuid.UUID `json:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Color       string    `json:"color"`
}

// Prediction is the AI's department guess for a first message.
type Prediction struct {
	DepartmentUUID *uuid.UUID `json:"department_uuid"`
	Confidence     float64    `json:"confidence"`
	Reason         string     `json:"reason"`
}

// Candidate is a department the AI may route to.
type Candidate struct {
	UUID        uuid.UUID `json:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	RoutingHint string    `json:"routing_hint"`
}
