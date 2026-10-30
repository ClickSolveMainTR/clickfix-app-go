# Session replay and co-browse (CF-126)

The backend and embeddable widget support consented rrweb recordings and an ephemeral
Centrifugo DOM stream. An agent playback UI consumes the manifest/live-token APIs;
that tenant UI is outside this backend/widget SDK change.

## Enablement

All features start disabled. A channel administrator sets `settings.replay` on an
installed, active widget channel, using the existing channel settings API:

```json
{
  "replay": {
    "replay_enabled": true,
    "cobrowse_enabled": true,
    "control_enabled": false,
    "retention_days": 7,
    "block_selectors": [".payment", "[data-account-details]"]
  }
}
```

Billing must separately grant `replay.enabled` / `cobrowse.enabled` and limits
`replay.bytes`, `replay.retention_days`, `cobrowse.sessions`. No plan gets automatic
grants. `replay.bytes` counts all stored compressed chunks, including expired ones
until physical deletion; `cobrowse.sessions` counts requests since database midnight.
Limits are checked under an organization advisory lock. Retention settings exceeding
the plan are rejected. A missing entitlement engine fails closed. No new env vars.

## Visitor consent and recording

The widget's record button appears only when the organization opted in, in an active
conversation. `ClickFix("startReplay")` opens the same consent prompt. rrweb is a
separate first-party `/widget-recorder.js` bundle; the app bundle never imports it.
It loads only after the visitor clicks Allow. The visible stop/revoke controls stay
outside capture. `ClickFix("stopCapture", true)` revokes consent and expires playback.
Reset/shutdown also revoke capture. The privacy notice is versioned `2026-10-29`;
accept/decline/revoke writes the existing contact consent ledger in the same
transaction as session state. Session and playback actions also write audit/outbox.

All text and input values are masked. Inputs, editable regions, media, frames,
canvas, `[data-private]`, `[data-clickfix-block]`, `.rr-block` and configured selectors
are blocked. URLs, arbitrary attributes, network payloads, console and plugin data
are excluded; the server re-masks uploaded events before storage/publication. Only
numeric DOM dimensions survive attribute filtering. This is a structural, masked
replay: external CSS, media and full visual styling are intentionally omitted.
Invalid selectors fail closed. Capture requires browser `CompressionStream` support.

Chunks are gzip/base64 JSON, at most 512 KiB compressed / 2 MiB expanded / 5000 events.
Replay sequences must be contiguous (0..299); duplicate/out-of-order chunks are
rejected, so the SDK stops/revokes on upload failure rather than silently skipping
history. Recorded chunks go through the private storage driver. A conversation links
the session to its current contact; visitor ownership additionally binds the widget
instance and signed external identity. Retention defaults to seven days, 1..30 days
subject to the plan. Capture expires after fifteen minutes.

## Co-browse and control

`POST /v1/tenant/conversations/{uuid}/cobrowse` creates a request; the existing private
visitor channel delivers it to the installed widget. No stream subscription or upload
is permitted before visitor acceptance. After acceptance both sides use the session's
`live-token` endpoints. Tokens expire at the immutable session timeout; clients cannot
publish to Centrifugo directly. DOM batches pass the API's consent, conversation,
origin, visitor and size checks before publication on `widget:live.{session_uuid}`.
They are never persisted. Revocation/close/timeout blocks subsequent publication.
The SDK batches live DOM once per second. There is no history/recovery for live DOM;
a reconnecting agent needs a new full snapshot (rrweb checkout is every 30 seconds).

Agents use the pointer endpoint with viewport x/y and `pointer` / `highlight` events.
Optional `control_click` additionally requires the org control setting, a request that
explicitly included control in its visitor consent prompt, and
`tenant.cobrowse.control`. The widget only clicks a host-marked
`<button type="button" data-clickfix-control>` outside blocked/masked areas. No
keyboard, input value changes, navigation or arbitrary JavaScript commands exist.

Agent routes require tenant membership, `tenant.conversations.read`, the inbox team
visibility rule, and `tenant.replays.read` or `tenant.cobrowse.use`. Dedicated
permissions are initially granted only to owner/admin template roles. Another org's
or another visitor's session is hidden. API rate limits and a 1000-session/org/day
hard ceiling also apply. Requests, consent, playback, control and stop are audited.

## Playback, erasure and cleanup

The manifest endpoint returns ordered, short-lived signed API chunk URLs (two minute
maximum), never bucket/object keys. Download verifies HMAC audience, expiry and the
current session/conversation state, including anonymization; responses are no-store.
Play rrweb events in a sandboxed player context.

Revocation and the conversation anonymization trigger invalidate signed links
immediately. The existing hourly diagnostic retention worker purges expired replay
objects, including object versions, before deleting metadata. Compliance erasure and
retention synchronously purge replay objects and anonymize the session's visitor id.
A failed object deletion keeps metadata for retry. Existing billing catalog entries
remain on migration rollback because grants/financial records can reference them.

## References and deviations

- `takwim-app-go/backend/internal/modules/consents/usecase/service.go`: consent history
  and transaction boundary; uses ClickFix's existing contact consent ledger instead.
- `comcode-app-go/backend/internal/platform/entitlement/service.go`: fail-closed quota;
  uses ClickFix's current entitlement engine and serialized resource usage.
- ClickFix `widget/usecase/diagnostics.go`, `conversations/access`, `storage`,
  `compliance`: ownership, retention, private objects, team visibility and erasure.
- `clickfix-app/docs/widget/02-backend-source-of-truth-bootstrap-sync.md`: server-authoritative state.
- [rrweb recording/privacy](https://rrweb.com/docs/cloud/recording-settings),
  [PostHog privacy](https://posthog.com/learn/session-replay/pocket-guide/protecting-user-privacy),
  [OpenReplay private mode](https://docs.openreplay.com/en/sdk/private-mode/): source
  masking, explicit capture and privacy behavior; no AGPL implementation was copied.

The named local reference repositories had no rrweb/co-browse implementation to port.
Centrifugo was selected over WebRTC because this scope is installed-site DOM assistance;
ClickFix already has independently managed LiveKit video rooms for screen/video calls.
