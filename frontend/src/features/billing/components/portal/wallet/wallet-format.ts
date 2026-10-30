import type { AppLocale } from "@/config/i18n";
import { formatCount, formatMinor } from "@/features/billing/lib/money";

export type CreditUnit = "ai_credits" | "video_minutes";

/**
 * Ledger reasons are stored in English by the backend (billing usecases,
 * the AI reservation SQL, video metering). Known ones map to a
 * `wallet.reasons.*` label; anything else (e.g. a note a platform admin
 * typed) is shown as written.
 */
const REASON_KEYS: Record<string, string> = {
  "Plan period AI allowance": "plan_ai",
  "Plan period video minutes": "plan_video",
  "Credit package purchase": "package_purchase",
  "Credit expiry": "expiry",
  "AI run reservation": "ai_reservation",
  "Unused AI reservation release": "ai_release",
  "Refund credit reservation": "refund_reservation",
  "Refund refused: credits restored": "refund_restored",
  "Trial extended": "trial_extended",
  "Onboarding AI allowance": "onboarding_ai",
  "Video call minutes": "video_usage",
  "Invoice payment": "invoice_payment",
  "Payment refunded to wallet": "payment_refund",
  "Referral reward": "referral_reward",
};

const CATEGORY_KEYS = new Set([
  "goodwill",
  "correction",
  "promotion",
  "sales",
  "migration",
  "other",
]);

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

export function reasonLabel(t: Translate, reason: string | undefined) {
  const raw = (reason ?? "").trim();
  const key = REASON_KEYS[raw];
  return key ? t(`wallet.reasons.${key}`) : raw || "—";
}

export function categoryLabel(t: Translate, category: string | null) {
  if (!category) return null;
  return CATEGORY_KEYS.has(category)
    ? t(`wallet.categories.${category}`)
    : category;
}

export function unitLabel(t: Translate, unit: string | undefined) {
  if (unit === "ai_credits") return t("wallet.units.ai_credits");
  if (unit === "video_minutes") return t("wallet.units.video_minutes");
  if (unit === "TRY") return t("wallet.units.try");
  return unit ?? "—";
}

/** Signed ledger amount: credits/minutes as counts, TRY in currency. */
export function signedAmount(
  unit: string | undefined,
  delta: number,
  locale: AppLocale,
) {
  const sign = delta > 0 ? "+" : delta < 0 ? "−" : "";
  const abs = Math.abs(delta);
  return `${sign}${unit === "TRY" ? formatMinor(abs, "TRY", locale) : formatCount(abs, locale)}`;
}

/** Price of one credit / minute; keeps up to four decimals (₺0,0450). */
export function formatUnitPrice(
  amountMinor: number,
  units: number,
  locale: AppLocale,
) {
  if (units <= 0) return null;
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    style: "currency",
    currency: "TRY",
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(amountMinor / units / 100);
}

/** Credit pack prices exclude VAT; the invoice adds the plan's VAT rate. */
export function withVat(amountMinor: number, vatBasisPoints: number) {
  return amountMinor + Math.round((amountMinor * vatBasisPoints) / 10_000);
}
