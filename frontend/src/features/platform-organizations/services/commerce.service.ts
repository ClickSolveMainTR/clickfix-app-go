import type { components } from "@/generated/api";
import type {
  BillingSubscription,
  BillingWalletEntry,
  BillingWalletEntryInput,
} from "@/features/billing/types";
import { platformRequest } from "@/lib/api/platform-request";

export type AdminPlanChange = components["schemas"]["BillingAdminPlanChange"];
export type WalletCategory = NonNullable<BillingWalletEntryInput["category"]>;

const org = (uuid: string) => `/v1/platform/organizations/${uuid}`;

/**
 * Platform commerce operations on one organization (CF-143). Both calls are
 * step-up guarded on the server; `platformRequest` shows the password /
 * passkey prompt and retries.
 */
export const organizationCommerceService = {
  changePlan(organization: string, body: AdminPlanChange) {
    return platformRequest<BillingSubscription>(
      "POST",
      `${org(organization)}/subscription/change-plan`,
      { body },
    );
  },

  /**
   * Wallet entry without the global error toast: the wizard posts opening
   * credits right after creating the organization and reports failures
   * itself.
   */
  walletEntry(
    organization: string,
    body: BillingWalletEntryInput,
    options?: { silent?: boolean },
  ) {
    return platformRequest<BillingWalletEntry>(
      "POST",
      `${org(organization)}/billing/wallet/entries`,
      { body, silent: options?.silent },
    );
  },
};
