import type {
  BillingFeature,
  BillingGrant,
  BillingPlan,
  BillingVersion,
  BillingVersionInput,
} from "@/features/billing/types";

export function latestPublished(plan: BillingPlan): BillingVersion | undefined {
  return plan.versions
    .filter((v) => v.status === "published")
    .sort((a, b) => b.version - a.version)[0];
}

export function latestDraft(plan: BillingPlan): BillingVersion | undefined {
  return plan.versions
    .filter((v) => v.status === "draft")
    .sort((a, b) => b.version - a.version)[0];
}

/** Default grant for a feature the version does not mention yet. */
export function defaultGrant(feature: BillingFeature): BillingGrant {
  return feature.kind === "boolean"
    ? {
        feature: feature.key,
        enabled: false,
        enforcement: "hard",
        warn_percent: 80,
      }
    : {
        feature: feature.key,
        limit: 0,
        unlimited: false,
        enforcement: "hard",
        warn_percent: 80,
      };
}

/**
 * Editable copy of a version: every catalog feature present exactly once
 * (the API rejects missing features), prices without server ids.
 */
export function versionDraft(
  features: BillingFeature[],
  source?: BillingVersion,
): BillingVersionInput {
  const byKey = new Map(source?.entitlements.map((g) => [g.feature, g]));
  return {
    trial_days: source?.trial_days ?? 14,
    prices: (
      source?.prices ?? [
        {
          interval: "monthly",
          currency: "TRY",
          amount_minor: 0,
          seat_amount_minor: 0,
          tax_inclusive: false,
          vat_basis_points: 2000,
          tiers: [],
        },
      ]
    ).map((p) => ({
      interval: p.interval,
      currency: p.currency,
      amount_minor: p.amount_minor,
      seat_amount_minor: p.seat_amount_minor ?? 0,
      tax_inclusive: p.tax_inclusive ?? false,
      vat_basis_points: p.vat_basis_points,
      tiers: p.tiers ?? [],
    })),
    entitlements: features.map((f) => {
      const g = byKey.get(f.key);
      if (!g) return defaultGrant(f);
      return f.kind === "boolean"
        ? {
            feature: f.key,
            enabled: Boolean(g.enabled),
            enforcement: g.enforcement ?? "hard",
            warn_percent: g.warn_percent ?? 80,
          }
        : {
            feature: f.key,
            limit: g.unlimited ? null : (g.limit ?? 0),
            unlimited: Boolean(g.unlimited),
            enforcement: g.enforcement ?? "hard",
            warn_percent: g.warn_percent ?? 80,
          };
    }),
  };
}

/** Request body: boolean grants carry `enabled`, limit grants `limit` xor `unlimited`. */
export function versionBody(
  features: BillingFeature[],
  draft: BillingVersionInput,
): BillingVersionInput {
  const kinds = new Map(features.map((f) => [f.key, f.kind]));
  return {
    trial_days: draft.trial_days,
    prices: draft.prices,
    entitlements: draft.entitlements.map((g) =>
      kinds.get(g.feature) === "boolean"
        ? {
            feature: g.feature,
            enabled: Boolean(g.enabled),
            enforcement: g.enforcement,
            warn_percent: g.warn_percent,
          }
        : g.unlimited
          ? {
              feature: g.feature,
              unlimited: true,
              enforcement: g.enforcement,
              warn_percent: g.warn_percent,
            }
          : {
              feature: g.feature,
              limit: g.limit ?? 0,
              unlimited: false,
              enforcement: g.enforcement,
              warn_percent: g.warn_percent,
            },
    ),
  };
}

export function featureLabel(feature: BillingFeature, locale: string) {
  return (locale === "tr" ? feature.label_tr : feature.label_en) || feature.key;
}

/** Daily AI limits (CF-229): their usage resets at midnight Istanbul time. */
export const DAILY_FEATURES: ReadonlySet<string> = new Set([
  "ai.daily_calls",
  "ai.daily_budget_micros",
]);
