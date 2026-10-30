package usecase

import (
	"context"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// TestHelpdeskRoleTemplatesAndSeats checks the CF-4 support role templates,
// their permission sets, team visibility (read vs read_all) and paid seat
// counting against the migrated schema.
func TestHelpdeskRoleTemplatesAndSeats(t *testing.T) {
	pool, q := testPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	users := map[string]db.User{}
	for _, name := range []string{"owner", "supervisor", "agent", "light_agent", "billing", "viewer"} {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: "hd-" + name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "T", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users[name] = u
	}
	svc := New(repository.NewPostgres(pool, q), nil)
	org, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Helpdesk " + suffix, OwnerUserID: users["owner"].ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		for _, u := range users {
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", u.ID)
		}
	})
	scopes := map[string]orgctx.Scope{}
	for name, u := range users {
		if name != "owner" {
			if _, err := svc.AddMember(ctx, org.UUID, u.Uuid, name); err != nil {
				t.Fatalf("add %s: %v", name, err)
			}
		}
		s, err := svc.Resolve(ctx, org.Slug, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		scopes[name] = s
	}

	type want struct {
		has, lacks []string
		seeAll     bool
	}
	cases := map[string]want{
		"owner": {has: []string{rbac.PermTenantRolesWrite, rbac.PermTenantConversationsDelete, rbac.PermTenantSlaWrite, rbac.PermTenantBillingWrite}, seeAll: true},
		"supervisor": {
			has:    []string{rbac.PermTenantConversationsReadAll, rbac.PermTenantConversationsAssign, rbac.PermTenantTeamsWrite, rbac.PermTenantReportsExport, rbac.PermTenantKnowledgePublish},
			lacks:  []string{rbac.PermTenantRolesWrite, rbac.PermTenantBillingWrite, rbac.PermTenantChannelsManage, rbac.PermTenantConversationsDelete},
			seeAll: true,
		},
		"agent": {
			has:   []string{rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReply, rbac.PermTenantConversationsNote, rbac.PermTenantTicketsWrite, rbac.PermTenantAiUse},
			lacks: []string{rbac.PermTenantConversationsReadAll, rbac.PermTenantReportsRead, rbac.PermTenantMembersWrite, rbac.PermTenantSettingsWrite},
		},
		"light_agent": {
			has:   []string{rbac.PermTenantConversationsRead, rbac.PermTenantConversationsNote},
			lacks: []string{rbac.PermTenantConversationsReply, rbac.PermTenantConversationsAssign, rbac.PermTenantTicketsWrite},
		},
		"billing": {
			has:   []string{rbac.PermTenantBillingRead, rbac.PermTenantBillingWrite},
			lacks: []string{rbac.PermTenantConversationsRead, rbac.PermTenantMembersWrite},
		},
		"viewer": {
			has:    []string{rbac.PermTenantConversationsReadAll, rbac.PermTenantTicketsReadAll, rbac.PermTenantReportsRead},
			lacks:  []string{rbac.PermTenantConversationsReply, rbac.PermTenantConversationsNote, rbac.PermTenantContactsWrite},
			seeAll: true,
		},
	}
	for role, w := range cases {
		s := scopes[role]
		for _, p := range w.has {
			if !s.Has(p) {
				t.Errorf("%s lacks %s", role, p)
			}
		}
		for _, p := range w.lacks {
			if s.Has(p) {
				t.Errorf("%s must not have %s", role, p)
			}
		}
		if s.CanSeeAll("conversations") != w.seeAll {
			t.Errorf("%s CanSeeAll(conversations) = %v", role, !w.seeAll)
		}
	}

	// Paid seats: owner, supervisor, agent are billable; light agent,
	// billing and viewer are free.
	seats, err := svc.Seats(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if seats.Billable != 3 || seats.Total != 6 {
		t.Fatalf("seats = %+v", seats)
	}
	roles, err := svc.ListTenantRoles(ctx, scopes["owner"])
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]orgmodel.TenantRole{}
	for _, r := range roles {
		byKey[r.Key] = r
	}
	if !byKey["agent"].IsBillable || byKey["light_agent"].IsBillable || byKey["viewer"].IsBillable || !byKey["owner"].IsTemplate {
		t.Fatalf("template flags = %+v", byKey)
	}

	// A free custom role keeps the seat count; giving the viewer a billable
	// role as well makes it take a seat.
	name, free, paid := "Stajyer", false, true
	perms := []string{rbac.PermTenantConversationsRead}
	intern, err := svc.CreateTenantRole(ctx, scopes["owner"], orgmodel.TenantRoleInput{Name: &name, Permissions: &perms, IsBillable: &free})
	if err != nil {
		t.Fatal(err)
	}
	if intern.IsBillable {
		t.Fatal("custom role must keep is_billable=false")
	}
	if _, err := svc.PatchTenantMember(ctx, scopes["owner"], users["viewer"].Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{byKey["viewer"].UUID, intern.UUID},
	}); err != nil {
		t.Fatal(err)
	}
	if s, _ := svc.Seats(ctx, org.ID); s.Billable != 3 {
		t.Fatalf("free custom role changed seats: %+v", s)
	}
	if _, err := svc.PatchTenantRole(ctx, scopes["owner"], intern.UUID, orgmodel.TenantRoleInput{IsBillable: &paid}); err != nil {
		t.Fatal(err)
	}
	if s, _ := svc.Seats(ctx, org.ID); s.Billable != 4 {
		t.Fatalf("billable custom role seats: %+v", s)
	}
}
