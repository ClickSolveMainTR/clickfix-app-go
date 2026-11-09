package usecase

import (
	"net/http"
	"strings"
	"testing"
)

func TestPublicRouteAllowlist(t *testing.T) {
	id := "74da01e6-edfd-4ac5-959c-e714ccac0ba2"
	for _, tc := range []struct {
		method, path, scope string
		ok                  bool
	}{
		{"POST", "conversations", "conversations:write", true},
		{"POST", "subscriptions", "webhooks:write", true},
		{"DELETE", "subscriptions/" + id, "webhooks:write", true},
		{"GET", "triggers", "triggers:read", true},
		{"PATCH", "contacts/" + id, "contacts:write", true},
		{"GET", "subscriptions", "", false},
		{"POST", "triggers", "", false},
		{"GET", "conversations", "conversations:read", true},
		{"PATCH", "conversations/" + id, "conversations:write", true},
		{"POST", "conversations/" + id + "/messages", "conversations:write", true},
		{"GET", "contacts", "contacts:read", true},
		// CF-195: contacts by the customer's own user id.
		{"POST", "contacts", "contacts:write", true},
		{"PUT", "contacts/by-external-id", "contacts:write", true},
		{"PUT", "contacts/" + id, "", false},
		{"DELETE", "contacts/" + id, "", false},
		{"POST", "companies", "", false},
		{"GET", "companies", "companies:read", true},
		{"PUT", "company-revenue", "revenue:write", true},
		{"PUT", "company-usage", "health:write", true},
		{"POST", "company-events", "events:write", true},
		{"GET", "company-events", "", false},
		{"GET", "company-revenue", "", false},
		{"PUT", "company-revenue/x", "", false},
		{"GET", "../platform/users", "", false},
		{"GET", "api-keys", "", false},
		{"GET", "conversations/not-a-uuid", "", false},
		{"POST", "conversations/" + id + "/messages/retry", "", false},
		{"DELETE", "conversations/" + id, "", false},
		{"GET", "auth/me", "", false},
	} {
		path, scope, ok := Route(tc.method, tc.path)
		if ok != tc.ok || scope != tc.scope {
			t.Errorf("%s %s: %q %q %t", tc.method, tc.path, path, scope, ok)
		}
		if ok && !strings.HasPrefix(path, "/v1/tenant/") {
			t.Fatal("escaped tenant namespace")
		}
	}
	_, _, ok := Route(http.MethodConnect, "conversations")
	if ok {
		t.Fatal("unsupported method")
	}
}

func TestTokenHashAndEntropy(t *testing.T) {
	a, err := RandomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RandomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 43 || a == b || len(TokenHash(a)) != 64 || TokenHash(a) == a {
		t.Fatal("key entropy/hash")
	}
}
