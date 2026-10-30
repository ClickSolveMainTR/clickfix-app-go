import type { AppLocale } from "@/config/i18n";
import { defaultGrant, versionDraft } from "@/features/billing/lib/plans";
import type {
  BillingFeature,
  BillingGrant,
  BillingPrice,
  BillingVersion,
  BillingVersionInput,
} from "@/features/billing/types";

/* ------------------------------------------------------------------ */
/* Credit products                                                     */
/* ------------------------------------------------------------------ */

export const CREDIT_UNITS = ["ai_credits", "video_minutes"] as const;

/** Product codes follow the coupon rule: A-Z, 0-9, `_` or `-`, 3–32 chars. */
export const PRODUCT_CODE_RE = /^[A-Z0-9_-]{3,32}$/;

const TR_MAP: Record<string, string> = {
  ç: "C",
  Ç: "C",
  ğ: "G",
  Ğ: "G",
  ı: "I",
  İ: "I",
  ö: "O",
  Ö: "O",
  ş: "S",
  Ş: "S",
  ü: "U",
  Ü: "U",
};

/** "1.000 AI kredisi" → "1000_AI_KREDISI" (suggested code, still editable). */
export function productCodeFromName(name: string) {
  return name
    .replace(/[çÇğĞıİöÖşŞüÜ]/g, (c) => TR_MAP[c] ?? c)
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .toUpperCase()
    .replace(/(\d)[.,](\d)/g, "$1$2")
    .replace(/[^A-Z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .slice(0, 32)
    .replace(/_+$/g, "");
}

/** Price of one credit / minute, with enough decimals to stay meaningful. */
export function formatUnitPrice(
  amountMinor: number,
  quantity: number,
  locale: AppLocale,
  currency = "TRY",
) {
  if (!quantity) return "—";
  const major = amountMinor / 100 / quantity;
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: major < 1 ? 4 : 2,
  }).format(major);
}

/* ------------------------------------------------------------------ */
/* Entitlement groups                                                  */
/* ------------------------------------------------------------------ */

export const FEATURE_GROUPS = [
  "ai",
  "channels",
  "team",
  "video",
  "automation",
  "other",
] as const;
export type FeatureGroup = (typeof FEATURE_GROUPS)[number];

export function featureGroup(key: string): FeatureGroup {
  if (key.startsWith("ai.")) return "ai";
  if (
    key === "channels" ||
    key.startsWith("channels.") ||
    key.startsWith("whatsapp.") ||
    key === "sms" ||
    key === "conversations.monthly"
  )
    return "channels";
  if (key === "seats" || key === "contacts") return "team";
  if (
    key.startsWith("video.") ||
    key.startsWith("replay.") ||
    key.startsWith("cobrowse.")
  )
    return "video";
  if (
    key.startsWith("workflows.") ||
    key.startsWith("engineering.") ||
    key === "api.calls"
  )
    return "automation";
  return "other";
}

/** Units the catalog uses; anything else renders without a unit label. */
export const FEATURE_UNITS = [
  "credit",
  "resolution",
  "token",
  "request",
  "brand",
  "channel",
  "session",
  "contact",
  "conversation",
  "day",
  "seat",
  "message",
  "minute",
  "workflow",
  "run",
] as const;

export function isBytes(feature: BillingFeature) {
  return feature.unit === "byte" || feature.unit === "bytes";
}

export const GIB = 1024 ** 3;

/* ------------------------------------------------------------------ */
/* Drafts                                                              */
/* ------------------------------------------------------------------ */

/** Catalog default for a feature the base version does not mention. */
export function catalogDefaultGrant(feature: BillingFeature): BillingGrant {
  const base = defaultGrant(feature);
  if (feature.kind === "boolean")
    return { ...base, enabled: Boolean(feature.default_enabled) };
  if (feature.default_unlimited)
    return { ...base, unlimited: true, limit: null };
  return { ...base, limit: feature.default_limit ?? 0 };
}

/**
 * Editable copy of a version where features the base lacks (added to the
 * catalog after it was published) start from the catalog default rather than
 * "off / 0". Returns the keys that were filled that way.
 */
export function draftFromVersion(
  features: BillingFeature[],
  source?: BillingVersion,
): { draft: BillingVersionInput; filled: string[] } {
  const draft = versionDraft(features, source);
  if (!source) {
    return {
      draft: {
        ...draft,
        entitlements: features.map(catalogDefaultGrant),
      },
      filled: [],
    };
  }
  const known = new Set(source.entitlements.map((g) => g.feature));
  const filled = features.filter((f) => !known.has(f.key)).map((f) => f.key);
  const byKey = new Map(features.map((f) => [f.key, f]));
  return {
    draft: {
      ...draft,
      entitlements: draft.entitlements.map((g) => {
        const f = byKey.get(g.feature);
        return f && !known.has(g.feature) ? catalogDefaultGrant(f) : g;
      }),
    },
    filled,
  };
}

/* ------------------------------------------------------------------ */
/* Prices                                                              */
/* ------------------------------------------------------------------ */

/** Net (KDV hariç) and gross (KDV dahil) amounts of a price, minor units. */
export function priceBreakdown(price: BillingPrice) {
  const rate = price.vat_basis_points / 10000;
  if (price.tax_inclusive) {
    const gross = price.amount_minor;
    const net = Math.round(gross / (1 + rate));
    return { net, gross, vat: gross - net };
  }
  const net = price.amount_minor;
  const gross = Math.round(net * (1 + rate));
  return { net, gross, vat: gross - net };
}

/** Yearly saving vs. twelve monthly payments, whole percent (or null). */
export function yearlySavingPercent(monthly?: number, yearly?: number) {
  if (!monthly || !yearly) return null;
  const full = monthly * 12;
  if (yearly >= full) return null;
  return Math.round(((full - yearly) / full) * 100);
}

/** Order the pricing page uses for its short feature list. */
const HIGHLIGHT_ORDER = [
  "seats",
  "conversations.monthly",
  "channels",
  "ai.credits",
  "contacts",
  "video.minutes",
  "storage.bytes",
  "ai.enabled",
  "workflows.enabled",
  "video.enabled",
  "channels.whatsapp",
  "brands",
];

/** Top entitlements a price card shows: granted ones, most telling first. */
export function highlightGrants(
  features: BillingFeature[],
  grants: BillingGrant[],
  count = 6,
) {
  const byKey = new Map(grants.map((g) => [g.feature, g]));
  const rank = (key: string) => {
    const i = HIGHLIGHT_ORDER.indexOf(key);
    return i === -1 ? HIGHLIGHT_ORDER.length : i;
  };
  return features
    .map((feature) => ({ feature, grant: byKey.get(feature.key) }))
    .filter(
      (x): x is { feature: BillingFeature; grant: BillingGrant } =>
        Boolean(x.grant) &&
        (x.feature.kind === "boolean"
          ? Boolean(x.grant?.enabled)
          : Boolean(x.grant?.unlimited) || (x.grant?.limit ?? 0) > 0),
    )
    .sort((a, b) => rank(a.feature.key) - rank(b.feature.key))
    .slice(0, count);
}
