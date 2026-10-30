import type { AppLocale } from "@/config/i18n";
import type { PricingCatalog } from "@/features/landing/lib/pricing";
import { brand } from "@/config/brand";
import { FAQ_KEYS } from "@/features/landing/constants";
import { FEATURE_AREAS } from "@/features/landing/content/areas";
import { featureCopy } from "@/features/landing/content/features";
import { landingHref } from "@/features/landing/lib/locale";
import { pricingOffers } from "@/features/landing/lib/pricing";
import { faqJsonLd, siteBase } from "@/features/landing/lib/seo";
import { translate } from "@/lib/i18n/messages";

import "../landing.css";

import { Audience } from "./audience";
import { ChannelStrip } from "./channel-strip";
import { Faq } from "./faq";
import { FinalCta } from "./final-cta";
import { Hero } from "./hero";
import { HowItWorks } from "./how-it-works";
import { JsonLd } from "./json-ld";
import { LandingProvider } from "./landing-context";
import { LandingNav } from "./landing-nav";
import { Platform, TrustBand } from "./platform";
import { Pricing } from "./pricing";
import { ProductTour } from "./product-tour";
import { SiteFooter } from "./site-footer";
import { MarketingTrackingSlot } from "./tracking/marketing-tracking-slot";

/**
 * Public marketing site (`/` without a session). Its own ink look in both
 * colour modes: Space Grotesk headings, Geist text, signal green accents.
 */
export function LandingPage({
  locale,
  signupOpen,
  pricing,
}: {
  locale: AppLocale;
  signupOpen: boolean;
  pricing: PricingCatalog;
}) {
  const t = (key: string) => translate(locale, key);
  const organization = {
    "@context": "https://schema.org",
    "@type": "Organization",
    name: brand.name,
    url: siteBase(),
    logo: `${siteBase()}/icon.svg`,
    email: brand.supportEmail,
  };
  const application: Record<string, unknown> = {
    "@context": "https://schema.org",
    "@type": "SoftwareApplication",
    name: brand.productName,
    applicationCategory: "BusinessApplication",
    applicationSubCategory: "Customer support software",
    operatingSystem: "Web, iOS, Android",
    description: t("landing.meta.description"),
    url: siteBase(),
    image: `${siteBase()}/opengraph-image`,
    inLanguage: ["tr", "en"],
    featureList: FEATURE_AREAS.map(
      (area) => featureCopy(locale)[area.slug].name,
    ),
    publisher: { "@type": "Organization", name: brand.name },
  };
  const offers = pricingOffers(pricing);
  if (offers.length > 0) {
    Object.assign(application, {
      offers: {
        "@type": "AggregateOffer",
        priceCurrency: offers[0]!.priceCurrency,
        lowPrice: Math.min(...offers.map((o) => o.price)),
        highPrice: Math.max(...offers.map((o) => o.price)),
        offerCount: offers.length,
      },
    });
  }
  const product =
    offers.length > 0
      ? {
          "@context": "https://schema.org",
          "@type": "Product",
          name: brand.productName,
          description: t("landing.meta.description"),
          brand: { "@type": "Brand", name: brand.name },
          image: `${siteBase()}/opengraph-image`,
          offers: offers.map(({ planName, ...offer }) => ({
            "@type": "Offer",
            name: translate(locale, "landing.pricing.offer_name", {
              plan: planName,
            }),
            ...offer,
            availability: "https://schema.org/InStock",
            url: `${siteBase()}${landingHref(locale)}#pricing`,
          })),
        }
      : null;
  const website = {
    "@context": "https://schema.org",
    "@type": "WebSite",
    name: brand.productName,
    url: siteBase(),
    inLanguage: ["tr", "en"],
    publisher: { "@type": "Organization", name: brand.name },
  };
  const faq = faqJsonLd(
    FAQ_KEYS.map((key) => ({
      q: t(`landing.faq.${key}_q`),
      a: t(`landing.faq.${key}_a`),
    })),
  );

  return (
    <LandingProvider locale={locale} signupOpen={signupOpen}>
      <div
        lang={locale}
        className="landing-root landing-grain relative min-h-svh overflow-x-clip"
      >
        <JsonLd data={organization} />
        <JsonLd data={website} />
        <JsonLd data={application} />
        {product ? <JsonLd data={product} /> : null}
        <JsonLd data={faq} />
        <a
          href="#content"
          className="sr-only z-[70] rounded-full bg-white px-4 py-2 text-sm text-black focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
        >
          {translate(locale, "landing.nav.skip")}
        </a>
        <LandingNav />
        <main id="content" tabIndex={-1} className="outline-none">
          <Hero />
          <ProductTour />
          <ChannelStrip />
          <Platform />
          <HowItWorks />
          <Audience />
          <TrustBand />
          <Pricing catalog={pricing} />
          <Faq />
          <FinalCta />
        </main>
        <SiteFooter />
        <MarketingTrackingSlot locale={locale} />
      </div>
    </LandingProvider>
  );
}
