import type {
  OrganizationLocale,
  WeekDay,
} from "@/features/tenant-settings/types";

/** API order of `business_hours.days` (always seven entries). */
export const WEEK_DAYS: WeekDay[] = [
  "mon",
  "tue",
  "wed",
  "thu",
  "fri",
  "sat",
  "sun",
];

/** Static label keys (no template keys, see check-i18n). */
export const WEEK_DAY_LABEL_KEYS: Record<WeekDay, string> = {
  mon: "tenant_settings.days.mon",
  tue: "tenant_settings.days.tue",
  wed: "tenant_settings.days.wed",
  thu: "tenant_settings.days.thu",
  fri: "tenant_settings.days.fri",
  sat: "tenant_settings.days.sat",
  sun: "tenant_settings.days.sun",
};

export const ORGANIZATION_LOCALES: OrganizationLocale[] = ["tr", "en"];

export const LOCALE_LABEL_KEYS: Record<OrganizationLocale, string> = {
  tr: "tenant_settings.locales.tr",
  en: "tenant_settings.locales.en",
};

/** Currencies offered first; the API accepts any ISO 4217 code. */
export const CURRENCIES = [
  "TRY",
  "EUR",
  "USD",
  "GBP",
  "CHF",
  "AED",
  "SAR",
  "AZN",
  "RUB",
] as const;

/** Countries offered in the picker (ISO 3166-1 alpha-2). */
export const COUNTRIES = [
  "TR",
  "AZ",
  "CY",
  "DE",
  "NL",
  "BE",
  "FR",
  "GB",
  "IE",
  "IT",
  "ES",
  "AT",
  "CH",
  "SE",
  "NO",
  "DK",
  "PL",
  "GR",
  "BG",
  "RO",
  "US",
  "CA",
  "AE",
  "SA",
  "QA",
  "KW",
  "EG",
  "GE",
  "KZ",
  "UZ",
] as const;

/** Fallback time zones when the runtime has no `Intl.supportedValuesOf`. */
const FALLBACK_TIME_ZONES = [
  "Europe/Istanbul",
  "Europe/London",
  "Europe/Berlin",
  "Europe/Amsterdam",
  "Europe/Paris",
  "Europe/Madrid",
  "Europe/Rome",
  "Europe/Athens",
  "Europe/Moscow",
  "Asia/Baku",
  "Asia/Dubai",
  "Asia/Riyadh",
  "Asia/Tbilisi",
  "Asia/Almaty",
  "America/New_York",
  "America/Chicago",
  "America/Los_Angeles",
  "UTC",
];

/** IANA time zones of the runtime (sorted), with the fallback list. */
export function timeZones(): string[] {
  const intl = Intl as typeof Intl & {
    supportedValuesOf?: (key: "timeZone") => string[];
  };
  const zones = intl.supportedValuesOf?.("timeZone") ?? FALLBACK_TIME_ZONES;
  return [...new Set([...zones, "UTC"])].sort((a, b) => a.localeCompare(b));
}

/** Localized display name of a region / currency code (falls back to the code). */
export function displayName(
  locale: string,
  type: "region" | "currency",
  code: string,
): string {
  try {
    return new Intl.DisplayNames([locale], { type }).of(code) ?? code;
  } catch {
    return code;
  }
}
