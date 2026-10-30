import { headers } from "next/headers";
import { cache } from "react";

import { upstreamConfig } from "@/config/api";
import { brand } from "@/config/brand";
import type { components } from "@/generated/api";
import { clientIpFromHeaders } from "@/lib/server/upstream";

export type HelpLocale = "tr" | "en";
export type HelpOrganization = components["schemas"]["HelpOrganization"];
export type HelpCollection = components["schemas"]["HelpCollection"];
export type HelpArticleSummary = components["schemas"]["HelpArticleSummary"];
export type HelpArticle = components["schemas"]["HelpArticle"];
export type HelpHome = components["schemas"]["HelpHome"];
export type HelpCollectionPage = components["schemas"]["HelpCollectionPage"];
export type HelpArticlePage = components["schemas"]["HelpArticlePage"];
export type HelpSearchResult = components["schemas"]["HelpSearchResult"];
export type HelpSitemap = components["schemas"]["HelpSitemap"];
export type ForumCategory = components["schemas"]["ForumCategory"];
export type ForumTopicSummary = components["schemas"]["ForumTopicSummary"];
export type ForumTopic = components["schemas"]["ForumTopic"];
export type ForumReply = components["schemas"]["ForumReply"];
export type ForumList<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type HelpSearchParams = Promise<{
  lang?: string | string[];
  q?: string | string[];
}>;

/** `?lang=en` selects English; anything else is Turkish. */
export async function resolveHelpLocale(
  searchParams: HelpSearchParams,
): Promise<HelpLocale> {
  const { lang } = await searchParams;
  const value = Array.isArray(lang) ? lang[0] : lang;
  return value === "en" ? "en" : "tr";
}

/** Thrown for a missing help center / page; pages turn it into notFound(). */
export class HelpNotFoundError extends Error {}

/**
 * Server-side GET of the public help center API (Go `/v1/public/help/*`).
 * The visitor IP is forwarded so the API's per-IP limits apply to visitors,
 * not to this server. Deduplicated per request (metadata + page).
 */
const fetchHelp = cache(async <T>(path: string): Promise<T> => {
  const incoming = await headers();
  const ip = clientIpFromHeaders(incoming);
  const response = await fetch(
    `${upstreamConfig.baseUrl}/public/help/${path}`,
    {
      headers: {
        Accept: "application/json",
        ...(ip ? { "X-Forwarded-For": ip } : {}),
      },
      cache: "no-store",
      signal: AbortSignal.timeout(5000),
    },
  );
  if (response.status === 404 || response.status === 422) {
    throw new HelpNotFoundError(path);
  }
  if (!response.ok) {
    throw new Error(`help center API ${response.status} for ${path}`);
  }
  const json = (await response.json()) as { data: T };
  return json.data;
});

const seg = encodeURIComponent;

export const helpApi = {
  home: (org: string, locale: HelpLocale) =>
    fetchHelp<HelpHome>(`${seg(org)}?locale=${locale}`),
  collection: (org: string, slug: string, locale: HelpLocale) =>
    fetchHelp<HelpCollectionPage>(
      `${seg(org)}/collections/${seg(slug)}?locale=${locale}`,
    ),
  article: (org: string, slug: string, locale: HelpLocale) =>
    fetchHelp<HelpArticlePage>(
      `${seg(org)}/articles/${seg(slug)}?locale=${locale}`,
    ),
  search: (org: string, q: string, locale: HelpLocale) =>
    fetchHelp<HelpSearchResult>(
      `${seg(org)}/search?q=${seg(q)}&locale=${locale}`,
    ),
  sitemap: (org: string) => fetchHelp<HelpSitemap>(`${seg(org)}/sitemap`),
  /** Community forum (CF-78): published content only. */
  forumCategories: (org: string) =>
    fetchHelp<ForumCategory[]>(`${seg(org)}/forum/categories`),
  forumTopics: (
    org: string,
    q: { limit: number; offset: number; category?: string; q?: string },
  ) =>
    fetchHelp<ForumList<ForumTopicSummary>>(
      `${seg(org)}/forum/topics?limit=${q.limit}&offset=${q.offset}${
        q.category ? `&category=${seg(q.category)}` : ""
      }${q.q ? `&q=${seg(q.q)}` : ""}`,
    ),
  forumTopic: (org: string, slug: string, limit: number, offset: number) =>
    fetchHelp<ForumTopic>(
      `${seg(org)}/forum/topics/${seg(slug)}?limit=${limit}&offset=${offset}`,
    ),
  forumSitemap: (org: string, limit: number, offset: number) =>
    fetchHelp<ForumList<{ slug: string; updated_at: string }>>(
      `${seg(org)}/forum/sitemap?limit=${limit}&offset=${offset}`,
    ),
};

/** Browser URL of the API-owned logo (same-origin BFF). */
export function helpLogoSrc(org: HelpOrganization): string | null {
  return org.logo_url ? `/api${org.logo_url}` : null;
}

/** Absolute site origin for canonical URLs, sitemaps and JSON-LD. */
export function siteOrigin(): string {
  const raw = process.env.SITE_URL || process.env.AUTH_URL || brand.siteUrl;
  return raw.replace(/\/$/, "");
}
