package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareLabelsMatchedPatternsAndStatus(t *testing.T) {
	t.Parallel()
	m := &Metrics{Registry: prometheus.NewRegistry(), latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "test_http_seconds", Help: "test"}, []string{"route", "method", "status"})}
	m.Registry.MustRegister(m.latency)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /contacts/{uuid}", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) })
	m.Middleware(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/contacts/private-id", nil))
	out := httptest.NewRecorder()
	m.Handler().ServeHTTP(out, httptest.NewRequest("GET", "/metrics", nil))
	b := out.Body.String()
	if strings.Contains(b, "private-id") || !strings.Contains(b, `route="GET /contacts/{uuid}"`) || !strings.Contains(b, `status="503"`) {
		t.Fatalf("wrong metric labels: %s", b)
	}
}
