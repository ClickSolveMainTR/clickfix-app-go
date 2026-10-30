package middleware

import (
	"context"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type ssoLoader struct{}

func (ssoLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{SSOOrganizationID: c.SSOOrganizationID, UserInternal: 1}, nil
}

type ssoResolver struct{ org int64 }

func (s ssoResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return orgctx.Scope{ID: s.org}, nil
}
func TestOrganizationTokenCannotAccessGlobalOrOtherTenant(t *testing.T) {
	t.Parallel()
	m, e := jwt.NewManager("sso-boundary-test", time.Minute, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	raw, _, e := m.IssueAccess(jwt.AccessInput{UserID: uuid.New(), SSOOrganizationID: 7})
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		name, path string
		org        int64
		want       int
	}{{"global profile", "/v1/auth/profile", 7, 403}, {"other organization", "/v1/tenant/contacts", 8, 403}, {"own organization", "/v1/tenant/contacts", 7, 204}} {
		t.Run(c.name, func(t *testing.T) {
			h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), Authenticate(m, ssoLoader{}), RequireOrganization(ssoResolver{c.org}))
			req := httptest.NewRequest("GET", c.path, nil)
			req.Header.Set("Authorization", "Bearer "+raw)
			req.Header.Set("X-Organization", "test")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != c.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

type policyResolver struct {
	ssoResolver
	err error
}

func (p policyResolver) CheckOrganizationAccess(*http.Request, orgctx.Scope, authctx.Principal) error {
	return p.err
}

func TestOrganizationPolicyDenialCodes(t *testing.T) {
	t.Parallel()
	m, e := jwt.NewManager("sso-policy-test", time.Minute, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	raw, _, e := m.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		name string
		err  error
		code string
	}{
		{"enforced sso", fmt.Errorf("denied: %w", orgctx.ErrSSORequired), "ORGANIZATION_SSO_REQUIRED"},
		{"ip allow-list", fmt.Errorf("denied: %w", orgctx.ErrIPNotAllowed), "ORGANIZATION_IP_NOT_ALLOWED"},
		{"other", errors.New("denied"), "FORBIDDEN"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), Authenticate(m, ssoLoader{}), RequireOrganization(policyResolver{ssoResolver{7}, c.err}))
			req := httptest.NewRequest("GET", "/v1/tenant/context", nil)
			req.Header.Set("Authorization", "Bearer "+raw)
			req.Header.Set("X-Organization", "test")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != 403 || !strings.Contains(w.Body.String(), `"`+c.code+`"`) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
