import { latestPublished } from "@/features/billing/lib/plans";
import type {
  BillingPlan,
  BillingPrice,
  BillingVersion,
} from "@/features/billing/types";

export type PlanOption = {
  plan: BillingPlan;
  version: BillingVersion;
  /** Prices of the published version, monthly first. */
  prices: (BillingPrice & { uuid: string })[];
};

/**
 * Sellable catalog for platform operators: every plan that is not archived
 * and has a published version, with the prices of its newest published
 * version (monthly before yearly).
 */
export function publishedPlanOptions(
  plans: BillingPlan[] | undefined,
): PlanOption[] {
  const out: PlanOption[] = [];
  for (const plan of plans ?? []) {
    if (plan.archived_at) continue;
    const version = latestPublished(plan);
    if (!version) continue;
    const prices = version.prices
      .filter((p): p is BillingPrice & { uuid: string } => Boolean(p.uuid))
      .sort((a, b) =>
        a.interval === b.interval ? 0 : a.interval === "monthly" ? -1 : 1,
      );
    if (prices.length === 0) continue;
    out.push({ plan, version, prices });
  }
  return out.sort(
    (a, b) =>
      (a.plan.sort_order ?? 0) - (b.plan.sort_order ?? 0) ||
      a.plan.name.localeCompare(b.plan.name),
  );
}

/** Plan, version and price behind a price uuid. */
export function findPrice(options: PlanOption[], priceUuid: string) {
  for (const option of options) {
    const price = option.prices.find((p) => p.uuid === priceUuid);
    if (price) return { ...option, price };
  }
  return null;
}

/** Plan and version for any version uuid (also unpublished / older ones). */
export function findVersion(
  plans: BillingPlan[] | undefined,
  versionUuid: string,
) {
  for (const plan of plans ?? []) {
    const version = plan.versions.find((v) => v.uuid === versionUuid);
    if (version) return { plan, version };
  }
  return null;
}

/** Whole days from now until `iso` (rounded up, never negative). */
export function daysUntil(iso: string | null | undefined, now = Date.now()) {
  if (!iso) return null;
  const ms = new Date(iso).getTime() - now;
  if (Number.isNaN(ms)) return null;
  return Math.max(0, Math.ceil(ms / 86_400_000));
}
