import type { Metadata } from "next";

import {
  ComparePage,
  isSelfSignupOpen,
  MarketingShell,
  marketingMetadata,
  resolveLandingLocale,
  type LandingSearchParams,
} from "@/features/landing";
import { COMPARE_PATH } from "@/features/landing/constants";
import { translate } from "@/lib/i18n/messages";

type Props = { searchParams: LandingSearchParams };

export async function renderMetadata({
  searchParams,
}: Props): Promise<Metadata> {
  const locale = await resolveLandingLocale(searchParams);
  return marketingMetadata({
    locale,
    path: COMPARE_PATH,
    title: translate(locale, "landing.compare.meta_title"),
    description: translate(locale, "landing.compare.meta_description"),
  });
}

/** Comparison with the global support suites (CF-133). */
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
        { name: translate(locale, "landing.nav.compare"), path: COMPARE_PATH },
      ]}
    >
      <ComparePage />
    </MarketingShell>
  );
}
