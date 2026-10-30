import type { AppLocale } from "@/config/i18n";
import {
  fetchMarketingSettings,
  trackingConfig,
} from "@/features/landing/lib/marketing";

import { MarketingTracking } from "./marketing-tracking";

/** Cached configuration keeps marketing/docs pages eligible for ISR.
 * The browser reads consent before opening the banner or loading tags. */
export async function MarketingTrackingSlot({ locale }: { locale: AppLocale }) {
  const settings = await fetchMarketingSettings();
  return (
    <MarketingTracking locale={locale} config={trackingConfig(settings)} />
  );
}
