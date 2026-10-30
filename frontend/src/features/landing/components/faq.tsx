"use client";

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";

import { FAQ_KEYS } from "@/features/landing/constants";

import { useLanding } from "./landing-context";
import { Reveal, SectionHeading } from "./section";

/** Frequently asked questions (an accordion; answers stay in the DOM). */
export function Faq() {
  const { t } = useLanding();

  return (
    <section
      id="faq"
      className="relative scroll-mt-20 border-t border-white/8 py-24 sm:py-32"
    >
      <div className="mx-auto grid max-w-7xl gap-12 px-4 sm:px-6 lg:grid-cols-[0.8fr_1.2fr]">
        <SectionHeading
          eyebrow={t("landing.faq.eyebrow")}
          title={t("landing.faq.title")}
          lead={t("landing.faq.lead")}
        />
        <Reveal>
          <Accordion type="single" collapsible className="w-full">
            {FAQ_KEYS.map((key) => (
              <AccordionItem key={key} value={key} className="border-white/10">
                <AccordionTrigger className="font-display py-5 text-base font-medium tracking-tight hover:no-underline sm:text-lg [&>svg]:text-white/50">
                  {t(`landing.faq.${key}_q`)}
                </AccordionTrigger>
                <AccordionContent className="pb-5 text-[15px] leading-relaxed text-white/65">
                  {t(`landing.faq.${key}_a`)}
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        </Reveal>
      </div>
    </section>
  );
}
