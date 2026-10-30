import { upstreamConfig } from "@/config/api";
import type { components } from "@/generated/api";

import type { TrackingConfig } from "./tracking";

export type MarketingSettings = components["schemas"]["MarketingSettings"];

/** Next.js data cache tag; the BFF revalidates it when an admin saves (CF-144). */
export const MARKETING_CACHE_TAG = "marketing-settings";

/**
 * Tracking and SEO defaults the platform admin manages under "Pazarlama &
 * izleme" (`GET /v1/public/marketing`). Cached for five minutes and
 * revalidated on save; `null` when the API cannot be reached, so the pages
 * render without any tag.
 */
export async function fetchMarketingSettings(): Promise<MarketingSettings | null> {
  try {
    const response = await fetch(`${upstreamConfig.baseUrl}/public/marketing`, {
      headers: { Accept: "application/json" },
      next: { revalidate: 300, tags: [MARKETING_CACHE_TAG] },
      signal: AbortSignal.timeout(3000),
    });
    if (!response.ok) return null;
    const json = (await response.json()) as { data?: MarketingSettings };
    return json.data ?? null;
  } catch {
    return null;
  }
}

/** The ids and snippets the browser needs (`TrackingConfig`). */
export function trackingConfig(
  settings: MarketingSettings | null,
): TrackingConfig {
  return {
    gtm_id: settings?.gtm_id ?? "",
    ga4_id: settings?.ga4_id ?? "",
    meta_pixel_id: settings?.meta_pixel_id ?? "",
    linkedin_partner_id: settings?.linkedin_partner_id ?? "",
    tiktok_pixel_id: settings?.tiktok_pixel_id ?? "",
    google_ads_id: settings?.google_ads_id ?? "",
    google_ads_signup_label: settings?.google_ads_signup_label ?? "",
    clarity_id: settings?.clarity_id ?? "",
    hotjar_id: settings?.hotjar_id ?? "",
    custom_head: settings?.custom_head ?? "",
    custom_body: settings?.custom_body ?? "",
  };
}

const META_TAG = /<meta\b[^>]*>/gi;
const ATTR = /([a-zA-Z:-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')/g;

/**
 * `<meta name content>` tags from the custom `<head>` snippet. They are inert,
 * so the layout renders them in the first HTML response: crawlers such as
 * Meta's domain verification run no JavaScript and give no consent (CF-156).
 */
export function headMetaTags(html: string | undefined): Record<string, string> {
  const tags: Record<string, string> = {};
  for (const [tag] of (html ?? "").matchAll(META_TAG)) {
    const attrs: Record<string, string> = {};
    for (const [, key, double, single] of tag.matchAll(ATTR)) {
      attrs[key.toLowerCase()] = double ?? single ?? "";
    }
    if (attrs.name && attrs.content) tags[attrs.name] = attrs.content;
  }
  return tags;
}
