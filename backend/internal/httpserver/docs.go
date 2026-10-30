package httpserver

import (
	"fmt"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/docs"
)

// mountDocs redirects public documentation to the web site. Only development
// environments expose the internal contract and its Scalar UI.
func (s *Server) mountDocs(mux *http.ServeMux) {
	redirect := func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://getclickfix.com/docs/api", http.StatusPermanentRedirect)
	}
	mux.HandleFunc("GET /docs", redirect)
	mux.HandleFunc("GET /docs/{$}", redirect)

	if s.cfg.App.Env == "production" {
		return
	}
	internal, err := docs.FS.ReadFile("openapi.yaml")
	if err != nil {
		s.log.Error("docs_openapi_missing", "error", err)
		return
	}
	mux.HandleFunc("GET /docs/internal/{$}", serveHTML(internalDocsHTML))
	mux.HandleFunc("GET /docs/openapi.yaml", serveYAML(internal))
}

func serveHTML(page string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(page))
	}
}

func serveYAML(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(body)
	}
}

func scalarPage(lang, title, specURL, extra string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>%s</title>
</head>
<body>
  <div id="app"></div>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.62.4"></script>
  <script>
    Scalar.createApiReference('#app', {
      url: '%s',
      persistAuth: true,%s
    });
  </script>
</body>
</html>
`, lang, title, specURL, extra)
}

var internalDocsHTML = scalarPage("en", "ClickFix internal API contract", "/docs/openapi.yaml", "")
