// Package supportaccess is time-limited support access (CF-14): an
// organization owner / admin grants ClickFix support staff access for a
// limited time (read-only or full); staff holding platform.support.access
// then work inside the organization under their own account, without a
// membership and without impersonating anybody. Every action they take is
// stamped via_support in the organization's audit log.
//
//	GET  /v1/tenant/support-access                grants history + active grant
//	POST /v1/tenant/support-access                grant {mode, duration_hours, reason}
//	POST /v1/tenant/support-access/{uuid}/revoke  revoke
//	GET  /v1/platform/support-access              organizations staff may enter now
package supportaccess

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	authmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxDuration bounds a grant; DefaultDuration is used when none is given.
const (
	MaxDuration     = 7 * 24 * time.Hour
	DefaultDuration = 24 * time.Hour
)

var (
	// ErrNotFound is an unknown grant of the organization.
	ErrNotFound = errors.New("support access grant not found")
	// ErrInvalid wraps field validation problems.
	ErrInvalid = errors.New("invalid support access request")
)

// FieldError is one validation problem.
type FieldError struct{ Field, Code, Message string }

// ValidationError lists field problems.
type ValidationError struct{ Details []FieldError }

func (e *ValidationError) Error() string { return "validation failed" }

// Person is who granted / revoked.
type Person struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
}

// Grant is a support access grant.
type Grant struct {
	UUID      uuid.UUID  `json:"uuid"`
	Mode      string     `json:"mode"`
	Reason    string     `json:"reason"`
	Status    string     `json:"status"` // active, expired, revoked
	StartsAt  time.Time  `json:"starts_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	GrantedBy *Person    `json:"granted_by"`
	RevokedBy *Person    `json:"revoked_by"`
	CreatedAt time.Time  `json:"created_at"`
}

// GrantInput is the body of POST /v1/tenant/support-access.
type GrantInput struct {
	Mode          string `json:"mode"`
	DurationHours int    `json:"duration_hours"`
	Reason        string `json:"reason"`
}

// PlatformGrant is an organization support staff may enter.
type PlatformGrant struct {
	UUID         uuid.UUID `json:"uuid"`
	Mode         string    `json:"mode"`
	Reason       string    `json:"reason"`
	ExpiresAt    time.Time `json:"expires_at"`
	Organization struct {
		UUID   uuid.UUID `json:"uuid"`
		Slug   string    `json:"slug"`
		Name   string    `json:"name"`
		Status string    `json:"status"`
	} `json:"organization"`
}

// Service manages grants.
type Service struct {
	q   *db.Queries
	now func() time.Time
}

// NewService builds the service.
func NewService(q *db.Queries) *Service { return &Service{q: q, now: time.Now} }

// Grant opens a new grant (replacing the open one).
func (s *Service) Grant(ctx context.Context, orgID, actorID int64, in GrantInput) (Grant, error) {
	ve := &ValidationError{}
	mode := strings.TrimSpace(in.Mode)
	if mode != orgctx.SupportReadOnly && mode != orgctx.SupportFull {
		ve.Details = append(ve.Details, FieldError{"mode", "invalid", "mode must be read_only or full"})
	}
	d := time.Duration(in.DurationHours) * time.Hour
	if in.DurationHours == 0 {
		d = DefaultDuration
	}
	if d <= 0 || d > MaxDuration {
		ve.Details = append(ve.Details, FieldError{"duration_hours", "invalid", "duration_hours must be between 1 and 168"})
	}
	reason := strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(reason) > 500 {
		ve.Details = append(ve.Details, FieldError{"reason", "too_long", "reason is longer than 500 characters"})
	}
	if len(ve.Details) > 0 {
		return Grant{}, ve
	}
	if _, err := s.q.RevokeOpenSupportGrants(ctx, db.RevokeOpenSupportGrantsParams{
		OrganizationID: orgID, RevokedBy: pgtype.Int8{Int64: actorID, Valid: true},
	}); err != nil {
		return Grant{}, err
	}
	row, err := s.q.CreateSupportGrant(ctx, db.CreateSupportGrantParams{
		OrganizationID: orgID, Mode: mode, Reason: reason, GrantedBy: pgtype.Int8{Int64: actorID, Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: s.now().UTC().Add(d), Valid: true},
	})
	if err != nil {
		return Grant{}, err
	}
	return s.get(ctx, orgID, row.Uuid)
}

// Revoke ends a grant early.
func (s *Service) Revoke(ctx context.Context, orgID, actorID int64, id uuid.UUID) (Grant, error) {
	if _, err := s.q.RevokeSupportGrant(ctx, db.RevokeSupportGrantParams{
		OrganizationID: orgID, Uuid: id, RevokedBy: pgtype.Int8{Int64: actorID, Valid: true},
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Grant{}, ErrNotFound
		}
		return Grant{}, err
	}
	return s.get(ctx, orgID, id)
}

// List returns the grant history of an organization (newest first) and the
// active grant, if any.
func (s *Service) List(ctx context.Context, orgID int64, limit, offset int32) ([]Grant, int64, *Grant, error) {
	rows, err := s.q.ListSupportGrants(ctx, db.ListSupportGrantsParams{OrganizationID: orgID, LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, 0, nil, err
	}
	total, err := s.q.CountSupportGrants(ctx, orgID)
	if err != nil {
		return nil, 0, nil, err
	}
	out := make([]Grant, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.mapRow(r))
	}
	var active *Grant
	if row, err := s.q.GetActiveSupportGrant(ctx, orgID); err == nil {
		g, err := s.get(ctx, orgID, row.Uuid)
		if err != nil {
			return nil, 0, nil, err
		}
		active = &g
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, nil, err
	}
	return out, total, active, nil
}

// Platform lists every organization staff may enter now.
func (s *Service) Platform(ctx context.Context) ([]PlatformGrant, error) {
	rows, err := s.q.ListActiveSupportGrantsPlatform(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PlatformGrant, 0, len(rows))
	for _, r := range rows {
		g := PlatformGrant{UUID: r.Uuid, Mode: r.Mode, Reason: r.Reason, ExpiresAt: r.ExpiresAt.Time.UTC()}
		g.Organization.UUID, g.Organization.Slug, g.Organization.Name, g.Organization.Status = r.OrganizationUuid, r.OrganizationSlug, r.OrganizationName, r.OrganizationStatus
		out = append(out, g)
	}
	return out, nil
}

func (s *Service) get(ctx context.Context, orgID int64, id uuid.UUID) (Grant, error) {
	// The history is short; scan the newest page for the grant.
	rows, err := s.q.ListSupportGrants(ctx, db.ListSupportGrantsParams{OrganizationID: orgID, LimitCount: 200})
	if err != nil {
		return Grant{}, err
	}
	for _, r := range rows {
		if r.Uuid == id {
			return s.mapRow(r), nil
		}
	}
	return Grant{}, ErrNotFound
}

func (s *Service) mapRow(r db.ListSupportGrantsRow) Grant {
	now := s.now()
	g := Grant{
		UUID: r.Uuid, Mode: r.Mode, Reason: r.Reason, StartsAt: r.StartsAt.Time.UTC(), ExpiresAt: r.ExpiresAt.Time.UTC(),
		CreatedAt: r.CreatedAt.Time.UTC(), Status: "active",
	}
	switch {
	case r.RevokedAt.Valid:
		t := r.RevokedAt.Time.UTC()
		g.RevokedAt, g.Status = &t, "revoked"
	case !now.Before(g.ExpiresAt):
		g.Status = "expired"
	}
	if r.GrantedByUuid.Valid {
		g.GrantedBy = &Person{UUID: r.GrantedByUuid.Bytes, Name: r.GrantedByName, Surname: r.GrantedBySurname}
	}
	if r.RevokedByUuid.Valid {
		g.RevokedBy = &Person{UUID: r.RevokedByUuid.Bytes, Name: r.RevokedByName, Surname: r.RevokedBySurname}
	}
	return g
}

// --- resolver --------------------------------------------------------------

// OrgLookup finds a live organization by slug or uuid (organizations module).
type OrgLookup interface {
	LookupOrganization(ctx context.Context, ref string) (id int64, uuid uuid.UUID, slug, name, status string, err error)
}

// Resolver decorates the membership resolver with support access: Resolve
// is the member path, ResolveSupport the staff path (see orgctx).
type Resolver struct {
	orgctx.Resolver
	svc  *Service
	orgs OrgLookup
}

// NewResolver wraps next.
func NewResolver(next orgctx.Resolver, svc *Service, orgs OrgLookup) *Resolver {
	return &Resolver{Resolver: next, svc: svc, orgs: orgs}
}

// InvalidateOrganization forwards cache invalidation when next supports it.
func (r *Resolver) InvalidateOrganization(ctx context.Context, organizationID int64) error {
	if inv, ok := r.Resolver.(orgctx.Invalidator); ok {
		return inv.InvalidateOrganization(ctx, organizationID)
	}
	return nil
}

// ResolveSupport implements orgctx.SupportResolver.
func (r *Resolver) ResolveSupport(ctx context.Context, ref string, userID int64) (orgctx.Scope, error) {
	id, orgUUID, slug, name, status, err := r.orgs.LookupOrganization(ctx, ref)
	if err != nil {
		return orgctx.Scope{}, orgctx.ErrNotMember
	}
	// Defence in depth: the middleware already checked the principal.
	ok, err := r.svc.q.UserHasPlatformPermission(ctx, db.UserHasPlatformPermissionParams{UserID: userID, Slug: rbac.PermPlatformSupportAccess})
	if err != nil {
		return orgctx.Scope{}, err
	}
	if !ok {
		return orgctx.Scope{}, orgctx.ErrNotMember
	}
	grant, err := r.svc.q.GetActiveSupportGrant(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return orgctx.Scope{}, orgctx.ErrNotMember
	}
	if err != nil {
		return orgctx.Scope{}, err
	}
	all, err := r.svc.q.ListTenantPermissionSlugs(ctx)
	if err != nil {
		return orgctx.Scope{}, err
	}
	ends := grant.ExpiresAt.Time.UTC()
	return orgctx.Scope{
		ID: id, UUID: orgUUID, Slug: slug, Name: name, Status: status,
		AccessStartsAt: grant.StartsAt.Time.UTC(), AccessEndsAt: &ends,
		MemberRole: orgctx.RoleSupport, MemberRoles: []string{orgctx.RoleSupport},
		Permissions: SupportPermissions(grant.Mode, all),
		Support:     true, SupportMode: grant.Mode,
	}, nil
}

// SupportPermissions derives the tenant permissions of a grant mode:
// read_only = every read permission; full = everything except managing
// people, roles, billing, the audit export and support access itself
// (those stay with the customer).
func SupportPermissions(mode string, all []string) []string {
	out := []string{}
	for _, p := range all {
		switch mode {
		case orgctx.SupportReadOnly:
			if strings.HasSuffix(p, ".read") || strings.HasSuffix(p, ".read_all") {
				out = append(out, p)
			}
		case orgctx.SupportFull:
			if !slices.Contains(supportExcluded, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

var supportExcluded = []string{
	rbac.PermTenantMembersWrite, rbac.PermTenantRolesWrite, rbac.PermTenantBillingWrite,
	rbac.PermTenantAuditExport, rbac.PermTenantSupportAccessManage, rbac.PermTenantSettingsWrite,
}

// --- /auth/me ----------------------------------------------------------------

// MeLister lists memberships and, for support staff, the organizations they
// may enter now (support: true).
type MeLister struct {
	Next interface {
		ListForUser(ctx context.Context, userID int64) ([]authmodel.MeOrganization, error)
	}
	Svc *Service
}

// ListForUser implements the auth OrganizationsLister.
func (l MeLister) ListForUser(ctx context.Context, userID int64) ([]authmodel.MeOrganization, error) {
	out, err := l.Next.ListForUser(ctx, userID)
	if err != nil || l.Svc == nil {
		return out, err
	}
	ok, err := l.Svc.q.UserHasPlatformPermission(ctx, db.UserHasPlatformPermissionParams{UserID: userID, Slug: rbac.PermPlatformSupportAccess})
	if err != nil || !ok {
		return out, err
	}
	grants, err := l.Svc.Platform(ctx)
	if err != nil {
		return nil, err
	}
	member := map[uuid.UUID]bool{}
	for _, o := range out {
		member[o.UUID] = true
	}
	all, err := l.Svc.q.ListTenantPermissionSlugs(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		if member[g.Organization.UUID] {
			continue
		}
		ends := g.ExpiresAt
		out = append(out, authmodel.MeOrganization{
			UUID: g.Organization.UUID, Slug: g.Organization.Slug, Name: g.Organization.Name,
			Status: g.Organization.Status, AccessEndsAt: &ends,
			Role:        authmodel.MeOrganizationRole{Key: orgctx.RoleSupport, Name: "Support"},
			Permissions: SupportPermissions(g.Mode, all),
			Support:     true,
			SupportMode: g.Mode,
		})
	}
	return out, nil
}
