package usecase

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool opens TEST_DATABASE_URL or skips.
func testPool(t *testing.T) (*pgxpool.Pool, *db.Queries) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, db.New(pool)
}

// TestTenantAccessMultiRoleAndGuards covers multi-role memberships (ported
// from rentopia-app-go) against the real schema: permissions are the union
// over a member's roles, role changes invalidate the cache, the grant guards
// hold, custom roles work end to end and members can leave.
func TestTenantAccessMultiRoleAndGuards(t *testing.T) {
	pool, q := testPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	owner, admin, carol := newUser("owner"), newUser("admin"), newUser("carol")
	inv := &fakeInvalidator{}
	svc := New(repository.NewPostgres(pool, q), nil)
	svc.SetInvalidator(inv)
	org, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Access " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, admin.ID, carol.ID})
	})
	if _, err := svc.AddMember(ctx, org.UUID, admin.Uuid, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddMember(ctx, org.UUID, carol.Uuid, "member"); err != nil {
		t.Fatal(err)
	}
	ownerScope, err := svc.Resolve(ctx, org.Slug, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	adminScope, err := svc.Resolve(ctx, org.Slug, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := svc.ListTenantRoles(ctx, ownerScope)
	if err != nil {
		t.Fatal(err)
	}
	roleUUID := map[string]uuid.UUID{}
	for _, r := range roles {
		roleUUID[r.Key] = r.UUID
	}
	for _, key := range []string{"owner", "admin", "member"} {
		if roleUUID[key] == uuid.Nil {
			t.Fatalf("template role %s missing from %v", key, roleUUID)
		}
	}

	// Custom role with members.write; member + custom = union of both.
	name := "Ekip Yöneticisi"
	perms := []string{"tenant.members.read", "tenant.members.write"}
	custom, err := svc.CreateTenantRole(ctx, ownerScope, orgmodel.TenantRoleInput{Name: &name, Permissions: &perms})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if custom.IsTemplate || custom.Key == "" || len(custom.Permissions) != 2 {
		t.Fatalf("custom role = %+v", custom)
	}
	if _, err := svc.CreateTenantRole(ctx, ownerScope, orgmodel.TenantRoleInput{Name: &name, Permissions: &perms}); !errors.Is(err, ErrRoleKeyTaken) {
		t.Fatalf("duplicate key err = %v", err)
	}
	var ve *ValidationError
	badPerms := []string{"platform.users.read"}
	if _, err := svc.CreateTenantRole(ctx, ownerScope, orgmodel.TenantRoleInput{Name: &name, Permissions: &badPerms}); !errors.As(err, &ve) {
		t.Fatalf("platform permission err = %v", err)
	}
	carolScope, _ := svc.Resolve(ctx, org.Slug, carol.ID)
	if carolScope.Has("tenant.members.write") {
		t.Fatal("member alone must not write members")
	}
	before := len(inv.ids)
	m, err := svc.PatchTenantMember(ctx, ownerScope, carol.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["member"], custom.UUID},
	})
	if err != nil {
		t.Fatalf("patch roles: %v", err)
	}
	if len(m.Roles) != 2 {
		t.Fatalf("carol roles = %v", m.Roles)
	}
	if len(inv.ids) == before {
		t.Fatal("role change must invalidate the organization cache")
	}
	carolScope, err = svc.Resolve(ctx, org.Slug, carol.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"tenant.members.write", "tenant.settings.read"} {
		if !carolScope.Has(p) {
			t.Fatalf("union missing %s: %v", p, carolScope.Permissions)
		}
	}
	if !slices.Contains(carolScope.MemberRoles, custom.Key) || carolScope.MemberRole != "member" {
		t.Fatalf("roles = %v primary %q", carolScope.MemberRoles, carolScope.MemberRole)
	}

	// Guards: a non-owner cannot grant owner, nor a role with permissions
	// it lacks (admin has no tenant.roles.write).
	if _, err := svc.PatchTenantMember(ctx, adminScope, carol.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["owner"]},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin grants owner err = %v", err)
	}
	rolesWriter := "Rol Yöneticisi"
	rwPerms := []string{"tenant.roles.read", "tenant.roles.write"}
	rw, err := svc.CreateTenantRole(ctx, ownerScope, orgmodel.TenantRoleInput{Name: &rolesWriter, Permissions: &rwPerms})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PatchTenantMember(ctx, adminScope, carol.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{rw.UUID},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin grants roles.write err = %v", err)
	}
	// Last owner stays.
	if _, err := svc.PatchTenantMember(ctx, ownerScope, owner.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["admin"]},
	}); !errors.Is(err, ErrLastOrganizationOwner) {
		t.Fatalf("demote last owner err = %v", err)
	}
	if err := svc.RemoveTenantMember(ctx, ownerScope, owner.Uuid); !errors.Is(err, ErrLastOrganizationOwner) {
		t.Fatalf("remove last owner err = %v", err)
	}
	if err := svc.LeaveOrganization(ctx, ownerScope, owner.ID); !errors.Is(err, ErrLastOrganizationOwner) {
		t.Fatalf("last owner leave err = %v", err)
	}

	// Templates are read-only; roles in use cannot be deleted.
	if err := svc.DeleteTenantRole(ctx, ownerScope, roleUUID["admin"]); !errors.Is(err, ErrRoleReadOnly) {
		t.Fatalf("delete template err = %v", err)
	}
	if err := svc.DeleteTenantRole(ctx, ownerScope, custom.UUID); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("delete in-use role err = %v", err)
	}
	// Editing the custom role's permissions changes the member's scope.
	less := []string{"tenant.members.read"}
	if _, err := svc.PatchTenantRole(ctx, ownerScope, custom.UUID, orgmodel.TenantRoleInput{Permissions: &less}); err != nil {
		t.Fatal(err)
	}
	carolScope, _ = svc.Resolve(ctx, org.Slug, carol.ID)
	if carolScope.Has("tenant.members.write") {
		t.Fatalf("patched role permissions = %v", carolScope.Permissions)
	}

	// A member can leave; afterwards the organization does not resolve.
	if err := svc.LeaveOrganization(ctx, carolScope, carol.ID); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, err := svc.Resolve(ctx, org.Slug, carol.ID); !errors.Is(err, orgctx.ErrNotMember) {
		t.Fatalf("left member resolve err = %v", err)
	}
	if err := svc.DeleteTenantRole(ctx, ownerScope, custom.UUID); err != nil {
		t.Fatalf("delete unused role: %v", err)
	}
	if err := svc.RemoveTenantMember(ctx, ownerScope, admin.Uuid); err != nil {
		t.Fatalf("remove admin: %v", err)
	}
}

// TestCrossOrganizationIsolation is the CF-3 acceptance check at the use
// case level: everything is addressed through the caller's scope, so an
// organization never sees or changes another organization's members, roles,
// invitations or profile.
func TestCrossOrganizationIsolation(t *testing.T) {
	pool, q := testPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	alice, bob := newUser("alice"), newUser("bob")
	repo := repository.NewPostgres(pool, q)
	svc := New(repo, nil)
	mgmt := NewManagement(svc, repo, ManagementConfig{FrontendURL: "http://localhost:3000"})
	orgA, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Acme " + suffix, OwnerUserID: alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Beta " + suffix, OwnerUserID: bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{alice.ID, bob.ID})
	})
	scopeA, err := svc.Resolve(ctx, orgA.Slug, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	scopeB, err := svc.Resolve(ctx, orgB.Slug, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Bob is not a member of A: neither slug nor uuid resolves for him.
	if _, err := svc.Resolve(ctx, orgA.Slug, bob.ID); !errors.Is(err, orgctx.ErrNotMember) {
		t.Fatalf("bob resolves A by slug: %v", err)
	}
	if _, err := svc.Resolve(ctx, orgA.UUID.String(), bob.ID); !errors.Is(err, orgctx.ErrNotMember) {
		t.Fatalf("bob resolves A by uuid: %v", err)
	}

	// Members: B's list holds only bob; alice is not found from B.
	membersB, total, err := svc.ListTenantMembers(ctx, scopeB, orgmodel.TenantMemberListFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || membersB[0].User.UUID != bob.Uuid {
		t.Fatalf("B members = %+v", membersB)
	}
	if _, err := svc.GetTenantMember(ctx, scopeB, alice.Uuid); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("B reads alice: %v", err)
	}
	if err := svc.RemoveTenantMember(ctx, scopeB, alice.Uuid); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("B removes alice: %v", err)
	}

	// Roles: A's custom role is invisible and unusable from B.
	name := "Acme Özel"
	perms := []string{"tenant.members.read"}
	roleA, err := svc.CreateTenantRole(ctx, scopeA, orgmodel.TenantRoleInput{Name: &name, Permissions: &perms})
	if err != nil {
		t.Fatal(err)
	}
	rolesB, err := svc.ListTenantRoles(ctx, scopeB)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rolesB {
		if r.UUID == roleA.UUID {
			t.Fatal("B lists A's custom role")
		}
	}
	if _, err := svc.GetTenantRole(ctx, scopeB, roleA.UUID); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("B reads A's role: %v", err)
	}
	if err := svc.DeleteTenantRole(ctx, scopeB, roleA.UUID); err == nil {
		t.Fatal("B deletes A's role")
	}
	var ve *ValidationError
	if _, err := svc.PatchTenantMember(ctx, scopeB, bob.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleA.UUID},
	}); !errors.As(err, &ve) {
		t.Fatalf("B assigns A's role: %v", err)
	}

	// Invitations: created in A, not visible / revocable from B.
	invA, err := mgmt.CreateInvitation(ctx, scopeA, Actor{UserID: alice.ID}, orgmodel.CreateInvitationInput{
		Email: "guest-" + suffix + "@example.test", RoleUUIDs: []uuid.UUID{roleA.UUID},
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	listB, totalB, err := mgmt.ListInvitations(ctx, scopeB.ID, orgmodel.InvitationListFilter{Limit: 50})
	if err != nil || totalB != 0 || len(listB) != 0 {
		t.Fatalf("B invitations = %v %d %v", listB, totalB, err)
	}
	if _, err := mgmt.RevokeInvitation(ctx, scopeB, Actor{UserID: bob.ID}, invA.UUID); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("B revokes A's invitation: %v", err)
	}

	// Profile: patching B leaves A untouched.
	tz := "Europe/Berlin"
	if _, _, err := mgmt.PatchProfile(ctx, scopeB.ID, orgmodel.PatchProfileInput{Timezone: &tz}); err != nil {
		t.Fatal(err)
	}
	profileA, err := mgmt.GetProfile(ctx, scopeA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if profileA.Timezone != "Europe/Istanbul" {
		t.Fatalf("A timezone changed: %s", profileA.Timezone)
	}
}

// TestOrganizationProfile checks defaults, validation and persistence of the
// tenant profile.
func TestOrganizationProfile(t *testing.T) {
	pool, q := testPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	owner, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "profile-" + suffix + "@example.test", PasswordHash: "x", Name: "P", Surname: "T", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPostgres(pool, q)
	svc := New(repo, nil)
	mgmt := NewManagement(svc, repo, ManagementConfig{})
	org, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Profile " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", owner.ID)
	})
	p, err := mgmt.GetProfile(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Timezone != "Europe/Istanbul" || p.Locale != "tr" || p.Currency != "TRY" || p.CountryCode != "TR" ||
		len(p.BusinessHours.Days) != 7 || p.BusinessHours.Days[0].Day != "mon" || p.BusinessHours.Days[6].Open {
		t.Fatalf("defaults = %+v", p)
	}

	// Invalid values come back as field errors, nothing is saved.
	badTZ, badLocale := "Mars/Base", "de"
	hours := orgmodel.DefaultBusinessHours()
	hours.Days[2].End = "08:00"
	_, _, err = mgmt.PatchProfile(ctx, org.ID, orgmodel.PatchProfileInput{
		Timezone: &badTZ, Locale: &badLocale, BusinessHours: &hours,
		Company:  &orgmodel.Company{TaxNumber: "12AB", Email: "nope", Website: "ftp://x"},
		Branding: &orgmodel.Branding{PrimaryColor: "red"},
	})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("invalid patch err = %v", err)
	}
	fields := map[string]bool{}
	for _, d := range ve.Details {
		fields[d.Field] = true
	}
	for _, f := range []string{"timezone", "locale", "business_hours.days.2.end", "company.tax_number", "company.email", "company.website", "branding.primary_color"} {
		if !fields[f] {
			t.Fatalf("missing field error %s in %v", f, ve.Details)
		}
	}

	name, cur, cc, loc := "Yeni Ad", "eur", "de", "en"
	hours = orgmodel.DefaultBusinessHours()
	hours.Enabled = true
	saved, changed, err := mgmt.PatchProfile(ctx, org.ID, orgmodel.PatchProfileInput{
		Name: &name, Currency: &cur, CountryCode: &cc, Locale: &loc, BusinessHours: &hours,
		Company:  &orgmodel.Company{LegalName: "Yeni Ad A.Ş.", TaxNumber: "1234567890", Email: "Info@Example.com", Website: "https://example.com"},
		Branding: &orgmodel.Branding{PrimaryColor: "#112233"},
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if len(changed) != 7 || saved.Name != name || saved.Currency != "EUR" || saved.CountryCode != "DE" || saved.Locale != "en" ||
		!saved.BusinessHours.Enabled || saved.Company.Email != "info@example.com" || saved.Branding.PrimaryColor != "#112233" {
		t.Fatalf("saved = %+v changed %v", saved, changed)
	}
	// Other settings keys survive a profile save.
	if _, err := pool.Exec(ctx, `UPDATE organizations SET settings = settings || '{"other":1}'::jsonb WHERE id = $1`, org.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mgmt.PatchProfile(ctx, org.ID, orgmodel.PatchProfileInput{Branding: &orgmodel.Branding{}}); err != nil {
		t.Fatal(err)
	}
	var other int
	if err := pool.QueryRow(ctx, `SELECT (settings->>'other')::int FROM organizations WHERE id = $1`, org.ID).Scan(&other); err != nil || other != 1 {
		t.Fatalf("other settings key lost: %d %v", other, err)
	}
	reloaded, _ := mgmt.GetProfile(ctx, org.ID)
	if reloaded.Company.LegalName != "Yeni Ad A.Ş." || reloaded.Branding.PrimaryColor != "" {
		t.Fatalf("reloaded = %+v", reloaded)
	}
}
