package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext/usecase"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresStore exercises the CF-152 queries against a migrated database.
func TestPostgresStore(t *testing.T) {
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
	suffix := uuid.NewString()[:8]
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "mobile-" + suffix + "@example.test", PasswordHash: "x", Name: "M", Surname: "S", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	org, err := orgusecase.New(orgrepository.NewPostgres(pool, q), nil).
		CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Mobile " + suffix, OwnerUserID: user.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", user.ID)
	})
	refresh := func() uuid.UUID {
		row, err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
			UserID: user.ID, TokenHash: uuid.NewString(), ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return row.Uuid
	}

	store := New(pool, q)
	svc := usecase.New(store, nil, nil)
	session := refresh()
	issued, err := svc.IssueWidgetToken(ctx, usecase.IssueInput{UserID: user.ID, OrganizationID: org.ID, SessionID: session, DeviceID: "dev-1"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.AuthenticateWidget(ctx, issued.Token)
	if err != nil || id.UserUUID != user.Uuid || id.OrganizationID != org.ID || id.SessionID != session {
		t.Fatalf("authenticate: %+v %v", id, err)
	}

	// Refresh rotation: old row revoked, tokens follow the new session.
	next := refresh()
	if _, err := q.RevokeRefreshTokenByUUIDForUser(ctx, db.RevokeRefreshTokenByUUIDForUserParams{Uuid: session, UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuthenticateWidget(ctx, issued.Token); err != usecase.ErrInvalidWidgetToken {
		t.Fatalf("revoked session must invalidate: %v", err)
	}
	if err := svc.SessionRotated(ctx, user.ID, session, next); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuthenticateWidget(ctx, issued.Token); err != nil {
		t.Fatalf("rebound token: %v", err)
	}

	// Re-issuing on the same device revokes the previous token.
	second, _ := svc.IssueWidgetToken(ctx, usecase.IssueInput{UserID: user.ID, OrganizationID: org.ID, SessionID: next, DeviceID: "dev-1"})
	if _, err := svc.AuthenticateWidget(ctx, issued.Token); err != usecase.ErrInvalidWidgetToken {
		t.Fatalf("superseded token: %v", err)
	}
	if err := svc.RevokeWidgetToken(ctx, user.ID, "dev-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuthenticateWidget(ctx, second.Token); err != usecase.ErrInvalidWidgetToken {
		t.Fatalf("logout must revoke: %v", err)
	}

	// Live Activity tokens.
	tok := func(c string) string { return strings.Repeat(c, 64) }
	reg := func(kind, token, ref string) {
		t.Helper()
		if err := svc.RegisterLiveActivityToken(ctx, user.ID, usecase.LiveActivityTokenInput{
			DeviceID: "dev-1", Kind: kind, ActivityType: usecase.ActivityUrgentConversation, Ref: ref, Token: token, Environment: "sandbox",
		}); err != nil {
			t.Fatal(err)
		}
	}
	reg(usecase.KindPushToStart, tok("a"), "")
	reg(usecase.KindPushToStart, tok("b"), "")
	starts, _ := store.PushToStartTokens(ctx, user.ID, usecase.ActivityUrgentConversation)
	if len(starts) != 1 || starts[0].Token != tok("b") {
		t.Fatalf("superseded push-to-start: %+v", starts)
	}
	ref := uuid.NewString()
	reg(usecase.KindUpdate, tok("c"), ref)
	updates, _ := store.UpdateTokensByRef(ctx, usecase.ActivityUrgentConversation, ref)
	if len(updates) != 1 {
		t.Fatalf("updates %+v", updates)
	}
	if err := store.DisableLiveActivityToken(ctx, updates[0].ID, "ended"); err != nil {
		t.Fatal(err)
	}
	if updates, _ = store.UpdateTokensByRef(ctx, usecase.ActivityUrgentConversation, ref); len(updates) != 0 {
		t.Fatal("disabled token listed")
	}
	reg(usecase.KindUpdate, tok("c"), ref) // re-registering re-enables
	if updates, _ = store.UpdateTokensByRef(ctx, usecase.ActivityUrgentConversation, ref); len(updates) != 1 {
		t.Fatal("re-registered token must be active")
	}

	rcp, err := store.Recipient(ctx, user.ID)
	if err != nil || rcp.PushEnabled || !rcp.LiveActivitiesEnabled || rcp.Locale == "" {
		t.Fatalf("recipient defaults: %+v %v", rcp, err)
	}
	if _, err := svc.SetLiveActivitySettings(ctx, user.ID, usecase.LiveActivitySettings{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if s, _ := svc.GetLiveActivitySettings(ctx, user.ID); s.Enabled {
		t.Fatal("setting must be off")
	}
	if starts, _ = store.PushToStartTokens(ctx, user.ID, usecase.ActivityUrgentConversation); len(starts) != 0 {
		t.Fatal("turning off must delete tokens")
	}
	if err := svc.DeleteLiveActivityToken(ctx, user.ID, tok("b")); err != nil {
		t.Fatal(err)
	}
}
