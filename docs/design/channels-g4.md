# G4 backend channels (CF-119–123)

All adapters use `channels.Adapter`, `channel_instances`, the organization-bound
AES-GCM vault, the existing webhook handler and the same queue/registry in API and
worker. No browser communicates directly with a provider. Creating or changing
an instance requires `tenant.channels.manage`; call logs require `tenant.channels.read`.
Credential values never appear in call logs, messages, settings or provider errors.

## SMS (CF-121)

Create type `sms` with credential `provider=netgsm|iletimerkezi|twilio`.

| Provider | Credentials | Connection check | Incoming traffic |
| --- | --- | --- | --- |
| Netgsm | usercode, password, msgheader | authenticated msgheader query | no inbound webhook implemented; reference delivery polling client is retained |
| İleti Merkezi | api_key, api_hash, msgheader; webhook_token for DLR | get-balance | DLR packet_id matches accepted order ID; no inbound SMS webhook in this implementation |
| Twilio | account_sid, auth_token, from_number | authenticated account query | SMS and delivery callbacks, signed with the account auth token |

Twilio callbacks use the exact `PUBLIC_API_URL` origin plus actual request path and
query when verifying signatures; forwarded headers are never trusted. Configure the
instance URL in the provider console. SMS responses are empty TwiML. İleti Merkezi
has no callback signature; configure `?token=<32+ random characters>` in its panel.
Tokens are stored as secret credentials; protect query strings in reverse-proxy logs.

`settings.commercial=true` or message `attributes.commercial=true` forces the İYS
check. `iys_username`, `iys_password`, `iys_code`, `iys_brand_code` are sealed with
other channel credentials. The hook queries the organization's current `MESAJ`
consent and only `ONAY` permits sending. Missing credentials, denial and service
failure prevent provider submission. Netgsm iysfilter=11 / İleti Merkezi iys=1
remain enabled as provider-side checks. Service replies default to noncommercial;
classification belongs to the caller, not a heuristic on message text.

One accepted SMS produces one `billing.usage_recorded` event for feature `sms`,
quantity 1 in the same transaction as the sent message, keyed by message UUID.
This measures logical messages, not carrier segments or a provider invoice.
Delivery callbacks do not charge again. Netgsm's report polling scheduler and
segment-level reconciliation are follow-up work; no delivery receipt is fabricated.

## Slack (CF-119)

Configure `SLACK_CHANNEL_CLIENT_ID`, `SLACK_CHANNEL_CLIENT_SECRET`,
`SLACK_CHANNEL_SIGNING_SECRET`, `SLACK_CHANNEL_REDIRECT_URL` on the server.
Start with `GET /v1/tenant/channels/oauth/slack/start`; send returned state and code
to `POST /v1/tenant/channels/oauth/slack/complete` in the same authenticated org/user
session. State is signed and expires after 15 minutes, following existing Meta
connect endpoints. The provider consumes each authorization code once.

The token is validated against team_id before sealing. Only one live instance can
own a workspace. `settings.channel_ids` is the explicit shared-channel allowlist;
without it no messages are ingested. Operators select channels where the bot is a
member; the adapter does not automatically import unrelated workspace channels. The panel lists those
channels through `GET /v1/tenant/channels/{uuid}/slack/conversations` (Slack
`users.conversations`, scopes `channels:read` and `groups:read`); an ID can also be
typed when the listing is unavailable.
The sender identity is the shared channel (group semantics, as Telegram), named
`#channel` from `conversations.info`, while message attributes retain the author ID
and `author_name` from `users.info` (`users:read`). Names are cached for 24 hours (Redis when
configured, keyed by organization and workspace) and looked up best effort: a failure
leaves the name empty for five minutes and never blocks ingestion.
Conversation messages show `author_name` as the contact sender name for every group
channel (Slack, Teams, Telegram groups). Thread keys are `channel:root_ts`; outbound
replies always use `chat.postMessage(thread_ts=root_ts)`. Bot echoes and edits are
ignored; duplicate provider message IDs are handled by the inbox core. `:ticket:`
reaction events produce a normalized ticket-request intent in the same thread.

Slack POST URL verification is handled before workspace routing and requires a
valid app signature and a fresh timestamp. Normal events verify with the instance
signing secret and must name its workspace. No token appears in event attributes.
Status emoji synchronization, actual ticket creation from a reaction and Slack AI
commands are outside this backend slice; the ordinary conversation AI layer can
reply via the registered adapter. No claim of Slack marketplace approval is made.

## Teams (CF-120)

Create type `teams` with `app_id`, `app_password`, `tenant_id`. This slice supports
single-tenant Entra/Bot registrations per organization; tenant_id is a UUID.
Configure the instance webhook URL as the Azure bot messaging endpoint. Microsoft
Partner/Store publication and actual app installation require the account owner.

Bot Connector JWT validation pins the official key source, accepts RS256 only,
checks issuer, app audience, expiry/not-before, key channel endorsements when
present and serviceUrl binding. Tenant ID must match the registration. Cached
keys refresh hourly; unknown key rotation refreshes at most once a minute. Only
Microsoft service HTTPS domains are permitted. Outbound tokens use client
credentials against that same tenant, with redirects disabled.

1:1 activity streams share a conversation reference; channel replies share their
root activity. The encoded thread reference retains service URL, conversation ID
and root ID, so queued sends return to the same thread. Sender identity is the
conversation; author ID/name remain message attributes. This slice is text-only;
cards, files, edits, reactions and sovereign cloud endpoints are not advertised.

## Custom API (CF-123)

See the OpenAPI `APIChannelMessage` and `APIChannelReply` schemas. Create an `api`
instance with a random 32+ character signing_secret and a public HTTPS webhook_url.
Inbound URL: `POST /v1/webhooks/channels/api/{instance}`.

Both directions use `X-ClickFix-Timestamp` (Unix seconds) and
`X-ClickFix-Signature: sha256=<hex HMAC-SHA256(secret, timestamp + "." + raw body)>`.
Inbound timestamps have a five-minute window; stable message IDs deduplicate
retries. Caller-supplied organization IDs are ignored; scope comes only from the
signed instance. Sender IDs are instance-scoped. Replies carry a stable message
UUID in `id` and `Idempotency-Key`; receivers must accept that ID only once.
The outbound client blocks private addresses at dial time (including DNS rebinding)
and redirects. Text-only; no credential-bearing provider attachment URLs are sent.

## Voice (CF-122, partial)

Create `voice` with `provider=twilio` and account_sid/auth_token/from_number, or
`provider=netgsm` with a random webhook_token for callback-only operation. Twilio
uses native HMAC-SHA1 validation; Netgsm CDR callbacks use the token query parameter
because the provider does not sign events. Netgsm health reports callback-only,
never a verified live registration. Unsupported Netgsm outbound calls fail permanently.

Verified lifecycle events create/update `channel_voice_calls`, scoped by org and
instance with composite FKs. Duplicates upsert one log; final statuses do not move
backwards on late events. `GET /v1/tenant/channels/{uuid}/voice/calls` lists logs.
The Twilio `CallSid` is the conversation thread; each lifecycle state is a stable
message ID. Completed recordings use `RecordingSid`, never trust `RecordingUrl`,
fetch only the authenticated account's Twilio recording path, cap reads at 25 MB,
and pass the stream to the existing message attachment/storage/scanner pipeline.
Recording-only callbacks resolve caller details using the same org + instance +
CallSid. Call rows reference recording messages with composite org FKs; clients
retrieve attachments through existing attachment APIs and API-owned URLs.

**Remaining live telephony work:** inbound TwiML currently acknowledges with an
empty Response; it does not answer as an AI agent or record without an independently
configured provider flow. Outbound Twilio performs a simple escaped Turkish TTS
call and lifecycle callbacks. Netgsm supports the documented flat CDR fields
uniqueid/source/destination/duration/direction; deployment-specific live scenario
envelopes need fixtures from the actual santral. SIP registration, browser WebRTC,
streaming audio, interruption, greeting, transfer, recording consent policy and
production provider validation remain incomplete. Inbound attachment failures use
the existing best-effort pipeline and are logged; a durable recording reconciliation
worker is still required before relying on recordings for compliance.

The current platform AI contract provides text generation and embeddings, not
speech transcription. Call rows therefore expose transcript_status=unavailable.
The next slice should add a metered organization-scoped speech provider to the AI
layer, enqueue transcription through the outbox after storage completes, read only
clean org-owned recording attachments, write transcript/status idempotently, then
publish the transcript into the conversation AI layer. No webhook-provided text is
accepted as a trusted transcript, and no stub claims that AI audio was processed.

## Reference ports and deviations

| Repository | Path | Applied |
| --- | --- | --- |
| takwim-app-go | backend/internal/modules/smsprovider/usecase/{channel,service}.go; backend/internal/platform/sms/netgsm/netgsm.go | Own-account send, connection check, Turkish normalization, provider codes, commercial filter, report client port |
| rentopia-app-go | backend/internal/modules/sms/providers.go; backend/internal/platform/sms/netgsm/netgsm.go | Provider construction, accepted/rejected response semantics |
| takwim-app-go | backend/internal/platform/iys/iys.go | OAuth password grant and organization brand MESAJ/ONAY check |
| takwim-app-go | backend/internal/modules/voicecall/usecase/channel.go; backend/internal/platform/voice/twilio/twilio.go; backend/internal/platform/voice/netgsm/netgsm.go | Account credentials, native signature, Twilio call/TTS protocol, callback-only Netgsm trust boundary |
| clickfix-app-go | backend/internal/platform/channels/{email,whatsapp,instagram,messenger,telegram}; internal/inbox | Existing adapter, vault, group sender/threading, connect/state, attachment, queue and receipt patterns |
| clickfix-app-go (ported from rentopia-app-go) | backend/internal/modules/webhooks/usecase/transport.go | Public target validation and private-address block at dial time |

Reference account types were adapted to `channels.Instance`; no second integration
credential store was introduced. Netgsm reference code is ported with its report
client, but the unrelated confirmation/notification orchestration was not copied.
Slack, Teams, İleti Merkezi and custom API have no matching implementation in the
local reference survey, so their wire protocols are new adapters. Plain/Pylon are
product-model references only; no source was copied. AGPL products were not copied.

Protocol references:
- https://docs.slack.dev/authentication/verifying-requests-from-slack/
- https://docs.slack.dev/reference/methods/oauth.v2.access/
- https://docs.slack.dev/reference/events/url_verification/
- https://learn.microsoft.com/en-us/azure/bot-service/rest-api/bot-framework-rest-connector-authentication
- https://www.twilio.com/docs/usage/security
- https://www.iletimerkezi.com/docs/api/send-sms
- https://www.iletimerkezi.com/docs/api/webhooks
- https://www.netgsm.com.tr/netsantraldokuman/
