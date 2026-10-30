# Billing payments: G2 backend handoff

## Provider truth and references

PayTR supports token-based recurring payment using `utoken` + `ctoken`, Non3D permission and `recurring_payment=1`. Takwim's adapter implements only iFrame + refund. Comcode's `platform/payment/paytr.go` does call `SavedCardPayment`, but **Comcode's billing renewal integration explicitly disables PayTR recurring** (`docs/BILLING.md`): its old synchronous abstraction cannot preserve `wait_callback`. Do not describe that application as already supporting PayTR automatic renewal.

ClickFix ports the request and handles `wait_callback`/transport ambiguity as unknown, backed by a committed payment claim. Signed notify or authenticated status inquiry can settle it. A status inquiry that finds no successful PayTR payment remains pending; it does not authorize a second charge. Known failures permit a later attempt. No PAN or CVV enters our API.

Contract sources verified 2026-10-28:

- [PayTR recurring outcomes and request](https://dev.paytr.com/en/direkt-api/kart-saklama-api/registerli-kart-tekrarlayan-odeme)
- [PayTR status inquiry](https://dev.paytr.com/en/durum-sorgu)
- [PayTR card list](https://dev.paytr.com/en/direkt-api/kart-saklama-api/registerli-kart-listesi)
- [PayTR card delete](https://dev.paytr.com/en/direkt-api/kart-saklama-api/registerli-kart-silme)
- [iyzico V3 verification](https://docs.iyzico.com/en/advanced/webhook)

## Runtime configuration

API and worker both call `DevelopmentGateways` (name retained for compatibility); it now registers configured real providers. Configuration is read only from process environment. No real `.env` was read or changed.

| Environment variable | Meaning |
|---|---|
| `PAYMENTS_PAYTR_ENABLED=true` | Register PayTR |
| `PAYMENTS_PAYTR_MODE=sandbox` or `live` | Explicit environment; defaults sandbox |
| `PAYMENTS_PAYTR_MERCHANT_ID`, `_MERCHANT_KEY`, `_MERCHANT_SALT` | Required merchant credentials |
| `PAYMENTS_PAYTR_ACCOUNT_UUID` | Optional stable merchant identity; default derives from provider/mode/merchant id |
| `PAYMENTS_IYZICO_ENABLED=true` | Register iyzico |
| `PAYMENTS_IYZICO_MODE=sandbox` or `live` | Defaults sandbox |
| `PAYMENTS_IYZICO_API_KEY`, `_SECRET_KEY` | Required merchant credentials |
| `PAYMENTS_IYZICO_ACCOUNT_UUID` | Optional stable merchant identity |
| `PAYMENTS_PAYTR_WEBHOOK_CIDRS`, `PAYMENTS_IYZICO_WEBHOOK_CIDRS` | Optional comma-separated source CIDRs |

Never change merchant identity without keeping the old merchant available for unsettled charges/refunds. Resolver refuses a stored account UUID that does not match the configured merchant. Credential rotation for the same merchant retains its UUID. Production never registers `fake`.

CIDRs check the transport `RemoteAddr`, never untrusted forwarded headers. A reverse proxy should enforce provider IP allowlists at the edge. No provider IP ranges are invented or hardcoded.

Coordinator must forward these variables to API and worker deployment environments; compose changes were outside this backend-only run. The configured frontend origin must route same-origin `/api/v1/public/billing/payments/*` requests to Go. Register PayTR notify URL in the merchant panel:

`{frontend}/api/v1/public/billing/payments/paytr/notify`

The iyzico checkout return URL is generated server-side with the order reference. Its token is checked against the stored session and confirmed through authenticated provider API. A browser success URL itself never marks an invoice paid.

## Tenant and platform APIs

- Tenant: invoice summaries, hosted checkout, active receiving bank accounts, transfer receipt reports/list, masked card list/removal.
- Platform subscriptions read/write permissions gate bank account management, transfer review queue, receipt downloads, refund/cancel and reconciliation reports. Existing permissions reused; no new frontend/RBAC catalog edits needed.
- New lists use `limit`/`offset`, fixed ordering and the standard page envelope. Unknown search/sort and forbidden page parameters are rejected.
- Upload a receipt through existing attachments first. Report its UUID with the invoice/account/amount; cross-organization, deleted, infected/unscanned files and reuse are rejected. PDF/image only. Upload ownership may be `billing_invoice`.
- Transfer reference is unique per invoice (`payment_reference`). Transfer report creation does not settle money. Admin approval records the asserted amount atomically with invoice credit/settlement and subscription transition. Rejection requires a reason and is terminal. Partial payments keep the invoice open. Incremental overpayment becomes an immutable TRY credit entry once per payment.
- Platform receipt download uses the attachments service and forces a private, no-store attachment response. Object keys never appear in billing responses.
- Refund uses a durable key, locks the original capture and counts succeeded/pending/unknown reversals against its refundable balance. `cancel: true` invokes iyzico cancellation for a full reversal. Ambiguous refunds stay unknown for operator verification and are never resubmitted automatically.

## Security and settlement

Notify verifies the merchant hash/V3 signature before storing encrypted raw evidence. The retained signature, digest and event identity allow forensic inspection. Receipt insertion and settlement share one organization-locked transaction. Invalid notifications are rejected without state changes; a failed transaction can be safely redelivered. Duplicates are acknowledged after verification and create no second payment or credit.

Stored amount/currency/reference and merchant identity are authoritative. PayTR signs total, not currency or card tokens: total must cover the invoice amount, optional original amount/currency must agree, and card tokens are resolved through authenticated CAPI list before saving. If PayTR returns only an owner and several cards, no arbitrary card is selected for automatic payment. Card ciphertext binds organization, row UUID and field. Stored card list responses contain masked metadata only.

Installments are disabled for monthly and annual subscriptions in both adapters. This avoids treating interest/financing fees as subscription revenue; enabling annual installments needs a separate product/finance decision.

## Renewal, trial and dunning integration

Worker schedules a due/recheck pass every five minutes and provider reconciliation nightly at 01:00 scheduler time. CF-22 deadline sweep remains every minute. Failure of one provider's reconciliation does not prevent other organizations' due processing. Unresolved claims independently block another charge.

An invoice is the financial authority. CF-25 must create the initial trial invoice before automatic conversion, at `period_start = subscription.period_end`, and provide legal buyer/seller snapshots. `BillingOpenTrialInvoice` is the integration lookup. Configured trial gateways use the same ledgered charge/notification/reconciliation path as renewals; missing invoice data never produces an untracked provider capture. On success the invoice and subscription transition together. Definitive failures start past_due -> grace, unknowns await query/notify, grace expires to suspended, and later invoice payment can reactivate a suspended subscription.

The included native-TRY fixed-price renewal builder copies the last paid seller snapshot, reads the current billing profile, applies VAT with exact minor-unit arithmetic and freezes the next invoice. Calendar periods clamp month ends. Delayed periods extend from processing time without charging a batch of missed historical periods. Invoice/claim uniqueness serializes concurrent sweeps. Retry offsets are 1, 3, 7 days from the persisted first overdue/failure timestamp, with a persisted 14-day grace deadline; unknowns start the access deadline but schedule no new capture; G2 instructions override the issue export's older 1/3/5/7 schedule. Subsequent attempts may rotate among this organization's active merchant cards after explicit auto-pay consent.

**CF-30 remaining integration:** FX, seat components and tiered/usage invoice pricing must use CF-25's authoritative invoice builder. This run explicitly refuses to invent those totals. That integration is not complete, so the coordinator handoff uses `Refs CF-30`, not `Fixes CF-30`. Missing profiles/seller snapshots also stop invoice creation instead of fabricating legal data. Legacy/manual subscriptions are not enrolled automatically: `renewal_enabled` starts false and settled checkout/bank payment enables it.

Annual renewal notices fire once at the 30/7-day thresholds; dunning and bank-transfer due notices include `/billing` and flow through transactional outbox to email/inapp owner/admin notifications with channel-level source-event deduplication. Durable transitions and transfer events provide worker audit history; HTTP mutations use activity.Recorder.

## Reconciliation scope and limits

Pending/unknown tracked payments are queried without charging again; recent settled local payments are compared with authenticated provider records during the nightly run. Keyset batches prevent the first page from starving later payments. Amount/currency/status mismatches and stuck unknowns produce persistent reports and outbox notifications. A completed daily run is not repeated; a failed run remains retryable.

This is reconciliation of **locally tracked orders**, not ingestion of all merchant settlement statements. Out-of-band/orphan provider transactions and automated recovery of ambiguous refunds are not implemented in G2. Do not treat this job as a complete finance settlement-statement audit; the handoff uses `Refs CF-40` until that additional reconciliation coverage is agreed or implemented.

Local validation never contacted live payment systems. Process environment had no PayTR/iyzico merchant credentials, so real sandbox card entry, merchant permissions/activation and actual callback delivery remain deployment checks. Tests use Takwim adapter tests, PayTR httptest endpoints, Rentopia's iyzico fake (extended for stored-card charges/status/card deletion), and the shared fake registry. No real credential, customer data or screenshot was added.
