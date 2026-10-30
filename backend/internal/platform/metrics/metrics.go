package metrics

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/platformhealth"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns an isolated registry; constructing two servers never double-registers collectors.
type Metrics struct {
	Registry *prometheus.Registry
	latency  *prometheus.HistogramVec
}

func NewMetrics(q *db.Queries, health *platformhealth.Service) *Metrics {
	m := &Metrics{Registry: prometheus.NewRegistry(), latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "clickfix_http_request_duration_seconds", Help: "HTTP latency by matched route and response status.", Buckets: prometheus.DefBuckets}, []string{"route", "method", "status"})}
	m.Registry.MustRegister(m.latency, &operationalCollector{q: q, health: health})
	return m
}
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

type metricWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *metricWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (w *metricWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *metricWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// Middleware measures only matched patterns, never raw paths or UUIDs.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		mw := &metricWriter{ResponseWriter: w}
		completed := false
		defer func() {
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			status := mw.status
			if status == 0 {
				status = 200
				if !completed {
					status = 500
				}
			}
			m.latency.WithLabelValues(route, normalizeMethod(r.Method), strconv.Itoa(status)).Observe(time.Since(start).Seconds())
		}()
		next.ServeHTTP(mw, r)
		completed = true
	})
}

type operationalCollector struct {
	q      *db.Queries
	health *platformhealth.Service
}

func (c *operationalCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, v := range operationalDescriptions {
		ch <- prometheus.NewDesc(v.name, v.help, v.labels, nil)
	}
}
func (c *operationalCollector) Collect(ch chan<- prometheus.Metric) {
	emit := func(name, help string, value float64, labels ...string) {
		names := []string{}
		vals := []string{}
		for i := 0; i < len(labels); i += 2 {
			names = append(names, labels[i])
			vals = append(vals, labels[i+1])
		}
		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(name, help, names, nil), prometheus.GaugeValue, value, vals...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	o, err := c.health.Overview(ctx)
	costs, costErr := c.q.ObservabilityAICost(ctx)
	up := 1.0
	if err != nil || costErr != nil || o.QueueError != "" {
		up = 0
	}
	emit("clickfix_operational_scrape_success", "Whether database and queue health could be collected.", up)
	if err == nil {
		for _, q := range o.Queues {
			emit("clickfix_asynq_queue_depth", "Pending and retry tasks.", float64(q.Pending+q.Retry), "queue", q.Name)
		}
		emit("clickfix_outbox_pending", "Pending outbox events.", float64(o.Outbox.Pending))
		emit("clickfix_outbox_failed", "Failed outbox events.", float64(o.Outbox.Failed))
		emit("clickfix_outbox_oldest_pending_seconds", "Oldest pending event age.", float64(o.Outbox.OldestPendingSeconds))
		for _, v := range o.Channels {
			emit("clickfix_channel_error_ratio", "Failed outbound deliveries over the last 24 hours.", v.ErrorRate, "channel", v.Type)
		}
	}
	if costErr == nil {
		for _, v := range costs {
			emit("clickfix_ai_cost_micros_24h", "AI provider cost in micros over the last 24 hours.", float64(v.CostMicros), "provider", v.Provider)
			emit("clickfix_ai_latency_seconds_24h", "Mean AI provider latency over the last 24 hours.", v.LatencySeconds, "provider", v.Provider)
		}
	}
}

type description struct {
	name, help string
	labels     []string
}

var operationalDescriptions = []description{
	{"clickfix_operational_scrape_success", "Whether database and queue health could be collected.", nil},
	{"clickfix_asynq_queue_depth", "Pending and retry tasks.", []string{"queue"}},
	{"clickfix_outbox_pending", "Pending outbox events.", nil},
	{"clickfix_outbox_failed", "Failed outbox events.", nil},
	{"clickfix_outbox_oldest_pending_seconds", "Oldest pending event age.", nil},
	{"clickfix_channel_error_ratio", "Failed outbound deliveries over the last 24 hours.", []string{"channel"}},
	{"clickfix_ai_cost_micros_24h", "AI provider cost in micros over the last 24 hours.", []string{"provider"}},
	{"clickfix_ai_latency_seconds_24h", "Mean AI provider latency over the last 24 hours.", []string{"provider"}},
}

// Ported from comcode-app-go: collapse arbitrary verbs to a fixed label set.
func normalizeMethod(m string) string {
	switch m = strings.ToUpper(m); m {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions:
		return m
	default:
		return "OTHER"
	}
}
