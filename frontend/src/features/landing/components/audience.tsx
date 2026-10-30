"use client";

import { Briefcase, Cloud, ShoppingBag } from "lucide-react";

import { useLanding } from "./landing-context";
import { Reveal, SectionHeading } from "./section";

const SEGMENTS = [
  { key: "ecommerce", icon: ShoppingBag },
  { key: "saas", icon: Cloud },
  { key: "services", icon: Briefcase },
] as const;

/**
 * Who it is for, plus the slot for pilot customers' logos. There are no
 * customer names yet, so the logo row is an explicit, labelled placeholder
 * (no invented companies or quotes).
 */
export function Audience() {
  const { t } = useLanding();

  return (
    <section className="relative py-24 sm:py-28">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <SectionHeading
          eyebrow={t("landing.audience.eyebrow")}
          title={t("landing.audience.title")}
          align="center"
        />
        <div className="mt-12 grid gap-4 md:grid-cols-3">
          {SEGMENTS.map(({ key, icon: Icon }, i) => (
            <Reveal key={key} delay={i * 0.08}>
              <article className="flex h-full flex-col gap-3 rounded-3xl border border-white/10 bg-white/[0.02] p-6">
                <Icon className="size-5 text-[#7DFF86]" aria-hidden />
                <h3 className="font-display text-lg font-semibold tracking-tight">
                  {t(`landing.audience.${key}_title`)}
                </h3>
                <p className="text-sm leading-relaxed text-white/60">
                  {t(`landing.audience.${key}_body`)}
                </p>
              </article>
            </Reveal>
          ))}
        </div>
        <Reveal className="mt-14 flex flex-col items-center gap-5">
          <p className="font-accent text-[11px] tracking-[0.24em] text-white/45 uppercase">
            {t("landing.audience.pilot_label")}
          </p>
          <ul
            className="grid w-full max-w-4xl grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6"
            aria-label={t("landing.audience.pilot_label")}
          >
            {Array.from({ length: 6 }, (_, i) => (
              <li
                key={i}
                className="flex h-14 items-center justify-center rounded-2xl border border-dashed border-white/12 text-[11px] text-white/30"
              >
                {t("landing.audience.pilot_slot")}
              </li>
            ))}
          </ul>
          <p className="max-w-lg text-center text-sm text-white/55">
            {t("landing.audience.pilot_body")}
          </p>
        </Reveal>
      </div>
    </section>
  );
}
