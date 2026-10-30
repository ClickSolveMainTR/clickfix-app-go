package usecase_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type entitled struct{}

func (entitled) Require(context.Context, int64, string) error { return nil }
func (entitled) RequireWritable(context.Context, int64) error { return nil }
func (entitled) CheckLimit(context.Context, int64, string, int64, int64) (entitlement.Decision, error) {
	return entitlement.Decision{Allowed: true}, nil
}
func TestAllowedIPUsesDirectPeerAndRejectsInvalidPrefixes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, peer string
		allow      []string
		want       bool
	}{{"allow", "192.0.2.4:4000", []string{"192.0.2.0/24"}, true}, {"deny", "198.51.100.4:4000", []string{"192.0.2.0/24"}, false}, {"mapped", "[::ffff:192.0.2.4]:4000", []string{"192.0.2.0/24"}, true}, {"invalid", "192.0.2.4:4000", []string{"invalid"}, false}, {"disabled", "192.0.2.4:4000", nil, true}} {
		t.Run(c.name, func(t *testing.T) {
			if got := usecase.AllowedIP(c.peer, c.allow); got != c.want {
				t.Fatalf("allowed=%v", got)
			}
		})
	}
}
func TestOIDCPKCEReplayTenantScopeAndSCIMDeprovision(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, dbURL)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	var org int64
	var orgUUID uuid.UUID
	if e = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'SSO') RETURNING id,uuid", "sso-"+uuid.NewString()[:12]).Scan(&org, &orgUUID); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", org) })
	box, e := crypto.NewSecretBox(strings.Repeat("x", 32))
	if e != nil {
		t.Fatal(e)
	}
	tokens, e := jwt.NewManager("sso-test-secret", 15*time.Minute, 24*time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	nonce := ""
	verifierSeen := ""
	issuer := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kid": "test", "kty": "RSA", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
		case "/token":
			if e := r.ParseForm(); e != nil {
				t.Error(e)
			}
			verifierSeen = r.Form.Get("code_verifier")
			if verifierSeen == "" {
				t.Error("missing PKCE verifier")
			}
			token := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, jwtlib.MapClaims{"iss": issuer, "sub": "external-001", "aud": "enterprise-client", "nonce": nonce, "exp": time.Now().Add(time.Minute).Unix()})
			token.Header["kid"] = "test"
			raw, e := token.SignedString(key)
			if e != nil {
				t.Error(e)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id_token": raw})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	issuer = server.URL
	s := &usecase.Service{Repo: repository.New(pool, q), Box: box, Tokens: tokens, Entitlements: entitled{}, BaseURL: "https://support.example.test/api", HTTP: server.Client()}
	// Direct SQL config is test-only so the fake IdP can use localhost HTTP.
	if _, e = pool.Exec(ctx, "INSERT INTO organization_sso(organization_id,enabled,enforced,issuer,client_id,authorization_url,token_url,jwks_url) VALUES($1,true,true,$2,'enterprise-client',$3,$4,$5)", org, issuer, server.URL+"/authorize", server.URL+"/token", server.URL+"/keys"); e != nil {
		t.Fatal(e)
	}
	in := usecase.SCIMUser{ExternalID: "external-001", UserName: uuid.NewString() + "@example.test"}
	in.Name.GivenName = "Provisioned"
	u, e := s.Provision(ctx, org, in)
	if e != nil {
		t.Fatal(e)
	}
	start, e := s.Start(ctx, orgUUID, "192.0.2.1:1234")
	if e != nil {
		t.Fatal(e)
	}
	auth, e := url.Parse(start.AuthorizationURL)
	if e != nil {
		t.Fatal(e)
	}
	nonce = auth.Query().Get("nonce")
	if auth.Query().Get("code_challenge_method") != "S256" || nonce == "" {
		t.Fatal("missing challenge or nonce")
	}
	out, e := s.Complete(ctx, orgUUID, start.State, "test-code", "oidc", "192.0.2.1:1234", nil)
	if e != nil {
		t.Fatal(e)
	}
	if verifierSeen == "" {
		t.Fatal("code exchange not performed")
	}
	claims, e := tokens.ParseAccess(out.AccessToken)
	if e != nil || claims.SSOOrganizationID != org || claims.IsSuperAdmin || len(claims.Roles) != 0 {
		t.Fatalf("unsafe SSO claims %+v %v", claims, e)
	}
	if _, e = s.Complete(ctx, orgUUID, start.State, "test-code", "oidc", "192.0.2.1:1234", nil); e == nil {
		t.Fatal("SSO state replay accepted")
	}
	row, e := q.SCIMUser(ctx, db.SCIMUserParams{OrganizationID: org, Uuid: u.ID})
	if e != nil {
		t.Fatal(e)
	}
	scope := orgctx.Scope{ID: org, UUID: orgUUID}
	request := httptest.NewRequest("GET", "/v1/tenant/contacts", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	principal := authctx.Principal{UserInternal: row.UserID, SSOOrganizationID: org}
	if e = s.CheckOrganizationAccess(request, scope, authctx.Principal{UserInternal: row.UserID}); !errors.Is(e, orgctx.ErrSSORequired) {
		t.Fatalf("enforced SSO accepted normal session: %v", e)
	}
	if e = s.CheckOrganizationAccess(request, scope, principal); e != nil {
		t.Fatal(e)
	}
	group, e := s.SaveGroup(ctx, org, uuid.Nil, usecase.SCIMGroup{ExternalID: "support", DisplayName: "Support", Members: []usecase.Member{{Value: u.ID}}})
	if e != nil || len(group.Members) != 1 {
		t.Fatalf("group %+v %v", group, e)
	}
	if _, e = s.SetActive(ctx, org, u.ID, false); e != nil {
		t.Fatal(e)
	}
	if e = s.CheckOrganizationAccess(request, scope, principal); e == nil {
		t.Fatal("deprovisioned user retained cached tenant access")
	}
}

func TestClientIPOnlyTrustsExplicitProxyChain(t *testing.T) {
	t.Parallel()
	s := &usecase.Service{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")}}
	for _, c := range []struct{ name, peer, forwarded, want string }{{"untrusted", "192.0.2.5:123", "198.51.100.2", "192.0.2.5:123"}, {"trusted", "10.0.0.1:123", "198.51.100.2, 10.0.0.2", "198.51.100.2:0"}, {"spoofed prefix", "10.0.0.1:123", "1.1.1.1, 198.51.100.2", "198.51.100.2:0"}, {"malformed", "10.0.0.1:123", "invalid", "10.0.0.1:123"}} {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.peer
			r.Header.Set("X-Forwarded-For", c.forwarded)
			if got := s.ClientIP(r); got != c.want {
				t.Fatalf("got=%s want=%s", got, c.want)
			}
		})
	}
}

// BACK-END-9: rotating the SCIM token before SSO was ever saved found no
// organization_sso row, returned an unmapped error and answered 500.
func TestRotateSCIMWithoutSSOConfig(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, dbURL)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	var org int64
	if e = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'SCIM') RETURNING id", "scim-"+uuid.NewString()[:12]).Scan(&org); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", org) })
	s := &usecase.Service{Repo: repository.New(pool, q), Entitlements: entitled{}}
	first, e := s.RotateSCIM(ctx, org)
	if e != nil || first == "" {
		t.Fatalf("first rotation: %q %v", first, e)
	}
	cfg, e := q.SSOConfig(ctx, org)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Enabled || cfg.Enforced || cfg.ScimTokenHash != usecase.Hash(first) {
		t.Fatalf("unexpected settings row %+v", cfg)
	}
	second, e := s.RotateSCIM(ctx, org)
	if e != nil || second == first {
		t.Fatalf("second rotation: %v", e)
	}
	if cfg, e = q.SSOConfig(ctx, org); e != nil || cfg.ScimTokenHash != usecase.Hash(second) {
		t.Fatalf("token not rotated: %v", e)
	}
}
