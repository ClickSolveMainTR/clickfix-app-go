# Conversation AI agents — CF-65/64/66/67/68

This backend extends the CF-62/63/69/71/73 AI core from PR #33. It does not create a second provider, wallet, RAG implementation or message-delivery pipeline. All tests use fake AI providers.

## Configuration and execution

`GET/PUT /v1/tenant/ai/agent` requires `tenant.settings.read/write`. Defaults are off, with triage disabled. Configuration has mode (`off`, `summary`, `draft`, `autonomous`), persona, channel/category allowlists, locale, timezone, weekdays, start/end minutes, confidence threshold and human-handoff keywords. Hours use the configured timezone; overnight schedules require a later extension. Supported organization response/retrieval locales are tr/en. Customer language detection is unrestricted.

The shared durable event-consumer registry subscribes `ai.conversation` to `messages.received`; the existing Asynq event dispatcher executes it in workers. It loads the message and conversation from the event's organization rather than trusting model or event-supplied conversation context. Analysis uses the cheap `triage` route and produces structured language, intent, category, sentiment, urgency, priority, summary, tags and team suggestions. Suggestions are emitted in `ai.triage_completed`; they do not silently assign unknown teams or tags.

A unique `(organization_id,message_id,task)` claim prevents repeated provider charges and duplicate replies on event retries. A crashed claim becomes `unknown` on the organization's next inbound event after ten minutes, and is never automatically replayed. Review the linked masked core run before a deliberate retry. Provider failures, unavailable configuration, guards and exhausted credits produce a human-handoff result with a masked customer-message summary. The core preserves ambiguous provider reservations.

`summary` mode runs analysis only. `draft` stores a proposed answer in the job result for an agent to review and send through the existing conversation message API. `autonomous` additionally requires allowed channel/hours/category, sufficient confidence, a nonempty answer and citation IDs that belong to the core's retrieved public knowledge. Fabricated citations cannot authorize sending. If a category allowlist or an intent procedure is used, enable triage so the current message has a category/intent decision.

Before sending, the conversation service locks the current configuration and conversation, rechecks policy revision, assignment, terminal state, newer inbound messages, human public replies and the channel reply window. The message uses sender `bot` and a deterministic client UUID derived from the inbound message. It uses the existing message outbox, send queue and channel delivery checks. A human assignment or reply stops sending. A reply queued before a human acts remains an auditable queued message; there is no provider recall guarantee.

An AI resolution is counted once per conversation only after the conversation is resolved, a bot reply has actually reached `sent/delivered/read`, an autonomous reply job completed, and there is no human public reply in that conversation. The unique resolution record and idempotent `billing.usage_recorded` event for `ai.resolutions` commit together. Reopening does not count a second resolution.

## Copilot

`POST /v1/tenant/conversations/{uuid}/copilot` requires `tenant.conversations.reply`. Operations: `suggest_reply`, `summarize`, `rewrite`, `translate`, `tone`, `shorten`, `expand`, `next_action`, `ask`. All provider calls use the core's credits, quota reservation, guards and masked trace. It returns suggestions and never sends messages. Reply suggestions retrieve public KB citations. `ask` adds up to ten previous conversations of the same customer; long history is bounded and identified as incomplete. Translation accepts a validated language/region code; KB retrieval still uses the configured tr/en core locales.

`GET /v1/tenant/conversations/{uuid}/ai` is the paged analysis/draft/handoff history. Core run UUIDs link to the existing masked run inspector.

## Tool registry and external HTTP tools

`GET/PUT /v1/tenant/ai/tools` and `GET /v1/tenant/ai/tool-calls` require settings permissions. `/tools/meta` describes supported capabilities. Built-ins are `lookup_contact`, `conversation_history`, `knowledge_search`, `create_note`, `set_status`. Contact and history context is always selected from the server's authorized conversation. Public KB search does not expose internal articles.

`POST /v1/tenant/conversations/{uuid}/ai/tools/invoke` requires `tenant.conversations.manage`. It accepts name, schema-validated arguments, an invocation UUID, `dry_run`, and explicit human `approve`. Note/status changes require a human actor and approval. The model runner cannot supply approval. A dry run validates without a domain mutation or network call, while retaining an invocation audit. Reusing an invocation UUID returns conflict; ambiguous HTTP side effects are never automatically retried. Daily tool limits are serialized under the organization billing lock; dry runs also count against that limit.

Organization tools specify JSON Schema, a fixed HTTPS URL, optional bearer/HMAC credentials, enabled/approval flags and a daily limit. Credentials use the shared AES-GCM secret box and never appear in list responses. `keep_auth` preserves existing credentials. With no encryption key, credential-backed tool configuration fails closed. JSON Schema compilation cannot fetch external references.

Remote requests use POST and a versioned envelope carrying the server-selected organization/conversation, tool, arguments and invocation ID. HMAC matches the Laravel reference: base64 HMAC-SHA256 of `timestamp + '.' + raw_body`. Headers are `X-Webhook-Event`, `X-Webhook-Timestamp`, `X-Webhook-Signature`, and `Idempotency-Key`. Authorization can additionally use bearer auth. Limits are 12 seconds and 4096 response bytes. Only port 443 is allowed; URL credentials/query/fragment, redirects, proxies, private/reserved IPs and DNS rebinding are blocked. Each connection validates all DNS results and pins its public dial address with normal TLS hostname verification. Raw arguments, credentials and remote response bodies are absent from invocation audit rows.

The internal core agent entry point adds application-owned instructions, schemas and a bounded tool loop. Public core run HTTP bodies cannot set them. Three provider rounds and two tool calls per round are allowed, with bounded turn/result sizes and conservative reservation of all possible rounds. Approval-required tools are denied before execution and lead to human handoff. Results are guarded and masked before returning to the provider. All rounds contribute measured usage to the one core run.

## Procedures

`/v1/tenant/ai/procedures` lists/creates drafts, `/{uuid}` updates with a revision check, and `/{uuid}/publish` publishes/unpublishes with an expected revision. Intent and locale are immutable. Draft changes preserve the published snapshot. `/meta` describes list capabilities.

A procedure supplies a title, ordered natural-language steps and an allowed-tool subset. Runtime selects the published version from the current message's triage intent. Its text remains untrusted organization preferences, subordinate to core safety rules. Its tool subset only narrows the live registry; publishing never grants permissions or approval. Steps are limited to 3000 bytes total.

## Bank statements — CF-29

Platform subscription administrators use organization-scoped `/v1/platform/organizations/{organization}/billing/bank-statements` list/meta, `/import` and `/{line}/review`. The import takes a raw CSV/XLSX/MT940 body, `format`, and an explicit bank-account UUID. It accepts up to 1000 incoming TRY credits and 8 MiB of compressed input. Shared XLSX decoding now caps uncompressed data at 32 MiB.

CSV/XLSX columns: `external_id`, `booked_at`, `amount`, `currency`; optional `description`, `reference`, `sender_iban`. Bank amount/date normalization is ported from Takwim cash statements, with strict monetary precision and rejection of unrelated alphabetic amount content. BOM headers, Turkish/English separators and Excel serial dates are supported. The bank's stable external transaction ID is mandatory.

MT940 supports incoming C entries, value/entry dates, :25: account validation, TRY balance currency and multiline :86: descriptions. Debit/reversal entries cannot approve payments. NONREF entries require a stable bank reference after `//`. Bank-specific encodings and unsupported :61: variants fail closed; adapt the source or normalize to CSV. MT940 account data must match the selected merchant bank account.

Matching constrains account and a three-day date window, then weights amount/reference/sender IBAN. Exactly one reference+amount candidate, with no known IBAN conflict, can auto-approve through CF-28. Amount-only/ambiguous matches remain in review. Manual approval still requires the same organization, account, amount and a reported transfer. Global bank-account/external-ID uniqueness prevents importing a bank credit into two organizations; a transfer can settle only one statement line. Reimports must preserve all financial facts. Import, decision, settlement and outbox commit atomically.

The parser reuses ioengine decoders, but this dedicated financial import deliberately does not expose generic row rollback: deleting an imported row cannot undo settled money. Corrections use billing refund/credit flows. `BankStatementSource` is the Open Banking adapter boundary.

## Remaining expanded tracker scope

CF-65's matching to open issues/similar issues is not implemented because this base has no ticket/issue matching service. CF-67's additional issue creation/linking, assignment/tag tools and status-page lookup are not included in this built-in set. Keep these tracker items linked with `Refs` until those integrations land. This worker changes no automation, outgoing-webhook, public-API or MCP modules owned by the other worker.
