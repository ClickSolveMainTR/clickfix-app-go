import type { AppLocale } from "@/config/i18n";

/** Time zones offered in the organization wizard (IANA ids). */
export const COMMON_TIMEZONES = [
  "Europe/Istanbul",
  "Europe/London",
  "Europe/Berlin",
  "Europe/Amsterdam",
  "Europe/Paris",
  "Europe/Madrid",
  "Europe/Rome",
  "Europe/Vienna",
  "Europe/Zurich",
  "Europe/Brussels",
  "Europe/Stockholm",
  "Europe/Warsaw",
  "Europe/Athens",
  "Europe/Bucharest",
  "Europe/Sofia",
  "Europe/Kyiv",
  "Europe/Moscow",
  "Asia/Baku",
  "Asia/Tbilisi",
  "Asia/Dubai",
  "Asia/Riyadh",
  "Asia/Qatar",
  "Asia/Tehran",
  "Asia/Almaty",
  "Asia/Tashkent",
  "Asia/Karachi",
  "Asia/Kolkata",
  "Asia/Singapore",
  "Asia/Shanghai",
  "Asia/Tokyo",
  "Australia/Sydney",
  "Africa/Cairo",
  "Africa/Lagos",
  "Africa/Johannesburg",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Toronto",
  "America/Mexico_City",
  "America/Sao_Paulo",
  "UTC",
] as const;

/** Countries offered in the organization wizard (ISO 3166-1 alpha-2). */
export const COMMON_COUNTRIES = [
  "TR",
  "AZ",
  "CY",
  "DE",
  "NL",
  "GB",
  "FR",
  "AT",
  "BE",
  "CH",
  "IT",
  "ES",
  "SE",
  "GR",
  "BG",
  "RO",
  "GE",
  "UA",
  "KZ",
  "UZ",
  "AE",
  "SA",
  "QA",
  "EG",
  "US",
  "CA",
] as const;

function intlLocale(locale: AppLocale) {
  return locale === "tr" ? "tr-TR" : "en-US";
}

/** "Türkiye" / "Turkey" for "TR"; the code itself when Intl cannot name it. */
export function countryName(code: string, locale: AppLocale) {
  if (!code) return "—";
  try {
    const names = new Intl.DisplayNames([intlLocale(locale)], {
      type: "region",
    });
    return names.of(code.toUpperCase()) ?? code;
  } catch {
    return code;
  }
}

/** "Türkçe" / "English" for the organization language. */
export function localeName(code: string, locale: AppLocale) {
  try {
    const names = new Intl.DisplayNames([intlLocale(locale)], {
      type: "language",
    });
    const name = names.of(code) ?? code;
    return name.charAt(0).toLocaleUpperCase(intlLocale(locale)) + name.slice(1);
  } catch {
    return code;
  }
}

/** "GMT+03:00" style offset of a time zone right now. */
export function timezoneOffset(zone: string) {
  try {
    const part = new Intl.DateTimeFormat("en-US", {
      timeZone: zone,
      timeZoneName: "longOffset",
    })
      .formatToParts(new Date())
      .find((p) => p.type === "timeZoneName");
    return part?.value === "GMT" ? "GMT+00:00" : (part?.value ?? "");
  } catch {
    return "";
  }
}

/** "İstanbul" style city label from an IANA id. */
export function timezoneCity(zone: string) {
  const city = zone.split("/").pop() ?? zone;
  return city.replace(/_/g, " ");
}
