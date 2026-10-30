import type { components } from "@/generated/api";
import type {
  BillingBankAccount,
  BillingCheckoutOutput,
  BillingInvoiceSummary,
  BillingPaymentCard,
  BillingPlan,
  BillingSubscription,
  BillingTransferSummary,
  BillingUsageSummary,
  EntitlementSnapshot,
  Page,
} from "@/features/billing/types";
import { platformRequest } from "@/lib/api/platform-request";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

export type BillingProfile = components["schemas"]["BillingProfileInput"];
export type PlanChangePreview =
  components["schemas"]["BillingPlanChangePreview"];
export type PlanChangeRecord = components["schemas"]["BillingPlanChangeRecord"];
export type TenantWallet = components["schemas"]["BillingWalletSummary"];
export type TenantWalletSettings =
  components["schemas"]["BillingWalletSettings"];
export type TenantWalletSettingsInput =
  components["schemas"]["BillingWalletSettingsInput"];
export type TenantWalletEntry = components["schemas"]["BillingWalletEntry"];
export type CreditPackage = components["schemas"]["BillingCreditPackageRecord"];
export type CreditPurchaseInvoice =
  components["schemas"]["BillingInvoiceRecord"];
export type PaymentAvailability =
  components["schemas"]["BillingPaymentAvailability"];
export type CheckoutStatus = components["schemas"]["BillingCheckoutStatus"];

const B = "/v1/tenant/billing";

/**
 * Customer billing portal (CF-33) for the active organization. Reads need
 * `tenant.billing.read`, payments / plan changes / profile `tenant.billing.write`.
 */
export const tenantBillingService = {
  subscription(slug: string) {
    return tenantRequest<BillingSubscription | null>(
      slug,
      "GET",
      `${B}/subscription`,
    );
  },
  entitlements(slug: string) {
    return tenantRequest<EntitlementSnapshot>(
      slug,
      "GET",
      "/v1/tenant/entitlements",
    );
  },
  usage(slug: string) {
    return tenantRequest<BillingUsageSummary[]>(slug, "GET", `${B}/usage`);
  },
  invoices(slug: string, params: { limit: number; offset: number }) {
    return tenantRequest<Page<BillingInvoiceSummary>>(
      slug,
      "GET",
      `${B}/invoices`,
      { query: params },
    );
  },
  downloadInvoice(slug: string, uuid: string, format: "pdf" | "xml") {
    return tenantDownloadFile(slug, `${B}/invoices/${uuid}/${format}`);
  },
  cards(slug: string) {
    return tenantRequest<Page<BillingPaymentCard>>(slug, "GET", `${B}/cards`, {
      query: { limit: 50, offset: 0 },
    });
  },
  removeCard(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${B}/cards/${uuid}`,
    );
  },
  bankAccounts(slug: string) {
    return tenantRequest<Page<BillingBankAccount>>(
      slug,
      "GET",
      `${B}/bank-accounts`,
      { query: { limit: 20, offset: 0 } },
    );
  },
  transfers(slug: string) {
    return tenantRequest<Page<BillingTransferSummary>>(
      slug,
      "GET",
      `${B}/bank-transfers`,
      { query: { limit: 20, offset: 0 } },
    );
  },
  uploadReceipt(slug: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<{ uuid: string }>(
      slug,
      "POST",
      "/v1/tenant/attachments",
      form,
    );
  },
  reportTransfer(
    slug: string,
    body: {
      invoice_uuid: string;
      bank_account_uuid: string;
      attachment_uuid: string;
      amount_minor: number;
      note?: string;
    },
  ) {
    return tenantRequest<{ uuid: string }>(
      slug,
      "POST",
      `${B}/bank-transfers`,
      { body },
    );
  },
  checkout(
    slug: string,
    body: {
      invoice_uuid: string;
      idempotency_key: string;
      store_card: boolean;
      auto_pay_consent: boolean;
    },
  ) {
    // Silent: the pay dialog explains a missing payment provider inline.
    return tenantRequest<BillingCheckoutOutput>(slug, "POST", `${B}/checkout`, {
      body,
      silent: true,
    });
  },
  /** Which payment / purchase paths work right now (CF-154). */
  paymentAvailability(slug: string) {
    return tenantRequest<PaymentAvailability>(
      slug,
      "GET",
      `${B}/payment-availability`,
    );
  },
  /** Verified state of a hosted checkout after the provider return. */
  checkoutStatus(slug: string, reference: string) {
    return tenantRequest<CheckoutStatus>(
      slug,
      "GET",
      `${B}/checkout/${encodeURIComponent(reference)}`,
      { silent: true },
    );
  },
  /** Balances, auto top-up / alert settings and one ledger page (CF-143). */
  wallet(slug: string, params: { limit: number; offset: number }) {
    return tenantRequest<TenantWallet>(slug, "GET", `${B}/wallet`, {
      query: params,
    });
  },
  saveWalletSettings(slug: string, body: TenantWalletSettingsInput) {
    return tenantRequest<{ status: string }>(
      slug,
      "PUT",
      `${B}/wallet/settings`,
      { body, silent: true },
    );
  },
  /** Credit packs this organization's plan may buy. */
  creditPackages(slug: string) {
    return tenantRequest<{ items: CreditPackage[] }>(
      slug,
      "GET",
      `${B}/credit-packages`,
    );
  },
  /** Opens an invoice for the pack; credits land once it is paid. */
  purchaseCredits(slug: string, packageUuid: string, idempotencyKey: string) {
    return tenantRequest<CreditPurchaseInvoice>(
      slug,
      "POST",
      `${B}/credit-purchases`,
      {
        body: { package_uuid: packageUuid, idempotency_key: idempotencyKey },
        silent: true,
      },
    );
  },
  profile(slug: string) {
    return tenantRequest<BillingProfile>(slug, "GET", `${B}/profile`, {
      silent: true,
    });
  },
  saveProfile(slug: string, body: BillingProfile) {
    return tenantRequest<BillingProfile>(slug, "PUT", `${B}/profile`, {
      body,
    });
  },
  previewPlanChange(slug: string, body: { price_uuid: string; seats: number }) {
    return tenantRequest<PlanChangePreview>(
      slug,
      "POST",
      `${B}/plan-change/preview`,
      { body, silent: true },
    );
  },
  applyPlanChange(
    slug: string,
    body: { price_uuid: string; seats: number; idempotency_key: string },
  ) {
    return tenantRequest<PlanChangeRecord>(
      slug,
      "POST",
      `${B}/plan-change/apply`,
      { body },
    );
  },
  attachCoupon(slug: string, code: string) {
    return tenantRequest<{ status: string }>(slug, "POST", `${B}/coupon`, {
      body: { code },
    });
  },
  /** Invoice of the first paid period of a trial (pay before it ends). */
  trialInvoice(slug: string, subscription: string) {
    return tenantRequest<BillingInvoiceSummary>(
      slug,
      "POST",
      `${B}/subscriptions/${subscription}/trial-invoice`,
    );
  },
  cancel(slug: string, subscription: string) {
    return tenantRequest<BillingSubscription>(
      slug,
      "POST",
      `${B}/subscriptions/${subscription}/cancel`,
      { body: { immediate: false } },
    );
  },
  /** Public catalog (`GET /v1/public/pricing`) for plan comparison. */
  catalog() {
    return platformRequest<Page<BillingPlan>>("GET", "/v1/public/pricing", {
      query: { limit: 24 },
    });
  },
};
