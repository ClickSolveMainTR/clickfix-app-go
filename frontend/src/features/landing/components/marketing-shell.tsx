import type { ReactNode } from "react";

import type { AppLocale } from "@/config/i18n";
import { breadcrumbJsonLd } from "@/features/landing/lib/seo";
import { translate } from "@/lib/i18n/messages";

import "../landing.css";

import { JsonLd } from "./json-ld";
import { LandingProvider } from "./landing-context";
import { LandingNav } from "./landing-nav";
import { SiteFooter } from "./site-footer";
import { MarketingTrackingSlot } from "./tracking/marketing-tracking-slot";

/**
 * Frame of every marketing and docs page: the ink look, the fixed nav, a
 * skip link to `#content` and the footer. `breadcrumb` (after the home page)
 * becomes the page's BreadcrumbList structured data.
 */
export function MarketingShell({
  locale,
  signupOpen,
  breadcrumb,
  children,
}: {
  locale: AppLocale;
  signupOpen: boolean;
  breadcrumb?: { name: string; path: string }[];
  children: ReactNode;
}) {
  return (
    <LandingProvider locale={locale} signupOpen={signupOpen}>
      <div
        lang={locale}
        className="landing-root landing-grain relative min-h-svh overflow-x-clip"
      >
        {breadcrumb ? (
          <JsonLd
            data={breadcrumbJsonLd(
              [
                {
                  name: translate(locale, "landing.nav.home_short"),
                  path: "/",
                },
                ...breadcrumb,
              ],
              locale,
            )}
          />
        ) : null}
        <a
          href="#content"
          className="sr-only z-[70] rounded-full bg-white px-4 py-2 text-sm text-black focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
        >
          {translate(locale, "landing.nav.skip")}
        </a>
        <LandingNav />
        <main id="content" tabIndex={-1} className="outline-none">
          {children}
        </main>
        <SiteFooter />
        <MarketingTrackingSlot locale={locale} />
      </div>
    </LandingProvider>
  );
}
