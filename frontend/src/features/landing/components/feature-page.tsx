"use client";

import { ArrowRight, BookOpenText, Check } from "lucide-react";
import Link from "next/link";
import type { CSSProperties } from "react";

import { DOCS_PATH, FEATURES_PATH } from "@/features/landing/constants";
import { featureCopy } from "@/features/landing/content/features";
import {
  FEATURE_AREAS,
  type FeatureSlug,
} from "@/features/landing/content/areas";
import { landingHref, localizedHref } from "@/features/landing/lib/locale";

import { FinalCta } from "./final-cta";
import { LandingCta } from "./landing-cta";
import { useLanding } from "./landing-context";
import { ModulesGrid } from "./modules-grid";
import { GhostLink, PageHero } from "./page-hero";
import { Reveal, SectionHeading, trackSpot } from "./section";

/** Hero visual: the area icon on a signal halo with its key terms around. */
function AreaVisual({ slug }: { slug: FeatureSlug }) {
  const { locale } = useLanding();
  const area = FEATURE_AREAS.find((a) => a.slug === slug)!;
  const Icon = area.icon;
  const chips = featureCopy(locale)[slug].chips;

  return (
    <div
      aria-hidden
      className="relative mx-auto flex aspect-[5/4] w-full max-w-lg items-center justify-center overflow-hidden rounded-[2rem] border border-white/10 bg-[#0F1310]/80 p-6 backdrop-blur"
    >
      <div className="landing-gridlines absolute inset-0" />
      <div className="absolute size-64 rounded-full bg-[#03FF13]/10 blur-3xl" />
      <div className="relative flex flex-col items-center gap-7">
        <span className="flex size-20 items-center justify-center rounded-3xl bg-[#03FF13] text-black shadow-[0_0_0_1px_rgb(3_255_19/0.5),0_20px_60px_-10px_rgb(3_255_19/0.55)]">
          <Icon className="size-9" />
        </span>
        <ul className="flex max-w-md flex-wrap justify-center gap-2">
          {chips.map((chip, i) => (
            <li
              key={chip}
              style={{ "--d": `${0.3 + i * 0.05}s` } as CSSProperties}
              className="landing-fade rounded-full border border-white/12 bg-white/[0.05] px-3 py-1.5 text-xs font-medium text-white/80"
            >
              {chip}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

/** `/features/<slug>`: one product area in depth. */
export function FeaturePage({
  slug,
  docs,
}: {
  slug: FeatureSlug;
  /** Related guides that exist in this language. */
  docs: { slug: string; title: string; description: string }[];
}) {
  const { t, locale } = useLanding();
  const copy = featureCopy(locale)[slug];

  return (
    <>
      <PageHero
        crumbs={[
          { href: landingHref(locale), label: t("landing.nav.home_short") },
          {
            href: localizedHref(FEATURES_PATH, locale),
            label: t("landing.nav.features"),
          },
          { label: copy.name },
        ]}
        eyebrow={copy.eyebrow}
        title={copy.title}
        lead={copy.lead}
        actions={
          <>
            <LandingCta />
            {docs[0] ? (
              <GhostLink
                href={localizedHref(`${DOCS_PATH}/${docs[0].slug}`, locale)}
              >
                <BookOpenText className="size-4" aria-hidden />
                {t("landing.features.read_docs")}
              </GhostLink>
            ) : null}
          </>
        }
        aside={<AreaVisual slug={slug} />}
      />

      {copy.highlights.length > 0 ? (
        <section className="relative py-20 sm:py-28">
          <div className="mx-auto max-w-7xl px-4 sm:px-6">
            <ol className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
              {copy.highlights.map((item, i) => (
                <li key={item.title}>
                  <Reveal delay={(i % 3) * 0.06} className="h-full">
                    <article
                      onPointerMove={trackSpot}
                      className="landing-spot flex h-full flex-col gap-4 rounded-3xl border border-white/10 bg-[#0F1310] p-6 sm:p-7"
                    >
                      <span className="font-accent text-xs tracking-[0.2em] text-[#7DFF86]">
                        {String(i + 1).padStart(2, "0")}
                      </span>
                      <h2 className="font-display text-xl font-semibold tracking-tight">
                        {item.title}
                      </h2>
                      <p className="text-[15px] leading-relaxed text-pretty text-white/60">
                        {item.body}
                      </p>
                    </article>
                  </Reveal>
                </li>
              ))}
            </ol>
          </div>
        </section>
      ) : null}

      {copy.capabilities.length > 0 ? (
        <section className="border-t border-white/8 py-20 sm:py-28">
          <div className="mx-auto grid max-w-7xl gap-12 px-4 sm:px-6 lg:grid-cols-[0.8fr_1.2fr]">
            <SectionHeading
              eyebrow={t("landing.features.included_eyebrow")}
              title={t("landing.features.included_title")}
            />
            <Reveal>
              <ul className="grid gap-x-8 gap-y-1 sm:grid-cols-2">
                {copy.capabilities.map((item) => (
                  <li
                    key={item}
                    className="flex items-start gap-3 border-b border-white/8 py-3.5 text-[15px] text-white/80"
                  >
                    <Check
                      className="mt-0.5 size-4 shrink-0 text-[#03FF13]"
                      aria-hidden
                    />
                    {item}
                  </li>
                ))}
              </ul>
            </Reveal>
          </div>
        </section>
      ) : null}

      {docs.length > 0 ? (
        <section className="border-t border-white/8 py-20 sm:py-24">
          <div className="mx-auto max-w-7xl px-4 sm:px-6">
            <SectionHeading
              eyebrow={t("landing.nav.docs")}
              title={t("landing.features.docs_title")}
            />
            <ul className="mt-10 grid gap-3 md:grid-cols-2 lg:grid-cols-3">
              {docs.map((doc) => (
                <li key={doc.slug}>
                  <Link
                    href={localizedHref(`${DOCS_PATH}/${doc.slug}`, locale)}
                    className="group flex h-full flex-col gap-2 rounded-2xl border border-white/10 bg-white/[0.02] p-5 transition-colors hover:border-white/25"
                  >
                    <span className="flex items-center justify-between gap-3 font-medium">
                      {doc.title}
                      <ArrowRight
                        aria-hidden
                        className="size-4 shrink-0 text-white/35 transition-transform group-hover:translate-x-0.5"
                      />
                    </span>
                    <span className="text-sm leading-relaxed text-white/55">
                      {doc.description}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        </section>
      ) : null}

      <section className="border-t border-white/8 py-20 sm:py-24">
        <div className="mx-auto max-w-7xl px-4 sm:px-6">
          <SectionHeading
            eyebrow={t("landing.features.more_eyebrow")}
            title={t("landing.features.more_title")}
          />
          <ModulesGrid exclude={slug} className="mt-10" />
        </div>
      </section>

      <FinalCta />
    </>
  );
}
