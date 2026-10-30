# CF-124 / CF-125 backend contract

Brand management uses `/v1/tenant/brands` and the existing `tenant.channels.read/manage`
permissions. `PUT /brands/{uuid}` replaces configuration; channel create/update accepts
nullable `brand_id`. Omitting it on channel PATCH preserves the current assignment.
All brand references use public UUIDs with organization-scoped database foreign keys.
Unbranded existing channels remain valid. Conversation creation snapshots its channel's
brand; changing a channel later does not change historical conversations. Inbox GET accepts
`brand_id`. Billing's `brands` gauge enforces creation under an organization lock; existing
plan versions receive one brand. Publish a new plan version or organization override for
more brands. New plan versions must explicitly grant this feature like other catalog features.

Upload logos through existing organization attachments, then supply an unattached PNG,
JPEG or WebP attachment UUID. Only API-issued short-lived URLs leave the service.
Brand business hours use the existing widget schedule shape and supply widget defaults;
explicit channel settings override defaults. SLA selection prefers the brand's active SLA
policy override, then the existing company/policy matching rules. The selected SLA policy
retains its own working-time calendar. AI persona is stored for a subsequent agent integration.

`reply` in `/brands/{uuid}/email-templates/{code}` wraps outbound email as plain text.
Its body must contain `{{content}}`; `{{brand_name}}` and `{{subject}}` are supported.
Brand signature is appended after rendering. From/name and optional Reply-To override the
connected email identity for regular replies and email side conversations. The email
provider still requires a verified sender domain/authorized SMTP identity. If Reply-To is
left empty, existing forwarding/IMAP threading behavior is preserved.

Assign a knowledge article with `PUT /brands/{uuid}/articles/{article}` (requires
`tenant.knowledge.write`). Public brand help endpoints expose only published public
article versions. Resolve the brand by organization slug plus UUID, configured subdomain,
or organization slug plus configured help path. A branded widget's help search never
falls back to another brand. Host/subdomain routing and help page rendering belong to
the frontend deployment; these endpoints provide resolution and content.

## Widget diagnostics

`POST /v1/widget/{public_id}/conversations/{conversation}/diagnostics` requires the
existing signed visitor token, origin allowlist and ownership of the widget conversation.
A JSON request with `consent: true` and `context` creates technical context alone.
Multipart accepts `kind`, `file`, `consent=true`, `pii_masked=true`, and a JSON `context`
string. The SDK must obtain explicit consent, mask PII in pixels before capture, and only
collect after consent. Pixel redaction cannot be inferred by this backend.

- Screenshot: PNG/JPEG, 5 MiB, 40 megapixels, each dimension at most 10000.
- Recording: WebM video, 10 MiB, at most 60 seconds from container timestamps. Client
  duration claims are ignored. MP4 is not accepted in this first backend contract.
- Context: 50 console errors / 50 network failures maximum, bounded nested metadata,
  64 KiB after masking. URL paths, queries, fragments and userinfo are removed. Email,
  phone-like numbers, credential expressions and sensitive metadata keys are redacted.
  Do not send request/response bodies, headers, cookies or storage snapshots.
- 10 requests/minute/visitor, 100 live diagnostics/conversation, 1000/org/day; database
  locks serialize quota checks. Captures also use existing attachment file/storage quotas
  and scanner. Expired blobs consume storage quota until physical cleanup completes.
- Retention is 30 days. Agent listing and signed attachment access reject expired data
  immediately. Hourly maintenance removes binaries and context; failures retry through
  the queue. This maintenance is registered for both standalone and in-process workers.

Agents use `GET /v1/tenant/conversations/{uuid}/diagnostics`, which applies the same org
and team visibility as inbox detail. Its download paths use existing attachment access
checks; raw object keys are never returned. A `widget.diagnostic_created` outbox event
and `conversation.diagnostic_added` realtime hint identify the conversation and diagnostic.

Email connection test SMTP timeouts on 465/587 return HTTP 422 with
`error.code=VALIDATION_ERROR`, Turkish `error.message`, and `data.smtp_port_hint` describing
the production provider's outbound 25/465/587 restriction and suggesting Resend/forwarding.
Other SMTP failures keep their original validation detail; successful tests return an
empty `smtp_port_hint`. No new environment variables or provider polling jobs are introduced.
