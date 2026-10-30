import type { Metadata } from "next";

import { listDocs } from "@/features/docs/lib/content";
import {
  isSelfSignupOpen,
  MarketingShell,
  marketingMetadata,
  resolveLandingLocale,
  SecurityPage,
  type LandingSearchParams,
} from "@/features/landing";
import { SECURITY_PATH } from "@/features/landing/constants";
import { translate } from "@/lib/i18n/messages";

type Props = { searchParams: LandingSearchParams };

const SECURITY_DOCS = ["kvkk", "sso-scim"];

export async function renderMetadata({
  searchParams,
}: Props): Promise<Metadata> {
  const locale = await resolveLandingLocale(searchParams);
  return marketingMetadata({
    locale,
    path: SECURITY_PATH,
    title: translate(locale, "landing.security.meta_title"),
    description: translate(locale, "landing.security.meta_description"),
  });
}

/** Security and KVKK (CF-133). */
export async function renderPage({ searchParams }: Props) {
  const [locale, signupOpen] = await Promise.all([
    resolveLandingLocale(searchParams),
    isSelfSignupOpen(),
  ]);
  const docs = listDocs(locale)
    .filter((page) => SECURITY_DOCS.includes(page.slug))
    .map(({ slug, title, description }) => ({ slug, title, description }));
  return (
    <MarketingShell
      locale={locale}
      signupOpen={signupOpen}
      breadcrumb={[
        {
          name: translate(locale, "landing.nav.security"),
          path: SECURITY_PATH,
        },
      ]}
    >
      <SecurityPage docs={docs} />
    </MarketingShell>
  );
}
