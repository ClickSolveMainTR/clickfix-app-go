import { addDays, format, parseISO, subDays } from "date-fns";

import type { AppLocale } from "@/config/i18n";

import type {
  ReportChartType,
  ReportDimension,
  ReportMetric,
  ReportWindow,
} from "../services/reports.service";

/** Allowlist of `GET /v1/tenant/reports/meta`; used before meta loads. */
export const METRICS: ReportMetric[] = [
  "volume",
  "first_response_seconds",
  "resolution_seconds",
  "csat_average",
  "qa_score",
  "sla_breaches",
];
export const DIMENSIONS: ReportDimension[] = [
  "agent",
  "team",
  "channel",
  "day",
];
export const CHART_TYPES: ReportChartType[] = ["table", "bar", "line", "pie"];

/** Lower is better for durations and breaches; higher for the rest. */
export const LOWER_IS_BETTER: ReadonlySet<ReportMetric> = new Set([
  "first_response_seconds",
  "resolution_seconds",
  "sla_breaches",
]);

export function toNumber(value: unknown): number | null {
  if (value === null || value === undefined || value === "") return null;
  const n = typeof value === "number" ? value : Number(value);
  return Number.isFinite(n) ? n : null;
}

/** Seconds as `1 sa 5 dk` / `1h 5m`; under a minute as seconds. */
export function formatSeconds(seconds: number, locale: AppLocale) {
  const tr = locale === "tr";
  if (seconds < 60) return `${Math.round(seconds)} ${tr ? "sn" : "s"}`;
  const minutes = Math.round(seconds / 60);
  const d = Math.floor(minutes / 1440);
  const h = Math.floor((minutes % 1440) / 60);
  const m = minutes % 60;
  const parts: string[] = [];
  if (d) parts.push(`${d} ${tr ? "g" : "d"}`);
  if (h) parts.push(`${h} ${tr ? "sa" : "h"}`);
  if (m && !d) parts.push(`${m} ${tr ? "dk" : "m"}`);
  return parts.join(" ") || `0 ${tr ? "dk" : "m"}`;
}

export function formatMetric(
  metric: string,
  value: unknown,
  locale: AppLocale,
): string {
  const n = toNumber(value);
  if (n === null) return "—";
  const nf = new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    maximumFractionDigits: 2,
  });
  if (metric.endsWith("_seconds")) return formatSeconds(n, locale);
  if (metric === "volume" || metric === "sla_breaches")
    return nf.format(Math.round(n));
  return nf.format(n);
}

/** Inclusive `yyyy-MM-dd` days → `[from, to)` in the browser's zone. */
export function windowFromDays(from: string, to: string): ReportWindow {
  return {
    from: parseISO(from).toISOString(),
    to: addDays(parseISO(to), 1).toISOString(),
  };
}

export function defaultRange(days = 30) {
  const today = new Date();
  return {
    from: format(subDays(today, days - 1), "yyyy-MM-dd"),
    to: format(today, "yyyy-MM-dd"),
  };
}

/** Browser IANA zone, for new report definitions and schedules. */
export function browserTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}
