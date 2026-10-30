package usecase

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/apps/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/apps/repository"
	convrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	staffjwt "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAppValidation(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*model.Input)
		valid bool
	}{
		{name: "valid", edit: func(*model.Input) {}, valid: true},
		{name: "http", edit: func(in *model.Input) { in.URL = "http://example.test" }},
		{name: "javascript", edit: func(in *model.Input) { in.URL = "javascript:alert(1)" }},
		{name: "credentials", edit: func(in *model.Input) { in.URL = "https://user:secret@example.test" }},
		{name: "fragment", edit: func(in *model.Input) { in.URL = "https://example.test/#token" }},
		{name: "html icon", edit: func(in *model.Input) { in.Icon = "<script>" }},
		{name: "unknown scope", edit: func(in *model.Input) { in.Scopes = []string{"system.admin"} }},
		{name: "duplicate scope", edit: func(in *model.Input) { in.Scopes = []string{"conversation.read", "conversation.read"} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := model.Input{Name: "Sidebar", URL: "https://example.test/app", Icon: "puzzle", Placement: "nav", Scopes: []string{"conversation.read"}}
			c.edit(&in)
			if err := in.Validate(); (err == nil) != c.valid {
				t.Fatal(err)
			}
		})
	}
}
func TestContextIsolationScopeIntersectionRevocationAndExpiry(t *testing.T) {
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
	q := db.New(pool)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var org, user int64
	var orgUUID, userUUID uuid.UUID
	suffix := uuid.NewString()
	must(pool.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Apps test',$1) RETURNING id,uuid", suffix).Scan(&org, &orgUUID))
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", org)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id=$1", user)
	}()
	must(pool.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname,status) VALUES($1,'test','App','User','active') RETURNING id,uuid", suffix+"@example.test").Scan(&user, &userUUID))
	svc, err := New(repository.New(q), conv.New(convrepo.New(pool, q), nil, nil), nil, strings.Repeat("test", 8))
	must(err)
	in := model.Input{Name: "App", URL: "https://example.test/sidebar", Placement: "nav", Scopes: []string{"conversation.read", "conversation.reply"}, Enabled: true}
	app, err := svc.Save(ctx, org, user, uuid.Nil, in)
	must(err)
	scope := orgctx.Scope{ID: org, UUID: orgUUID, Permissions: []string{"tenant.conversations.read"}}
	ctx = orgctx.WithScope(ctx, scope)
	ctx = authctx.WithPrincipal(ctx, authctx.Principal{UserID: userUUID, UserInternal: user})
	launch, err := svc.Launch(ctx, app.Uuid, model.LaunchInput{})
	must(err)
	var claims model.Claims
	_, err = jwt.ParseWithClaims(launch.Token, &claims, func(*jwt.Token) (any, error) { return svc.key.Public(), nil }, jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithIssuer(issuer), jwt.WithAudience(app.Uuid.String()), jwt.WithExpirationRequired())
	must(err)
	if claims.Organization != orgUUID || claims.Subject != userUUID.String() || len(claims.Scopes) != 1 || claims.Scopes[0] != "conversation.read" {
		t.Fatal(claims)
	}
	if d := claims.ExpiresAt.Sub(claims.IssuedAt.Time); d != 2*time.Minute {
		t.Fatal(d)
	}
	if strings.Contains(launch.URL, launch.Token) {
		t.Fatal("token in URL")
	}
	if _, ok := svc.key.Public().(ed25519.PublicKey); !ok {
		t.Fatal("wrong key kind")
	}
	raw, err := json.Marshal(svc.JWKS())
	must(err)
	if strings.Contains(string(raw), "private") {
		t.Fatal("private key exposure")
	}
	_, err = svc.Action(ctx, app.Uuid, model.ActionInput{Token: launch.Token, Action: "conversation.reply", RequestID: uuid.New()})
	if !errors.Is(err, model.ErrDenied) {
		t.Fatal("scope escalation accepted", err)
	}
	other := orgctx.WithScope(ctx, orgctx.Scope{ID: org + 100000, UUID: uuid.New(), Permissions: scope.Permissions})
	_, err = svc.Launch(other, app.Uuid, model.LaunchInput{})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cross org app", err)
	}
	forged := claims
	forged.Scopes = []string{"conversation.reply"}
	forged.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	expired, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, forged).SignedString(svc.key)
	must(err)
	_, err = svc.Action(ctx, app.Uuid, model.ActionInput{Token: expired, Action: "conversation.read", RequestID: uuid.New()})
	if !errors.Is(err, model.ErrDenied) {
		t.Fatal("expired context accepted", err)
	}
	// A permitted action is forwarded through the inbox bridge with a stable namespaced message key.
	var contact int64
	var conversation uuid.UUID
	must(pool.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'App contact') RETURNING id", org).Scan(&contact))
	must(pool.QueryRow(ctx, "INSERT INTO conversations(organization_id,number,channel_type,contact_id,status) VALUES($1,1,'email',$2,'pending') RETURNING uuid", org, contact).Scan(&conversation))
	scope.Permissions = append(scope.Permissions, "tenant.conversations.read_all", "tenant.conversations.reply")
	ctx = orgctx.WithScope(ctx, scope)
	contextual, err := svc.Launch(ctx, app.Uuid, model.LaunchInput{Conversation: &conversation})
	must(err)
	tokens, err := staffjwt.NewManager(strings.Repeat("test", 8), time.Minute, time.Hour)
	must(err)
	forwarded := []uuid.UUID{}
	svc.Bridge = &apibridge.Bridge{Tokens: tokens, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/tenant/conversations/"+conversation.String()+"/messages" {
			t.Errorf("unexpected action route %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Organization") != orgUUID.String() {
			t.Error("missing organization binding")
		}
		var body struct {
			ClientID uuid.UUID `json:"client_id"`
			Content  string    `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Content != "App reply" {
			t.Error("wrong action content")
		}
		forwarded = append(forwarded, body.ClientID)
		w.WriteHeader(200)
	})}
	action := model.ActionInput{Token: contextual.Token, Action: "conversation.reply", Content: "App reply", RequestID: uuid.New()}
	for range 2 {
		_, err = svc.Action(ctx, app.Uuid, action)
		must(err)
	}
	if len(forwarded) != 2 || forwarded[0] == uuid.Nil || forwarded[0] != forwarded[1] {
		t.Fatal("unstable message idempotency key", forwarded)
	}
	differentUser := authctx.WithPrincipal(ctx, authctx.Principal{UserID: uuid.New(), UserInternal: user})
	_, err = svc.Action(differentUser, app.Uuid, action)
	if !errors.Is(err, model.ErrDenied) {
		t.Fatal("context used by another user", err)
	}
	revokedScope := scope
	revokedScope.Permissions = []string{"tenant.conversations.read", "tenant.conversations.read_all"}
	_, err = svc.Action(orgctx.WithScope(ctx, revokedScope), app.Uuid, action)
	if !errors.Is(err, model.ErrDenied) {
		t.Fatal("revoked permission accepted", err)
	}

	in.Revision = app.Revision
	in.Enabled = false
	_, err = svc.Save(ctx, org, user, app.Uuid, in)
	must(err)
	_, err = svc.Save(ctx, org, user, app.Uuid, in)
	if !errors.Is(err, model.ErrConflict) {
		t.Fatal("stale app revision accepted", err)
	}
	_, err = svc.Action(ctx, app.Uuid, model.ActionInput{Token: launch.Token, Action: "conversation.read", RequestID: uuid.New()})
	if !errors.Is(err, model.ErrDenied) {
		t.Fatal("disabled app accepted", err)
	}
	_, err = svc.Launch(ctx, app.Uuid, model.LaunchInput{})
	if !errors.Is(err, model.ErrDenied) {
		t.Fatal("disabled app launched", err)
	}
}
