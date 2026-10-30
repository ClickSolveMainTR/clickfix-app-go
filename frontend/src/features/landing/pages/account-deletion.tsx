import type { Metadata } from "next";

import { MarketingTrackingSlot } from "@/features/landing/components/tracking/marketing-tracking-slot";
import Link from "next/link";

import { AppWordmark } from "@/components/brand";
import {
  DATA_DELETION_PATH,
  DATA_DELETION_SECTIONS,
} from "@/features/landing/constants";
import {
  landingHref,
  localizedHref,
  resolveLandingLocale,
  type LandingSearchParams,
} from "@/features/landing/lib/locale";
import { translate } from "@/lib/i18n/messages";

type Props = { searchParams: LandingSearchParams };

const SUPPORT_EMAIL = "destek@getclickfix.com";

export async function renderMetadata({
  searchParams,
}: Props): Promise<Metadata> {
  const locale = await resolveLandingLocale(searchParams);
  return {
    title: translate(locale, "landing.deletion.title"),
    description: translate(locale, "landing.deletion.intro"),
    alternates: {
      canonical: localizedHref(DATA_DELETION_PATH, locale),
      languages: {
        tr: DATA_DELETION_PATH,
        en: localizedHref(DATA_DELETION_PATH, "en"),
      },
    },
    robots: { index: true, follow: true },
  };
}

/**
 * Public data deletion instructions (CF-135), the "data deletion
 * instructions URL" of the Meta app review. The text describes what the
 * code does: self-service account deactivation (auth `DeactivateAccount`),
 * the KVKK export/erasure and retention tools (CF-127, `modules/compliance`)
 * and channel removal (`channels` `Delete`: credentials wiped, webhooks
 * ignored). Same layout as `/legal/[slug]`.
 */
export async function renderPage({ searchParams }: Props) {
  const locale = await resolveLandingLocale(searchParams);
  const t = (key: string) => translate(locale, key);
  const mail = (
    <a
      href={`mailto:${SUPPORT_EMAIL}`}
      className="text-[#7DFF86] underline-offset-4 hover:underline"
    >
      {SUPPORT_EMAIL}
    </a>
  );

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
            {t("landing.deletion.title")}
          </h1>
          <p className="text-base leading-relaxed text-white/70">
            {t("landing.deletion.intro")}
          </p>
          <p className="text-xs text-white/45">
            {t("landing.deletion.updated")}
          </p>

          {DATA_DELETION_SECTIONS.map(({ id, items }) => (
            <section
              key={id}
              aria-labelledby={`deletion-${id}`}
              className="space-y-3 pt-4"
            >
              <h2
                id={`deletion-${id}`}
                className="font-display text-xl font-semibold tracking-[-0.02em]"
              >
                {t(`landing.deletion.${id}.title`)}
              </h2>
              <ul className="list-disc space-y-2 pl-5 text-sm leading-relaxed text-white/70 marker:text-white/30">
                {Array.from({ length: items }, (_, i) => (
                  <li key={i}>{t(`landing.deletion.${id}.item_${i + 1}`)}</li>
                ))}
              </ul>
            </section>
          ))}

          <div className="mt-4 space-y-1 rounded-2xl border border-white/10 bg-white/[0.03] p-5 text-sm leading-relaxed text-white/65">
            <p className="font-medium text-white">
              {t("landing.deletion.contact.title")}
            </p>
            <p>
              {t("landing.deletion.contact.body")} {mail}
            </p>
          </div>
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
