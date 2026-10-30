"use client";

import { GrainGradient } from "@paper-design/shaders-react";
import { motion } from "motion/react";

import { ShaderStage } from "@/components/brand/shader-stage";
import { brand } from "@/config/brand";
import { CONTACT_PATH } from "@/features/landing/constants";
import { localizedHref } from "@/features/landing/lib/locale";
import { BRAND_EASE } from "@/lib/motion";

import { LandingCta } from "./landing-cta";
import { useLanding } from "./landing-context";
import { GhostLink } from "./page-hero";

/** Closing call to action on a grain-gradient shader. */
export function FinalCta() {
  const { t, locale } = useLanding();

  return (
    <section className="relative isolate overflow-hidden px-4 py-28 sm:px-6 sm:py-40">
      <ShaderStage className="-z-10 bg-[#0A0D0B]">
        {(active) => (
          <GrainGradient
            className="size-full"
            colorBack="#0A0D0B"
            colors={[...brand.shader.grain]}
            softness={0.75}
            intensity={0.4}
            noise={0.3}
            shape="wave"
            speed={active ? 0.45 : 0}
            maxPixelCount={1600 * 900}
          />
        )}
      </ShaderStage>
      <div
        aria-hidden
        className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,#0A0D0B,transparent_30%,transparent_70%,#0A0D0B)]"
      />
      <div className="mx-auto flex max-w-5xl flex-col items-center gap-8 text-center">
        <motion.h2
          initial={{ opacity: 0, y: 30 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, margin: "-20% 0px" }}
          transition={{ duration: 1, ease: BRAND_EASE }}
          className="font-display text-[clamp(2.5rem,7vw,5.5rem)] leading-[0.98] font-semibold tracking-[-0.045em] text-balance"
        >
          {t("landing.final.title")}
        </motion.h2>
        <p className="max-w-xl text-base leading-relaxed text-white/75 sm:text-lg">
          {t("landing.final.body")}
        </p>
        <div className="flex flex-col items-center gap-3 sm:flex-row">
          <LandingCta />
          <GhostLink href={localizedHref(CONTACT_PATH, locale)}>
            {t("landing.final.contact")}
          </GhostLink>
        </div>
      </div>
    </section>
  );
}
