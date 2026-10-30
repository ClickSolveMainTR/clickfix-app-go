package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type testResolver struct{}

func (testResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return orgctx.Scope{}, nil
}
func TestKeyIsolationRevocationAndIdempotency(t *testing.T) {
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
	var org, user int64
	suffix := uuid.NewString()
	if err = tx.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('C2 API test',$1) RETURNING id", suffix).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, "INSERT INTO users(email,password_hash,name,surname,status) VALUES($1,'test','API','Test','active') RETURNING id", suffix+"@example.test").Scan(&user); err != nil {
		t.Fatal(err)
	}
	box, err := crypto.NewSecretBox(strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := jwt.NewManager(strings.Repeat("test", 16), time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	bridge := &apibridge.Bridge{Tokens: tokens, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"stored":true}}`))
	})}
	svc := &Service{Store: repository.New(q), Bridge: bridge, Box: box, Resolver: testResolver{}}
	created, err := svc.Create(ctx, org, user, model.KeyInput{Name: "server", Scopes: []string{"conversations:write"}})
	if err != nil {
		t.Fatal(err)
	}
	raw := created["key"].(string)
	key, err := svc.Authenticate(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if key.TokenHash == raw || key.TokenHash != TokenHash(raw) {
		t.Fatal("unhashed key storage")
	}
	n, err := q.CountIntegrationKeys(ctx, org+100000)
	if err != nil || n != 0 {
		t.Fatal("cross-organization key leak", err)
	}
	for range 2 {
		result, err := svc.Execute(ctx, key, "POST", "/v1/tenant/conversations/"+uuid.NewSHA1(uuid.Nil, []byte("test")).String()+"/messages", "same", []byte(`{"content":"private"}`))
		if err != nil || result.Status != 200 {
			t.Fatal(result, err)
		}
	}
	if calls != 1 {
		t.Fatalf("idempotency executed %d writes", calls)
	}
	if _, err = svc.Execute(ctx, key, "POST", "different", "same", nil); !errors.Is(err, ErrConflict) {
		t.Fatal("changed request reused key", err)
	}
	stored, err := q.GetIntegrationRequest(ctx, db.GetIntegrationRequestParams{OrganizationID: org, KeyID: key.ID, RequestKey: "same"})
	if err != nil {
		t.Fatal(err)
	}
	if json.Valid([]byte(stored.ResponseEnc.String)) {
		t.Fatal("response was stored without encryption")
	}
	if _, err = q.GetIntegrationRequest(ctx, db.GetIntegrationRequestParams{OrganizationID: org + 100000, KeyID: key.ID, RequestKey: "same"}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cross-organization replay", err)
	}
	if _, err = q.RevokeIntegrationKey(ctx, db.RevokeIntegrationKeyParams{OrganizationID: org, Uuid: key.Uuid}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Authenticate(ctx, raw); err == nil {
		t.Fatal("revoked key accepted")
	}
}
