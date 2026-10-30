import { upstreamConfig } from "@/config/api";
import { fetchMarketingSettings } from "@/features/landing/lib/marketing";
import { ogImage, OG_SIZE } from "@/features/landing/lib/og";
import { translate } from "@/lib/i18n/messages";

export const size = OG_SIZE;
export const contentType = "image/png";
export const alt = "ClickFix";
/** Picks up a newly uploaded image without a redeploy. */
export const revalidate = 300;

/**
 * Default share card for every page without its own: the image uploaded
 * under "Pazarlama & izleme" when there is one (CF-144), else the generated
 * ClickFix card.
 */
export default async function OpenGraphImage() {
  const settings = await fetchMarketingSettings();
  if (settings?.og_image_url) {
    try {
      const upstream = await fetch(
        `${upstreamConfig.baseUrl}${settings.og_image_url.replace(/^\/v1/, "")}`,
        { next: { revalidate: 300 }, signal: AbortSignal.timeout(3000) },
      );
      if (upstream.ok) {
        return new Response(await upstream.arrayBuffer(), {
          headers: {
            "Content-Type": upstream.headers.get("content-type") ?? "image/png",
            "Cache-Control":
              "public, max-age=300, stale-while-revalidate=86400",
          },
        });
      }
    } catch {
      // Fall back to the generated card.
    }
  }
  return ogImage({
    eyebrow: translate("tr", "landing.hero.eyebrow"),
    title: translate("tr", "landing.meta.title").replace(/^ClickFix · /, ""),
  });
}
