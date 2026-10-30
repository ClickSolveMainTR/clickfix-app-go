// Ported from takwim-app-go modules/mcpauth/usecase/usecase_test.go.
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
)

type fakeGate struct{ allowed, write bool }

func (g *fakeGate) Check(context.Context, string, model.Subject) (GateResult, error) {
	return GateResult{Allowed: g.allowed, CanWrite: g.write, ToolCount: 3}, nil
}

const issuer = "https://app.test"

// TestTokenLifecycle covers the security properties of the authorization
// server against a real database: PKCE, single-use codes (reuse revokes the
// connection), refresh rotation with reuse detection, scope narrowing,
// audience binding, revocation, the gate at refresh and cleanup.
func TestTokenLifecycle(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	var orgID int64
	var orgUUID uuid.UUID
	if err = tx.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('MCP test',$1) RETURNING id,uuid", uuid.NewString()).Scan(&orgID, &orgUUID); err != nil {
		t.Fatal(err)
	}
	suffix := uuid.NewString()[:8]
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "mcpauth-" + suffix + "@example.test", PasswordHash: "x", Name: "M", Surname: "A", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE scope='tenant' AND organization_id IS NULL AND slug='owner'", orgID, user.ID); err != nil {
		t.Fatal(err)
	}

	gate := &fakeGate{allowed: true, write: true}
	svc := New(tx, nil, issuer, nil)
	svc.SetGate(gate)
	redirect := "https://client.example/cb"
	client, err := svc.Register(ctx, "203.0.113.1", model.RegisterInput{ClientName: "lifecycle " + suffix, RedirectURIs: []string{redirect}})
	if err != nil {
		t.Fatal(err)
	}

	authorize := func(verifier, scope string) string {
		t.Helper()
		sum := sha256.Sum256([]byte(verifier))
		to, err := svc.Authorize(ctx, "203.0.113.1", model.AuthorizeInput{
			ClientID: client.ClientID, RedirectURI: redirect, ResponseType: "code",
			CodeChallenge: base64.RawURLEncoding.EncodeToString(sum[:]), CodeChallengeMethod: "S256",
			State: "s", Scope: scope, Resource: issuer + model.ResourceBusiness,
		})
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(to)
		id, err := uuid.Parse(u.Query().Get("request"))
		if err != nil {
			t.Fatalf("consent url %s", to)
		}
		dec, err := svc.Decide(ctx, Actor{UserID: user.ID}, id, model.DecideInput{Decision: model.DecisionApprove, AllowWrite: true, OrganizationUUID: &orgUUID})
		if err != nil {
			t.Fatal(err)
		}
		back, _ := url.Parse(dec.RedirectURL)
		if back.Query().Get("iss") != issuer {
			t.Errorf("redirect without iss: %s", dec.RedirectURL)
		}
		return back.Query().Get("code")
	}
	exchange := func(code, verifier string) (model.TokenResponse, error) {
		return svc.Exchange(ctx, "203.0.113.1", model.TokenInput{
			GrantType: model.GrantAuthorizationCode, ClientID: client.ClientID, Code: code,
			RedirectURI: redirect, CodeVerifier: verifier,
		})
	}
	refresh := func(tok, scope string) (model.TokenResponse, error) {
		return svc.Exchange(ctx, "203.0.113.1", model.TokenInput{
			GrantType: model.GrantRefreshToken, ClientID: client.ClientID, RefreshToken: tok, Scope: scope,
		})
	}
	invalidGrant := func(err error, step string) {
		t.Helper()
		var oe *model.Error
		if !errors.As(err, &oe) || oe.Code != model.ErrInvalidGrant {
			t.Fatalf("%s: err = %v, want invalid_grant", step, err)
		}
	}
	valid := func(raw string) bool {
		_, err := svc.ValidateAccessToken(ctx, raw, model.ResourceBusiness)
		return err == nil
	}

	verifier := strings.Repeat("a", 43)
	code := authorize(verifier, "mcp:read mcp:write")
	_, err = exchange(code, strings.Repeat("b", 43))
	invalidGrant(err, "wrong verifier")

	code = authorize(verifier, "mcp:read mcp:write")
	tok, err := exchange(code, verifier)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Scope != "mcp:read mcp:write" || !valid(tok.AccessToken) {
		t.Fatalf("token = %+v", tok)
	}
	at, _ := svc.ValidateAccessToken(ctx, tok.AccessToken, model.ResourceBusiness)
	if at.Subject.UserID != user.ID || at.Layer() != model.LayerBusiness {
		t.Fatalf("access token = %+v", at)
	}
	if _, err := svc.ValidateAccessToken(ctx, tok.AccessToken, model.ResourcePlatform); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("other audience: %v", err)
	}
	// Reusing the code revokes the connection it issued.
	_, err = exchange(code, verifier)
	invalidGrant(err, "code reuse")
	if valid(tok.AccessToken) {
		t.Fatal("code reuse left the access token alive")
	}

	// Refresh rotates; narrowing drops write; reusing the old refresh token
	// revokes the family.
	tok, err = exchange(authorize(verifier, "mcp:read mcp:write"), verifier)
	if err != nil {
		t.Fatal(err)
	}
	next, err := refresh(tok.RefreshToken, "mcp:read")
	if err != nil {
		t.Fatal(err)
	}
	if next.Scope != "mcp:read" || next.RefreshToken == tok.RefreshToken || !valid(next.AccessToken) {
		t.Fatalf("refresh = %+v", next)
	}
	_, err = refresh(next.RefreshToken, "mcp:read mcp:write")
	if err == nil {
		t.Fatal("refresh widened the scope")
	}
	_, err = refresh(tok.RefreshToken, "")
	invalidGrant(err, "refresh reuse")
	if valid(next.AccessToken) {
		t.Fatal("refresh reuse left the family alive")
	}

	// The gate is asked again at refresh.
	tok, err = exchange(authorize(verifier, ""), verifier)
	if err != nil || tok.Scope != "mcp:read" {
		t.Fatalf("read only token = %+v, %v", tok, err)
	}
	gate.allowed = false
	_, err = refresh(tok.RefreshToken, "")
	invalidGrant(err, "gate at refresh")
	gate.allowed = true

	// Revocation (RFC 7009).
	tok, err = exchange(authorize(verifier, ""), verifier)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Revoke(ctx, "203.0.113.1", client.ClientID, tok.AccessToken); err != nil {
		t.Fatal(err)
	}
	if valid(tok.AccessToken) {
		t.Fatal("revoked token still valid")
	}
	if err := svc.Revoke(ctx, "203.0.113.1", client.ClientID, "unknown"); err != nil {
		t.Fatalf("unknown token: %v", err)
	}
	if _, err := svc.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
}
