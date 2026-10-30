import type { components } from "@/generated/api";
import type { BillingAdminSubscription, Page } from "@/features/billing/types";
import { platformRequest } from "@/lib/api/platform-request";

type S = components["schemas"];

/** A credit product sold to tenants (AI credits or video minutes). */
export type CreditProduct = S["BillingCreditPackageRecord"];
export type CreditProductInput = S["BillingCreditPackageInput"];
export type CreditProductUnit = CreditProduct["unit"];
export type PlanMigrationInput = S["BillingMigration"];

const PACKAGES = "/v1/platform/billing/credit-packages";

/**
 * Platform commerce catalog (CF-143): credit products
 * (`platform.subscriptions.*`) and scheduled plan migrations
 * (`platform.plans.write`). Writes may answer STEP_UP_REQUIRED;
 * `platformRequest` retries them after the password/passkey prompt.
 */
export const creditProductsService = {
  list() {
    return platformRequest<{ items: CreditProduct[] }>("GET", PACKAGES);
  },
  create(body: CreditProductInput) {
    return platformRequest<CreditProduct>("POST", PACKAGES, { body });
  },
  update(uuid: string, body: CreditProductInput) {
    return platformRequest<CreditProduct>("PUT", `${PACKAGES}/${uuid}`, {
      body,
    });
  },
  /** One page of every organization's subscriptions (max 100 per page). */
  subscriptions(params: { limit: number; offset: number; live?: "true" }) {
    return platformRequest<Page<BillingAdminSubscription>>(
      "GET",
      "/v1/platform/billing/subscriptions",
      { query: params },
    );
  },
  scheduleMigration(body: PlanMigrationInput) {
    return platformRequest<{ scheduled?: number }>(
      "POST",
      "/v1/platform/plan-migrations",
      { body },
    );
  },
};
