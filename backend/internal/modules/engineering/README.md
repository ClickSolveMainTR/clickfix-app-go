# Engineering bridge (CF-88–93)

The API and standalone worker compose this module through `internal/developerplatform`.
Routes live below `/v1/tenant/engineering`; all require a resolved organization.
Native and provider-backed issues share one model. `tenant.tickets.read/write`
control the tracker; settings credentials need `tenant.settings.write`. Conversation
links, previews and suggestions also require conversation read and use the existing
team visibility rule. Revenue impact and delivery audit require conversation read_all.

Issue patches require the last observed `revision`. References use public UUIDs;
an empty reference in a patch clears it. Labels are lightweight strings and cycles
are names. Projects and releases are separate organization-owned resources.

Creating from a conversation snapshots its recent diagnostic metadata and attachment
UUIDs. Attachment content stays behind the original API's access checks. Set
`ai_summary=true` to run an explicit, Turkish engineering summary through the existing
AI policy, PII masking and credit reservation boundary. It requires conversation reply
permission. Dedupe suggestions are a bounded lexical fallback and spend no AI credits.

## Customer follow-up

Moving an issue to done or publishing its release inserts durable follow-up records
in the same transaction. The minute sweep posts approved Turkish product copy through
the inbox's normal message queue and channel adapters, then reopens resolved/closed
conversations. Release copy includes the version and optional HTTPS changelog URL.
The preview endpoint returns content and eligibility without sending anything.

A link's `notify_customer=false` disables that customer follow-up. Contact custom fields
`engineering_notifications=false` or `notifications_opt_out=true` also opt out. Both
queueing and the channel delivery recheck preferences, link existence and current
resolution/release state. Deterministic message client IDs and per-resolution/release
uniqueness make retries idempotent. A queued record means the inbox accepted it;
the underlying message holds actual sent/failed status. Channel policy still applies:
WhatsApp messages outside a service window need an approved provider template.
Configure the existing channel instance settings with
`engineering_followup: {resolution_template: "issue_resolved", release_template:
"issue_released", language: "tr"}`. Templates must already be approved by Meta and
have one body parameter for the fixed customer copy. The release template falls back
to the resolution template if omitted. This module never bypasses provider policy or
pretends delivery succeeded.

## Provider setup

PUT `/integrations/{linear|jira|github}` with `config`, `enabled` and optional
`credentials`. Access/refresh tokens, OAuth client secrets and webhook secrets are
encrypted using the existing AES-256-GCM SecretBox. Reads return configuration and
configured booleans only. Omitted/empty secret fields preserve stored values.
Disable an integration with `enabled=false`.

- Linear: configure `team_id`, optional `project_id`, and `states` mapping each local
  status to its Linear state UUID. The GraphQL issue-create payload is ported from
  `linear-push-issue-plugin/lib/linear.ts`.
- GitHub: configure `repository` as `owner/repo`. OAuth requests repository access.
  Closed/completed maps to done; closed/not_planned maps to canceled; open maps to todo.
- Jira Cloud: configure `cloud_id`, project key in `project_id`, HTTPS `site_url`, optional
  `issue_type` (default Task), `states` (local status → Jira status ID), and
  `transitions` (local status → Jira transition ID). Descriptions/comments use ADF.

For OAuth configure client credentials and an HTTPS frontend `redirect_uri`.
POST `/integrations/{provider}/oauth/start` returns the authorization URL. The frontend
callback submits code/state to the authenticated `/oauth/complete` in the same user
and organization session. State is random, hashed, expires in ten minutes and is
single-use. Refresh rotation is serialized and committed before sync operations.

POST an issue's `/external-links` with the provider and optional external ID. Empty ID
queues provider creation; a supplied ID links and adopts the current remote title,
description and status. The minute sweep pushes native changes and comments. Configure
provider webhooks at `/v1/webhooks/engineering/{integration_uuid}`:

- Linear: `Linear-Signature`, HMAC-SHA256 of raw bytes; signed `webhookTimestamp` must
  be within one minute.
- GitHub: `X-Hub-Signature-256: sha256=…`.
- Jira: **signed admin webhook**, `X-Hub-Signature: sha256=…`. Unsigned dynamic 3LO
  webhooks are rejected; provision a signed admin webhook with a secret.

Use a random webhook secret of at least 16 bytes. Body hashes dedupe replays even if
a delivery header changes. Issue webhooks refetch current provider state to avoid
applying delayed snapshots; external comments are upserted by provider ID. Native
comment markers prevent echoes and recover successful remote comments after a local
crash. Sync metadata exposes generic errors, never provider response bodies or tokens.
Remote issue creation has the provider's at-least-once failure boundary: after a lost
create response, reconciliation may be required for Jira/GitHub to avoid duplicates.

## Handoffs

POST a conversation's `/escalations` with the next tier, target team, reason and
`sla_seconds` (60…2592000). Only L1 → L2 → engineering is accepted. The transaction
clears the individual assignee, assigns the target organization team, reopens the
conversation, records the handoff and resets its resolution clock. Inbox team
visibility applies immediately. The SLA engine preserves the explicit handoff target
on subsequent inbox events and handles pauses, completion, warning and breach events.

## Sentry

Configure the `sentry` integration with an internal integration access token and its
client secret as `webhook_secret`. Attach an error to a visible conversation with
POST `/conversations/{uuid}/errors`, naming an existing engineering issue, Sentry issue
ID, event ID, event URL and summary. GET returns its error references. Only Sentry SaaS
HTTPS URLs are accepted. Multiple affected conversations can attach the same event.
Subscribe the Sentry app to issue webhooks at the same integration webhook endpoint.
`Sentry-Hook-Signature` is checked with the app client secret. A webhook refetches the
current issue from Sentry SaaS before applying status, preserving local support context.
Resolution queues the same opted-in customer follow-ups; body hashes dedupe replay.

Frontend board/list/cards, frontend OAuth callback screens, provider app registration,
and real-provider acceptance testing are coordinator follow-ups.
