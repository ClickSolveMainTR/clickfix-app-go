package oidc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOrganizationClientRefusesInternalEndpoints: an organization-supplied
// IdP URL that resolves to a non-public address is never dialed.
func TestOrganizationClientRefusesInternalEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	for _, target := range []string{
		"http://localhost:" + port + "/.well-known/openid-configuration",
		"http://127.0.0.1:" + port + "/",
		"http://100.100.100.200/latest/meta-data",
		"http://169.254.169.254/latest/meta-data",
	} {
		resp, err := OrganizationClient().Get(target)
		if err == nil {
			_ = resp.Body.Close()
			t.Fatalf("GET %s succeeded", target)
		}
		if !strings.Contains(err.Error(), "private endpoint forbidden") {
			t.Fatalf("GET %s: %v", target, err)
		}
	}
}
