"use client";

import { usePathname } from "next/navigation";
import { useEffect } from "react";

import {
  readConsent,
  takeConversion,
  trackSignup,
  type TrackingConfig,
} from "@/features/landing/lib/tracking";

/**
 * Fires the sign-up conversion (dataLayer `sign_up`, GA4, Google Ads,
 * Meta / TikTok `CompleteRegistration`) on the first page after a
 * successful sign-up (CF-144). The BFF marks it with the `cf_conv` cookie;
 * without that cookie this only reads `document.cookie` and does nothing.
 * Tags load here only when the visitor allowed them on the marketing site.
 */
export function SignupConversion() {
  const pathname = usePathname();
  useEffect(() => {
    const consent = readConsent();
    if (!takeConversion() || !consent) return;
    if (!consent.analytics && !consent.marketing) return;
    void fetch("/api/v1/public/marketing", {
      headers: { Accept: "application/json" },
    })
      .then((response) => (response.ok ? response.json() : null))
      .then((json: { data?: Partial<TrackingConfig> } | null) => {
        if (!json?.data) return;
        trackSignup(
          {
            gtm_id: "",
            ga4_id: "",
            meta_pixel_id: "",
            linkedin_partner_id: "",
            tiktok_pixel_id: "",
            google_ads_id: "",
            google_ads_signup_label: "",
            ...json.data,
            // Session recording and custom snippets stay on the marketing pages.
            clarity_id: "",
            hotjar_id: "",
            custom_head: "",
            custom_body: "",
          },
          { analytics: consent.analytics, marketing: consent.marketing },
        );
      })
      .catch(() => undefined);
  }, [pathname]);
  return null;
}
