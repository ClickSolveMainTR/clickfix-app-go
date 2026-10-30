import type { Metadata } from "next";

import { MarketingTrackingSlot } from "@/features/landing/components/tracking/marketing-tracking-slot";
import Link from "next/link";
import { notFound } from "next/navigation";

import { AppWordmark } from "@/components/brand";
import {
  LEGAL_DOCS,
  LEGAL_KEYS,
  type LegalDoc,
} from "@/features/landing/constants";
import {
  landingHref,
  resolveLandingLocale,
  type LandingSearchParams,
} from "@/features/landing/lib/locale";
import { translate } from "@/lib/i18n/messages";

type Props = {
  params: Promise<{ slug: string }>;
  searchParams: LandingSearchParams;
};

function isLegalDoc(slug: string): slug is LegalDoc {
  return (LEGAL_DOCS as readonly string[]).includes(slug);
}

export async function renderMetadata({
  params,
  searchParams,
}: Props): Promise<Metadata> {
  const [{ slug }, locale] = await Promise.all([
    params,
    resolveLandingLocale(searchParams),
  ]);
  if (!isLegalDoc(slug)) return {};
  return {
    title: translate(locale, `landing.legal.${LEGAL_KEYS[slug]}_title`),
    alternates: { canonical: `/legal/${slug}` },
    openGraph: {
      url: `/legal/${slug}`,
      title: translate(locale, `landing.legal.${LEGAL_KEYS[slug]}_title`),
    },
    robots: { index: false, follow: true },
  };
}

/**
 * Legal documents linked from the marketing footer (KVKK, privacy, terms,
 * cookies). The final texts come from counsel before launch; until then the
 * page says so plainly instead of showing invented legal text.
 */
export async function renderPage({ params, searchParams }: Props) {
  const [{ slug }, locale] = await Promise.all([
    params,
    resolveLandingLocale(searchParams),
  ]);
  if (!isLegalDoc(slug)) notFound();
  const t = (key: string) => translate(locale, key);

  return (
    <div
      lang={locale}
      className="min-h-svh bg-[#0A0D0B] px-4 py-10 text-white sm:px-6"
    >
      <div className="mx-auto flex max-w-2xl flex-col gap-10">
        <Link href={landingHref(locale)} className="w-fit rounded-md">
          <AppWordmark tone="dark" className="text-2xl" />
        </Link>
        <article className="space-y-5">
          <p className="font-accent text-[11px] tracking-[0.24em] text-[#7DFF86] uppercase">
            {t("landing.footer.legal")}
          </p>
          <h1 className="font-display text-4xl font-semibold tracking-[-0.03em]">
            {t(`landing.legal.${LEGAL_KEYS[slug]}_title`)}
          </h1>
          <p className="text-base leading-relaxed text-white/70">
            {t(`landing.legal.${LEGAL_KEYS[slug]}_scope`)}
          </p>
          <p className="rounded-2xl border border-white/10 bg-white/[0.03] p-5 text-sm leading-relaxed text-white/65">
            {t("landing.legal.pending")}
          </p>
        </article>
        <Link
          href={landingHref(locale)}
          className="w-fit text-sm text-white/60 hover:text-white"
        >
          ← {t("landing.legal.back")}
        </Link>
      </div>
      <MarketingTrackingSlot locale={locale} />
    </div>
  );
}
