/** Working-time durations ("2 sa 15 dk" / "2h 15m"). */
export function formatDuration(seconds: number, locale: string): string {
  const abs = Math.abs(Math.round(seconds / 60));
  const days = Math.floor(abs / 1440);
  const hours = Math.floor((abs % 1440) / 60);
  const minutes = abs % 60;
  const tr = locale === "tr";
  const parts: string[] = [];
  if (days) parts.push(`${days}${tr ? " g" : "d"}`);
  if (hours) parts.push(`${hours}${tr ? " sa" : "h"}`);
  if (minutes || parts.length === 0)
    parts.push(`${minutes}${tr ? " dk" : "m"}`);
  return parts.slice(0, 2).join(" ");
}

/** Minutes as a short target label (90 → "1 sa 30 dk"). */
export function formatMinutes(
  minutes: number | null | undefined,
  locale: string,
) {
  if (!minutes) return "—";
  return formatDuration(minutes * 60, locale);
}

export const WEEK_DAYS = [
  "mon",
  "tue",
  "wed",
  "thu",
  "fri",
  "sat",
  "sun",
] as const;

export const PRIORITIES = ["urgent", "high", "normal", "low"] as const;

export const CHANNEL_TYPES = [
  "widget",
  "email",
  "whatsapp",
  "instagram",
  "messenger",
  "telegram",
] as const;

export const TIMEZONES = [
  "Europe/Istanbul",
  "UTC",
  "Europe/London",
  "Europe/Berlin",
  "Asia/Dubai",
  "Asia/Baku",
  "America/New_York",
] as const;
