package statuspage_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/handler"
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

// Agents hold tenant.conversations.read without read_all: they may list
// incidents (CF-162) but nothing else under /v1/tenant/status.
func TestAgentsReadIncidentsButNothingElse(t *testing.T) {
	tokens, err := jwt.NewManager("test-statuspage-access-key-123456", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.New()
	token, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user})
	if err != nil {
		t.Fatal(err)
	}
	orgs := &resolver{scope: orgctx.Scope{ID: 1, UUID: uuid.New()}}
	mux := http.NewServeMux()
	statuspage.RegisterRoutes(mux, handler.New(nil, nil, nil), tokens, loader{authctx.Principal{UserID: user, UserInternal: 1}}, orgs)
	request := func(method, path string) int {
		r := httptest.NewRequest(method, path, strings.NewReader("{}"))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(orgctx.HeaderName, "test")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code
	}
	// `sort` is rejected by the handler before it reaches the service, so a
	// 400 proves the request got past the permission gate.
	const list = "/v1/tenant/status/incidents?sort=title"
	if got := request("GET", list); got != 403 {
		t.Fatalf("GET incidents without permissions = %d want 403", got)
	}
	orgs.scope.Permissions = []string{rbac.PermTenantConversationsRead, rbac.PermTenantConversationsManage}
	if got := request("GET", list); got != 400 {
		t.Fatalf("GET incidents as agent = %d want 400", got)
	}
	for _, route := range [][2]string{
		{"POST", "/v1/tenant/status/incidents"},
		{"POST", "/v1/tenant/status/incidents/" + uuid.NewString() + "/updates"},
		{"GET", "/v1/tenant/status/page"},
		{"GET", "/v1/tenant/status/components"},
	} {
		if got := request(route[0], route[1]); got != 403 {
			t.Fatalf("%s %s as agent = %d want 403", route[0], route[1], got)
		}
	}
}
