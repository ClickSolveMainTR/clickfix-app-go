package httpserver

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func docsMux(env string) *http.ServeMux {
	mux := http.NewServeMux()
	s := &Server{log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))}
	s.cfg.App.Env = env
	s.mountDocs(mux)
	return mux
}

func get(mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestDocsIndex_RedirectsToWeb(t *testing.T) {
	t.Parallel()
	for _, env := range []string{"production", "development"} {
		for _, path := range []string{"/docs", "/docs/"} {
			rec := get(docsMux(env), path)
			if rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != "https://getclickfix.com/docs/api" {
				t.Fatalf("%s %s: status = %d, location = %q", env, path, rec.Code, rec.Header().Get("Location"))
			}
		}
	}
}

func TestDocsPublicSpec_NotServed(t *testing.T) {
	t.Parallel()
	for _, env := range []string{"production", "development"} {
		if rec := get(docsMux(env), "/docs/public-api.yaml"); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d", env, rec.Code)
		}
	}
}

func TestDocsInternalContract_HiddenInProduction(t *testing.T) {
	t.Parallel()
	prod := docsMux("production")
	for _, path := range []string{"/docs/openapi.yaml", "/docs/internal/"} {
		if rec := get(prod, path); rec.Code != http.StatusNotFound {
			t.Fatalf("%s in production: status = %d", path, rec.Code)
		}
	}
	dev := docsMux("development")
	rec := get(dev, "/docs/openapi.yaml")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "/v1/auth/login") {
		t.Fatalf("internal contract in development: status = %d", rec.Code)
	}
	if rec := get(dev, "/docs/internal/"); rec.Code != http.StatusOK {
		t.Fatalf("internal docs page in development: status = %d", rec.Code)
	}
}
