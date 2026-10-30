---
name: search-engine
description: >-
  Cmd+K command palette and Meilisearch-backed record search: backend
  searchengine adapters, compose service, frontend catalog, and RBAC-filtered
  specs. Use when adding searchable resources or extending the command palette.
---

# Search engine

Use when adding **record search** to the Cmd+K palette or wiring a new resource into Meilisearch.

## Backend adapter

1. Implement `searchengine.Adapter` in `backend/internal/platform/searchengine/adapters/`.
   - `Spec()` — `id`, `label_key` (`search.specs_<id>`), `permission`, `icon`, `searchable_fields`
   - `ListAll` — full reindex source
   - `Document` — single entity by UUID
   - Never index secrets (passwords, tokens, SMTP, VAPID).
2. Register in `searchengine.NewRegistry(...)` from `internal/httpserver/server.go` **and** `cmd/worker/main.go`.
3. After create/update/delete in the owning use case, call `Indexer.EnqueueUpsert` / `EnqueueDelete` (fail-soft).
4. OpenAPI: generic `/v1/search` + `/v1/search/specs` already cover new specs; no new routes per resource unless you add custom behavior.

## Organization (tenant) specs — CF-45

ClickFix indexes `conversations`, `messages` (internal notes included), `contacts` and `companies`
(`adapters/tenant.go`, sources in `internal/database/queries/search.sql`).

- `Spec.Tenant = true`, `Permission` is a tenant slug; every `Document` sets `OrganizationID`.
  `EnsureIndex` makes `organization_id` filterable for tenant specs.
- Served only by `GET /v1/tenant/search` / `GET /v1/tenant/search/specs` (`X-Organization`), which
  always add `searchengine.OrgFilter(orgID)`. The platform `/v1/search` never lists tenant specs.
- Large tables implement `searchengine.BatchLister` (keyset batches) so the reindex never loads a
  whole table; message content is cut to `MaxIndexedContent` and hits link to the conversation.
- `Document` returning `searchengine.ErrNotFound` makes the indexer delete the stale document.
- Incremental: the conversations usecase calls its optional `SearchIndexer` hook after commit
  (Receive / Post / Update / tags); `internal/inbox` wires it only when search is enabled.
- Inbox `?q`: the usecase's `SearchFinder` adds index matches (`match_uuids`) to the database
  ILIKE match; inbox filters and paging still run in Postgres. Search off = ILIKE only.
- Adapters list: `searchadapters.All(queries)` (API, worker, `cmd/search-reindex`).

## Frontend spec (optional)

Remote specs are listed by `GET /v1/search/specs`. For local metadata/icons, export from the feature:

```ts
// frontend/src/features/users/search/index.tsx
export const usersSearchSpec = defineSearchSpec({
  id: "users",
  labelKey: "search.specs_users",
  icon: Users,
});
```

Pages/navigation items are built from `config/nav.ts` automatically (permission-filtered).

## Compose + env

- Local: `compose.local.yml` → Meilisearch on `${MEILI_PORT:-7700}`
- Prod: private `app` network only (not `dokploy-network`)
- Root `.env`: `SEARCH_ENABLED`, `MEILI_HOST`, `MEILI_MASTER_KEY`, `MEILI_INDEX_PREFIX`

## Ops

```sh
make search-reindex                              # full rebuild (every adapter)
make search-reindex ARGS="-spec messages -fresh"  # one spec, emptied first
```

Boot: worker/API call `Indexer.Bootstrap` when an index is empty.

## RBAC

- Palette open: any session (`auth.session`)
- Spec chips + hits: filtered by each adapter's `permission` (e.g. `platform.users.read`)
- Super admin sees all registered specs

## Do not

- Expose Meilisearch to the browser
- Use `search` query param (use `q`)
- Index logs/activity in v1 (noisy + sensitive)
