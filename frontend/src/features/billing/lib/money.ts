import type { AppLocale } from "@/config/i18n";

export type BillingCurrency = "TRY" | "USD" | "EUR";

export const BILLING_CURRENCIES: BillingCurrency[] = ["TRY", "USD", "EUR"];

function numberLocale(locale: AppLocale) {
  return locale === "tr" ? "tr-TR" : "en-US";
}

/**
 * Formats an amount stored in minor units (kuruş / cents): 129900 TRY →
 * "₺1.299,00" (tr-TR). Billing amounts are BIGINT minor units end to end
 * (docs/billing/ARCHITECTURE.md); never float them through the UI.
 */
export function formatMinor(
  minor: number,
  currency: string = "TRY",
  locale: AppLocale = "tr",
  options: { compact?: boolean } = {},
) {
  const whole = Number.isInteger(minor / 100);
  return new Intl.NumberFormat(numberLocale(locale), {
    style: "currency",
    currency,
    minimumFractionDigits: options.compact && whole ? 0 : 2,
    maximumFractionDigits: 2,
  }).format(minor / 100);
}

/** Integer formatting for limits and usage (1.000, 10.000). */
export function formatCount(value: number, locale: AppLocale = "tr") {
  return new Intl.NumberFormat(numberLocale(locale)).format(value);
}

/**
 * Parses a user-typed major amount ("1.299,50", "1299.5", "1 299") into
 * minor units. Returns `null` for anything that is not a non-negative amount
 * with at most two decimals.
 */
export function parseMajorToMinor(raw: string): number | null {
  const text = raw.trim().replace(/\s/g, "").replace(/[₺$€]/g, "");
  if (!text) return null;
  let normalized = text;
  const lastComma = text.lastIndexOf(",");
  const lastDot = text.lastIndexOf(".");
  if (lastComma > lastDot) {
    // tr-TR: dots group thousands, comma is the decimal separator.
    normalized = text.replace(/\./g, "").replace(",", ".");
  } else if (lastDot > lastComma && lastComma !== -1) {
    normalized = text.replace(/,/g, "");
  } else if (lastDot !== -1 && /^\d{1,3}(\.\d{3})+$/.test(text)) {
    // "1.299" typed the Turkish way is one thousand two hundred ninety-nine.
    normalized = text.replace(/\./g, "");
  }
  if (!/^\d+(\.\d{1,2})?$/.test(normalized)) return null;
  const [whole, fraction = ""] = normalized.split(".");
  const minor = Number(whole) * 100 + Number(fraction.padEnd(2, "0"));
  return Number.isSafeInteger(minor) ? minor : null;
}

/** Minor units → editable major string ("1299,00" in tr, "1299.00" in en). */
export function minorToInput(minor: number, locale: AppLocale = "tr") {
  const value = (minor / 100).toFixed(2);
  return locale === "tr" ? value.replace(".", ",") : value;
}

/** Yearly price expressed per month, for comparisons and MRR. */
export function monthlyEquivalent(minor: number, interval: string) {
  return interval === "yearly" ? Math.round(minor / 12) : minor;
}

/** Bytes → "5 GB" style label for storage limits. */
export function formatBytes(bytes: number, locale: AppLocale = "tr") {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${new Intl.NumberFormat(numberLocale(locale), {
    maximumFractionDigits: value >= 10 ? 0 : 1,
  }).format(value)} ${units[unit]}`;
}
