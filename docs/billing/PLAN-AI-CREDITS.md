# Plan AI credits (CF-136)

Plan AI allowances fund the existing append-only `billing_credit_ledger` wallet.
Creating an active or trial subscription, activating payment, renewing, or applying
an immediate upgrade writes the grant in the same transaction as the subscription
change, under the organization lock. Pending payments grant nothing. Trials use
their own plan version. Scheduled downgrades use the target version at renewal.

Each subscription period receives the full finite `ai.credits` allowance, including
a trial period. Its positive lot expires at `period_end`, with idempotency key
`plan-credits:<subscription UUID>:<period_start in UTC RFC3339Nano>` (database
microsecond precision). Within the same period, upgrades grant only the difference
between the target allowance and all previous plan grants for that period. The
additional key ends in `:upgrade:<plan version ID>`. Usage never reduces the amount
already granted for idempotency purposes. Repeated activation, payment callbacks,
recovery from grace and backfills cannot replenish spent plan credits.

This is a full-period allowance policy, rather than monetary proration of credits.
It keeps integer credit grants predictable while billing still prorates upgrade
prices as before. Seat-only changes do not multiply credits. When the existing
monthly/yearly change rules start a new subscription period, the new period gets
its full allowance; existing lots keep their original expiry. No wallet grant is
made for a zero, missing or unlimited credit allowance.

Unlimited `ai.credits` bypasses wallet funding while AI enablement, writable
subscription state, daily policy, concurrency, budgets and token caps still apply.
Completion checks whether that run actually debited the wallet before releasing
unused credits. Changing a plan or override while a run is executing cannot mint
credits or prevent refunding a funded reservation.

Finite credits are enforced by the wallet, including purchased and referral lots.
The old additional calendar-month `ai.credits` check is removed: it would incorrectly
deny purchased credits and apply calendar boundaries to subscription-period grants.
The existing wallet reservation/expiry/allocation logic determines spendability;
`expireWallet` and `WalletSweep` expire the unconsumed amount at the period end.

## Token entitlement decision

The entitlement engine is explicitly fail-closed (`internal/platform/entitlement`),
and `billing/model.VersionInput.Validate` requires every catalog feature when a
version is saved or published. `ai.tokens` remains required. A legacy plan missing
that grant now receives `ENTITLEMENT_REQUIRED`, with `ai.tokens` in the response's
error details, instead of an unexplained zero-limit error. Operators must publish
a corrected version with an explicit token limit or `unlimited: true`; missing
configuration never implicitly grants unlimited provider usage. `ai.credits`
unlimited and `ai.tokens` unlimited remain independent choices.

## Backfill and deployment

After deploying, run from `backend/` using the canonical root environment:

```sh
go run ./cmd/backfill-plan-credits
```

The command pages current active/trial subscriptions by ID, takes each organization
lock, re-reads the current subscription and grants only its missing period allowance.
It can be rerun or run concurrently. It requires no schema migration and does not
change, copy or delete the separate manual ledger entry with key
`manual:2026-10-29:ai-credits-10000`. That entry stays an independent credit; it is
not counted as a plan grant. Purchased/referral credits are likewise untouched.
The deployment runner must run this command; starting the API does not backfill.

The tenant wallet already returns ledger entries with plan/purchase reasons and
idempotency keys. Numeric balances by funding source are deferred: unused AI
reservation releases preserve lot lineage and need to be classified through that
lineage, not just by their positive ledger key. No HTTP response changed.

## References

- `takwim-app-go/backend/internal/modules/billing/usecase/service.go`: reviewed
  period billing and payment activation boundaries; its billing model has no
  equivalent plan-credit wallet to port.
- `comcode-app-go/backend/internal/modules/subscriptions/usecase/service.go` and
  `docs/BILLING.md`: subscription lifecycle and immediate upgrade / scheduled
  downgrade semantics; adapted to ClickFix's versioned, organization-scoped model.
- ClickFix `billing/usecase/wallet.go`, `plan_changes.go`, `lifecycle.go` and
  migration `000060_ai_knowledge`: reused organization locks, outbox, immutable
  ledger, expiring lots and AI reservation/release functions.
- `clickfix-app/app/Services/TokenUsage/TokenPeriodRenewalService.php`: adapted
  period-key duplicate detection and paged organization processing to sqlc and
  the existing ledger. Unlike Laravel's configurable rollover/reset policy,
  ClickFix expires period lots and never resets purchased credit balances.
- `clickfix-app/app/Services/TokenUsage/ApplyOrganizationTokenEntitlementChange.php`:
  reviewed immediate change semantics; use an allowance-difference top-up instead
  of resetting the wallet or adding the full allowance again, preserving spend.
- `otopoly-go/backend/internal/modules/billing/usecase/subscriptions.go` and
  `rentopia-app-go/backend/internal/modules/payments/usecase/service.go`: reviewed
  trial boundaries and confirmed-payment-only activation; retained ClickFix's
  existing lifecycle and provider abstractions.
- `olexfilms-ecosystem/backend/internal/modules/ai/usecase/usage.go`: reviewed
  organization-scoped usage reporting; no equivalent period wallet grant to port.
