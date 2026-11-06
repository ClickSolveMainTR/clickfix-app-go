package engineering_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	convmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations"
	convhandler "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type resolver struct{ scope orgctx.Scope }

func (r *resolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) { return r.scope, nil }

type loader struct{ principal authctx.Principal }

func (l loader) LoadPrincipal(*http.Request, jwt.Claims) (authctx.Principal, error) {
	return l.principal, nil
}
func TestRoutesRequireOrganizationPermissionsAndRejectScopeInjection(t *testing.T) {
	tokens, err := jwt.NewManager("test-engineering-access-key-123456", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.New()
	token, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user})
	if err != nil {
		t.Fatal(err)
	}
	orgs := &resolver{scope: orgctx.Scope{ID: 1, UUID: uuid.New(), Permissions: []string{rbac.PermTenantTicketsRead, rbac.PermTenantConversationsReadAll}}}
	mux := http.NewServeMux()
	identity := loader{authctx.Principal{UserID: user, UserInternal: 1}}
	// Register alongside the complete existing inbox route set; ServeMux panics
	// immediately if an engineering route overlaps an existing wildcard pattern.
	convmodule.RegisterRoutes(mux, convhandler.New(nil, nil), tokens, identity, orgs)
	engineering.RegisterRoutes(mux, &handler.Handler{Service: &usecase.Service{Store: &repository.Store{}}}, tokens, identity, orgs)
	request := func(method, path, body string, authenticated, organization bool) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if authenticated {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if organization {
			r.Header.Set(orgctx.HeaderName, "test")
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code
	}
	for _, test := range []struct {
		method, path, body string
		auth, org          bool
		want               int
	}{
		{"GET", "/v1/tenant/engineering/issues", "", false, true, 401},
		{"GET", "/v1/tenant/engineering/issues/meta", "", true, false, 400},
		{"GET", "/v1/tenant/engineering/issues/meta", "", true, true, 200},
		{"PATCH", "/v1/tenant/engineering/issues/" + uuid.NewString(), `{"revision":1,"status":"done"}`, true, true, 403},
		{"GET", "/v1/tenant/engineering/integrations", "", true, true, 403},
		{"POST", "/v1/tenant/engineering/releases", "{}", true, true, 403},
	} {
		if got := request(test.method, test.path, test.body, test.auth, test.org); got != test.want {
			t.Fatalf("%s %s = %d want %d", test.method, test.path, got, test.want)
		}
	}
	// Reading masked integration status needs tenant.settings.read; it never
	// grants configuring credentials or starting OAuth.
	orgs.scope.Permissions = append(orgs.scope.Permissions, rbac.PermTenantSettingsRead)
	for _, test := range []struct{ method, path, body string }{
		{"PUT", "/v1/tenant/engineering/integrations/github", `{"config":{"repository":"acme/app"},"enabled":true}`},
		{"POST", "/v1/tenant/engineering/integrations/github/oauth/start", ""},
		{"POST", "/v1/tenant/engineering/integrations/github/oauth/complete", `{"state":"x","code":"y"}`},
	} {
		if got := request(test.method, test.path, test.body, true, true); got != 403 {
			t.Fatalf("%s %s with settings.read = %d want 403", test.method, test.path, got)
		}
	}
	orgs.scope.Permissions = append(orgs.scope.Permissions, rbac.PermTenantTicketsWrite, rbac.PermTenantConversationsRead)
	if got := request("POST", "/v1/tenant/engineering/issues", `{"title":"x","organization_id":999}`, true, true); got != 400 {
		t.Fatalf("scope injection accepted: %d", got)
	}
	if got := request("POST", "/v1/tenant/engineering/issues", `{"title":"x"} {"title":"y"}`, true, true); got != 400 {
		t.Fatalf("multiple JSON documents accepted: %d", got)
	}
}
