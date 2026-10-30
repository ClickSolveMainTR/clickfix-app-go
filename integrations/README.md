# ClickFix workflow integrations (CF-97)

Use the Go API public origin with `/v1/public-api`; browser clients use the same-origin `/api/v1` BFF. Issue an API key in the organization settings. Keys inherit the creator's **current** membership and route permissions; revocation takes effect on subsequent requests. Never embed a server key in browser JavaScript.

| Operation | Endpoint | Key scope | Current member permissions |
|---|---|---|---|
| Subscribe REST hook | `POST /subscriptions` | `webhooks:write` | `tenant.channels.manage` |
| Unsubscribe | `DELETE /subscriptions/{uuid}` | `webhooks:write` | `tenant.channels.manage` |
| Poll trigger events | `GET /triggers?event=conversation.created&cursor=0` | `triggers:read` | conversations read + read_all, contacts read |
| Create conversation | `POST /conversations` | `conversations:write` | conversations read + manage |
| Add internal note | `POST /conversations/{uuid}/messages` | `conversations:write` | conversations read + note, inbox visibility |
| Update contact | `PATCH /contacts/{uuid}` | `contacts:write` | contacts write |

All requests use `Authorization: Bearer cf_sk_…`. All mutations require `Idempotency-Key`, unique per logical action and reused unchanged on retry. The response envelope is `{success,data,meta}`. Rate limits return 429 and Retry-After. Error codes should drive retries; validation/auth failures require correction.

Create a conversation with `{ "contact_uuid": "…", "inbox_uuid": "…", "subject": "Support request" }`. Both contact and active inbox must belong to the key organization. The creator is assigned to the new conversation. Add notes with `{ "content": "Internal context", "private": true, "client_id": "<stable UUID>" }` (the normal message validator controls note/reply permission). Update contacts with the regular contact patch fields, e.g. `{ "name": "New name" }`.

## REST hooks in Zapier, Make and n8n

At activation, POST `/subscriptions` with `{ "name": "Workflow name", "url": "https://hooks.example.com/receiver", "events": ["conversation.created"], "enabled": true }`. Supported workflow triggers are `conversation.created`, `conversation.resolved`, `contact.created`, `csat.submitted`, `conversation.tag_added`. Save `data.uuid` for deactivation and save the creation-only `data.secret` securely. DELETE that subscription on deactivation.

The existing webhook pipeline persists deliveries, signs requests and retries failures. Verify `ClickFix-Webhook-Signature`: `v1=` followed by HMAC-SHA256(secret, `<ClickFix-Webhook-Timestamp>.<raw body>`). Reject timestamps outside a short tolerance and deduplicate `ClickFix-Webhook-Id`. Return 2xx after durably accepting the event. Subscriptions use existing organization channel management permissions; manage keys accordingly. An API key can administer organization webhook subscriptions, including ones created by another key.

For polling fallback, save `data.next_cursor` after processing and use it on the next call. Pages contain at most 100 items in ascending per-organization commit order. Poll again while `has_more` is true. Empty polls retain the cursor. Poll items include stable event IDs, event name, entity UUID and occurrence time; fetch resource details through the normal API and its visibility rules. IDs deduplicate retries shared with REST hooks. History is retained for 30 days. `410 CURSOR_EXPIRED` requires a resource resynchronization followed by cursor `0`; this is deliberate rather than silently skipping history. Sequences can have gaps from deduplication and other trigger types.

Zapier adapters map these endpoints to subscribe/unsubscribe/perform-list and action perform. Make uses a custom webhook plus the same lifecycle requests and HTTP actions. Each platform should unwrap `data`, retain idempotency keys and implement signature validation where its receiver supports it.

## n8n community node description

`n8n/ClickFix.description.json` is an `INodeTypeDescription`-compatible declarative description and `ClickFix.credentials.json` is its generic header credential description. A community-node package can load them as its node `description` and credential properties/authentication; supply the usual TypeScript wrapper, package metadata and icon in that package. This repository does not publish an npm package or claim marketplace approval. REST hook trigger nodes need their lifecycle wrapper to retain the subscription UUID/secret and verify signatures. The descriptor includes polling and subscription operations for HTTP-based workflows.

Description shape follows the [official n8n declarative node guide](https://docs.n8n.io/connect/create-nodes/build-your-node/tutorial-build-a-declarative-style-node.md). Contracts and examples are also in `backend/docs/openapi.yaml`.
