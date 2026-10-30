package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTriggersScopeDedupCursorAndExpiry(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
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
	var org, other int64
	for _, dst := range []*int64{&org, &other} {
		if err = tx.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Polling test') RETURNING id", uuid.NewString()).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	eventID := uuid.New()
	for range 2 {
		if err = q.IntegrationRecordTrigger(ctx, db.IntegrationRecordTriggerParams{OrganizationID: org, EventID: eventID, EventName: "contact.created", Payload: []byte(`{"id":"` + eventID.String() + `","event":"contact.created"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	h := &Handler{Service: &usecase.Service{Store: repository.New(q)}}
	type page struct {
		Success bool `json:"success"`
		Data    struct {
			Items  []json.RawMessage `json:"items"`
			Cursor string            `json:"next_cursor"`
		} `json:"data"`
	}
	poll := func(organization int64, cursor string, permissions []string) (int, page) {
		t.Helper()
		req := httptest.NewRequest("GET", "/v1/tenant/integration-triggers?event=contact.created&cursor="+cursor, nil)
		req = req.WithContext(orgctx.WithScope(req.Context(), orgctx.Scope{ID: organization, Permissions: permissions}))
		out := httptest.NewRecorder()
		h.Triggers(out, req)
		var p page
		if err := json.Unmarshal(out.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return out.Code, p
	}
	permissions := []string{rbac.PermTenantContactsRead, rbac.PermTenantConversationsRead, rbac.PermTenantConversationsReadAll}
	code, p := poll(org, "0", permissions)
	if code != 200 || len(p.Data.Items) != 1 || p.Data.Cursor != "1" {
		t.Fatalf("deduplicated first page: %d %+v", code, p)
	}
	code, p = poll(org, "1", permissions)
	if code != 200 || len(p.Data.Items) != 0 || p.Data.Cursor != "1" {
		t.Fatalf("empty page cursor: %d %+v", code, p)
	}
	code, p = poll(other, "0", permissions)
	if code != 200 || len(p.Data.Items) != 0 {
		t.Fatal("cross organization event", code, p)
	}
	if code, _ = poll(org, "0", nil); code != 403 {
		t.Fatal("missing permissions", code)
	}
	if code, _ = poll(org, "-1", permissions); code != 400 {
		t.Fatal("negative cursor", code)
	}
	if _, err = tx.Exec(ctx, "UPDATE integration_event_cursors SET pruned_sequence=2 WHERE organization_id=$1", org); err != nil {
		t.Fatal(err)
	}
	if code, _ = poll(org, "1", permissions); code != 410 {
		t.Fatal("expired cursor", code)
	}
}
