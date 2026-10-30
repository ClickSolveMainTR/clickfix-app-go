# CF-96 / CF-118 backend contracts

## Existing automation inventory

The main branch already has the following production paths. This change extends them.

| Concern | Existing code | Reuse |
|---|---|---|
| Ordered automation rules, scheduled scans, idempotency | `internal/modules/automation` | Graphs live in the same module; reuse principal resolution, condition validation/evaluation, API bridge, action adapters and Asynq scan task |
| Routing CF-53 and capacity CF-54 | `internal/modules/routing` | Explicit import snapshots conditions, team, skills and routing settings; `RouteWorkflow` calls the same route implementation, capacity/presence snapshot, cursor, assignment and audit path |
| Macros/canned responses CF-48 | `internal/modules/macros` | `send_template` renders an existing permitted canned response; replies/notes go through inbox with stable client IDs |
| SLA | `internal/modules/sla`, `platform/events/domain.go` | Subscribe to existing `sla.breached`, do not implement a second SLA clock |
| Reliable events | `platform/events`, `platform/outbox`, `platform/eventdispatch` | Existing reliable bus subscription persists workflow ingress before outbox success. Existing canonical alias expansion and automation trace propagate across produced events |
| AI procedures | `modules/aiagents/usecase/procedures.go`, `modules/aicore` | Read a published org procedure, run it through the metered AI agent runner, and retain the result on the step |

### Graph and versions

Routes are `/v1/tenant/workflows`, `/{uuid}`, `/{uuid}/publish`, `/{uuid}/enable`, `/{uuid}/versions`, `/{uuid}/test`, `/{uuid}/runs`, `/{uuid}/webhook`; run detail is `/v1/tenant/workflow-runs/{run}`. Require existing `tenant.automations.read/write`; execution re-resolves the publishing user's current membership, active status and action permissions. Run detail also checks current conversation-read permission and conversation visibility before returning step output or the pinned version graph. No new permission grants are invented.

Editor document `schema_version: 1` has `nodes` and `edges`. Node kinds: `trigger`, `condition`, `branch`, `action`, `wait`, `end`. Position is inert editor metadata. Control inputs are `in`; output is `next`, except boolean condition/branch outputs `true` and `false`. Each output has exactly one target. Exactly one trigger is required; all nodes must be reachable from it and able to reach an end. Ordinary edges form a DAG. A loop is allowed only through an explicit `loop: true` edge leaving a wait. Waits are 60 seconds to 30 days; loops still stop at 256 executed steps or 30 days per run. Maximum graph: 100 nodes, 200 edges, 128 KiB. Existing bounded AND/OR condition trees use `all`/`any` and field/operator/value leaves.

Triggers use canonical existing event names: `conversation.created`, `conversation.updated`, `message.received`, `conversation.tag_added`, `sla.breached`, plus `scheduled` and authenticated `webhook`. Schedule intervals are integer minutes. SLA events without a conversation are ignored. Webhook callers supply a conversation UUID and a stable event UUID; this is an authenticated tenant endpoint, not an unprotected public callback.

Actions: `assign_user`, `add_tag`, `remove_tag`, `set_priority`, `set_status`, `send_canned_reply` (content), `send_template` (canned response target UUID), `add_note`, `notify` (user target UUID), `call_webhook` (registered endpoint target UUID), `run_ai_procedure` (published procedure UUID), `create_engineering_issue` (title in value, description in content), and `route` (validated routing snapshot JSON). Wait is a separate node so delay-loop permissions are explicit in the graph.

Saving changes only the draft and increments its optimistic revision. Publishing validates the graph and creates an immutable version. Existing runs remain pinned to that version and publishing identity. Disabling prevents new ingress; already admitted runs retain their version. A plan downgrade blocks admitted runs. Read/history/dry-run remain available without a plan upgrade.

`POST /v1/tenant/workflow-imports/routing` explicitly creates a published **disabled** workflow. It never runs as a migration. The original routing rule is unchanged. Review the graph and choose which routing source should remain enabled before enabling the workflow. Capacity/presence stay live; conditions, team, skills and routing settings are snapshots.

### Execution and limits

The existing Asynq `app:automation:scan` task executes workflow runs. PostgreSQL holds run state, version pin, due time, lease and ordered step log; Redis is only the scheduler. Each tick claims at most 20 runs, one at a time, with a five-minute lease and two-minute node timeout. Ordinary nodes execute immediately; waits release the lease and resume on a later scan, without sleeping. Schedule scans page at 100 conversations with a persistent cursor and stable event identity. Tick granularity is one minute.

Admission is serialized on the org row. `(organization, workflow, event, conversation)` is unique. Org entitlements are `workflows.enabled`, `workflows.count`, `workflows.runs_daily`; they are fail-closed until assigned through the existing billing catalog or org overrides. The migration defines features without silently changing commercial plan grants. A hard ceiling of 100 new runs/org/minute applies even to unlimited plans. Trace ancestry stops repeat workflow IDs and chains deeper than eight.

Each node has a durable started/succeeded/failed/uncertain step. A completed step whose run checkpoint was interrupted resumes from its stored output. A side effect interrupted before its completion checkpoint is marked uncertain and the run fails for manual verification; it is never blindly repeated. Existing message client IDs and outgoing webhook keys additionally deduplicate their effects. This deliberately favors avoiding duplicate external effects over automatic retries of an ambiguous outcome. No exactly-once claim is made for external systems.

AI procedure actions produce a metered suggestion in the step output. They do not grant model-requested tool permissions or send a customer reply. An explicit subsequent reply action handles delivery. The existing AI procedure format and safety pipeline remain authoritative.

Dry-run reads a sample conversation through current visibility checks, produces an inert node/branch trace, simulates deterministic status/priority/assignment/tag mutations, and stops at a wait. It writes no runs, steps, usage counters or outbound effects. External action outcomes are hypothetical.

## Sidebar applications

Org owners/admins with `tenant.settings.write` may register/update/delete HTTPS iframe apps using `/v1/tenant/apps`. Registration holds name, inert icon identifier, placement (`conversation_sidebar`, `contact_panel`, `nav`), explicit scopes, enabled state and optimistic revision. Existing tenant conversation-read permission gates discovery and launch; contact context also requires contact-read. No provider URL is fetched by the backend.

`POST /{uuid}/launch` validates the optional conversation/contact belongs to the org and is visible to the caller. It returns the app URL, exact origin, 120-second EdDSA context JWT and effective scopes. Claims: `iss=clickfix:sidebar-apps`, `aud=app UUID`, `sub=user UUID`, organization UUID, permitted conversation/contact UUIDs, app revision, scope intersection, iat/nbf/exp/jti. Tokens use a purpose-derived Ed25519 key from the existing encryption key; they cannot authenticate as staff access tokens. No new environment variable is required. `GET /v1/app-context/jwks` exposes the public verification key inside the standard response envelope; iframe services verify `data.keys`, issuer, audience, EdDSA and expiration.

### Host postMessage protocol and action allowlist

Frontend host implementation is a separate task. The host must sandbox each iframe (`allow-scripts allow-forms allow-same-origin`, omit top-navigation/popups), set a restrictive referrer policy, compare both `event.origin` with the returned exact origin and `event.source` with that iframe's contentWindow, and never send tokens via a URL/query string, wildcard target origin, logs or storage. Deliver `{type:"clickfix.context", context_token, scopes}` only after `{type:"clickfix.ready"}` from that verified iframe. Refresh context on expiry and whenever selected conversation/contact changes; destroy the old iframe context.

Iframe action request: `{type:"clickfix.action", request_id:<UUID>, action, value?, content?, target_uuid?}`. The host forwards it with the launch token to `POST /v1/tenant/apps/{uuid}/actions` using the host's current session. Respond to the exact requesting frame with `{type:"clickfix.action.result", request_id, success, error?}`. Do not proxy arbitrary URLs, methods or paths. All unknown message types/actions are rejected.

| Scope/action | Additional permission | Input |
|---|---|---|
| `conversation.read` | `tenant.conversations.read` plus current visibility | Context only |
| `contact.read` | `tenant.contacts.read` | Context only |
| `conversation.reply` | `tenant.conversations.reply` | content |
| `conversation.note` | `tenant.conversations.note` | content |
| `conversation.status` | `tenant.conversations.manage` | value |
| `conversation.priority` | `tenant.conversations.manage` | value |
| `conversation.assign` | `tenant.conversations.assign` | target_uuid |
| `conversation.tag` | `tenant.conversations.manage` | target_uuid |

Actions require conversation context and a request UUID. Every action verifies token signature/expiry, app/user/org binding, enabled state, revision, current app scopes, current user permissions, and current conversation visibility before using the existing authenticated inbox API bridge. Every registration edit revokes older tokens. Message IDs are namespaced by app and user; a replayed request cannot create a duplicate reply. Read scopes are never a generic API proxy. No user access token is delivered to the iframe.

## References and deviations

- `takwim-app-go/backend/internal/platform/{jwt/token.go,entitlement/entitlement.go}`: inspected first; reuse the separated-token-realm and fail-closed quota approach. This repo's shared entitlement engine is already adapted from Takwim/comcode.
- `comcode-app-go/backend/internal/platform/entitlement/quota.go`: nil engine denies; no implicit quota bypass.
- `olexfilms-ecosystem/backend/internal/platform/jwt/token.go`: separate audiences/realms inform app-specific audience and issuer validation.
- `otopoly-go/backend/internal/modules/integrations/oauthprovider/usecase/service.go`: bounded allowlists and existing credential conventions.
- `rentopia-app-go/backend/internal/modules/integrations/jobs/usecase/runner.go`: ambiguous external outcomes require verification before retry; adapted to uncertain workflow steps.
- `clickfix-app/app/Services/Tickets/TicketPendingHumanRoutingSuggestionResolver.php`: product separation between AI suggestions and actual human assignments.
- ClickFix main paths in the inventory table supply the actual reusable implementation. No equivalent versioned graph or iframe app registry exists in the surveyed reference paths, so those models and ports are new. No AGPL code was copied.
