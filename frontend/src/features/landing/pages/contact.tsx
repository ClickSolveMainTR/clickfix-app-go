import type { Metadata } from "next";

import {
  ContactPage,
  isSelfSignupOpen,
  MarketingShell,
  marketingMetadata,
  resolveLandingLocale,
  type LandingSearchParams,
} from "@/features/landing";
import { CONTACT_PATH } from "@/features/landing/constants";
import { translate } from "@/lib/i18n/messages";

type Props = { searchParams: LandingSearchParams };

export async function renderMetadata({
  searchParams,
}: Props): Promise<Metadata> {
  const locale = await resolveLandingLocale(searchParams);
  return marketingMetadata({
    locale,
    path: CONTACT_PATH,
    title: translate(locale, "landing.contact.meta_title"),
    description: translate(locale, "landing.contact.meta_description"),
  });
}

/** Contact and demo request (CF-133). */
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
        { name: translate(locale, "landing.nav.contact"), path: CONTACT_PATH },
      ]}
    >
      <ContactPage />
    </MarketingShell>
  );
}
