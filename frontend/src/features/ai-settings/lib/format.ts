import type { AppLocale } from "@/config/i18n";

/**
 * AI costs are stored in USD micro-units (1 USD = 1,000,000 micros, see
 * backend `platform/ai` route prices). Small amounts keep enough decimals to
 * stay meaningful (a single run is often a fraction of a cent).
 */
export function formatMicros(micros: number, locale: AppLocale) {
  const value = micros / 1_000_000;
  const abs = Math.abs(value);
  const digits = abs === 0 ? 2 : abs < 0.01 ? 6 : abs < 1 ? 4 : 2;
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: digits,
  }).format(value);
}

export function formatNumber(value: number, locale: AppLocale) {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US").format(
    value,
  );
}

export function formatLatency(ms: number, locale: AppLocale) {
  const tag = locale === "tr" ? "tr-TR" : "en-US";
  if (ms < 1000) {
    return new Intl.NumberFormat(tag, {
      style: "unit",
      unit: "millisecond",
      unitDisplay: "short",
    }).format(ms);
  }
  return new Intl.NumberFormat(tag, {
    style: "unit",
    unit: "second",
    unitDisplay: "short",
    maximumFractionDigits: 1,
  }).format(ms / 1000);
}

/** Minutes after midnight <-> `HH:MM` for `<input type="time">`. */
export function minuteToTime(minute: number) {
  const m = Math.max(0, Math.min(1439, minute));
  return `${String(Math.floor(m / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`;
}

export function timeToMinute(value: string) {
  const [h, m] = value.split(":").map((part) => Number.parseInt(part, 10));
  if (Number.isNaN(h) || Number.isNaN(m)) return null;
  return h * 60 + m;
}
