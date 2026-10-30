package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Platform member management (/v1/platform/organizations/{uuid}/members).
// A platform operator acts with owner rights inside the organization: no
// grant limits, but the last-owner guard (409) still holds.

// OperatorScope is the scope a platform operator acts with inside an
// organization: owner role.
func OperatorScope(org orgmodel.Organization) orgctx.Scope {
	return orgctx.Scope{
		ID: org.ID, UUID: org.UUID, Slug: org.Slug, Name: org.Name, Status: org.Status,
		MemberRole: orgctx.RoleOwner, MemberRoles: []string{orgctx.RoleOwner},
	}
}

// PlatformScope loads an organization and returns the operator scope.
func (s *Service) PlatformScope(ctx context.Context, orgUUID uuid.UUID) (orgctx.Scope, error) {
	org, err := s.GetOrganization(ctx, orgUUID)
	if err != nil {
		return orgctx.Scope{}, err
	}
	return OperatorScope(org), nil
}

// PlatformListMembers lists the members of an organization with roles,
// and last login.
func (s *Service) PlatformListMembers(ctx context.Context, orgUUID uuid.UUID, f orgmodel.TenantMemberListFilter) ([]orgmodel.TenantMember, int64, error) {
	scope, err := s.PlatformScope(ctx, orgUUID)
	if err != nil {
		return nil, 0, err
	}
	return s.ListTenantMembers(ctx, scope, f)
}

// PlatformAddMember adds an existing user with one or more roles (default:
// the legacy role key).
func (s *Service) PlatformAddMember(ctx context.Context, orgUUID uuid.UUID, in orgmodel.PlatformMemberInput) (orgmodel.TenantMember, error) {
	scope, err := s.PlatformScope(ctx, orgUUID)
	if err != nil {
		return orgmodel.TenantMember{}, err
	}
	user, err := s.repo.GetUserByUUID(ctx, in.UserUUID)
	if err != nil {
		return orgmodel.TenantMember{}, mapRepoNotFound(err, ErrUserNotFound)
	}
	ve := &ValidationError{}
	var roles []repository.TenantRoleRow
	if in.RoleUUIDs != nil {
		roles, err = s.resolveRoles(ctx, scope.ID, *in.RoleUUIDs, "role_uuids", ve)
		if err != nil {
			return orgmodel.TenantMember{}, err
		}
	}
	if err := ve.orNil(); err != nil {
		return orgmodel.TenantMember{}, err
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		if err := s.repo.LockOrganizationRow(ctx, q, scope.ID); err != nil {
			return err
		}
		if in.RoleUUIDs == nil {
			role, err := s.repo.GetTenantRoleByKey(ctx, q, scope.ID, normalizeRole(in.RoleKey))
			if err != nil {
				return mapRepoNotFound(err, ErrRoleNotFound)
			}
			roles = []repository.TenantRoleRow{{ID: role.ID, Uuid: role.Uuid, Slug: role.Slug, Name: role.Name, IsTemplate: !role.OrganizationID.Valid}}
		}
		if _, err := s.repo.GetOrganizationMember(ctx, q, scope.ID, user.ID); err == nil {
			return fmt.Errorf("%w: user is already a member", ErrConflict)
		} else if !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		return s.insertMember(ctx, q, scope.ID, user.ID, roles)
	})
	if err != nil {
		return orgmodel.TenantMember{}, err
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, scope.ID)
	return s.GetTenantMember(ctx, scope, user.Uuid)
}

// PlatformPatchMember changes the roles of a member. A
// body with only role_key keeps the legacy single-role behaviour.
func (s *Service) PlatformPatchMember(ctx context.Context, orgUUID, userUUID uuid.UUID, in orgmodel.PlatformMemberInput) (orgmodel.TenantMember, error) {
	scope, err := s.PlatformScope(ctx, orgUUID)
	if err != nil {
		return orgmodel.TenantMember{}, err
	}
	if in.RoleUUIDs == nil && strings.TrimSpace(in.RoleKey) != "" {
		if _, err := s.ChangeMemberRole(ctx, orgUUID, userUUID, in.RoleKey); err != nil {
			return orgmodel.TenantMember{}, err
		}
		return s.GetTenantMember(ctx, scope, userUUID)
	}
	return s.PatchTenantMember(ctx, scope, userUUID, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: in.RoleUUIDs,
	})
}

// UserDisplayName returns "Name Surname" (or the e-mail) of a user.
func (s *Service) UserDisplayName(ctx context.Context, userID int64) (string, error) {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return "", mapRepoNotFound(err, ErrUserNotFound)
	}
	if name := strings.TrimSpace(u.Name + " " + u.Surname); name != "" {
		return name, nil
	}
	return u.Email, nil
}

// insertMember creates a membership with its roles (primary role by
// precedence).
func (s *Service) insertMember(ctx context.Context, q *db.Queries, orgID, userID int64, roles []repository.TenantRoleRow) error {
	primary := primaryRole(roles)
	if err := s.repo.AddOrganizationMember(ctx, q, db.AddOrganizationMemberParams{
		OrganizationID: orgID, UserID: userID, RoleID: primary.ID,
	}); err != nil {
		return err
	}
	member, err := s.repo.GetOrganizationMember(ctx, q, orgID, userID)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(roles))
	for _, r := range roles {
		ids = append(ids, r.ID)
	}
	return s.repo.ReplaceMemberRoles(ctx, q, orgID, member.ID, ids, primary.ID)
}

// resolveRoles maps role uuids of the organization (templates included).
func (s *Service) resolveRoles(ctx context.Context, orgID int64, ids []uuid.UUID, field string, ve *ValidationError) ([]repository.TenantRoleRow, error) {
	if len(ids) == 0 {
		ve.add(field, "required", "at least one role is required")
		return nil, nil
	}
	out := []repository.TenantRoleRow{}
	seen := map[uuid.UUID]bool{}
	for i, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		role, err := s.repo.GetTenantRoleByUUID(ctx, nil, orgID, id)
		if errors.Is(err, repository.ErrNotFound) {
			ve.add(fmt.Sprintf("%s.%d", field, i), "not_found", "role was not found")
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, nil
}

// checkGrant enforces "you can only grant what you hold": the owner role
// only by owners, and every permission of the roles must be the caller's.
func (s *Service) checkGrant(ctx context.Context, scope orgctx.Scope, roles []repository.TenantRoleRow) error {
	if isOwnerScope(scope) {
		return nil
	}
	ids := make([]int64, 0, len(roles))
	for _, r := range roles {
		if r.IsTemplate && r.Slug == orgctx.RoleOwner {
			return fmt.Errorf("%w: only owners can grant the owner role", ErrForbidden)
		}
		ids = append(ids, r.ID)
	}
	perms, err := s.repo.ListRolePermissionSlugs(ctx, ids)
	if err != nil {
		return err
	}
	for _, list := range perms {
		for _, p := range list {
			if !scope.Has(p) {
				return fmt.Errorf("%w: cannot grant a role with permission %s you do not hold", ErrForbidden, p)
			}
		}
	}
	return nil
}

func nilIfEmpty(v []string) []string {
	out := []string{}
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// AccessState is 'upcoming' before the access window, 'expired' after it and
// 'open' inside it.
func AccessState(starts time.Time, ends *time.Time, now time.Time) string {
	switch {
	case now.Before(starts):
		return orgmodel.AccessUpcoming
	case ends != nil && !now.Before(*ends):
		return orgmodel.AccessExpired
	default:
		return orgmodel.AccessOpen
	}
}

func mapOrganizationSummary(row db.ListOrganizationsWithStatsRow, now time.Time) orgmodel.OrganizationSummary {
	org := mapOrganization(db.Organization{
		ID: row.ID, Uuid: row.Uuid, Slug: row.Slug, Name: row.Name, LogoObjectKey: row.LogoObjectKey,
		Status: row.Status, AccessStartsAt: row.AccessStartsAt, AccessEndsAt: row.AccessEndsAt,
		Settings: row.Settings, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		Timezone: row.Timezone, Locale: row.Locale, Currency: row.Currency, CountryCode: row.CountryCode,
	})
	out := orgmodel.OrganizationSummary{
		Organization: org, MemberCount: row.MemberCount, PendingInvitationCount: row.PendingInvitationCount,
		LastActivityAt: timePtr(row.LastActivityAt),
		BillableSeats:  row.BillableSeats, PlanCode: row.PlanCode, PlanName: row.PlanName,
		SubscriptionStatus: row.SubscriptionStatus,
		TrialEndsAt:        timePtr(row.TrialEndsAt), PeriodEnd: timePtr(row.PeriodEnd),
		AICredits: row.AiCredits, VideoMinutes: row.VideoMinutes,
		AccessState: AccessState(org.AccessStartsAt, org.AccessEndsAt, now),
	}
	if id, err := uuid.Parse(row.OwnerUuid); err == nil {
		out.Owner = &orgmodel.OwnerRef{UUID: id, Email: row.OwnerEmail, Name: row.OwnerName, Surname: row.OwnerSurname}
	}
	return out
}
