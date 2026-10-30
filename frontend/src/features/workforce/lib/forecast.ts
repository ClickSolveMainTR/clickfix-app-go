import { parseISO } from "date-fns";

import type { AppLocale } from "@/config/i18n";

import type {
  Forecast,
  ForecastDay,
  ForecastHour,
} from "../services/workforce.service";

const HOUR = 3_600_000;
const DAY = 24 * HOUR;

/** Horizons offered in the filter; the API allows at most 31 days. */
export const HORIZONS = [1, 7, 14, 31] as const;

/** Forecasts may start at most 90 days ahead. */
export const MAX_LEAD_DAYS = 90;

function floorUtcHour(d: Date) {
  const out = new Date(d);
  out.setUTCMinutes(0, 0, 0);
  return out;
}

/**
 * `[since, until)` for a forecast starting on local day `day` (`yyyy-MM-dd`)
 * for `days` days. The API wants whole UTC hours starting no earlier than
 * the current hour, so today starts at the current hour.
 */
export function forecastWindow(day: string, days: number, now = new Date()) {
  const start = parseISO(day);
  const current = floorUtcHour(now);
  let since = floorUtcHour(start);
  if (since < current) since = current;
  const until = floorUtcHour(new Date(start.getTime() + days * DAY));
  const valid =
    !Number.isNaN(start.getTime()) &&
    until > since &&
    since.getTime() <= now.getTime() + MAX_LEAD_DAYS * DAY;
  return {
    since: since.toISOString(),
    until: until.toISOString(),
    valid,
  };
}

/** One forecast bucket in a shape shared by hourly and daily results. */
export type ForecastRow = {
  key: string;
  /** Hour start (RFC 3339) or local date (`yyyy-MM-dd`). */
  at: string;
  volume: number;
  required: number;
  scheduled: number;
  gap: number;
  capacityLimited: boolean;
};

export function forecastRows(
  forecast: Forecast,
  granularity: "hour" | "day",
): ForecastRow[] {
  if (granularity === "day") {
    return (forecast.buckets as ForecastDay[]).map((d) => ({
      key: d.date,
      at: d.date,
      volume: d.volume,
      required: d.required_agent_hours,
      scheduled: d.scheduled_agent_hours,
      gap: d.gap_agent_hours,
      capacityLimited: d.capacity_limited,
    }));
  }
  return (forecast.buckets as ForecastHour[]).map((h) => ({
    key: h.start,
    at: h.start,
    volume: h.volume,
    required: h.required_agents,
    scheduled: h.scheduled_agents,
    gap: h.gap,
    capacityLimited: h.capacity_limited,
  }));
}

function intlLocale(locale: AppLocale) {
  return locale === "tr" ? "tr-TR" : "en-US";
}

/**
 * Bucket label in the organization's zone (the API groups days and the
 * weekday × hour model by that zone, not the browser's).
 */
export function bucketLabel(
  row: ForecastRow,
  granularity: "hour" | "day",
  timezone: string,
  locale: AppLocale,
  long = false,
) {
  try {
    if (granularity === "day") {
      // A calendar date: format it at noon UTC so no zone shifts the day.
      return new Intl.DateTimeFormat(intlLocale(locale), {
        timeZone: "UTC",
        weekday: "short",
        day: "numeric",
        month: "short",
      }).format(new Date(`${row.at}T12:00:00Z`));
    }
    return new Intl.DateTimeFormat(intlLocale(locale), {
      timeZone: timezone,
      weekday: "short",
      ...(long ? { day: "numeric", month: "short" } : {}),
      hour: "2-digit",
      minute: "2-digit",
    }).format(new Date(row.at));
  } catch {
    return row.at;
  }
}

export function formatNumber(value: number, locale: AppLocale, digits = 1) {
  return new Intl.NumberFormat(intlLocale(locale), {
    maximumFractionDigits: digits,
  }).format(value);
}

export type ForecastTotals = {
  volume: number;
  peakRequired: number;
  gapBuckets: number;
  gap: number;
  capacityLimited: boolean;
};

export function forecastTotals(rows: ForecastRow[]): ForecastTotals {
  return rows.reduce<ForecastTotals>(
    (acc, r) => ({
      volume: acc.volume + r.volume,
      peakRequired: Math.max(acc.peakRequired, r.required),
      gapBuckets: acc.gapBuckets + (r.gap > 0 ? 1 : 0),
      gap: acc.gap + r.gap,
      capacityLimited: acc.capacityLimited || r.capacityLimited,
    }),
    {
      volume: 0,
      peakRequired: 0,
      gapBuckets: 0,
      gap: 0,
      capacityLimited: false,
    },
  );
}
