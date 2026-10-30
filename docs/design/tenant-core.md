# Tenant core — design study

Status: **phase 1 implemented** (backend: schema, organizations module, per-request org context, cache; see §11). Later phases are proposals. Reference implementation: `otopoly-go` (forked from this boilerplate, added organizations on top). This document keeps what worked there, fixes what hurt, and makes it reusable for every app cloned from the boilerplate.

## 1. Goal

Apps built from the boilerplate need a multi-tenant shell (`/t/{slug}`) the same way they already get a platform shell (`/platform`): organizations, memberships, per-organization permissions, organization-scoped data and an admin UI to manage them. A single-scope app (today's CMS shell) must keep working with tenancy switched off.

Out of scope for the core: billing/plans, entitlements, domain seeds, letterhead. These plug in through hooks (§7).

## 2. Lessons from otopoly

| What happened in otopoly | Consequence | Decision here |
|---|---|---|
| Tenant permissions come from **global** roles (`organization_user`, `organization_owner`) unioned across memberships. | An owner of org A carries owner-level `tenant.*` permissions in org B; only routes with `RequireOrgRole("owner")` stop it. Role sync on member removal is a hack. | **Per-organization roles**: permissions resolved from the membership in the current org (§4). |
| Org context lives in the access token (`oid`), set by `POST /v1/auth/organization-context`. | Tab A on `/t/x` silently runs against org y after tab B switches. Every switch rotates single-use refresh tokens, which feeds the parallel-refresh logout bug. The Auth.js cookie grew past nginx buffers (502 on switch). Impersonation, passkey and OAuth sessions lose `oid`, so the client needs a re-switch dance with a ref-held `update()` (infinite-loop fix a13dc92). Refresh must re-validate and quietly drop `oid`. | **Org context per request**: the tenant shell sends `X-Organization: <slug>`; the API checks membership on each request (§5). No `oid`, no switch endpoint, no token rotation on navigation. |
| Tenant login page `/t/{slug}/login`. | Blank-page bugs, two login flows to keep in sync. Removed in TEC-644. | One login (`/login?next=/t/{slug}/…`). |
| Owner sets staff passwords on global accounts. | Cross-tenant account takeover (fixed by `ensureOrgOwnedAccount`, dc250ef). | **Invitations** (email + token); owners never manage another account's credentials (§6). |
| Isolation by convention: every query must filter `organization_id`. | One missed `WHERE` leaks data. | Same convention plus guard rails: repository helper, a lint test over `queries/*.sql`, optional Postgres RLS later (§8). |
| Billing gate (`read_only`) hard-coded inside `RequireOrganization`. | Core and domain mixed. | `AccessPolicy` hook (§7). |

## 3. Data model (migrations 000025+)

```sql
-- 000025_organizations
organizations (
  id BIGSERIAL PK, uuid UUID UNIQUE DEFAULT gen_random_uuid(),
  slug VARCHAR(64) NOT NULL,            -- unique among live rows
  name VARCHAR(255) NOT NULL,
  logo_object_key TEXT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended')),
  access_starts_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  access_ends_at   TIMESTAMPTZ NULL,    -- NULL = no end
  settings JSONB NOT NULL DEFAULT '{}', -- app-defined profile/preferences
  created_at, updated_at (trigger), deleted_at
)
CREATE UNIQUE INDEX ... ON organizations (slug) WHERE deleted_at IS NULL;

-- 000026_tenant_roles: roles get a scope; tenant roles may be org-specific
ALTER TABLE roles ADD COLUMN scope VARCHAR(16) NOT NULL DEFAULT 'platform'
  CHECK (scope IN ('platform','tenant'));
ALTER TABLE roles ADD COLUMN organization_id BIGINT NULL REFERENCES organizations(id) ON DELETE CASCADE;
-- organization_id NULL + scope='tenant' = template role available to every org
-- (seeded: owner, admin, member). Org-specific custom roles come later.

-- 000027_organization_members
organization_members (
  id BIGSERIAL PK,
  organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id BIGINT NOT NULL REFERENCES roles(id),   -- must be scope='tenant'
  created_at, updated_at,
  UNIQUE (organization_id, user_id)
)

-- 000028_organization_invitations
organization_invitations (
  id, uuid, organization_id FK, email CITEXT, role_id FK,
  token_hash TEXT NOT NULL, expires_at, accepted_at NULL, revoked_at NULL,
  invited_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL, created_at
)

-- 000029_org_scoped_engines
ALTER TABLE export_jobs, import_jobs, bulk_jobs, activity_events, notifications
  ADD COLUMN organization_id BIGINT NULL REFERENCES organizations(id) ON DELETE CASCADE/SET NULL;
-- + indexes (organization_id, created_at DESC)

-- 000030_seed_tenant_rbac
-- platform.organizations.read/write (super_admin)
-- tenant.settings.read/write, tenant.members.read/write, tenant.roles.read/write
-- template roles owner (all tenant.*), admin (all but tenant.roles.write), member (read)
```

Existing leftovers get wired or deleted in the same work: `EnqueueInput.TenantID/WorkspaceID`, `events.TenantID`, `outbox.TenantID`, `apiquery.Tenant*Sort`, `storage.TenantLogoObjectKey/WorkspaceLogoObjectKey`, deprecated `middleware.RequireTenant`.

## 4. Permissions

- Platform permissions stay global (`user_roles` → `platform.*`). Nothing changes for `/platform`.
- Tenant permissions come only from `organization_members.role_id` for the **current** organization. A user who is owner in A and member in B has owner rights in A and member rights in B.
- `super_admin` gets no tenant bypass: tenant access needs a membership (or impersonating a member). Platform admins manage organizations through `/v1/platform/organizations`.
- Slug convention: `tenant.<resource>.<action>`. Same triad rule as today (migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts`).
- `/v1/auth/me` returns `organizations[]: {uuid, slug, name, logo_url, status, access_ends_at, role: {key, name}, permissions: string[]}`; the frontend `usePermission()` reads the current org's list inside the tenant shell. Permissions are **not** put in the JWT (cookie size).

## 5. Request flow

```
Browser (/t/acme/...)            BFF (/api/v1/*)                 Go API
  tenantRequest()  ── X-Organization: acme ──▶ forwards header ──▶ authn (JWT, no oid)
                                                                   RequireOrganization
                                                                     • resolve slug → org (cache)
                                                                     • membership(user, org) (cache)
                                                                     • status/access window
                                                                     • AccessPolicy hooks
                                                                     • orgctx.Scope{ID, UUID, Slug, Role, Permissions}
                                                                   RequireTenantPermission("tenant.x.read")
                                                                   handler → usecase(scope.ID) → sqlc (WHERE organization_id=$n)
```

- Header name `X-Organization`, value = slug (readable in logs; uuid also accepted).
- Missing header on `/v1/tenant/*` → `400 ORGANIZATION_REQUIRED`; not a member → `403 NO_TENANT_MEMBERSHIP`; suspended → `403 ORGANIZATION_SUSPENDED`; outside the access window → `403 ORGANIZATION_ACCESS_EXPIRED`.
- Cache: Redis key `org:{slug}` and `orgmember:{orgID}:{userID}` (60 s TTL) plus a per-org version counter bumped on suspend / extend / member change / role change, so revocation is effectively immediate.
- The BFF allow-list forwards `X-Organization` only on `/api/v1/tenant/*` paths; realtime channel tokens for `org:{uuid}` channels check membership the same way.

## 6. Membership flows

- **Create organization**: platform admin (`POST /v1/platform/organizations`) or self-serve (`POST /v1/auth/organizations`, gated by a config flag + registration setting). Creator becomes `owner`. Slug allocated from the name with a reserved list.
- **Invite**: `POST /v1/tenant/invitations {email, role}` → email with `/invite/{token}`. Accepting signs in (or registers through the normal flows) and creates the membership. Expiry 7 days, single use, revocable.
- **Change role / remove member**: row lock on the org, "last owner" guard (`409 LAST_ORGANIZATION_OWNER`), cache version bump.
- **Leave organization**: a member can leave unless they are the last owner.
- **Account deletion** (already exists): memberships are removed; organizations where the user was the last owner are flagged for platform follow-up.

## 7. Extension points (keep the core domain-free)

```go
type OrganizationHooks interface {
    OnCreated(ctx context.Context, tx pgx.Tx, org Organization, owner User) error // seeds, trials
    OnDeleted(ctx context.Context, org Organization) error
}
type AccessPolicy interface { // e.g. billing read-only
    Check(ctx context.Context, scope orgctx.Scope, r *http.Request) error
}
```

Registered in `internal/httpserver/server.go`; the core ships with none.

## 8. Isolation guard rails

1. Every tenant table has `organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE` and an index starting with it.
2. Repositories take `orgID int64` explicitly (never read it from context inside SQL helpers).
3. `internal/database/queries_test.go` parses `queries/*.sql`: any query touching a table listed in `tenant_tables.txt` must reference `organization_id`. CI fails otherwise.
4. IO / bulk / activity engines stamp `organization_id` from `orgctx` (otopoly: `ioengine.QueryOrganizationID`, `activity.WithOrganization`, bulk `executeTenant`).
5. Optional later: Postgres RLS with `SET LOCAL app.organization_id` per transaction.

## 9. Frontend

- Route group `app/(tenant)/t/[slug]/` with `TenantProvider` (public org by slug) → `TenantRouteGuard` (membership/expiry cards, optimistic render from the cached user) → `AppLayout variant="tenant"`.
- `tenantRequest()` API helper adds `X-Organization` from the route slug; no session switching, so tabs on different orgs are independent.
- `tenantNav(slug)` catalog in `config/nav.ts`; `Tenant*` permission keys; `routes.tenant.*` builders; `/login?next=/t/{slug}` only.
- Org switcher in the header lists `me.organizations`; `/onboarding/organization` for signed-in users without one (when self-serve is on).
- Platform UI: organizations list/detail (status, access window, members, invitations, activity).
- Shell choice after login: `next` wins; otherwise platform users → `/platform`, users with one org → `/t/{slug}`, several → org picker, none → onboarding or CMS (single-scope apps).

## 10. Configuration

| Key | Default | Meaning |
|---|---|---|
| `TENANCY_ENABLED` | `true` | Mounts tenant routes. ClickFix is always multi-tenant; off only for tooling. |
| `TENANCY_INVITE_TTL` | `168h` | Invitation lifetime. |

## 11. Delivery plan

| Phase | Scope | Done when |
|---|---|---|
| 1. Schema + core | Migrations 000025–000030, sqlc, `orgctx`, `RequireOrganization` + `RequireTenantPermission`, cache, response codes, `/me` organizations, platform org CRUD + members, OpenAPI | Go tests cover membership, per-org permissions, suspension, access window, last-owner guard, cache invalidation |
| 2. Frontend shell | `(tenant)` group, provider/guard, `tenantRequest`, nav, switcher, permissions per org, login routing, platform organizations UI | `/t/{slug}` works in two tabs on two orgs at once |
| 3. Invitations + settings | Invitation flow + email template, tenant settings (name, logo, `settings` JSON), members page | Invite → accept → role change → remove, end to end |
| 4. Engines | IO / bulk / activity / notifications org stamping, realtime org channels, query lint test | A tenant export only contains its org's rows; lint test in CI |
| 5. Docs + skill | Rewrite the AGENTS.md "no tenants" rule, ARCHITECTURE.md, auth.md, new skill `add-tenant-resource` | A new tenant resource can be added by following the skill |

Each phase is one PR (auto-merge after CI) behind `TENANCY_ENABLED`, so `main` stays shippable throughout.

## 12. Open questions

1. Org-specific custom roles in v1, or only the three templates? (Proposal: templates only; the schema already allows custom roles.)
2. Should the CMS shell stay as the "no organization" home, or be dropped when tenancy is on? (Proposal: keep; apps choose.)
3. Domain-based tenants (`acme.example.com`) — not in v1; `X-Organization` can be derived from the host later without API changes.

## 13. ClickFix additions (CF-3, CF-4, CF-5, CF-12, CF-14)

- **Shell**: the workspace is the CF-15 `(cms)` shell at `/`; the active organization comes from the sidebar switcher (persisted store) and the API client sends it as `X-Organization` on `/v1/tenant/*`. There is no `/t/{slug}` route group.
- **Roles**: members hold several roles (`organization_member_roles`); permissions are the union. Templates: owner, admin, supervisor, agent, light_agent, billing, viewer, member. `roles.is_billable` marks paid seats (`GET /v1/tenant/seats`).
- **Team visibility**: `tenant.<resource>.read` = own teams / assignments, `.read_all` = whole organization. Modules check `orgctx.Scope.CanSeeAll("conversations")` and otherwise limit queries to the member's teams (CF-5). The inbox rule is spelled out in §14 (CF-44).
- **Audit**: `activity.Recorder` stamps `organization_id` from the request scope and scrubs personal data (`activity.Scrub`); org admins read it at `/v1/tenant/audit` (CF-12).
- **Support access**: owners/admins grant time-limited `read_only` / `full` access (`/v1/tenant/support-access`); staff with `platform.support.access` then resolve the organization through `orgctx.SupportResolver` (no membership, no impersonation). Their scope has `Support = true` and every recorded action carries `via_support: true` (CF-14).
- **Isolation guard rails**: `internal/database/queries_test.go` with `queries/tenant_tables.txt` (add every new organization-owned table), `mixed_scope_tables.txt` and `isolation_exempt.txt`. Postgres RLS is not enabled; isolation is explicit `organization_id` filtering plus these tests.

## 14. Inbox team visibility and isolation checks (CF-44)

### Rule

A member holding `tenant.conversations.read_all` (owner, admin, supervisor, viewer templates; support access) sees every conversation of the organization. A member with only `tenant.conversations.read` (agent, light_agent) sees a conversation when **any** of these holds:

| # | Condition | Why |
|---|---|---|
| 1 | `assignee_user_id` = the member | Its own work. |
| 2 | `assignee_team_id` is one of the member's teams (`team_members`) | Team queue, assigned or not to a teammate. |
| 3 | `assignee_user_id IS NULL AND assignee_team_id IS NULL` | The unrouted queue: new inbound conversations land here and must be pickable. Inboxes have no team ownership yet; when routing gives channel instances teams, condition 3 narrows to "unrouted conversations of my teams' inboxes". |
| 4 | The member is a participant (`conversation_participants`: added, @mentioned, or a former assignee) | A mention or a hand-over must keep working. |

Everything else (another agent's conversation outside the member's teams, another team's queue) is invisible: lists and badges leave it out and every single-conversation endpoint answers **404** (not 403, so existence does not leak).

### Where it is enforced

- **SQL, written once**: `queries/conversations.sql` arguments `see_all`, `viewer_id`, `viewer_team_ids` on `ListConversations`, `CountConversations`, `ConversationInboxCounts`, `ConversationStatusFacets`; `ConversationVisibleTo` (one conversation) and `FilterVisibleConversationUUIDs` (a set).
- **Go**: `internal/modules/conversations/access` builds the `Viewer` (`access.For`: read_all or team ids). Every inbox route runs behind `handler.WithViewer` (mounted by `conversations.RegisterRoutes` for all routes), which puts the viewer on the context; the usecase filters lists with it and calls `mustSee` after loading a conversation (detail, messages, read marker, reply / note, patch, tags, retry). Workers and webhooks call the usecase without a viewer and are not restricted.
- **Inbox search** (`GET /v1/tenant/conversations?q=`): index matches only add uuids to the SQL `match_uuids`; the SQL rule still applies, so hidden snippets never return.
- **Palette search** (`GET /v1/tenant/search`): conversation and message hits are post-filtered against the database (`FilterVisibleConversationUUIDs`) with a 3× over-fetch. The rule is not indexed as a Meilisearch filter on purpose: an assignment change would have to reindex every message of the conversation, and a stale index must never widen visibility. The filter fails closed (no access queries or a DB error drops those hits).
- **Attachments**: files owned by a message or a conversation (`owner_type` `message` / `conversation`) follow the same rule: `access.AttachmentGuard` (wired with `attachments/handler.WithOwnerAccess`) answers 404 on metadata, download, signed URL and delete for a hidden conversation's files and drops them from `GET /v1/tenant/attachments`. Other owners (invoices, unattached uploads) only need `tenant.attachments.read`.
- **Realtime**: `inbox:{org}` carries every event and is granted to read_all members only. Others subscribe to `inbox:{org}:{user}` (only that user); `LiveNotifier` and the widget fan each event out to the conversation's audience (`ListConversationAudience`: assignee, assigned team's members, participants, or every active member while unrouted). `conversation:{uuid}` (presence, typing) and `POST /v1/tenant/realtime/typing` apply `ConversationVisibleTo`.
- **Tests**: `internal/inbox/visibility_http_test.go` drives the real routes, resolver and realtime token endpoint for list, counts, detail, messages, notes, patch, tags, inbox search, channel subscriptions and the live fan-out, across two organizations.

### Isolation lint

`queries/tenant_tables.txt` lists every table with a NOT NULL `organization_id` (inbox, channels, CRM, custom fields, billing, teams ...); `TestEveryOrgOwnedTableIsListed` parses the migrations and fails when a new one is missing. A query on a listed table must constrain every alias by `organization_id`, or be in `isolation_exempt.txt` with its reason: webhook lookups that resolve the organization from the row (`GetChannelInstanceByPublicUUID`, `GetChannelInstanceByExternal`, provider callbacks), platform admin aggregates, and cross-organization workers (renewals, reconciliation, wallet expiry, snooze wake-up). Stale exemptions fail the test.
