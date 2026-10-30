package model

import (
	"time"

	"github.com/google/uuid"
)

// --- tenant member management (/v1/tenant/members, /v1/tenant/roles) -----

// TenantRoleRef is a role held by a member.
type TenantRoleRef struct {
	UUID       uuid.UUID `json:"uuid"`
	Key        string    `json:"key"`
	Name       string    `json:"name"`
	IsTemplate bool      `json:"is_template"`
}

// TenantMemberUser is the public user part of a tenant member.
type TenantMemberUser struct {
	UUID    uuid.UUID `json:"uuid"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Status  string    `json:"status"`
}

// TenantMember is a member as the tenant shell sees it.
type TenantMember struct {
	ID    int64            `json:"-"`
	User  TenantMemberUser `json:"user"`
	Roles []TenantRoleRef  `json:"roles"`
	// Skills are the member's routing skill tags (CF-5).
	Skills      []string   `json:"skills"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TenantMemberListFilter filters GET /v1/tenant/members.
type TenantMemberListFilter struct {
	Limit     int32
	Offset    int32
	Q         string
	RoleUUIDs []uuid.UUID
	SortField string
	SortDesc  bool
}

// PatchTenantMemberInput changes the roles of a member.
type PatchTenantMemberInput struct {
	RoleUUIDs *[]uuid.UUID `json:"role_uuids"`
}

// TenantRole is a template or organization-specific tenant role.
type TenantRole struct {
	ID          int64     `json:"-"`
	UUID        uuid.UUID `json:"uuid"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsTemplate  bool      `json:"is_template"`
	IsBillable  bool      `json:"is_billable"`
	MemberCount int64     `json:"member_count"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TenantRoleInput creates or patches an organization-specific role.
type TenantRoleInput struct {
	Key         *string   `json:"key"`
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Permissions *[]string `json:"permissions"`
	// IsBillable marks the role as taking a paid seat (default true).
	IsBillable *bool `json:"is_billable"`
}

// Seats is the paid seat usage of an organization (CF-4).
type Seats struct {
	Billable int64 `json:"billable"`
	Total    int64 `json:"total"`
}

// TenantPermission is an assignable tenant permission.
type TenantPermission struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}
