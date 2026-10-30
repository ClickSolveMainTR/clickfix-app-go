import type { Metadata } from "next";

import {
  isSelfSignupOpen,
  MarketingShell,
  marketingMetadata,
  resolveLandingLocale,
  type LandingSearchParams,
} from "@/features/landing";
import { FeaturesIndex } from "@/features/landing/components/features-index";
import { FEATURES_PATH } from "@/features/landing/constants";
import { translate } from "@/lib/i18n/messages";

type Props = { searchParams: LandingSearchParams };

export async function renderMetadata({
  searchParams,
}: Props): Promise<Metadata> {
  const locale = await resolveLandingLocale(searchParams);
  return marketingMetadata({
    locale,
    path: FEATURES_PATH,
    title: translate(locale, "landing.features.meta_title"),
    description: translate(locale, "landing.features.meta_description"),
  });
}

/** Every product area (CF-133). */
export async function renderPage({ searchParams }: Props) {
  const [locale, signupOpen] = await Promise.all([
    resolveLandingLocale(searchParams),
    isSelfSignupOpen(),
  ]);
  return (
    <MarketingShell
      locale={locale}
      signupOpen={signupOpen}
      breadcrumb={[
        {
          name: translate(locale, "landing.nav.features"),
          path: FEATURES_PATH,
        },
      ]}
    >
      <FeaturesIndex />
    </MarketingShell>
  );
}
