# Community forum backend (CF-78)

Each organization owns categories and a moderated forum under its public help
center: `/v1/public/help/{organization_slug}/forum`. Public topic paths use stable
human-readable slugs with a UUID suffix. `GET /categories`, `GET /topics` and
`GET /topics/{topic_slug}` expose only published text, public UUIDs, vote counts
and a published accepted answer. No author contact ID, email, moderation reason
or linked private conversation ID enters a public response. Clients render text
safely and can build SSR pages and canonical URLs. `GET /sitemap` provides
paginated topic slugs and modification times for sitemap generation. Lists use
`limit`, `offset`, optional category slug and `q` (title search).

Public reads cache for 30 seconds and throttle at 180 requests/minute per
organization, action and client IP. Public mutations and the private notification
inbox throttle at 20/minute. Responses on errors/mutations/inbox are `no-store`.
Deploy behind the same trusted proxy/BFF as the existing help center; the server
uses its forwarded client IP. Forum routes intentionally use same-origin requests.

Posting a topic/reply, voting and subscribing require a verified widget visitor
Bearer token plus `X-Widget-ID`. The widget must belong to the path's organization;
the author is resolved from that widget's signed identity, never a client-supplied
contact ID or email. Anonymous tokens cannot write. Initialize and identify using
the existing widget API, then call the same-origin `/api/v1/...` BFF.

Every submission enters `pending`. Deterministic link-spam/instruction checks add
flags but never publish. Agents with existing `tenant.feedback.manage` manage
categories, review `GET /v1/tenant/forum/moderation`, and change a post to
`published` or `rejected` using `PATCH /posts/{uuid}`. Explicit
`POST /posts/{uuid}/assist` sends bounded content through the existing AI core:
organization enablement, credits, daily limits, concurrency and budget reservation
remain enforced. It saves a suggestion only; provider failures or exhausted credits
leave the pending post untouched. No public action spends AI credits.

`POST /topics/{uuid}/accept` accepts only a published reply to that same published
topic. Rejecting an accepted reply hides it from public reads immediately (subject
to the 30-second cache). Votes have a composite unique key and are idempotent;
pass `remove:true` to unvote. To vote for a reply pass `reply_uuid`; omission votes
for the topic.

`POST /topics/{topic_slug}/subscribe` subscribes/unsubscribes (`remove:true`) the
signed contact; topic authors subscribe automatically. Publishing a reply inserts
one durable notification per other subscriber in the same transaction. Unique
post/contact keys prevent duplicate notifications after repeated moderation.
`GET /notifications` returns that visitor's notification inbox;
`POST /notifications/{uuid}/read` marks their own notification read. Hidden replies
and topics never appear in the inbox. Delivery here is a durable in-product inbox,
not an email subscription. Domain events are also emitted via the transactional
outbox for future delivery integrations.

`POST /v1/tenant/forum/from-conversation` requires explicit `title`, redacted `body`,
`category` and `conversation_uuid`. It applies normal conversation visibility checks
before creating a pending topic, and does not copy transcripts automatically.
`POST /topics/{uuid}/conversation` requires `tenant.conversations.manage` and a
same-organization active `inbox_uuid`: it atomically creates the assigned conversation,
its first inbound message, the linkage and the standard conversation-created outbox
event. Repeated conversion returns the linked conversation after normal visibility
checks. A conversation maps to at most one topic. Tenant mutations use the existing
support-operations audit recorder; no new permission slugs or environment variables.

All five forum tables are listed in the tenant SQL audit catalog. Composite foreign
keys enforce organization ownership. Migration 000192 follows this worktree's
origin/main highest (000191); the coordinator must renumber if main advances.
The down migration deletes forum data explicitly and leaves converted conversations.

Reference survey: takwim's AI trusted-instruction contract and ClickFix's existing
widget verification, help-center organization resolver, billing transaction/outbox
repository, support-operations visibility checks and credit-controlled AI core.
The surveyed reference repos contain no matching forum module; domain logic is new.
Frontend forum pages and email notifications are outside this backend change.
