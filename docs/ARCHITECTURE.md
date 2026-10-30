# Architecture

This repo is a two-app monorepo plus Compose files. Dependencies point inward: HTTP handlers and Next.js pages call use cases / services; they do not own SQL or SMTP.

## Layout

```
.
├── backend/                      Go API + worker
│   ├── cmd/                      server, worker, create-super-admin
│   ├── internal/modules/         auth, notifications, exports, imports,
│   │                             bulk, settings, activity, logs, storage,
│   │                             access, search, integrations
│   ├── internal/platform/        jwt, rbac, events, storage, mail, outbox,
│   │                             ioengine, bulkengine, activity, resourcemeta,
│   │                             stepup, searchengine, crypto
│   ├── migrations/               sequential golang-migrate SQL
│   └── docs/openapi.yaml         OpenAPI 3.1 (Scalar at /docs/)
├── frontend/                     Next.js BFF
│   ├── src/app/(platform)        /platform admin
│   ├── src/app/(cms)             /dashboard workspace shell
│   ├── src/app/(guest)           login, reset, verify
│   ├── src/app/api/v1            BFF proxy → Go
│   ├── src/app/api/mobile/v1     Bearer passthrough for the mobile app → Go
│   └── src/features/             users, roles, notifications, io, bulk-engine,
│                                 storage, logs, nav-engine, step-up-engine,
│                                 search-engine, access, integrations, account, auth
├── mobile/                       Expo app (agent inbox; mobile/README.md)
├── compose.local.yml             infra only
└── compose.prod.yml              full stack
```

## Shells

| Surface | Path | Who |
|---------|------|-----|
| Platform | `/platform` | `super_admin` or any `platform.*` permission |
| CMS | `/dashboard` | organization members (workspace access comes from membership; any account without platform perms) |
| Guest | `/login` (legacy alias at `/platform/login`) | signed-out users |

ClickFix is multi-tenant: organization-scoped modules use `orgctx`. There is no `/business` shell. The public home `/` always renders the marketing site; the workspace starts at `/dashboard`.

## Request flow

```
Browser
  → same-origin /api/v1/*          Next.js BFF (HttpOnly session)
  → $API_URL/...                   Go API (Bearer JWT)
  → use case → repository (sqlc)   PostgreSQL
```

- Access and refresh tokens never enter JavaScript. The BFF stores them in HttpOnly cookies and strips them from JSON responses.
- NextAuth is the frontend session layer; the Go adapter talks to `/v1/internal/auth/*`.
- OpenAPI types are generated into `frontend/src/generated/api.d.ts` via `pnpm api:generate`.

```
Mobile app (mobile/)
  → https://<panel>/api/mobile/v1/*   Next.js passthrough (no session, no cookies)
  → $API_URL/v1/...                   Go API (the app's own Bearer token pair)
```

- The app keeps its token pair in SecureStore and sends `Authorization: Bearer`; the passthrough
  (`frontend/src/lib/server/mobile-proxy.ts`) forwards it unchanged, refuses requests that carry a
  Cookie header and blocks the same paths as the panel BFF (`internal/*`, the QR token exchange).
- Mobile API types: `cd mobile && npm run api:generate` after an OpenAPI change.

## Backend layers

```
handler → usecase → repository interface → sqlc/pgx → PostgreSQL
```

Cross-module signals go through the transactional outbox and durable Asynq consumers (see [EVENTS.md](./EVENTS.md)); `internal/platform/events.Bus` remains for in-process, fail-soft listeners. Handlers never call SMTP or SMS; they enqueue via the notification service (`app:notification:deliver` on the `notifications` queue).

Audit: `activity.Recorder.Record` (fail-soft). Application logs: `internal/modules/logs`.

## Platform engines

| Engine | Package | Job |
|--------|---------|-----|
| Resource meta | `internal/platform/resourcemeta` | `GET /v1/.../meta` — columns, filters, capabilities |
| I/O | `internal/platform/ioengine` | Export / import adapters; jobs on Asynq `exports` / `imports` |
| Bulk | `internal/platform/bulkengine` | Batch mutations; jobs on Asynq `bulk` |
| Search | `internal/platform/searchengine` | Meilisearch indexing + `GET /v1/search` for Cmd+K |
| Storage | `internal/platform/storage` + `modules/storage` | Object bytes in SeaweedFS/S3; metadata in Postgres |
| Activity | `internal/platform/activity` | Audit events for mutations and I/O |
| Step-up | `internal/platform/stepup` | Re-auth grant (password/passkey) before sensitive routes |
| Secrets | `internal/platform/crypto` | AES-256-GCM box for GitHub App credentials |

Adapters register in `internal/httpserver/server.go` and `cmd/worker/main.go`.

## Data stores

| Store | Role |
|-------|------|
| PostgreSQL 18 | Source of truth |
| Redis | Cache, Asynq, ephemeral state |
| SeaweedFS/S3 | Object bytes; metadata stays in Postgres |
| Meilisearch | Cmd+K record search; HMAC/master key stays on the API |
| Centrifugo | Websocket fan-out; HMAC stays on the API |

Media: the browser must only use API-returned `public_url` / stream paths. See `frontend/src/lib/media/urls.ts`.

## RBAC

Users are global. Permissions are the union of assigned roles.

| Seed role | Notes |
|-----------|--------|
| `super_admin` | System role (`is_system=true`). Created by `make create-super-admin`, not a user column. |

Custom roles and permission assignment live under `/v1/platform/roles`. New permission slugs are added only via migrations. Details: [backend/docs/auth.md](../backend/docs/auth.md).
