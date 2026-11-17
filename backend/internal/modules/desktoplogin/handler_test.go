package desktoplogin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/google/uuid"
)

type auditRow struct {
	actor   int64
	action  string
	payload map[string]any
}

type fakeAudit struct {
	mu   sync.Mutex
	rows []auditRow
}

func (a *fakeAudit) Record(_ context.Context, actor *int64, action, _ string, _ *uuid.UUID, payload map[string]any, _ *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rows = append(a.rows, auditRow{*actor, action, payload})
}

func (a *fakeAudit) actions() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []string{}
	for _, r := range a.rows {
		out = append(out, r.action)
	}
	return out
}

func TestHandlerFlowAndAudit(t *testing.T) {
	f := newFixture(t)
	audit := &fakeAudit{}
	h := NewHandler(f.svc, nil, audit)

	body := `{"state":"` + testState + `","code_challenge":"` + S256(testVerifier) + `","code_challenge_method":"S256"}`
	req := httptest.NewRequest("POST", "/v1/auth/desktop/codes", strings.NewReader(body))
	req = req.WithContext(authctx.WithPrincipal(req.Context(), authctx.Principal{UserID: f.user, UserInternal: 7}))
	rec := httptest.NewRecorder()
	h.CreateCode(rec, req)
	if rec.Code != http.StatusCreated || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create %d %s", rec.Code, rec.Body)
	}
	var created struct {
		Data Created `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	exchange := func(verifier string) *httptest.ResponseRecorder {
		b := `{"code":"` + created.Data.Code + `","code_verifier":"` + verifier + `","state":"` + testState + `","client_os":"windows"}`
		rec := httptest.NewRecorder()
		h.Exchange(rec, httptest.NewRequest("POST", "/v1/auth/desktop/exchange", strings.NewReader(b)))
		return rec
	}
	if rec := exchange(strings.Repeat("y", 43)); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "DESKTOP_LOGIN_INVALID") {
		t.Fatalf("wrong verifier %d %s", rec.Code, rec.Body)
	}
	rec = exchange(testVerifier)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"refresh_token":"rt"`) {
		t.Fatalf("exchange %d %s", rec.Code, rec.Body)
	}
	if rec := exchange(testVerifier); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "DESKTOP_LOGIN_INVALID") {
		t.Fatalf("reuse %d %s", rec.Code, rec.Body)
	}

	want := []string{ActionCodeCreated, ActionVerifierMismatch, ActionExchanged, ActionCodeReused}
	if got := audit.actions(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("audit %v", got)
	}
	for _, r := range audit.rows {
		if r.actor != 7 {
			t.Fatalf("actor %+v", r)
		}
		for _, v := range r.payload {
			if s, ok := v.(string); ok && (s == created.Data.Code || s == testVerifier || s == "rt") {
				t.Fatalf("secret in audit payload %+v", r)
			}
		}
	}
}

func TestHandlerRejectsUnknownFields(t *testing.T) {
	f := newFixture(t)
	h := NewHandler(f.svc, nil, nil)
	rec := httptest.NewRecorder()
	h.Exchange(rec, httptest.NewRequest("POST", "/v1/auth/desktop/exchange", strings.NewReader(`{"code":"x","access_token":"y"}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestHandlerWebSessionRefusesImpersonation(t *testing.T) {
	f := newFixture(t)
	h := NewHandler(f.svc, nil, nil)
	imp := uuid.New()
	req := httptest.NewRequest("POST", "/v1/auth/desktop/web-session", nil)
	req = req.WithContext(authctx.WithPrincipal(req.Context(), authctx.Principal{UserID: f.user, UserInternal: 7, ImpersonatorUserID: &imp}))
	rec := httptest.NewRecorder()
	h.WebSession(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}

	req = httptest.NewRequest("POST", "/v1/auth/desktop/web-session", nil)
	req = req.WithContext(authctx.WithPrincipal(req.Context(), authctx.Principal{UserID: f.user, UserInternal: 7, SessionID: uuid.New()}))
	rec = httptest.NewRecorder()
	h.WebSession(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"refresh_token":"web-rt"`) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
