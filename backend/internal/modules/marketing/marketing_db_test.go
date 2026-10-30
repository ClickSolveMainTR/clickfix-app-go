package marketing_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/marketing"
)

// TestCaptureSignupAgainstPostgres: a 201 from the register route stores the
// header's attribution on the user; organization creation without a header
// copies the owner's; the sources report counts it; settings round-trip.
func TestCaptureSignupAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for marketing DB tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)

	var userID int64
	var userUUID uuid.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO users (email, password_hash, name, surname, status) VALUES ($1, 'x', 'Elif', 'Yıldız', 'active') RETURNING id, uuid", uuid.NewString()+"@example.test").Scan(&userID, &userUUID); err != nil {
		t.Fatal(err)
	}
	var orgID int64
	var orgUUID uuid.UUID
	slug := "mkt-" + uuid.NewString()[:8]
	if err := pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, $1) RETURNING id, uuid", slug).Scan(&orgID, &orgUUID); err != nil {
		t.Fatal(err)
	}
	var roleID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM roles WHERE scope = 'tenant' LIMIT 1").Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO organization_members (organization_id, user_id, role_id) VALUES ($1, $2, $3)", orgID, userID, roleID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", orgID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})

	created := func(body string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(body))
		})
	}
	mw := marketing.CaptureSignup(q, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", nil)
	req.Header.Set(marketing.AttributionHeader, url.QueryEscape(`{"ft":{"source":"linkedin","medium":"paid","campaign":"lansman"},"lt":{"gclid":"x1"}}`))
	mw(created(fmt.Sprintf(`{"success":true,"data":{"user":{"uuid":%q}}}`, userUUID))).ServeHTTP(httptest.NewRecorder(), req)

	req = httptest.NewRequest(http.MethodPost, "/v1/onboarding/organizations", nil)
	mw(created(fmt.Sprintf(`{"success":true,"data":{"organization":{"uuid":%q}}}`, orgUUID))).ServeHTTP(httptest.NewRecorder(), req)

	svc := marketing.NewService(q)
	attr, err := svc.ForOrganization(ctx, orgUUID)
	if err != nil {
		t.Fatal(err)
	}
	if attr.Source != "linkedin" || attr.Campaign != "lansman" || attr.LastTouch.Source != "google" {
		t.Fatalf("attribution = %+v", attr)
	}

	rep, err := svc.Sources(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rep.Rows {
		if r.Source == "linkedin" && r.Campaign == "lansman" && r.Organizations >= 1 {
			found = true
		}
	}
	if !found || rep.Total < 1 {
		t.Fatalf("report = %+v", rep)
	}

	before, err := svc.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = svc.Save(context.Background(), before.Config, 0) })
	saved, err := svc.Save(ctx, marketing.Config{GA4ID: "G-TEST1234"}, userID)
	if err != nil || saved.GA4ID != "G-TEST1234" {
		t.Fatalf("save = %+v, %v", saved, err)
	}
}
