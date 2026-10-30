package usecase

import (
	"context"
	"slices"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine/adapters"
)

func registry() *searchengine.Registry {
	return searchengine.NewRegistry(searchadapters.All(nil)...)
}

func ids(specs []searchengine.Spec) []string {
	out := []string{}
	for _, s := range specs {
		out = append(out, s.ID)
	}
	return out
}

func TestPlatformSpecsExcludeTenantSpecs(t *testing.T) {
	svc := New(nil, registry(), nil)
	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{IsSuperAdmin: true, Permissions: []string{
		rbac.PermPlatformUsersRead, rbac.PermPlatformRolesRead, rbac.PermTenantConversationsRead, rbac.PermTenantContactsRead,
	}})
	got := ids(svc.ListSpecs(ctx))
	if !slices.Equal(got, []string{"users", "roles"}) {
		t.Fatalf("platform specs = %v", got)
	}
}

func TestTenantSpecsFollowTenantPermissions(t *testing.T) {
	svc := New(nil, registry(), nil)
	agent := orgctx.Scope{ID: 4, Permissions: []string{rbac.PermTenantConversationsRead}}
	if got := ids(svc.TenantSpecs(agent)); !slices.Equal(got, []string{"conversations", "messages"}) {
		t.Fatalf("agent specs = %v", got)
	}
	admin := orgctx.Scope{ID: 4, Permissions: []string{rbac.PermTenantConversationsRead, rbac.PermTenantContactsRead}}
	if got := ids(svc.TenantSpecs(admin)); !slices.Equal(got, []string{"conversations", "messages", "contacts", "companies"}) {
		t.Fatalf("admin specs = %v", got)
	}
	if got := svc.TenantSpecs(orgctx.Scope{ID: 4}); len(got) != 0 {
		t.Fatalf("member without permissions sees %v", ids(got))
	}
}

func TestTenantSpecIDs(t *testing.T) {
	allowed := []searchengine.Spec{{ID: "conversations"}, {ID: "messages"}}
	if got := TenantSpecIDs(allowed, ""); !slices.Equal(got, []string{"conversations", "messages"}) {
		t.Fatalf("all = %v", got)
	}
	if got := TenantSpecIDs(allowed, "messages"); !slices.Equal(got, []string{"messages"}) {
		t.Fatalf("one = %v", got)
	}
	if got := TenantSpecIDs(allowed, "contacts"); len(got) != 0 {
		t.Fatalf("a spec the member may not read must give nothing, got %v", got)
	}
}

func TestTenantSearchDisabled(t *testing.T) {
	svc := New(nil, registry(), nil)
	scope := orgctx.Scope{ID: 4, Permissions: []string{rbac.PermTenantConversationsRead}}
	if hits := svc.TenantSearch(context.Background(), scope, 1, "fatura", "", 10); hits != nil {
		t.Fatalf("disabled search = %v", hits)
	}
}
