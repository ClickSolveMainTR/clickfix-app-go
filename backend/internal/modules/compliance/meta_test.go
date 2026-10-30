package compliance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/usecase"
)

func signMeta(payload, secret string) string {
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write([]byte(enc))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)) + "." + enc
}
func TestMetaSignedRequest(t *testing.T) {
	valid := `{"algorithm":"HMAC-SHA256","user_id":"123456"}`
	for _, tt := range []struct {
		name, raw, secret string
		valid             bool
	}{
		{"valid", signMeta(valid, "facebook-test"), "facebook-test", true},
		{"foreign app", signMeta(valid, "facebook-test"), "instagram-test", false},
		{"missing secret", signMeta(valid, "facebook-test"), "", false},
		{"tampered", signMeta(valid, "facebook-test") + "A", "facebook-test", false},
		{"wrong algorithm", signMeta(`{"algorithm":"none","user_id":"123"}`, "facebook-test"), "facebook-test", false},
		{"no subject", signMeta(`{"algorithm":"HMAC-SHA256"}`, "facebook-test"), "facebook-test", false},
		{"not numeric", signMeta(`{"algorithm":"HMAC-SHA256","user_id":"a"}`, "facebook-test"), "facebook-test", false},
		{"invalid encoding", "*.%%%", "facebook-test", false},
		{"invalid json", signMeta("{", "facebook-test"), "facebook-test", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, err := metaSubject(tt.raw, tt.secret)
			if (err == nil) != tt.valid {
				t.Fatalf("id=%s err=%v", id, err)
			}
			if tt.valid && id != "123456" {
				t.Fatal(id)
			}
		})
	}
}
func TestMetaRoutesRejectWithoutSideEffects(t *testing.T) {
	mux := http.NewServeMux()
	RegisterMetaRoutes(mux, &usecase.Service{}, "facebook-test", "instagram-test")
	for _, app := range []string{"facebook", "instagram"} {
		for _, action := range []string{"data-deletion", "deauthorize"} {
			form := url.Values{"signed_request": {signMeta(`{"algorithm":"HMAC-SHA256","user_id":"1"}`, "foreign")}}
			r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/meta/"+app+"/"+action, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 400 {
				t.Fatal(w.Code)
			}
			var body map[string]any
			if json.Unmarshal(w.Body.Bytes(), &body) != nil {
				t.Fatal("invalid JSON")
			}
		}
	}
	mux = http.NewServeMux()
	RegisterMetaRoutes(mux, &usecase.Service{}, "", "")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/webhooks/meta/instagram/data-deletion", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func TestMetaPublicContractForUnknownSubject(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	svc := &usecase.Service{Repo: repository.New(pool, db.New(pool))}
	mux := http.NewServeMux()
	RegisterMetaRoutes(mux, svc, "facebook-test", "instagram-test")
	for _, app := range []string{"facebook", "instagram"} {
		secret := app + "-test"
		form := url.Values{"signed_request": {signMeta(`{"algorithm":"HMAC-SHA256","user_id":"789456123789456"}`, secret)}}
		r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/meta/"+app+"/data-deletion", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", app, w.Code, w.Body.String())
		}
		var out map[string]string
		if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		code := out["confirmation_code"]
		if len(out) != 2 || len(code) != 32 || out["url"] != "https://getclickfix.com/account-deletion?code="+code {
			t.Fatalf("Meta contract: %+v", out)
		}
		t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM meta_deletion_receipts WHERE code=$1", code) })
		if err = svc.ProcessMetaDeletion(ctx, code); err != nil {
			t.Fatal(err)
		}
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/public/meta/data-deletion/"+code, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"completed"`) {
			t.Fatalf("lookup: %d %s", w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/public/meta/data-deletion/"+strings.Repeat("0", 32), nil))
	if w.Code != 404 {
		t.Fatalf("unknown code: %d", w.Code)
	}
}
