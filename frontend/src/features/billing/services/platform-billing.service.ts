import type {
  AdminListParams,
  BillingAdminPayment,
  BillingAdminSubscription,
  BillingAdminSubscriptionStats,
  BillingAdminTransfer,
  BillingBankAccount,
  BillingBankAccountInput,
  BillingFeature,
  BillingHistoryEntry,
  BillingOverride,
  BillingPlan,
  BillingPlanInput,
  BillingSubscription,
  BillingTransition,
  BillingUsageSummary,
  BillingVersion,
  BillingVersionInput,
  BillingWalletEntry,
  BillingWalletEntryInput,
  BillingWalletSummary,
  EntitlementSnapshot,
  Page,
} from "@/features/billing/types";
import { platformRequest } from "@/lib/api/platform-request";

const PLANS = "/v1/platform/plans";
const BILLING = "/v1/platform/billing";
const org = (uuid: string) => `/v1/platform/organizations/${uuid}`;

/**
 * Platform billing management (CF-39): catalog (`platform.plans.*`),
 * subscriptions and payments (`platform.subscriptions.*`), entitlements
 * (`platform.entitlements.*`). Manual operations on subscriptions and
 * entitlements require a fresh step-up; `platformRequest` retries them
 * after the password/passkey prompt.
 */
export const platformBillingService = {
  stats() {
    return platformRequest<BillingAdminSubscriptionStats>(
      "GET",
      `${BILLING}/subscriptions/stats`,
    );
  },
  subscriptions(params: AdminListParams) {
    return platformRequest<Page<BillingAdminSubscription>>(
      "GET",
      `${BILLING}/subscriptions`,
      { query: params },
    );
  },
  payments(params: AdminListParams) {
    return platformRequest<Page<BillingAdminPayment>>(
      "GET",
      `${BILLING}/payments`,
      { query: params },
    );
  },
  transferQueue(params: AdminListParams) {
    return platformRequest<Page<BillingAdminTransfer>>(
      "GET",
      `${BILLING}/bank-transfers/queue`,
      { query: params },
    );
  },
  reviewTransfer(
    organization: string,
    transfer: string,
    body: { approve: boolean; reason: string },
  ) {
    return platformRequest<{ status: string }>(
      "POST",
      `${org(organization)}/billing/bank-transfers/${transfer}/review`,
      { body },
    );
  },
  receiptUrl(organization: string, transfer: string) {
    return `/api${org(organization)}/billing/bank-transfers/${transfer}/receipt`;
  },
  refund(
    organization: string,
    payment: string,
    body: { cancel: boolean; amount_minor: number; idempotency_key: string },
  ) {
    return platformRequest<{ status: string }>(
      "POST",
      `${org(organization)}/billing/payments/${payment}/refund`,
      { body },
    );
  },
  bankAccounts() {
    return platformRequest<Page<BillingBankAccount>>(
      "GET",
      `${BILLING}/bank-accounts`,
      { query: { limit: 100, offset: 0 } },
    );
  },
  saveBankAccount(body: BillingBankAccountInput) {
    return platformRequest<BillingBankAccount>(
      "PUT",
      `${BILLING}/bank-accounts`,
      { body },
    );
  },

  plans(params: { limit: number; offset: number; q?: string }) {
    return platformRequest<Page<BillingPlan>>("GET", PLANS, { query: params });
  },
  plan(uuid: string) {
    return platformRequest<BillingPlan>("GET", `${PLANS}/${uuid}`);
  },
  createPlan(body: BillingPlanInput) {
    return platformRequest<BillingPlan>("POST", PLANS, { body });
  },
  updatePlan(uuid: string, body: BillingPlanInput) {
    return platformRequest<BillingPlan>("PUT", `${PLANS}/${uuid}`, { body });
  },
  archivePlan(uuid: string) {
    return platformRequest<{ status: string }>("DELETE", `${PLANS}/${uuid}`);
  },
  features() {
    return platformRequest<BillingFeature[]>("GET", `${BILLING}/features`);
  },
  saveVersion(plan: string, version: string | null, body: BillingVersionInput) {
    return version
      ? platformRequest<BillingVersion>(
          "PUT",
          `${PLANS}/${plan}/versions/${version}`,
          { body },
        )
      : platformRequest<BillingVersion>("POST", `${PLANS}/${plan}/versions`, {
          body,
        });
  },
  publishVersion(plan: string, version: string) {
    return platformRequest<BillingVersion>(
      "POST",
      `${PLANS}/${plan}/versions/${version}/publish`,
    );
  },

  subscription(organization: string) {
    return platformRequest<BillingSubscription | null>(
      "GET",
      `${org(organization)}/subscription`,
    );
  },
  history(organization: string, subscription: string) {
    return platformRequest<BillingHistoryEntry[]>(
      "GET",
      `${org(organization)}/subscriptions/${subscription}/history`,
    );
  },
  createSubscription(body: {
    organization_uuid: string;
    version_uuid: string;
    price_uuid: string;
    status: "active" | "trialing";
    period_start: string;
    period_end: string;
    trial_ends_at?: string | null;
  }) {
    return platformRequest<BillingSubscription>(
      "POST",
      "/v1/platform/subscriptions",
      { body },
    );
  },
  transition(
    organization: string,
    subscription: string,
    body: BillingTransition,
  ) {
    return platformRequest<BillingSubscription>(
      "POST",
      `${org(organization)}/subscriptions/${subscription}/transitions`,
      { body },
    );
  },
  extendTrial(
    organization: string,
    subscription: string,
    body: { ends_at: string; reason: string },
  ) {
    return platformRequest<{ status: string }>(
      "POST",
      `${org(organization)}/subscriptions/${subscription}/extend-trial`,
      { body },
    );
  },
  entitlements(organization: string) {
    return platformRequest<EntitlementSnapshot>(
      "GET",
      `${org(organization)}/entitlements`,
    );
  },
  setOverride(organization: string, feature: string, body: BillingOverride) {
    return platformRequest<{ status: string }>(
      "PUT",
      `${org(organization)}/entitlements/${encodeURIComponent(feature)}`,
      { body },
    );
  },
  clearOverride(organization: string, feature: string) {
    return platformRequest<{ status: string }>(
      "DELETE",
      `${org(organization)}/entitlements/${encodeURIComponent(feature)}`,
    );
  },
  usage(organization: string) {
    return platformRequest<BillingUsageSummary[]>(
      "GET",
      `${org(organization)}/billing/usage`,
    );
  },
  wallet(organization: string, params: { limit: number; offset: number }) {
    return platformRequest<BillingWalletSummary>(
      "GET",
      `${org(organization)}/billing/wallet`,
      { query: params },
    );
  },
  createWalletEntry(organization: string, body: BillingWalletEntryInput) {
    return platformRequest<BillingWalletEntry>(
      "POST",
      `${org(organization)}/billing/wallet/entries`,
      { body },
    );
  },
};
