import type { Metadata } from "next";

import {
  isSelfSignupOpen,
  MarketingShell,
  marketingMetadata,
  resolveLandingLocale,
  type LandingSearchParams,
} from "@/features/landing";
import { DownloadPage } from "@/features/landing/components/download-page";
import { DOWNLOAD_PATH } from "@/features/landing/constants";
import { fetchDesktopDownloads } from "@/features/landing/lib/desktop";
import { translate } from "@/lib/i18n/messages";

type Props = { searchParams: LandingSearchParams };

export async function renderMetadata({
  searchParams,
}: Props): Promise<Metadata> {
  const locale = await resolveLandingLocale(searchParams);
  return marketingMetadata({
    locale,
    path: DOWNLOAD_PATH,
    title: translate(locale, "landing.download.meta_title"),
    description: translate(locale, "landing.download.meta_description"),
  });
}

/** Desktop apps download page (CF-240). */
export async function renderPage({ searchParams }: Props) {
  const [locale, signupOpen, items] = await Promise.all([
    resolveLandingLocale(searchParams),
    isSelfSignupOpen(),
    fetchDesktopDownloads(),
  ]);
  return (
    <MarketingShell
      locale={locale}
      signupOpen={signupOpen}
      breadcrumb={[
        {
          name: translate(locale, "landing.nav.download"),
          path: DOWNLOAD_PATH,
        },
      ]}
    >
      <DownloadPage items={items} />
    </MarketingShell>
  );
}
