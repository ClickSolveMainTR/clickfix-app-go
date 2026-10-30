# G2 billing commerce — coordinator handoff

Backend implementation on `cf-31-invoices`. No git mutations are performed by the worker. Migration `000045_billing_commerce` is provisional: coordinator must renumber against main, regenerate sqlc and repeat migration checks before shipping. API changes are in `backend/docs/openapi.yaml`; generated frontend contracts are the only frontend changes.

## Runtime configuration

- `BILLING_SELLER_JSON`: validated `ProfileInput` JSON (kind, legal_name, tax_id, tax_office, country_code, address with street/district/city/postal_code, emails). Required for a first invoice. Existing paid invoice seller snapshots are a legacy fallback. Never commit real taxpayer data.
- `BILLING_INVOICE_SERIES`: three uppercase ASCII letters; default `CFX`. Number is series + Istanbul issue year + nine-digit sequence. Allocation, legal snapshots and document archival commit before provider submission.
- `BILLING_EINVOICE_PROVIDER`: unset disables provider operations. `fake` is accepted only in development/test. Any unimplemented live integrator fails initialization. Configure a real `EInvoiceProvider` with the owner's integrator account before claiming a GİB transmission.
- `PAYMENTS_IYZICO_STATEMENTS_ENABLED`: defaults enabled when an iyzico account is configured; `false` disables reporting API fetch. Existing payment credential environment variables are unchanged. Reporting uses the same merchant binding and HTTP timeouts as payment adapters.
- `BILLING_REFERRAL_CREDITS`: explicit positive integer, maximum 1,000,000. Referral enrollment is disabled when missing/invalid. No promotion amount or production coupon is invented or seeded.

The API and worker both load invoicing configuration. The five-minute billing job runs renewal, statement matching, ambiguous refund checks, credit notes, wallet expiry/top-up, invoice issuance and referral awards independently, joining errors. The nightly job fetches TCMB, imports all pages of iyzico's previous-day report, performs existing payment reconciliation, matches statements and checks refunds. No new public worker endpoint is added.

## Invoice and tax contract (CF-30/31/32)

Invoice commands lock the organization. New invoices require complete buyer and seller details. Domestic individuals use checksum-valid TCKN; companies use checksum-valid VKN plus tax office. The anonymous final-consumer TCKN is deliberately rejected for a SaaS billing profile. Foreign service-export exemption is explicit and cannot be enabled for TR buyers. KDV is configured on immutable published plan prices; all persisted invoice totals and payment collection are TRY integer minor units.

USD/EUR invoices freeze the latest TCMB ForexSelling rate and its actual published business date, up to seven calendar days old. Weekend fetching never invents a rate date. No floating point arithmetic is used. Invoice VAT is rounded once per discounted line; tax-inclusive prices are split before recording invoice lines.

Renewals and the first auto-pay trial invoice use the same builder: base plan, contracted/actual seats, subscription add-ons, immutable closed usage settlements, discount and VAT. Each closed usage period is claimed by exactly one invoice in the same transaction. A calendar usage month need not fit wholly inside the subscription's anniversary cycle. Graduated usage tiers are evaluated separately for each closed usage period. The next period is anchored to the contractual end, including month-end clamp; a delayed worker does not silently move the anniversary. A zero-due invoice settles and advances the renewal without a gateway call. Dunning retries are days 1/3/5/7; a failed credit/add-on purchase does not suspend the main subscription.

Numbered invoice economics, numbered lines and archived XML/PDF/HTML are immutable in PostgreSQL. XML is ported from Rentopia's go-ubltr mapper, including IADE references and exemption propagation. XSD and Schematron both run before preparation. Export invoices use ISTISNA. HTML uses the reference XSLT; PDF uses this repository's embedded-font ioengine renderer, so Chromium is not required. Archived downloads continue to work when a provider is disabled. Artifacts are stored privately in PostgreSQL and streamed by org-scoped API routes with no-store/CSP headers. Browser code never assembles object keys.

Taxpayer lookup selects TEMELFATURA or EARSIVFATURA. Provider Send is UUID-idempotent; a lost response leaves a submitted document. Retry queries Status first, resubmitting the same UUID only on an explicit provider not-found result. Only authenticated provider confirmation sets issued. The fake's issued state is a test result, never evidence of a real GİB transmission.

## Plan changes and add-ons (CF-34/35)

`POST /v1/tenant/billing/plan-change/{preview|apply}` takes price_uuid and seats (apply also requires idempotency_key). Preview includes currency, amount, effective time and required reductions based on measured usage plus actual seats. Apply rechecks under the organization lock. Currency changes across plan prices are rejected: migrate/quote separately instead of mixing currencies in proration. Preview amounts are in the selected source price currency; the resulting invoice performs the frozen TRY conversion.

Same-interval increases charge the remaining-time fraction. Monthly-to-yearly starts a new annual cycle after payment and credits the unused old cycle. Annual-to-monthly and decreases are scheduled for the current period end. An upgrade retains existing access until its invoice is paid. One open change per subscription prevents competing unpaid upgrades. Price/version and entitlement revision change only on settlement. Usage adapters must keep feature counters current for quota-reduction previews.

Add-on units feed the existing entitlement engine, including revision cache invalidation. Increases create prorated invoices and activate only after payment; decreases/removal are scheduled for renewal. A pending add-on invoice is unique per subscription/add-on. Add-ons must use the subscription's source currency; the catalog amount is quoted per subscription billing interval. Paid/idempotent replays bind to add-on and quantity.

## Coupons and referrals (CF-36)

Takwim's uppercase-code and serialized validation pattern is adapted to global subscription campaigns, not reservation/service catalogs. Codes support percent or fixed TRY settlement amounts, dates, plan/interval restrictions, global redemption caps, per-org caps, one-off, N-month and recurring durations. Specifying duration_months implies recurring. Expired/exhausted attached coupons cease discounting; they do not block renewal. Explicit attachment rejects invalid/exhausted codes. Redemption is immutable and commits with invoice creation.

An org can redeem one different org's referral code before its first paid renewal. Rewards go to both append-only AI ledgers after a paid invoice. Organizations lock in ascending id order. An unpaid/trial registration earns no credit. UI workers can call the coupon/referral endpoints immediately after signup and before checkout.

## Wallet and refunds (CF-37/38)

Ledger entries are append-only. Negative commands lock the organization and allocate consumption to positive credit lots, earliest expiry first. Expiry removes only unused units from the expiring lot and cannot burn a later purchase. Zero available AI balance returns `stop_and_handoff`; AI workers should call WalletCommand with an idempotent negative ai_credits command for over-quota work and obey this action. The billing worker does not modify AI product modules owned by another stream.

Credit packages produce ordinary payable invoices; credits are granted only on settlement. Auto-top-up requires explicit consent plus a consented stored payment method. It reuses any outstanding package invoice, never resends an unknown charge, and deliberately caps automatic purchases at one invoice per org/day. After that cap, manual purchase or human handoff is required. TRY wallet credit (including excess bank transfer) can pay an invoice atomically, including partial payments, through `/v1/tenant/billing/wallet/pay`.

Card refund claims remain durable and bounded by the original payment. Wallet refunds and manually evidenced bank refunds use the same ceiling. Replay binds amount, original payment, destination and cancel intent. Refunding a purchased AI package first reserves the unused purchased credits; already consumed credits cannot be automatically refunded. Ambiguous results retain the reservation; definitive refusal appends a compensating credit entry. Ledger history is never edited.

Every confirmed refund creates an IADE credit note with the original legal snapshots and VAT. Minor-unit VAT rounding is represented explicitly so XML and refund totals remain equal. Credit notes cannot be checked out or collected by renewal jobs. Provider-confirmed electronic cancellation of an existing e-Arşiv document is still an owner/integrator follow-up: no statutory cancellation window is fabricated and Provider.Cancel is not exposed as a production cancellation workflow. Therefore CF-38 remains linked with Refs until that fiscal policy/account integration is supplied.

## Statements and notification contract (CF-40/41)

The platform statement API accepts normalized authenticated merchant evidence, up to 1,000 rows per batch. Daily iyzico reporting is paginated and uses the official SDK's GET signature (path + `{}`; query parameters excluded from the signed path). Split basket rows are aggregated before matching the local payment amount. Refunds keep the refund conversation reference; an original payment reference alone never proves which refund succeeded. PayTR full merchant statements are imported by an administrator/export adapter; existing PayTR status queries continue automatically for known local payments.

Records bind provider, merchant UUID, operation, reference, amount and currency. Orphans are persisted without inventing a tenant/payment, appear in a platform report, and emit a deduplicated alert to platform administrators. Unknown refunds are query-only via RefundChecker where supported or resolve through exact statement evidence. Rows without a reliable unique refund reference remain reviewable. Keyset scanning prevents old orphan rows from starving later records.

TR/EN templates cover welcome, trial ending, payment success/failure, invoice, card expiry, renewal, bank transfer states, quota warning/exhaustion, suspension, cancellation and refund. Reliable outbox listeners enqueue email/inapp through the notification service. Locale follows each recipient. Invoice emails use billing-profile addresses, with stable per-address event IDs anchored to the organization owner for notification replay deduplication and preference handling. Owner/admin inapp copies remain available. Card-expiry template/listener is ready; gateway adapters currently return no expiry metadata, so a provider/card metadata producer must emit that event.

All new commerce HTTP mutations use activity.Recorder with the existing settings.updated action and an exact operation route in the payload, avoiding frontend locale edits outside this worker's scope. Financial state changes also commit domain events to outbox. Existing billing permissions are reused; no permission migration or frontend product code is added.

## References

| Repository | Paths | Adaptation |
|---|---|---|
| rentopia-app-go | backend/internal/modules/invoices/ubl/{ubl.go,validate.go,render.go,invoice.xsl} | UBL builders, IADE, exemption propagation, XSD/Schematron, Istanbul business-day handling, XSLT |
| olexfilms-ecosystem | backend/internal/modules/einvoice/{provider/provider.go,ubl/taxid.go} | Integrator boundary and checksum validation; explicit fake replaces noop for tests |
| go-ubltr | builder/, codelist/, schema/, render/ | Pinned published Go modules; generated UBL types and validators, no local replace |
| otopoly-go | backend/internal/modules/billing/{invoice/mapper.go,usecase/invoices.go,usecase/admin_subscriptions.go} | Sequence/immutable order snapshot model reviewed; Rentopia mapper selected for line-level VAT and return references |
| takwim-app-go | backend/internal/modules/{coupons/usecase/service.go,coupons/model/types.go,entitlements/service.go} | Code normalization, row-lock validation, existing add-on entitlement semantics |
| comcode-app-go | backend/internal/database/queries/subscription_renewal.sql | Durable period claim, no blind retry, stored-card/recipient renewal conventions |
| clickfix-app | app/Models/{TokenLedgerEntry,TokenCommerceSettings,OrganizationTokenBalance}.php | Append-only prepaid credit model and package semantics; explicit FIFO expiry allocations added |
| current repo | backend/internal/platform/ioengine/pdf.go; events/outbox; notifications | Embedded-font PDF, durable side effects, per-recipient language and preferences |
| iyzico official | https://docs.iyzico.com/en/advanced/reporting-service ; https://github.com/iyzico/iyzipay-node/blob/master/lib/IyzipayResource.js | Daily reports, pagination and authenticated GET signature |

Rollback restores the previous schema on an empty/disposable database. A production rollback after creating new invoice/payment kinds requires archiving those financial records first; enum checks intentionally refuse to reinterpret wallet payments or credit notes as older kinds.
