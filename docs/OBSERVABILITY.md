# ClickFix launch telemetry (CF-130)

`GET /metrics` is Prometheus text format, authenticated by a Bearer JWT principal
with platform.organizations.read. Do not expose a public scrape route. Use a
restricted service account and a token rotation mechanism (file_sd/secret-managed
bearer_token_file), or place an internal authenticated scrape proxy in front.
JWT access tokens expire: a static one-time token is not an operational setup.
Use job name `clickfix` for the bundled alert expressions. Limit network access
to the API and metrics collector. The scrape endpoint has no organization labels,
user labels, raw paths or message text.

HTTP latency/count histograms use matched ServeMux patterns, method and status;
404s use `unmatched`. Queue depth is pending + retry; outbox pending/failed and
oldest age, channel outbound failure ratio, AI cost (micros) and mean latency are
rolling operational snapshots. AI/channel windows are 24h, not monotonically
increasing counters. HTTP histograms are process-local, so sum rates across API
replicas and never across duplicated exporters scraping the same API instance.
An operational scrape success gauge becomes 0 on database/queue health failures.

Load `deploy/prometheus-alerts.yml` as rule_files. Validate with `promtool check
rules deploy/prometheus-alerts.yml`; tune queue, latency and AI budget thresholds
for the deployment. `deploy/alertmanager.yml` is a receiver skeleton; configure
an actual on-call receiver from secrets and test delivery before launch. Its
empty operations receiver intentionally sends nothing until configured. Route
critical HTTP/outbox/telemetry alerts to paging and warnings to the ops channel.
Use the existing `/v1/platform/health` page as the launch overview.

`/healthz` is process liveness. `/readyz` checks Postgres and Redis with a two-second
budget (503 on failure); search/storage checks are advisory. Dokploy should use
readyz for routing and healthz for liveness. Graceful shutdown stops accepting
requests and flushes Sentry. SENTRY_DSN empty is an explicit no-op; configure DSN,
environment, release and tracing sample rate for both API and worker. Existing
observability tests use a fake Sentry transport and verify scrubbing; acceptance
includes a synthetic error in the deployment and a queue task that exhausts its
retries. Frontend/mobile Sentry release verification belongs to their streams.

X-Request-ID is validated (1–128 ASCII identifier characters), generated when
invalid/missing, and included in API response metadata and structured logs. The
outbox envelope preserves it; Asynq event tasks and direct conversation/SLA/analytics, notification/push, search and import/export/bulk tasks
carry it in headers and restore it on worker context. Scheduled work naturally
has no original request id. Review downstream HTTP provider integrations before
assuming providers echo the same identifier.
