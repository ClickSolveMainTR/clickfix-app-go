import type { Metadata } from "next";
import type { ReactNode } from "react";

import {
  fetchMarketingSettings,
  headMetaTags,
} from "@/features/landing/lib/marketing";

/**
 * Search engine verification tokens from "Pazarlama & izleme" (CF-144) and
 * the `<meta>` tags of the custom `<head>` snippet (CF-156); every marketing
 * and docs page carries them in the server-rendered HTML.
 */
export async function generateMetadata(): Promise<Metadata> {
  const settings = await fetchMarketingSettings();
  const other = headMetaTags(settings?.custom_head);
  if (settings?.bing_verification) {
    other["msvalidate.01"] = settings.bing_verification;
  }
  return {
    verification: {
      google: settings?.google_verification || undefined,
      yandex: settings?.yandex_verification || undefined,
      other,
    },
  };
}

/** Public marketing site and docs; tags load per page (`MarketingTrackingSlot`). */
export default function LandingLayout({ children }: { children: ReactNode }) {
  return children;
}
