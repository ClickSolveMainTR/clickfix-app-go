# Development

## Prerequisites

- Go 1.22+ (match `backend/go.mod`)
- Node 20.9+ and pnpm
- Docker + Compose
- golang-migrate, sqlc, air, golangci-lint, gofumpt — see [backend/TOOLCHAIN.md](../backend/TOOLCHAIN.md)

## First clone

```sh
cp .env.example .env
```

Set these **before** sharing a Docker host with another stack:

| Variable | Purpose |
|----------|---------|
| `NAME_PREFIX` | **Required in prod.** Container, volume and network names (`${NAME_PREFIX}-postgres`, `${NAME_PREFIX}-app`, …) |
| `COMPOSE_PROJECT_NAME` | Compose project name |
| `LOCAL_NETWORK_NAME` | Local bridge |
| `APP_NETWORK_NAME` | Prod private bridge (default `${NAME_PREFIX}-app`) |
| `*_VOLUME_NAME` | Named volumes so SeaweedFS/Postgres do not collide |

In `compose.prod.yml` every internal host is qualified as `<service>.${APP_NETWORK_NAME}` (`postgres.myapp-app`, `backend.myapp-app`, …). Never use a bare service name: on the shared `dokploy-network` it can resolve to another stack's container.

Root `.env` is canonical. `backend/.env` is a fallback only. Do not keep a long-lived `frontend/.env.local` for API URLs; `API_URL` comes from the root env.

`API_URL` **must** include `/v1` (Go routes are `/v1/...`). Default: `http://127.0.0.1:8080/v1`.

## Local

```sh
make local-dev
make create-super-admin
```

Helpers:

| Target | What it does |
|--------|----------------|
| `make infra` / `make infra-down` | Compose infra only |
| `make backend-dev` | air on `:8080` |
| `make frontend-dev` | `pnpm dev` on `:3000` |
| `make migrate-up` | Apply SQL migrations |
| `make create-super-admin` | Upsert platform admin + `super_admin` role |
| `make search-reindex` | Rebuild Meilisearch indexes (users, roles, …) |
| `make seed-demo` | Create / refresh the demo organization ([demo-org.md](./demo-org.md)) |
| `make check-i18n` | Find missing / unwired translation keys (en+tr, activity, RBAC, exports) |

### Local URLs

| Service | URL |
|---------|-----|
| Frontend | http://localhost:3000 |
| API | http://127.0.0.1:8080 |
| Scalar docs | http://127.0.0.1:8080/docs/ |
| Health / ready | http://127.0.0.1:8080/healthz · `/readyz` |
| Adminer | http://127.0.0.1:8081 |
| S3 API (SeaweedFS) | http://127.0.0.1:9000 |
| MailHog | http://127.0.0.1:8025 |
| Meilisearch | http://127.0.0.1:7700 |
| Centrifugo WS | ws://127.0.0.1:8000/connection/websocket |

Login: http://localhost:3000/login

## Branding

Swap name and colors in `frontend/src/config/brand.ts` and the CSS tokens in the frontend theme. Default product name is **App**.

## VAPID (optional Web Push)

```sh
npx web-push generate-vapid-keys
```

Set `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, and `VAPID_SUBJECT` in `.env`. The frontend registers via `/app-push-sw.js` → BFF → `/v1/notifications/vapid-public-key` + push-subscriptions.

Closed-browser push does not work until these three are set.

Optional frontend public vars (root `.env`): `NEXT_PUBLIC_BFF_BASE_URL`, `NEXT_PUBLIC_REALTIME_ENABLED`, `NEXT_PUBLIC_CENTRIFUGO_URL`, `NEXT_PUBLIC_DEFAULT_LOCALE`, `NEXT_PUBLIC_FALLBACK_LOCALE`.

## Yayınlama

Ürün adı **ClickFix**, alan adı **getclickfix.com**. Dağıtım Dokploy üzerinden, sürüm etiketiyle yapılır:

1. Bir PR `main`'e birleşir ve `ci` yeşil biter.
2. CI'daki `release-tag` işi `main`'in ucuna sıradaki `v0.N` etiketini koyar (`v0.1`, `v0.2`, ...). Etiket zaten varsa tekrar koymaz.
3. Dokploy `v0.*` etiketini görünce compose yığınını (`compose.prod.yml`) yeniden kurar. Elle dağıtım gerekmez; geri dönmek için eski bir etiketi Dokploy'da yeniden dağıtın.

Mobil (`mobile/`) değişiklikleri aynı yeşil `main` çalışmasında EAS ile gönderilir.

| Host | Ne | Dokploy servisi |
| --- | --- | --- |
| `https://getclickfix.com` | Tanıtım sitesi ve uygulama (`/`, `/dashboard`, `/platform`, `/login`; Next.js BFF) | frontend `:3000` |
| `https://api.getclickfix.com` | Go API; webhook ve kanal bağlantı adresleri (`PUBLIC_API_URL`) | backend `:8080` |
| `https://widget.getclickfix.com` | Web widget betiği ve CDN | frontend `:3000` (widget yolları) |
| `wss://rt.getclickfix.com/connection/websocket` | Centrifugo gerçek zamanlı bağlantı (`NEXT_PUBLIC_CENTRIFUGO_URL`, `CENTRIFUGO_WS_URL`) | centrifugo `:8000` |
| `destek@getclickfix.com` | Destek gelen kutusu, VAPID konusu (`VAPID_SUBJECT`) | posta sağlayıcısı |
| `noreply@getclickfix.com` | Giden e-posta (`MAIL_FROM`) | posta sağlayıcısı |
| `in.getclickfix.com` | Gelen e-posta kanalı (`EMAIL_INBOUND_DOMAIN`, MX Resend'e) | Resend |

Alan adı sahibinin oluşturacağı DNS kayıtları (`<SUNUCU_IP>` Dokploy sunucusunun genel IP'si):

| Kayıt | Ad | Değer |
| --- | --- | --- |
| A | `getclickfix.com` | `<SUNUCU_IP>` |
| CNAME | `www` | `getclickfix.com` |
| A | `api` | `<SUNUCU_IP>` |
| A | `widget` | `<SUNUCU_IP>` |
| A | `rt` | `<SUNUCU_IP>` |
| MX | `in` | Resend'in verdiği alıcı MX değeri |
| TXT / CNAME | `getclickfix.com`, `resend._domainkey` | SPF + DKIM (Resend / SMTP sağlayıcısının verdiği değerler) |
| TXT | `_dmarc` | `v=DMARC1; p=none; rua=mailto:destek@getclickfix.com` |
| MX | `getclickfix.com` | `destek@` kutusunu barındıran posta sağlayıcısı |

Dokploy ortam değişkenleri (üretim): `AUTH_URL=https://getclickfix.com`, `PUBLIC_FRONTEND_URL=https://getclickfix.com`, `PUBLIC_API_URL=https://api.getclickfix.com`, `CORS_ALLOWED_ORIGINS=https://getclickfix.com`, `CENTRIFUGO_ALLOWED_ORIGINS=https://getclickfix.com`, `AUTH_WEBAUTHN_RP_ID=getclickfix.com`, `MAIL_FROM=noreply@getclickfix.com`, `MAIL_FROM_NAME=ClickFix`, `VAPID_SUBJECT=mailto:destek@getclickfix.com`. Sırlar yalnızca Dokploy'da tutulur, repoya girmez. Sitemap ve robots `SITE_URL` (yoksa `AUTH_URL`) kökünü kullanır.

## Production / Dokploy

1. `cp .env.example .env.server`, set a unique `NAME_PREFIX` and replace every secret (`${VAR:?…}` is required)
2. `make prod-config` then `make prod-up`
3. Plain Docker host only (Dokploy creates it for you): `docker network create dokploy-network` once

Networks: Compose creates a private bridge network per app (`${APP_NETWORK_NAME:-${NAME_PREFIX}-app}`) and every service joins it. Only **frontend** (`:3000`) and **centrifugo** (`:8000`) also join the shared `dokploy-network`, so the proxy can route their domains. The backend is reached through the frontend's `/api` proxy, and SeaweedFS, Postgres, Redis, Meilisearch and the worker stay private (files reach browsers through API endpoints, never a bucket URL). No service publishes host ports.

`APP_ENCRYPTION_KEY` (32-byte, raw or base64) encrypts GitHub App secrets at rest. Generate: `openssl rand -base64 32`. Replace the example key in production.

## Error tracking (optional, Sentry protocol)

The tracker speaks the Sentry protocol. Every runtime variable is optional; an empty DSN turns reporting off.

Source maps (CF-227) are uploaded only by the `publish-images` CI job on `main`, and only when the repository has the `SENTRY_AUTH_TOKEN` Actions secret plus the `SENTRY_ORG` and `SENTRY_PROJECT` Actions variables. The frontend image build then injects debug IDs into the server and browser chunks and uploads the maps. The token reaches only the `pnpm build` step, as a BuildKit secret (never a build arg, env var or image layer), and `.next/static` maps are deleted before the runtime stage, so none are served. Local, PR and Dokploy builds have no token: no upload, no release, nothing sent at build time. A failed upload is logged and does not fail the build.

| Env (Dokploy / `.env.server`) | Passed as | Read by |
|---|---|---|
| `BACKEND_SENTRY_DSN` | `SENTRY_DSN` | backend API + worker |
| `FRONTEND_SENTRY_DSN` | `SENTRY_DSN` | Next.js server, edge, browser |
| `SENTRY_ENVIRONMENT` (default `production`) | same | both |
| `SENTRY_TRACES_SAMPLE_RATE` (default `0`) | same | backend only |

Local dev reads `SENTRY_DSN` from the root `.env` directly.

- **Backend**: 5xx responses and panics (route pattern, status, `request_id` tag), auth misconfiguration warnings (adapter secret mismatch, provider disabled, id_token audience/issuer), queue tasks that failed for good (`task_type` tag; at most one event per task type per 10 minutes, attempts that will be retried are logged at WARN as `queue_task_retry` and not reported), and `ERROR` log lines (one event per message per minute). Business conditions are 4xx with a specific code and never reported (`AI_DISABLED`, `AI_NOT_CONFIGURED`, `AI_NOT_ENTITLED`, `AI_CREDITS_EXHAUSTED`, `VIDEO_NOT_CONFIGURED`, `ASSISTANT_SUPPORT_UNAVAILABLE`, ...). A deliberate, coded `503` (an upstream such as the AI provider did not answer) is logged at WARN (`http_dependency_unavailable`) and reported as a warning at most once per route and code per 15 minutes. Requests the client cancelled are logged as `499` at INFO and never reported. One `http_request` INFO line is written per 4xx and per `/v1/auth/*` / `/v1/internal/auth/*` request (method, route, status, duration, request_id, error_code, scrubbed reason).

## E-mail delivery

Notification e-mails go through the `notifications` queue, never inline. Each delivery is one attempt (`notifications.attempt_count`, at most `max_attempts`, default 5): a transient failure (connect timeout, refused, SMTP 4xx) is retried with exponential backoff (30 s, 1 m, 2 m, 4 m ... capped at 30 m, with jitter); a permanent one (`SMTP_HOST` empty, SMTP 5xx, bad credentials, certificate error) or the last attempt dead-letters the row (`status=failed`, no attempts left, `notification_history` event `dead_lettered`). Rows stuck in `processing` are re-queued at worker start and count as an attempt, so nothing is reclaimed forever. The tracker gets one event per channel and cause per hour.

SMTP settings: `SMTP_DIAL_TIMEOUT` (default `10s`) bounds the TCP connect and `SMTP_SEND_TIMEOUT` (`30s`) the whole exchange. `SMTP_TLS_MODE=starttls` (default) or `implicit` (SMTPS). `SMTP_FALLBACK_PORT` (e.g. `465`, or `2465` for Resend) retries a message once over implicit TLS when the primary port cannot be reached. `GET /v1/platform/health` (Platform → Health) shows the transport (`mail.status`: `ok` / `down` / `not_configured`, probed at most every 5 minutes, no mail sent) and 24-hour delivery counts (sent, retrying, dead-lettered, last error); `/readyz` lists the cached result under `checks.mail` without affecting readiness.
- **Frontend**: `instrumentation.ts` (server/edge, `onRequestError`), `instrumentation-client.ts` (browser), `global-error.tsx`, route error boundaries, the `/api/v1` BFF proxy, and the NextAuth `logger.error` hook (user errors like `CredentialsSignin` are logged, not reported). The browser gets its DSN at runtime from `/api/client-env` (loaded `beforeInteractive` in the root layout), so the image needs no build arg.
- **Never sent**: request bodies, cookies, query strings, `Authorization` / adapter headers, tokens, secrets, emails, phone numbers. The user is the internal user uuid only. Code: `backend/internal/observability`, `frontend/src/lib/observability`.

## Super admin

From the repo root (loads `.env`):

```sh
make create-super-admin
make create-super-admin SA_EMAIL=you@example.com SA_PASSWORD='Secret1' SA_NAME=Ada SA_SURNAME=Admin
```

The same target exists under `backend/` (`make -C backend create-super-admin …`).

## After a backend contract change

```sh
make -C backend sqlc
make -C backend openapi-lint
cd frontend && pnpm api:generate && pnpm typecheck
```
