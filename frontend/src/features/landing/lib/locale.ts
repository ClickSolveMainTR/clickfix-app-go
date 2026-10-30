import type { Metadata } from "next";

import { brand } from "@/config/brand";
import { i18nConfig, type AppLocale } from "@/config/i18n";
import { routes } from "@/config/routes";
import { DEFAULT_SHARE_IMAGE } from "@/features/landing/lib/seo";
import { translate } from "@/lib/i18n/messages";

export type LandingSearchParams = Promise<
  Record<string, string | string[] | undefined>
>;

/**
 * The marketing pages render on the server in the language of the link:
 * `?lang=en` for English, Turkish otherwise.
 */
export async function resolveLandingLocale(
  searchParams: LandingSearchParams,
): Promise<AppLocale> {
  const { lang } = await searchParams;
  const value = Array.isArray(lang) ? lang[0] : lang;
  return i18nConfig.supportedLocales.find((l) => l === value) ?? "tr";
}

/** `/` in the given language (Turkish carries no query). */
export function landingHref(locale: AppLocale) {
  return locale === "tr" ? routes.public.root : `/?lang=${locale}`;
}

/** A path in the given language (`?lang=en` for English). */
export function localizedHref(path: string, locale: AppLocale) {
  if (locale === "tr") return path;
  const [base, hash] = path.split("#", 2);
  return `${base}${base.includes("?") ? "&" : "?"}lang=${locale}${hash ? `#${hash}` : ""}`;
}

/**
 * Title, description and both language versions; canonical is `/`. The
 * Turkish page takes the site title / description set under "Pazarlama &
 * izleme" when there is one (CF-144).
 */
export function landingMetadata(
  locale: AppLocale,
  defaults?: { site_title?: string; site_description?: string } | null,
): Metadata {
  const title =
    (locale === "tr" && defaults?.site_title) ||
    translate(locale, "landing.meta.title");
  const description =
    (locale === "tr" && defaults?.site_description) ||
    translate(locale, "landing.meta.description");
  return {
    title: { absolute: title },
    description,
    alternates: {
      canonical: landingHref(locale),
      languages: { tr: "/", en: "/?lang=en", "x-default": "/" },
    },
    openGraph: {
      type: "website",
      url: landingHref(locale),
      title,
      description,
      siteName: brand.name,
      locale: locale === "tr" ? "tr_TR" : "en_US",
      alternateLocale: locale === "tr" ? ["en_US"] : ["tr_TR"],
      images: [DEFAULT_SHARE_IMAGE],
    },
    // X falls back to og:image, which a page's own card file overrides.
    twitter: { card: "summary_large_image", title, description },
    robots: {
      index: true,
      follow: true,
      googleBot: { "max-image-preview": "large", "max-snippet": -1 },
    },
  };
}

/** Link of a top-level marketing page (`SITE_PAGES`), with its section hash. */
export function sitePageHref(
  page: { path: string; hash?: string },
  locale: AppLocale,
) {
  const href =
    page.path === "/" ? landingHref(locale) : localizedHref(page.path, locale);
  return page.hash ? `${href}#${page.hash}` : href;
}
