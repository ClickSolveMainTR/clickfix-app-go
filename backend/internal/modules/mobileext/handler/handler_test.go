package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

type fakeAuth struct {
	id  usecase.WidgetIdentity
	err error
	raw string
}

func (f *fakeAuth) AuthenticateWidget(_ context.Context, raw string) (usecase.WidgetIdentity, error) {
	f.raw = raw
	return f.id, f.err
}

type fakeLoader struct {
	claims jwt.Claims
	err    error
}

func (l *fakeLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	l.claims = c
	if l.err != nil {
		return authctx.Principal{}, l.err
	}
	id, _ := c.UserUUID()
	// A loader that (wrongly) granted platform power must be neutralized.
	return authctx.Principal{UserID: id, UserInternal: 9, IsSuperAdmin: true, Permissions: []string{"platform.x"}, SessionID: c.SessionUUID()}, nil
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthenticateWidget(t *testing.T) {
	user, session := uuid.New(), uuid.New()
	auth := &fakeAuth{id: usecase.WidgetIdentity{UserID: 9, UserUUID: user, OrganizationID: 4, SessionID: session}}
	loader := &fakeLoader{}
	var got authctx.Principal
	var org int64
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = authctx.MustPrincipal(r.Context())
		org, _ = WidgetOrganizationFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	mw := AuthenticateWidget(auth, loader)(next)

	req := httptest.NewRequest(http.MethodGet, "/v1/mobile/widget/counts", nil)
	req.Header.Set("Authorization", "widget  tok123 ")
	if rec := serve(mw, req); rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if auth.raw != "tok123" {
		t.Fatalf("raw %q", auth.raw)
	}
	if loader.claims.Subject != user.String() || loader.claims.SessionID != session.String() || len(loader.claims.Roles) != 0 || loader.claims.IsSuperAdmin {
		t.Fatalf("claims %+v", loader.claims)
	}
	if got.IsSuperAdmin || len(got.Permissions) != 0 || got.UserInternal != 9 || org != 4 {
		t.Fatalf("principal %+v org %d", got, org)
	}

	for name, header := range map[string]string{"missing": "", "bearer": "Bearer tok123", "no value": "Widget"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		if rec := serve(mw, req); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}

	auth.err = usecase.ErrInvalidWidgetToken
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Widget x")
	if rec := serve(mw, req); rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token: %d", rec.Code)
	}
	auth.err = nil
	loader.err = errors.New("disabled user")
	if rec := serve(mw, req); rec.Code != http.StatusUnauthorized {
		t.Fatalf("loader failure: %d", rec.Code)
	}
}

func TestRequireWidgetOrganization(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mw := RequireWidgetOrganization(ok)
	req := func(scopeID, tokenOrg int64) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := orgctx.WithScope(r.Context(), orgctx.Scope{ID: scopeID})
		ctx = context.WithValue(ctx, widgetOrgKey{}, tokenOrg)
		return r.WithContext(ctx)
	}
	if rec := serve(mw, req(4, 4)); rec.Code != http.StatusNoContent {
		t.Fatalf("same org: %d", rec.Code)
	}
	if rec := serve(mw, req(5, 4)); rec.Code != http.StatusForbidden {
		t.Fatalf("other org: %d", rec.Code)
	}
}

func withPrincipal(r *http.Request, p authctx.Principal) *http.Request {
	return r.WithContext(authctx.WithPrincipal(r.Context(), p))
}

func errorFields(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Details []struct {
				Field string `json:"field"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("code %q", env.Error.Code)
	}
	out := []string{}
	for _, d := range env.Error.Details {
		out = append(out, d.Field)
	}
	return out
}

func TestHandlerValidation(t *testing.T) {
	// Validation fails before the store is used, so none is needed.
	h := New(usecase.New(nil, nil, nil), nil)
	p := authctx.Principal{UserInternal: 1, SessionID: uuid.New()}

	r := httptest.NewRequest(http.MethodPost, "/v1/mobile/widget-token", strings.NewReader(`{"device_id":""}`))
	r = withPrincipal(r, p)
	r = r.WithContext(orgctx.WithScope(r.Context(), orgctx.Scope{ID: 4}))
	rec := serve(http.HandlerFunc(h.IssueWidgetToken), r)
	if rec.Code != http.StatusBadRequest || strings.Join(errorFields(t, rec), ",") != "device_id" {
		t.Fatalf("issue: %d %s", rec.Code, rec.Body)
	}

	imp := uuid.New()
	r = httptest.NewRequest(http.MethodPost, "/v1/mobile/widget-token", strings.NewReader(`{"device_id":"d"}`))
	r = withPrincipal(r, authctx.Principal{UserInternal: 1, SessionID: uuid.New(), ImpersonatorUserID: &imp})
	r = r.WithContext(orgctx.WithScope(r.Context(), orgctx.Scope{ID: 4}))
	if rec := serve(http.HandlerFunc(h.IssueWidgetToken), r); rec.Code != http.StatusForbidden {
		t.Fatalf("impersonation: %d", rec.Code)
	}

	r = withPrincipal(httptest.NewRequest(http.MethodPost, "/v1/mobile/live-activity-tokens",
		strings.NewReader(`{"device_id":"d","kind":"x","activity_type":"call","token":"zz","environment":"production"}`)), p)
	rec = serve(http.HandlerFunc(h.RegisterLiveActivityToken), r)
	if rec.Code != http.StatusBadRequest || strings.Join(errorFields(t, rec), ",") != "kind,token" {
		t.Fatalf("live activity: %d %s", rec.Code, rec.Body)
	}

	r = withPrincipal(httptest.NewRequest(http.MethodPost, "/v1/mobile/live-activity-tokens", strings.NewReader(`{"unknown":1}`)), p)
	if rec := serve(http.HandlerFunc(h.RegisterLiveActivityToken), r); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", rec.Code)
	}

	r = withPrincipal(httptest.NewRequest(http.MethodPut, "/v1/mobile/live-activity-settings", strings.NewReader(`{}`)), p)
	rec = serve(http.HandlerFunc(h.PutLiveActivitySettings), r)
	if rec.Code != http.StatusBadRequest || strings.Join(errorFields(t, rec), ",") != "enabled" {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}

	r = withPrincipal(httptest.NewRequest(http.MethodDelete, "/v1/mobile/widget-token", nil), p)
	if rec := serve(http.HandlerFunc(h.RevokeWidgetToken), r); rec.Code != http.StatusBadRequest {
		t.Fatalf("revoke without device: %d", rec.Code)
	}
}

type denyAll struct{}

func (denyAll) Allow(context.Context, string, string, int, time.Duration) (bool, time.Duration) {
	return false, 30 * time.Second
}

func TestRateLimitWidget(t *testing.T) {
	h := New(usecase.New(nil, nil, nil), denyAll{})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	r := withPrincipal(httptest.NewRequest(http.MethodGet, "/", nil), authctx.Principal{UserInternal: 3})
	rec := serve(h.RateLimitWidget(next), r)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "31" {
		t.Fatalf("status %d retry %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}
