import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { listDocs } from "@/features/docs/lib/content";
import {
  breadcrumbJsonLd,
  FeaturePage,
  isSelfSignupOpen,
  JsonLd,
  MarketingShell,
  marketingMetadata,
  resolveLandingLocale,
  type LandingSearchParams,
} from "@/features/landing";
import { FEATURES_PATH } from "@/features/landing/constants";
import { featurePath, findFeatureArea } from "@/features/landing/content/areas";
import { featureCopy } from "@/features/landing/content/features";
import { translate } from "@/lib/i18n/messages";

type Props = {
  params: Promise<{ slug: string }>;
  searchParams: LandingSearchParams;
};

export async function renderMetadata({
  params,
  searchParams,
}: Props): Promise<Metadata> {
  const [{ slug }, locale] = await Promise.all([
    params,
    resolveLandingLocale(searchParams),
  ]);
  const area = findFeatureArea(slug);
  if (!area) return {};
  const copy = featureCopy(locale)[area.slug];
  return marketingMetadata({
    locale,
    path: featurePath(area),
    title: copy.name,
    description: copy.description,
  });
}

/** One product area (CF-133). */
export async function renderPage({ params, searchParams }: Props) {
  const [{ slug }, locale, signupOpen] = await Promise.all([
    params,
    resolveLandingLocale(searchParams),
    isSelfSignupOpen(),
  ]);
  const area = findFeatureArea(slug);
  if (!area) notFound();
  const copy = featureCopy(locale)[area.slug];
  const pages = listDocs(locale);
  const docs = area.docs.flatMap((docSlug) => {
    const page = pages.find((p) => p.slug === docSlug);
    return page
      ? [{ slug: page.slug, title: page.title, description: page.description }]
      : [];
  });

  return (
    <MarketingShell locale={locale} signupOpen={signupOpen}>
      <JsonLd
        data={breadcrumbJsonLd(
          [
            { name: translate(locale, "landing.nav.home_short"), path: "/" },
            {
              name: translate(locale, "landing.nav.features"),
              path: FEATURES_PATH,
            },
            { name: copy.name, path: featurePath(area) },
          ],
          locale,
        )}
      />
      <FeaturePage slug={area.slug} docs={docs} />
    </MarketingShell>
  );
}
