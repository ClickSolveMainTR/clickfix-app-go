# Metering, trials and gateways (CF-21 / CF-23 / CF-24)

## Usage contract

`entitlements.Service.Meter` accepts organization ID plus `{feature, quantity,
event_key, occurred_at}`. Business modules must emit `billing.usage_recorded` to
**their transaction's outbox**, using an event key stable across delivery retries.
The API and queue worker both register the reliable metering listener. Transient
listener errors propagate to the outbox's existing retry/dead-letter policy.

Sum metrics: `ai.resolutions`, `ai.tokens`, `ai.credits`, `whatsapp.templates`,
`sms`, `conversations.monthly`, `api.calls`. Positive integer quantities only.
Gauges: `seats`, `storage.bytes` are absolute nonnegative observations; bytes
avoid floating point rounding when a UI displays GB. The latest observation by
source time wins; daily gauge graphs show peaks. Source modules should emit an
initial gauge observation and subsequent changes. Resource allocation still uses
CF-20 `WithinLimit` with the real resource count in its transaction.

The organization row serializes quota evaluation and ledger writes. A sum event
uses CF-20 `ConsumeAt` (the original `Consume` supplies current time). Monthly
periods use Europe/Istanbul/TRT boundaries, consistent with CF-20. Occurrence time
selects the accounting month. Current effective policy enforces late events;
closed periods reject new events. Same-key same-feature same-quantity replay is
accepted without counting again, even after closure; changed payload conflicts.

Soft limits count overages and emit `entitlements.quota_warning` at crossings.
Hard limits return the existing structured quota error and `Meter` records one
`billing.usage_denied` outbox signal per source event. Business preflight should
use `Consume`/`WithinLimit` before initiating a billable action. A rejected
post-operation source event requires product reconciliation; it is not silently
included in an invoice or retried forever as a transient error.

Tenant reads use existing `tenant.billing.read`; administrative metering writes
and closure use `platform.entitlements.write`. Summary includes period UUID,
feature, quantity, boundaries and closure; `/daily` uses an RFC3339 range bounded
to 366 days. Current limits/remaining quota come from the existing entitlements
endpoint. No client-facing usage mutation is exposed.

The existing minute lifecycle scheduler also closes ended usage periods. Closure
and immutable `billing_usage_settlements` snapshots share a transaction with one
`billing.usage_closed` event containing invoice usage quantities. Invoice creation,
price tiers, FX, VAT and ledger posting remain owned by the invoice engine; it
must consume `(organization_id, period_uuid)` idempotently. This event is the
handoff, not a prematurely created or marked-paid invoice.

## Trials

`TrialHooks` runs in `CreateOrganization`'s transaction, after owner membership.
Optional `trial_price_uuid` chooses a published trial-enabled price. Otherwise
`billing_gateway_settings.trial_price_uuid` selects it; if unset, the first public
trial-enabled plan in catalog order is used. Duration is the immutable version's
`trial_days` (publish a 14-day plan for the requested default). No price, duration
or quota is invented in Go. A fresh installation with **no published trial plan**
can create an organization but stays read-only until a plan is configured.

`billing_trial_enrollments` allows a trial once per organization, including after
terminal expiry. Platform extension requires `platform.subscriptions.write`, a
reason and a later deadline (at most one year from now). It leaves transition
history and activity. Billing subscription responses expose `trial_ends_at` and
`trial_days_remaining`; badges are the frontend worker's responsibility.

The minute sweep sends the nearest crossed 7/3/1-day threshold after downtime.
Each deadline/threshold has a unique reminder row; an extension gets its own
schedule. Outbox delivery enqueues email and in-app notifications for owner/admin
members. A source-event/channel/user unique index prevents duplicate notification
rows on replay. Existing notification preferences still apply. Reliable event
subscriptions propagate enqueue failures for outbox retry.

At expiry, a consented automatic payment method is charged through `TrialGateway`.
Only a settled result enters CF-22 `active`; a definite refusal or no method enters
`expired` (read-only). No card is required for signup. Unknown results retain the
trial record and read-only deadline policy while reconciliation queries the
provider; the same key never submits another charge. A missing live gateway is a
configuration error, never proof of payment. One organization's failure does not
prevent the rest of that trial sweep.

## Gateway extension contract

`platform/payments.Provider` (`Gateway` / `PaymentGateway` aliases) ports Takwim's
hosted checkout, parse/verify/acknowledge contract. Optional `ThreeDSInitializer`,
`CardTokenizer`, `Confirmer`, `CardCharger`, `ChargeChecker`, `CardRemover`,
`Refunder`, `RefundChecker` describe capabilities explicitly. Unsupported features
return `ErrUnsupported`; there is no PAN/CVV API. Monetary values are int64 minor
units, matching CF-18. Callback parsing is untrusted until signature verification;
settlement must also match merchant, order, currency and original amount.

`Registry.Select` takes currency and optional organization/plan provider override;
otherwise it reads current platform currency routing/default. Real provider modules
register adapters and resolve encrypted merchant credentials. `GatewayExecutor`
reserves `billing_payment_attempts` before external calls, fingerprints the request
including merchant account, persists safe results/failure codes, and prevents
reissuing pending/unknown operations. `ReconcileCharge` only queries and verifies
the reference/currency/amount before recording a settled result. Callers must keep
the same key and merchant for a logical operation.

`CardVault` ports Takwim's scope/row/field binding and keyed token index using the
repo's existing root encryption key. AES-GCM authenticates scope/row/field; separate
HMAC-derived encryption/index keys replace the unavailable Takwim FieldCipher.
Tokens are encrypted before persistence and never stored in attempt results.

The in-process fake ports Takwim testpay's HMAC callbacks and Rentopia's isolated
fake pattern. It supports hosted/3DS/card-init, recurring charges, status lookup,
card removal and cumulative partial/full refund limits. Fake registration is
rejected in production. `DevelopmentGateways` wires it only outside production;
its remote state is deliberately process-local. CF-25/26 PayTR/iyzico workers must
register their real adapters and supply `TrialGateway.Account` in production.
This change does not claim live provider certification or a provider settings UI.

No new permission slugs: existing billing/plan entitlement permissions are reused.
New migrations are provisionally 000033 and 000034; coordinator renumbers after
PR #5 and concurrent migrations merge. Backend source plus generated API types and
required activity translations only; no product UI was added.
