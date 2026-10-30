"use client";

import { ArrowRight, Scale, ShieldCheck } from "lucide-react";
import Link from "next/link";

import {
  COMPARE_PATH,
  FEATURES_PATH,
  SECURITY_PATH,
} from "@/features/landing/constants";
import { localizedHref } from "@/features/landing/lib/locale";

import { useLanding } from "./landing-context";
import { ModulesGrid } from "./modules-grid";
import { Reveal, SectionHeading, trackSpot } from "./section";

/** Home: every product area, each linking to its page. */
export function Platform() {
  const { t, locale } = useLanding();

  return (
    <section
      id="platform"
      className="relative scroll-mt-20 border-t border-white/8 py-24 sm:py-32"
    >
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <div className="flex flex-col justify-between gap-6 lg:flex-row lg:items-end">
          <SectionHeading
            eyebrow={t("landing.platform.eyebrow")}
            title={t("landing.platform.title")}
            lead={t("landing.platform.lead")}
          />
          <Reveal>
            <Link
              href={localizedHref(FEATURES_PATH, locale)}
              className="group inline-flex items-center gap-2 text-sm font-medium text-[#7DFF86] hover:text-white"
            >
              {t("landing.platform.all")}
              <ArrowRight
                aria-hidden
                className="size-4 transition-transform group-hover:translate-x-0.5"
              />
            </Link>
          </Reveal>
        </div>
        <ModulesGrid className="mt-14" />
      </div>
    </section>
  );
}

/** Home: security / KVKK and the comparison, side by side. */
export function TrustBand() {
  const { t, locale } = useLanding();
  const cards = [
    { key: "security", icon: ShieldCheck, path: SECURITY_PATH },
    { key: "compare", icon: Scale, path: COMPARE_PATH },
  ] as const;

  return (
    <section className="border-t border-white/8 py-20 sm:py-24">
      <ul className="mx-auto grid max-w-7xl gap-4 px-4 sm:px-6 md:grid-cols-2">
        {cards.map(({ key, icon: Icon, path }, i) => (
          <li key={key}>
            <Reveal delay={i * 0.08} className="h-full">
              <Link
                href={localizedHref(path, locale)}
                onPointerMove={trackSpot}
                className="landing-spot group flex h-full flex-col gap-6 rounded-3xl border border-white/10 bg-[#0F1310] p-7 transition-colors hover:border-white/25 sm:p-9"
              >
                <Icon className="size-7 text-[#7DFF86]" aria-hidden />
                <span className="space-y-3">
                  <span className="font-display block text-2xl font-semibold tracking-tight sm:text-3xl">
                    {t(`landing.trust.${key}_title`)}
                  </span>
                  <span className="block max-w-md text-[15px] leading-relaxed text-white/60">
                    {t(`landing.trust.${key}_body`)}
                  </span>
                </span>
                <span className="mt-auto inline-flex items-center gap-2 text-sm font-medium text-[#7DFF86]">
                  {t(`landing.trust.${key}_action`)}
                  <ArrowRight
                    aria-hidden
                    className="size-4 transition-transform group-hover:translate-x-0.5"
                  />
                </span>
              </Link>
            </Reveal>
          </li>
        ))}
      </ul>
    </section>
  );
}
