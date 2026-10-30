# G3 AI core — CF-62, CF-73, CF-63, CF-71, CF-69

This is the backend core. It returns agent-reviewed suggestions; it does not autonomously send messages, change tickets, or execute model-proposed financial actions. All organization HTTP routes use `RequireOrganization`; the platform run inspector requires `platform.logs.read` and an explicit organization UUID. Knowledge authoring and tenant run inspection reuse `tenant.settings.read/write`. Execution requires `tenant.conversations.reply`, a writable subscription, `ai.enabled`, monthly `ai.tokens`/`ai.credits`, an enabled organization AI policy and prepaid wallet credits. No new permission slugs or product frontend are added in this backend-only run.

## Composition and configuration

`internal/aisystem.New` composes the same KB, RAG and AI services in API and worker. Provider keys are read only from process environment. Keys, URLs, tool definitions, trusted instructions, model selection and prices are never accepted from a tenant HTTP body. BYOK is optional in CF-62 and is not exposed. No plaintext provider-setting persistence is introduced.

- `ANTHROPIC_API_KEY`: default generation provider.
- `OPENAI_API_KEY`, optional `OPENAI_BASE_URL` (default `https://api.openai.com/v1`): OpenAI-compatible generation/embeddings. An operator can configure another compatible service here; tenant input cannot change this URL.
- `AI_EMBEDDING_MODEL`, `AI_EMBEDDING_INPUT_PRICE_MICROS`: embedding model identifier and effective USD micro-units per million input tokens. Both credentials and a positive price are required for paid indexing/search embeddings.
- For each prefix `AI_TRIAGE`, `AI_REPLY`, `AI_COMPLEX`: `_INPUT_PRICE_MICROS` and `_OUTPUT_PRICE_MICROS` are required positive effective USD micro-units per million tokens. No guessed/live price is embedded. Example unit conversion: USD 5 per million means `5000000`, not `5`. This is an illustration of units, not a model-price recommendation.
- Optional per-task `_PROVIDER=openai` plus `_MODEL` routes that task to the compatible provider. Defaults are `claude-haiku-5-5` for triage, `claude-sonnet-5-5` for reply, `claude-opus-5-5` for complex.
- Optional per-task `_FALLBACK_MODEL`, `_FALLBACK_INPUT_PRICE_MICROS`, `_FALLBACK_OUTPUT_PRICE_MICROS` enable an Anthropic → OpenAI-compatible fallback. Both primary/fallback prices must be configured. The effective price is the larger of the configured rates, conservatively covering either route. Actual provider/model are recorded.
- Existing `SEARCH_ENABLED`, Meilisearch host/key and index prefix configure the private lexical index `<prefix>_knowledge_chunks`. No search key is returned to browsers.

Use `PUT /v1/tenant/ai/policy` to explicitly configure `enabled`, `mask_pii`, `allow_unmask`, `daily_calls`, `concurrency`, `daily_budget_micros`. Default policy is disabled, PII masking on, unmasking off. Costs are estimates calculated from configured effective prices and measured provider token usage, in USD micro-units. Effective input rates must cover any provider cache-write multipliers. No currency conversion or provider-invoice reconciliation is claimed.

## Provider contract

`platform/ai` ports Takwim's separate trusted instructions/untrusted user turns, structured-output contract, tool descriptors and bounded chat rounds. The wire adapters preserve tool-use IDs/results and accumulate streamed tool JSON and usage. JSON Schema output and tool arguments are locally validated; remote `$ref` fetching is prohibited. OpenAI-compatible embeddings are reordered by response index and validate batch completeness; RAG rejects non-finite, zero and inconsistent-dimension vectors.

Explicit 429/503/529 rejections receive at most two bounded retries. Fallback applies only to explicit transient rejection before any stream delta. Transport failures and interrupted streams are not replayed; model refusals and output guards cannot trigger safety-bypassing failover. HTTP calls have a 90-second timeout; response sizes and SSE event sizes are bounded. No real provider API is used in tests.

The public run endpoint offers no tools. Internal agents can use `ai.Chat` with an application-owned ToolRunner: it rejects unknown tools, validates schemas, caps rounds, and denies tools marked `RequiresApproval`. Conversation/org authorization must be implemented in that application's runner using server context, never model arguments. This core never grants approval or executes a sensitive action from a prompt.

## Reservations, billing and recovery

One AI credit is 1,000 input+output tokens, rounded up per run. Embedding runs consume the same unit. Monthly plan quotas, daily calls/cost, concurrent generation/embedding calls, wallet debit and run creation are serialized on the existing billing organization lock. The upper-bound reservation uses input bytes plus protocol overhead and the requested maximum output; successful calls release unused credits. Cost estimates use measured tokens. Failed but definitively unaccepted requests release reservations; ambiguous accepted outcomes retain their upper bound. Missing usage is ambiguous, never a free call.

The append-only `billing_credit_ledger` is the sole wallet. No second balance table is created. `ai_wallet_reserve/release` work with the current base schema and with CF-37 (now present on cached `origin/main`, migration `000046_billing_commerce`, PR #23):

- Once the CF-37 allocations table exists, expire lots first and allocate reservations in its expiry/FIFO order, under the same organization lock as `WalletCommand` and refunds.
- Record original lot/expiry lineage in `ai_credit_reservation_lots`. Unused reservation credits restore the original subscription, invoice and expiry. Purchased/refund-restored lots use CF-37's `refund-restore:` eligibility convention, so unused restored purchase credits remain refundable. Allocations and ledger rows remain immutable.
- Credits that expire during a call are restored and immediately expired; they do not become permanent credits. Already-issued financial entries survive migration rollback; rollback does not refund potentially billable runs.
- Settling is idempotent. Final trace, release, and `billing.usage_recorded` outbox events commit together. `ai.run_completed` carries only UUID/status, never raw prompt content. Wallet exhaustion returns `AI_CREDITS_EXHAUSTED`, whose product action is `stop_and_handoff`.

The worker's `app:ai:reconcile` maintenance task runs each minute. A run older than ten minutes becomes `unknown` while keeping reserved cost/credits; this frees concurrency after a process crash. Unknown costs need operator/provider evidence before any financial adjustment. They are not silently refunded or replayed. Provider HTTP timeouts are shorter than the stale-run window.

The AI run ledger is authoritative for AI pending/monthly quota enforcement; CF-20 metering receives actual/unknown usage through the outbox for billing reports. This closes the gap between a finished run and asynchronous usage delivery. Rejections without provider execution do not consume daily provider-call quota. Reservations carry their start date across day/month boundaries.

## Knowledge and RAG

Collections are roots; categories belong to a root collection. Collection/category kind and parent are immutable after creation; names/slugs can be edited. Articles have organization-scoped locale+slug uniqueness, optional collection/category, title, raw rich-text/Markdown body, tags, author and immutable versions. `tr/en`, `public/internal`, optimistic version checks, draft publication pointers and soft deletion are enforced. Editing a draft never changes the published snapshot. Public help-center rendering/UI is outside this backend scope; raw HTML is data and must be sanitized by a renderer.

Publish/unpublish/delete writes the publication pointer and outbox event in one transaction. A publication revision distinguishes republishing the same content version, avoiding event dedupe loss. The shared durable `knowledge.index` event consumer is fanned out via Asynq; failures use the existing retries/dead-letter surface. It embeds masked chunks outside the article lock, then checks the publication version under lock before replacing chunks and the lexical index. Replays converge on the current publication. Enable policy, credits and embedding configuration before publishing; otherwise indexing fails closed and can be replayed after configuration.

The compose image is stock `postgres:18-alpine`. On the running worker DB, `SELECT name FROM pg_available_extensions WHERE name='vector'` returned no rows. Migration 60 therefore stores `real[]` (`float4[]`) vectors and computes exact cosine similarity in Postgres. No Docker image or shared service was modified. This has O(n) vector scanning per organization; an ANN/pgvector migration can replace the repository query when an extension-enabled image is deliberately deployed.

Meilisearch supplies its native full-text ranking, fused with the top cosine candidates using reciprocal-rank fusion (`k=60`), with an optional application-owned reranker. Meilisearch's native ranking is not literal BM25; this is an explicit departure from the issue's “BM25 via Meilisearch” wording. When search is disabled, Postgres full-text ranking supplies the lexical side. The vector model is stored per chunk and filtered at search time to prevent mixing incompatible embedding models. A model change requires publication reindexing.

Chunking preserves Markdown headings/paragraphs/small fenced blocks and HTML text/block boundaries; script/style text is removed. Oversized blocks split on rune boundaries with overlap. Limits: 200 KB article bodies, 256 chunks/article, 1,200 runes/chunk, 100-rune oversized-block overlap, 40 candidates per retrieval channel, 10 final citations. Each citation includes immutable article version, article/chunk UUID, locale, slug, title and masked text. Postgres revalidates every lexical hit against organization, current published version, locale, deletion and visibility. A reranker may only reorder/drop validated candidates. Customer reply suggestions retrieve public knowledge only.

This run implements KB-source indexing. The original CF-63 tracker also names business context, resolved conversations and imported documents; those source adapters are not implemented here. Use `Refs CF-63` until they land. The KB pipeline, embedding abstraction, hybrid retrieval and reranker hook are functional, not placeholder implementations.

## PII and prompt injection

TCKN, Turkish IBAN, common card, Turkish phone and email formats are masked before provider input by default. Matching is deliberately conservative and does not require a valid checksum before masking. Every trace and indexed chunk stays masked even if an authorized organization disables provider-input masking. A per-run random placeholder map lives only in memory. Unmasking restores only known input placeholders, only when both policy and request allow it; newly generated PII is always masked. There is no trace endpoint that reveals the original map. SSE provider output is accumulated, guarded and masked before one final `result` SSE event reaches HTTP callers; token-by-token public delivery is deliberately withheld to prevent split-token PII leakage.

Customer text, retrieved knowledge and tool results are explicitly untrusted data. English/Turkish instruction-override and system/developer-marker heuristics guard inputs and outputs. These heuristics are defense in depth, not a guarantee against arbitrary prompt injection. Independent org permissions, the absence of public executable tools, application ToolRunner authorization and mandatory high-risk approval remain the security boundary.

## API summary

- `/v1/tenant/knowledge/articles`: list/create; `/{uuid}` read/update/delete; `/{uuid}/versions` paged history; `/{uuid}/publish` publish/unpublish with expected version; `/meta` metadata.
- `/v1/tenant/knowledge/collections`: paged list/create; `/{uuid}` rename/delete; empty-collection/FK checks prevent deleting referenced taxonomy.
- `/v1/tenant/knowledge/search?q=&locale=&public_only=`: hybrid retrieval with citations.
- `/v1/tenant/ai/policy`: read/replace organization limits and privacy policy.
- `/v1/tenant/ai/runs`: execute with task/input/locale/max_tokens/retrieve/stream/unmask; paged masked audit list; `/{uuid}` inspect; `/meta` metadata.
- `/v1/platform/organizations/{organization}/ai/runs[/{uuid}]`: platform masked inspector.

All JSON uses the existing success/error envelope; primary lists use `apiquery`. Provider SSE and the final guarded HTTP SSE response are explicit exceptions. New mutation activity entries reuse existing `organizations.updated` labels with precise `area/operation/entity_uuid` metadata and never include article/prompt bodies. No new frontend locales are required. OpenAPI and generated TS types describe the same routes.

## References and deliberate adaptations

| Reference | Paths | Port/adaptation |
|---|---|---|
| takwim-app-go | `backend/internal/platform/ai/{ai,anthropic,chat}.go` | trusted/input split, model IDs, structured output, tool-runner boundary and bounded chat loop; adapted from SDK-specific types to both wire protocols and SSE |
| clickfix-app (Laravel) | `app/Ai/Support/{TicketAiProviderConcurrencyGate,TicketAiDailyCallLimiter,AiSdkConfigSynchronizer,TicketAiTextProviderFailover,TicketAiToolGate,TicketAiGuard}.php` | per-org gates, ordered failover, secret boundary, independent tool authorization; durable Postgres reservation replaces expiring cache locks |
| clickfix-app (Laravel) | `app/Models/KnowledgeArticle.php`, `app/Ai/Support/{KnowledgeArticleSearchForTicket,TicketAiPreloadedKnowledgeFormatter,TicketAiFlowRecorder}.php`, `docs/ticket-ai-agents/SECURITY-PROMPT-INJECTION.md` | organization/draft filtering, preloaded source context and trace lifecycle; versioned PostgreSQL KB and vector/RRF retrieval extend Laravel's LIKE search |
| otopoly-go | `backend/internal/modules/ai/provider/{provider,openai}.go` | inspected provider-neutral tool/message and OpenAI-compatible transport conventions |
| ClickFix CF-20 / CF-37 landing | `internal/modules/entitlements/service.go`; `origin/cf-31-invoices:.../billing/usecase/{wallet,refund_credits}.go`, `.../queries/billing_commerce.sql`, `.../migrations/000046_billing_commerce.up.sql` | existing org lock/ledger, FIFO expiry, allocation and refund-restoration lineage; no billing-owned schema duplication |

Primary protocol/design sources: [Anthropic streaming](https://platform.claude.com/docs/en/build-with-claude/streaming), [Anthropic structured output](https://platform.claude.com/docs/en/build-with-claude/structured-outputs), [OpenAI-compatible chat protocol](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create), [PostgreSQL extension availability](https://www.postgresql.org/docs/17/view-pg-available-extensions.html), [Meilisearch ranking rules](https://specs.meilisearch.dev/specifications/text/0123-ranking-rules-setting-api.html).

## Resume verification — 28 October 2026

The resumed tree passed deterministic sqlc/TS generation, Go build/vet, golangci-lint (0 issues), unit-only `go test ./...` with integration environment unset, provider/chunker race tests, Redocly (3 existing warnings), frontend typecheck/lint/format/production build, i18n (0 errors, 52 warnings), gofmt and diff whitespace checks. Regression coverage preserves final streamed usage for refusal/truncation/schema errors; definitively unaccepted requests are excluded from the daily call quota.

Required full integration verification is **not complete**: PostgreSQL `127.0.0.1:5432` and Redis `:6379` refuse connections after the host crash; Docker's socket is absent. The full suite with `app_test_s4` / Redis DB 4 fails for unavailable services, and migration up/roundtrip cannot run. An isolated native PostgreSQL init attempt failed sandbox shared-memory permissions. No shared Docker service was started or stopped. Earlier-run extension observations above are historical evidence, not a fresh successful connection.

Coordinator must integrate the four newer cached `origin/main` commits (f09bb0b), renumber migration 60, regenerate code and rerun the full DB/Redis suite plus migration up/down2/up after restoring services. No git write commands, commit or PR were performed by this worker. Native Gemini, optional BYOK, non-KB RAG source adapters and product frontend are outside the assigned run; keep CF-63 open.
