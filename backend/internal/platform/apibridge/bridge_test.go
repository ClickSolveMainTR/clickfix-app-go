package apibridge

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBridgePreservesIdentityScopeAndTrace(t *testing.T) {
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{UserUUID: uuid.New(), OrganizationUUID: uuid.New()}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() != "/v1/tenant/conversations?q=test" || r.Method != "GET" {
			t.Fatal(r.URL, r.Method)
		}
		claims, err := tokens.ParseAccess(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if err != nil {
			t.Fatal(err)
		}
		if claims.Subject != p.UserUUID.String() || r.Header.Get("X-Organization") != p.OrganizationUUID.String() {
			t.Fatal("identity/scope mismatch")
		}
		if trace := events.AutomationTrace(r.Context()); len(trace) != 1 || trace[0] != "rule" {
			t.Fatal("trace lost")
		}
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"success":false}`))
	})
	bridge := &Bridge{Handler: handler, Tokens: tokens}
	out, err := bridge.Do(events.WithAutomationTrace(context.Background(), []string{"rule"}), p, "GET", "/v1/tenant/conversations?q=test", nil)
	if err != nil || out.Status != 403 || string(out.Body) != `{"success":false}` {
		t.Fatal(out, err)
	}
}
func TestCaptureBoundsToolResult(t *testing.T) {
	c := &capture{header: http.Header{}, status: 200}
	body := []byte(strings.Repeat("x", MaxResultBytes+10))
	n, err := c.Write(body)
	if err != nil || n != len(body) || c.buf.Len() != MaxResultBytes || !c.truncated {
		t.Fatal("unbounded response")
	}
	c.WriteHeader(500)
	if c.status != 200 {
		t.Fatal("changed already written status")
	}
}
