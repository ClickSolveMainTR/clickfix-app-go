import { getCalendars, getLocales } from 'expo-localization';

import { DEFAULT_TIME_ZONE, formatDateTime } from '@/core/date-format';
import { i18n } from '@/core/i18n';

export { DEFAULT_TIME_ZONE };

/** The device's first calendar (zone, 12/24-hour clock); undefined where the native module is missing. */
function deviceCalendar() {
  try {
    return getCalendars()[0];
  } catch {
    return undefined;
  }
}

let localeCache: { lang: string; tag: string } | null = null;

/**
 * BCP 47 tag for `Intl`: the app language with the device's region when the
 * device speaks that language (`en-US` writes "Oct 27", `en-GB` "27 Oct"),
 * else `tr-TR` / `en-GB`.
 */
export function appLocale() {
  const lang = i18n.language === 'en' ? 'en' : 'tr';
  if (localeCache?.lang === lang) return localeCache.tag;
  let tag = lang === 'en' ? 'en-GB' : 'tr-TR';
  try {
    const device = getLocales().find((l) => l.languageCode === lang && l.regionCode);
    if (device) {
      const candidate = `${lang}-${device.regionCode}`;
      if (Intl.DateTimeFormat.supportedLocalesOf([candidate]).length) tag = candidate;
    }
  } catch {
    // Keep the default tag.
  }
  localeCache = { lang, tag };
  return tag;
}

/** `h23` / `h12` from the device setting, so times follow the user's clock choice. */
function hourCycle(): Intl.DateTimeFormatOptions['hourCycle'] {
  const uses24 = deviceCalendar()?.uses24hourClock;
  return uses24 == null ? undefined : uses24 ? 'h23' : 'h12';
}

/** The device's zone, else Istanbul (the API sends instants in UTC). */
export function displayTimeZone() {
  return deviceCalendar()?.timeZone ?? DEFAULT_TIME_ZONE;
}

function format(
  options: Intl.DateTimeFormatOptions,
  value: Date | string | null | undefined,
  timeZone?: string | null,
) {
  const withClock = options.hour ? { hourCycle: hourCycle(), ...options } : options;
  return formatDateTime(appLocale(), { ...withClock, timeZone: timeZone || displayTimeZone() }, value);
}

/** A plain number with grouping in the app language ("1.250", "4,5"). */
export function formatNumber(
  value: number | string | null | undefined,
  maximumFractionDigits = 0,
  minimumFractionDigits = 0,
) {
  const n = typeof value === 'number' ? value : Number(value ?? 0);
  return new Intl.NumberFormat(appLocale(), { maximumFractionDigits, minimumFractionDigits }).format(
    Number.isFinite(n) ? n : 0,
  );
}

export function formatTime(iso: string, timeZone?: string | null) {
  return format({ hour: '2-digit', minute: '2-digit' }, iso, timeZone);
}

/** Chat timestamps always use a compact 24-hour clock in the display zone. */
export function formatMessageTime(iso: string) {
  return format({ hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }, iso);
}

export function formatDate(iso: string, timeZone?: string | null) {
  return format({ day: 'numeric', month: 'long', weekday: 'short' }, iso, timeZone);
}

/** Compact calendar date without a time (billing periods). */
export function formatCompactDate(iso: string | null | undefined) {
  return format({ day: 'numeric', month: 'short' }, iso);
}

/** Compact date and time for history rows ("26 Eki 14:30"). */
export function formatShort(iso: string | null | undefined, timeZone?: string | null) {
  return format({ day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }, iso, timeZone);
}

/** "5 dk önce", "yarın", "in 2 hours": the nearest whole unit, `now` for under a minute. */
export function formatRelative(iso: string | null | undefined, now = Date.now()) {
  if (!iso) return '';
  const at = new Date(iso).getTime();
  if (Number.isNaN(at)) return '';
  const seconds = (at - now) / 1000;
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ['year', 31_536_000],
    ['month', 2_592_000],
    ['week', 604_800],
    ['day', 86_400],
    ['hour', 3_600],
    ['minute', 60],
  ];
  try {
    const rtf = new Intl.RelativeTimeFormat(appLocale(), { numeric: 'auto' });
    for (const [unit, size] of units)
      if (Math.abs(seconds) >= size) return rtf.format(Math.round(seconds / size), unit);
    return rtf.format(0, 'second');
  } catch {
    return formatShort(iso);
  }
}
