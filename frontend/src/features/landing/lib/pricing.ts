import { upstreamConfig } from "@/config/api";
import type { components } from "@/generated/api";

type BillingPlan = components["schemas"]["BillingPlan"];

export type PricingInterval = "monthly" | "yearly";
export type PricingCurrency = "TRY" | "USD" | "EUR";
export const PRICING_CURRENCIES: PricingCurrency[] = ["TRY", "USD", "EUR"];

/** Features the pricing cards know how to label (landing.pricing.feature.*). */
export const PRICING_FEATURES = [
  "seats",
  "channels",
  "conversations.monthly",
  "ai.resolutions",
  "ai.credits",
  "contacts",
  "storage.bytes",
  "ai.enabled",
  "engineering.enabled",
  "status_page.enabled",
] as const;

export type PricingFeature = (typeof PRICING_FEATURES)[number];

export type PricingLine =
  | { feature: PricingFeature; kind: "limit"; value: number | "unlimited" }
  | { feature: PricingFeature; kind: "flag" };

/** One price of a plan: minor units, and the price id for sign-up. */
export type PricingAmount = {
  amount: number;
  /** Absent for the sample catalog (nothing to sign up for). */
  uuid?: string;
  vatBasisPoints: number;
  taxInclusive: boolean;
};

/** One plan card: amounts per currency and interval. */
export type PricingPlan = {
  code: string;
  name: string;
  description?: string;
  trialDays: number;
  prices: Partial<
    Record<PricingCurrency, Partial<Record<PricingInterval, PricingAmount>>>
  >;
  lines: PricingLine[];
};

export type PricingCatalog = {
  /** `api`: published plans; `fallback`: config sample, shown as such. */
  source: "api" | "fallback";
  /** Currencies at least one plan is sold in, TRY first. */
  currencies: PricingCurrency[];
  plans: PricingPlan[];
};

const tryPrice = (amount: number): PricingAmount => ({
  amount,
  vatBasisPoints: 2000,
  taxInclusive: false,
});

/**
 * Sample catalog shown while no plan is published (or the API is down).
 * The section labels it as indicative; real prices come from
 * `GET /v1/public/pricing` once the platform team publishes them.
 */
export const FALLBACK_PRICING: PricingCatalog = {
  source: "fallback",
  currencies: ["TRY"],
  plans: [
    {
      code: "baslangic",
      name: "Başlangıç",
      trialDays: 14,
      prices: {
        TRY: { monthly: tryPrice(99000), yearly: tryPrice(990000) },
      },
      lines: [
        { feature: "seats", kind: "limit", value: 3 },
        { feature: "channels", kind: "limit", value: 2 },
        { feature: "conversations.monthly", kind: "limit", value: 1000 },
        { feature: "ai.resolutions", kind: "limit", value: 100 },
      ],
    },
    {
      code: "buyume",
      name: "Büyüme",
      trialDays: 14,
      prices: {
        TRY: { monthly: tryPrice(249000), yearly: tryPrice(2490000) },
      },
      lines: [
        { feature: "seats", kind: "limit", value: 10 },
        { feature: "channels", kind: "limit", value: "unlimited" },
        { feature: "conversations.monthly", kind: "limit", value: 10000 },
        { feature: "ai.resolutions", kind: "limit", value: 1000 },
        { feature: "status_page.enabled", kind: "flag" },
      ],
    },
    {
      code: "kurumsal",
      name: "Kurumsal",
      trialDays: 0,
      prices: {},
      lines: [
        { feature: "seats", kind: "limit", value: "unlimited" },
        { feature: "channels", kind: "limit", value: "unlimited" },
        { feature: "conversations.monthly", kind: "limit", value: "unlimited" },
        { feature: "engineering.enabled", kind: "flag" },
        { feature: "status_page.enabled", kind: "flag" },
      ],
    },
  ],
};

function isPricingFeature(value: string): value is PricingFeature {
  return (PRICING_FEATURES as readonly string[]).includes(value);
}

/** Maps a published plan to a card; `null` when it has no price at all. */
export function toPricingPlan(plan: BillingPlan): PricingPlan | null {
  const version = plan.versions
    .filter((v) => v.status === "published")
    .sort((a, b) => b.version - a.version)[0];
  if (!version) return null;
  const prices: PricingPlan["prices"] = {};
  for (const price of version.prices) {
    const byInterval = (prices[price.currency] ??= {});
    byInterval[price.interval] = {
      amount: price.amount_minor,
      uuid: price.uuid,
      vatBasisPoints: price.vat_basis_points,
      taxInclusive: Boolean(price.tax_inclusive),
    };
  }
  if (Object.keys(prices).length === 0) return null;
  const lines: PricingLine[] = [];
  for (const grant of version.entitlements) {
    if (!isPricingFeature(grant.feature)) continue;
    if (grant.unlimited) {
      lines.push({ feature: grant.feature, kind: "limit", value: "unlimited" });
    } else if (typeof grant.limit === "number") {
      lines.push({ feature: grant.feature, kind: "limit", value: grant.limit });
    } else if (grant.enabled) {
      lines.push({ feature: grant.feature, kind: "flag" });
    }
  }
  lines.sort(
    (a, b) =>
      PRICING_FEATURES.indexOf(a.feature) - PRICING_FEATURES.indexOf(b.feature),
  );
  return {
    code: plan.code ?? plan.uuid,
    name: plan.name,
    description: plan.description || undefined,
    trialDays: version.trial_days ?? 0,
    prices,
    lines,
  };
}

export function catalogFromPlans(items: BillingPlan[]): PricingCatalog | null {
  const plans = items
    .map(toPricingPlan)
    .filter((p): p is PricingPlan => p !== null);
  if (plans.length === 0) return null;
  const currencies = PRICING_CURRENCIES.filter((c) =>
    plans.some((p) => p.prices[c]),
  );
  return { source: "api", currencies, plans };
}

/**
 * Public plans from `GET /v1/public/pricing` (cached 5 minutes). Falls back
 * to the sample catalog when nothing is published or the API is down.
 */
export async function fetchPricing(): Promise<PricingCatalog> {
  try {
    const response = await fetch(
      `${upstreamConfig.baseUrl}/public/pricing?limit=12`,
      {
        headers: { Accept: "application/json" },
        next: { revalidate: 300 },
        signal: AbortSignal.timeout(3000),
      },
    );
    if (!response.ok) return FALLBACK_PRICING;
    const json = (await response.json()) as {
      data?: { items?: BillingPlan[] };
    };
    return catalogFromPlans(json.data?.items ?? []) ?? FALLBACK_PRICING;
  } catch {
    return FALLBACK_PRICING;
  }
}

/** Where the chosen plan waits between the pricing page and the first organization. */
export const SIGNUP_PLAN_STORAGE_KEY = "clickfix.signup.plan";

export type StoredSignupPlan = { price: string; payNow: boolean };

export function readSignupPlan(): StoredSignupPlan | null {
  try {
    const raw = window.localStorage.getItem(SIGNUP_PLAN_STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<StoredSignupPlan>;
    return typeof parsed.price === "string" &&
      /^[0-9a-f-]{36}$/i.test(parsed.price)
      ? { price: parsed.price, payNow: Boolean(parsed.payNow) }
      : null;
  } catch {
    return null;
  }
}

export function writeSignupPlan(value: StoredSignupPlan | null) {
  try {
    if (value)
      window.localStorage.setItem(
        SIGNUP_PLAN_STORAGE_KEY,
        JSON.stringify(value),
      );
    else window.localStorage.removeItem(SIGNUP_PLAN_STORAGE_KEY);
  } catch {
    // Private mode / blocked storage: the default trial plan applies.
  }
}

/**
 * schema.org offers of the published plans (monthly price, TRY first). The
 * sample catalog has none: indicative prices are never sent to search
 * engines as real offers.
 */
export function pricingOffers(catalog: PricingCatalog) {
  if (catalog.source !== "api") return [];
  const currency = catalog.currencies[0] ?? "TRY";
  const out: {
    planName: string;
    price: number;
    priceCurrency: string;
    description?: string;
    priceSpecification: object;
  }[] = [];
  for (const plan of catalog.plans) {
    const price = plan.prices[currency]?.monthly;
    if (!price) continue;
    const amount = price.amount / 100;
    out.push({
      planName: plan.name,
      price: amount,
      priceCurrency: currency,
      description: plan.description,
      priceSpecification: {
        "@type": "UnitPriceSpecification",
        price: amount,
        priceCurrency: currency,
        unitCode: "MON",
        valueAddedTaxIncluded: price.taxInclusive,
      },
    });
  }
  return out;
}
