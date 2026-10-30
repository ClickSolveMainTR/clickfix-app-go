import type { Metadata } from "next";

import { translate } from "@/lib/i18n/messages";

import type { HelpLocale, HelpOrganization } from "./api";

/** `path` in the help center language (`?lang=en` for English). */
export function localized(path: string, locale: HelpLocale) {
  return locale === "tr" ? path : `${path}?lang=${locale}`;
}

/**
 * Title, description, canonical, hreflang alternates (only languages the
 * organization publishes in) and Open Graph for a help center page.
 */
export function helpMetadata({
  org,
  locale,
  path,
  title,
  description,
  type = "website",
  publishedTime,
  modifiedTime,
  alternates = true,
  index = true,
}: {
  org: HelpOrganization;
  locale: HelpLocale;
  path: string;
  title: string;
  description: string;
  type?: "website" | "article";
  publishedTime?: string;
  modifiedTime?: string;
  /** False for pages that exist in one language only (articles). */
  alternates?: boolean;
  index?: boolean;
}): Metadata {
  const fullTitle = `${title} · ${org.name}`;
  const languages: Record<string, string> = {};
  if (alternates) {
    for (const l of org.locales)
      languages[l.locale] = localized(path, l.locale);
  }
  const ogImage = org.logo_url ? `/api${org.logo_url}` : undefined;
  return {
    title: { absolute: fullTitle },
    description,
    alternates: {
      canonical: localized(path, locale),
      ...(Object.keys(languages).length > 1 ? { languages } : {}),
    },
    openGraph: {
      type,
      title: fullTitle,
      description,
      url: localized(path, locale),
      siteName: `${org.name} · ${translate(locale, "help_center.title")}`,
      locale: locale === "tr" ? "tr_TR" : "en_US",
      ...(ogImage ? { images: [{ url: ogImage }] } : {}),
      ...(type === "article" ? { publishedTime, modifiedTime } : {}),
    },
    twitter: { card: "summary", title: fullTitle, description },
    robots: index
      ? { index: true, follow: true }
      : { index: false, follow: true },
  };
}
