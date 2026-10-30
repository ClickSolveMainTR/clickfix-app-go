import type { AppLocale } from "@/config/i18n";
import { formatSeconds } from "@/features/reports/lib/format";

import type { GameMetric } from "../services/coaching.service";

export const METRICS: GameMetric[] = ["resolutions", "frt", "csat", "qa"];

/** Upper bound of a threshold per metric (the API rejects more). */
export const METRIC_MAX: Record<GameMetric, number> = {
  resolutions: 1_000_000,
  frt: 1_000_000,
  csat: 5,
  qa: 100,
};

/** First response time is the only metric where lower wins. */
export function thresholdLabel(
  metric: GameMetric,
  threshold: number,
  locale: AppLocale,
) {
  const nf = new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    maximumFractionDigits: 2,
  });
  switch (metric) {
    case "frt":
      return `≤ ${formatSeconds(threshold, locale)}`;
    case "csat":
      return `≥ ${nf.format(threshold)} / 5`;
    case "qa":
      return `≥ ${nf.format(threshold)} / 100`;
    default:
      return `≥ ${nf.format(threshold)}`;
  }
}

export function formatScore(value: number, locale: AppLocale, digits = 1) {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    maximumFractionDigits: digits,
  }).format(value);
}

export function formatDate(iso: string | null | undefined, locale: AppLocale) {
  if (!iso) return "—";
  return new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-US", {
    dateStyle: "medium",
  }).format(new Date(iso));
}
