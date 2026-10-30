"use client";

import { Building2, Code2, Plug, UsersRound } from "lucide-react";

import { useLanding } from "./landing-context";
import { Reveal, SectionHeading } from "./section";

const STEPS = [
  { key: "org", icon: Building2 },
  { key: "channel", icon: Plug },
  { key: "team", icon: UsersRound },
  { key: "widget", icon: Code2 },
] as const;

/** "First message in ten minutes": the four onboarding steps. */
export function HowItWorks() {
  const { t } = useLanding();

  return (
    <section
      id="how-it-works"
      className="relative scroll-mt-20 border-t border-white/8 py-24 sm:py-32"
    >
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <SectionHeading
          eyebrow={t("landing.how.eyebrow")}
          title={t("landing.how.title")}
          lead={t("landing.how.lead")}
        />
        <div className="relative mt-14">
          <span
            aria-hidden
            className="absolute top-7 right-8 left-8 hidden h-px bg-linear-to-r from-[#03FF13]/60 via-white/15 to-transparent lg:block"
          />
          <ol className="relative grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {STEPS.map(({ key, icon: Icon }, i) => (
              <li key={key} className="h-full">
                <Reveal delay={i * 0.08} className="h-full">
                  <div className="relative flex h-full flex-col gap-4 rounded-3xl border border-white/10 bg-[#0F1310] p-6">
                    <span className="flex items-center gap-3">
                      <span className="font-accent flex size-14 items-center justify-center rounded-2xl bg-[#0A0D0B] text-lg font-semibold text-[#7DFF86] ring-1 ring-white/12">
                        0{i + 1}
                      </span>
                      <Icon className="size-5 text-white/40" aria-hidden />
                    </span>
                    <h3 className="font-display text-lg font-semibold tracking-tight">
                      {t(`landing.how.${key}_title`)}
                    </h3>
                    <p className="text-sm leading-relaxed text-white/60">
                      {t(`landing.how.${key}_body`)}
                    </p>
                  </div>
                </Reveal>
              </li>
            ))}
          </ol>
        </div>
      </div>
    </section>
  );
}
