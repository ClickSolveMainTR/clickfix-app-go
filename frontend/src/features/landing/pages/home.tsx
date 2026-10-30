import type { Metadata } from "next";

import {
  fetchPricing,
  isSelfSignupOpen,
  LandingPage,
  landingMetadata,
  resolveLandingLocale,
  type LandingSearchParams,
} from "@/features/landing";
import { fetchMarketingSettings } from "@/features/landing/lib/marketing";

type Props = { searchParams: LandingSearchParams };

export async function renderMetadata({
  searchParams,
}: Props): Promise<Metadata> {
  const [locale, settings] = await Promise.all([
    resolveLandingLocale(searchParams),
    fetchMarketingSettings(),
  ]);
  return landingMetadata(locale, settings);
}

/** Public home for visitors and signed-in users alike. */
export async function renderPage({ searchParams }: Props) {
  const [locale, signupOpen, pricing] = await Promise.all([
    resolveLandingLocale(searchParams),
    isSelfSignupOpen(),
    fetchPricing(),
  ]);
  return (
    <LandingPage locale={locale} signupOpen={signupOpen} pricing={pricing} />
  );
}
