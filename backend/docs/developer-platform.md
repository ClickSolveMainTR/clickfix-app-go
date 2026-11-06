# ClickFix developer platform and automation (C2)

This backend slice implements CF-94/95/115/116 on the existing organization,
conversation, notification and transactional outbox infrastructure. It does not
create a second tenant model or bypass the tenant HTTP permission checks.

## REST API keys (CF-115)

The integration API lives at `/v1/public-api/...`. It accepts `Authorization:
Bearer cf_sk_...`. `POST /v1/tenant/api-keys` creates a key and returns the secret
once; list responses contain the prefix, scopes and timestamps only. Storage is
SHA-256 of 32 random bytes. Revoke with `DELETE /v1/tenant/api-keys/{uuid}`.

A key is bound to its organization and creator. Every call resolves the creator's
current membership. The key's scopes are intersected with the creator's current
API permissions: a short-lived internal JWT enters the same authenticated tenant
router as a panel request, with the key's organization header. Revoked keys,
inactive/deleted users and unavailable organizations are rejected. The existing
Redis limiter enforces the configured requests/minute per key (1–6000, default
120); its existing application-wide behavior is to fail open on Redis outages.

The explicit allowlist supports conversation listing/detail/update, message
listing/reply/note, contact/company listing and webhook subscription management.
Scopes are `conversations:read`, `conversations:write`, `contacts:read`,
`companies:read`, `webhooks:read` and `webhooks:write`. Platform, auth, key management
and arbitrary tenant paths cannot be reached through this proxy.

Writes require `Idempotency-Key` (at most 200 bytes). Organization + key + request
key identifies a claim. Method, full target including query and body are hashed.
Repeated identical writes replay the encrypted stored response; mismatched or
unfinished claims return 409. Claims last 24 hours. An unfinished claim is kept
until expiry because automatically re-running an uncertain write can duplicate
side effects. Message `client_id` provides additional domain-level deduplication.

OpenAPI and the Scalar documentation site remain served at `/docs/openapi.yaml`
and `/docs/`. The contract includes the supported public routes and key security
scheme. Additional public resource CRUD must be added explicitly as its owning
module lands; unsupported operations return 404.

## Automation rules (CF-94)

CRUD: `/v1/tenant/automations`, `/{uuid}` (PUT replaces configuration), `/meta`,
`/triggers`, `/executions`, and `POST /{uuid}/test` with `conversation_uuid`.
`position`, `enabled` and `stop_on_match` control event-driven rule execution.
Saving a rule binds subsequent execution to the user who last saved it. Live
membership and automation permission are re-resolved on each execution; mutation
routes re-check their own permissions. Dry-run uses the current caller and writes
neither an execution record nor a conversation change.

Condition trees have exactly one of `all`, `any`, or `field` at each node. The
maximum depth is 8 and the maximum node count is 128. `all: []` is unconditional;
`any: []` never matches. Leaves use `operator` and `value`:

- `eq`, `neq`, `contains`, `not_contains`, `in`, `not_in`;
- numeric `gt`, `gte`, `lt`, `lte`;
- `exists`, `not_exists`.

Missing/null fields never satisfy comparisons, including negated comparisons.
Conversation projection fields, company/contact/inbox/assignee references, tags,
`custom_fields.<path>` JSONB values, waiting/inbound/created/updated elapsed hours,
SLA breach flag, and UTC `clock.hour` / `clock.weekday` can be compared. Tenant
business-calendar/timezone resolution is not implemented in this slice.

Actions: assign user, status, priority, add/remove tag, private note, saved reply
text (`send_canned_reply` with an inline content snapshot), enqueue a configured
webhook, and notify an organization member. Webhook actions also require channel
management permission. Message actions have deterministic `client_id` values.
The existing conversation/channel delivery pipeline sends replies asynchronously.
Team assignment is deliberately rejected until a real organization team catalog
is available; the existing conversation team id has no backing table here.

Execution identity is organization + rule + source event + conversation. A lease
and action index coordinate concurrent/retried executions. Notes/replies and
webhooks use deterministic action ids; arbitrary external delivery and in-app
notifications retain at-least-once semantics around a process crash. Successful
rules stop replaying even if their own actions changed their conditions.

The rule ancestry is copied into the conversation event outbox, not just kept in
memory. A rule may not reappear in its ancestry, and a chain is limited to 8 rules.
Inbox legacy plural event names are expanded into catalog names, including
specific status/priority transitions. Stable derived event ids distinguish a
broad update from its specific transition.

## Scheduled scans and webhooks (CF-95)

A `scheduled` rule requires `schedule_minutes > 0`. The standalone Asynq worker
registers a scan every minute. Rules scan active conversations by id in bounded
pages; hours-based conditions use current database projections. A successful
scan advances its due timestamp; a failed scan keeps it so its deterministic
execution ids and checkpoints can resume. A scan lease/heartbeat coordinates
replicas. Pending executions are resumed even if a previous action resolved the
conversation and it has left the active scan. Scheduled rules run independently;
`stop_on_match` applies to event-driven ordering.

Manage subscriptions at `/v1/tenant/webhooks` (GET/POST), `/{uuid}` (PUT/DELETE),
`/meta` and `/events`. The creation response returns the signing secret once;
it is AES-GCM encrypted at rest. Subscriptions use the domain event catalog.
Transactional outbox listeners insert durable deliveries with endpoint/event
uniqueness. Automation webhook actions enqueue an existing organization endpoint
and never perform outbound HTTP in a handler.

A separate every-minute Asynq task claims deliveries with `SKIP LOCKED` and a
2-minute lease. There are at most eight attempts; backoff is 1m, 5m, 30m, 2h, 6h,
12h, 24h. HTTPS targets only; userinfo, fragments, private/link-local/loopback
addresses and local hostnames are rejected. A dial-time IP check blocks DNS
rebinding, environment proxies are disabled, and redirects are not followed.
The timeout is 10s. Response bodies and transport errors are not stored in logs.

Delivery headers:

- `ClickFix-Webhook-Id`: stable delivery UUID across attempts and replay;
- `ClickFix-Webhook-Event` and `ClickFix-Webhook-Timestamp` (Unix seconds);
- `ClickFix-Webhook-Signature`: `v1=` + hex HMAC-SHA256(secret, `timestamp.body`).

Receivers should compare the signature in constant time, enforce timestamp
freshness and deduplicate the delivery UUID. Log/replay endpoints are
`/v1/tenant/webhook-deliveries` and `POST /{uuid}/replay`. Replay refuses an active
lease or pending delivery; it resets a terminal delivery's attempts and queues it.
Delivery remains at least once: a receiver can accept an attempt whose response
is lost.

The hourly maintenance task deletes expired idempotency and OAuth artifacts.
Run the standalone worker for periodic scans/delivery/cleanup. The HTTP process
also wires task handlers when its in-process worker is enabled, but it does not
start a second periodic scheduler.

## MCP and OAuth (CF-116)

Takwim's OAuth 2.1/PKCE implementation and official MCP Go SDK transport are
ported for one resource: `/mcp/business`, bound to one organization. Other layers
are not mounted. Discovery, registration, authorize, token and revoke endpoints
are outside `/v1` at their standard OAuth paths. Access/refresh tokens and codes
are SHA-256 stored; codes are single-use, refresh tokens rotate, and replay
revokes their token family. Redirect URIs match registered values exactly (HTTPS
or HTTP loopback), PKCE requires S256, and tokens are audience-bound. Current
membership is checked at consent, refresh and every MCP request.

The JSON consent API is `/v1/mcp/consent/{uuid}` and `/{uuid}/decide`; connected apps
are `/v1/mcp/grants` and `DELETE /{uuid}`. Authorize redirects to
`<AUTH_FRONTEND_URL>/oauth/consent?request=UUID`. The frontend provides the other
half (CF-173): the consent screen at `/oauth/consent` and a session-less
passthrough for exactly `/mcp/business`, `/oauth/{authorize,register,token,revoke}`,
`/.well-known/oauth-authorization-server` and
`/.well-known/oauth-protected-resource/mcp/business`
(`frontend/src/lib/server/mcp-proxy-core.ts`). It forwards the client's own
`Authorization` header and never a cookie.

Because registration is anonymous, `/oauth/authorize` does not redirect a
request error to a remote `redirect_uri` (that would be an open redirect): it is
shown as a page, and the browser leaves for the client's site only after a
decision on the consent screen. A loopback `redirect_uri` (a program on the
user's computer) does receive the RFC 6749 error redirect. Redirect hosts must be ASCII (punycode for IDN), client
names are cleaned of control and format characters and capped at 80 characters,
registration is limited per address and globally and fails closed when the
limiter is down, never-approved registrations live 24 hours, and `/mcp/business`
is limited per address before authentication.

`mcp:read` is always included. `mcp:write` is opt-in at consent and additionally
requires organization settings write permission. Tool listing filters by live
resource permissions; every call re-enters the authenticated tenant API. Tools
are `search_conversations`, `get_conversation`, `reply`, `assign_conversation`,
`set_status`, and `list_contacts`. Reply requires a caller-supplied UUID
`client_id` so retries are safe. KB search awaits the KB module and is not
advertised as a nonfunctional tool. Tool calls record masked audit arguments,
outcome, client/grant, duration, and tool name. The connection limit is 60/minute;
the organization limit is 1000/hour.

## Existing permission groups

No new permission slugs or frontend permission UI were introduced:

| Integration operation | Existing tenant permission |
|---|---|
| Read/write automation rules | `tenant.automations.read` / `.write` |
| Read/write webhook subscriptions/deliveries | `tenant.channels.read` / `.manage` |
| Manage API keys | `tenant.settings.write` |
| Connect MCP / allow MCP mutation consent | `tenant.settings.read` / `.write` |

Each bridged operation separately needs the resource permission (conversation
read/reply/note/assign/manage, contacts read, etc.). New audit action/resource
labels must be added by the frontend worker when the management UI is built;
this backend change intentionally touches only generated frontend API types.

## References

- takwim-app-go: `modules/mcp/{server,tools,bridge,audit}.go`,
  `modules/mcpauth/{model,usecase,handler,routes.go}`,
  `migrations/000085_mcp_oauth.*`, `queries/mcp_oauth.sql`: ported protocol,
  transport, bridge, masking and OAuth lifecycle tests; organization staff only.
- rentopia-app-go: `modules/publicapi/usecase/{service,keys,idempotency,webhooks}.go`:
  adapted key entropy/hashing, one-time secrets, idempotency claim/replay,
  timestamped HMAC, bounded retries/leases and SSRF-resistant transport.
- clickfix-app: `app/Models/WebhookDelivery.php`, `routes/api.php`: delivery
  lifecycle/replay vocabulary and organization API product surface.
- olexfilms-ecosystem: `modules/mcp/consent.go`: reviewed live principal/tool
  filtering at consent. Takwim supplied the executable port because its runtime
  matches this boilerplate more closely.
- comcode-app-go: inspected `backend/internal/modules`; the referenced
  `modules/webhooks` is absent from the available clone. The available otopoly-go
  clone likewise has no `modules/mcp/consent.go`; neither was invented or copied
  from a different version.
