import type { Metadata } from "next";

import { brand } from "@/config/brand";
import type { AppLocale } from "@/config/i18n";

import { localizedHref } from "./locale";

/** Absolute origin of the public site (sitemap, JSON-LD). */
export function siteBase() {
  return (process.env.SITE_URL || brand.siteUrl).replace(/\/$/, "");
}

/** Absolute URL of a marketing path in the given language. */
export function absoluteUrl(path: string, locale: AppLocale = "tr") {
  return `${siteBase()}${localizedHref(path, locale)}`;
}

/**
 * Root share card (`app/opengraph-image.tsx`: the admin's default image or
 * the generated card). A page's own `openGraph` replaces the parent's, image
 * included, so it is listed explicitly; a segment's own `opengraph-image`
 * file still wins over it.
 */
export const DEFAULT_SHARE_IMAGE = {
  url: "/opengraph-image",
  width: 1200,
  height: 630,
  alt: brand.productName,
};

/**
 * hreflang alternates of a page: Turkish (default, also `x-default`) and
 * English (`?lang=en`).
 */
export function languageAlternates(path: string) {
  return {
    tr: path,
    en: localizedHref(path, "en"),
    "x-default": path,
  };
}

/**
 * Metadata of a marketing / docs page: the title gets the brand suffix from
 * the root template, canonical is the page in its own language and both
 * languages are listed as alternates (`?lang=en`).
 */
export function marketingMetadata({
  locale,
  path,
  title,
  description,
  type = "website",
}: {
  locale: AppLocale;
  path: string;
  title: string;
  description: string;
  type?: "website" | "article";
}): Metadata {
  return {
    title,
    description,
    alternates: {
      canonical: localizedHref(path, locale),
      languages: languageAlternates(path),
    },
    openGraph: {
      type,
      title,
      description,
      url: localizedHref(path, locale),
      siteName: brand.name,
      locale: locale === "tr" ? "tr_TR" : "en_US",
      alternateLocale: locale === "tr" ? ["en_US"] : ["tr_TR"],
      images: [DEFAULT_SHARE_IMAGE],
    },
    // X falls back to og:image, which a page's own card file overrides.
    twitter: { card: "summary_large_image", title, description },
    robots: { index: true, follow: true },
  };
}

/** schema.org BreadcrumbList for a trail of `{ name, path }`. */
export function breadcrumbJsonLd(
  items: { name: string; path: string }[],
  locale: AppLocale,
) {
  return {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: items.map((item, i) => ({
      "@type": "ListItem",
      position: i + 1,
      name: item.name,
      item: absoluteUrl(item.path, locale),
    })),
  };
}

/** schema.org FAQPage from question / answer pairs. */
export function faqJsonLd(items: { q: string; a: string }[]) {
  return {
    "@context": "https://schema.org",
    "@type": "FAQPage",
    mainEntity: items.map((item) => ({
      "@type": "Question",
      name: item.q,
      acceptedAnswer: { "@type": "Answer", text: item.a },
    })),
  };
}
