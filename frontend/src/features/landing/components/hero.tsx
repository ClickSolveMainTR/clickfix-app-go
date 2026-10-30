"use client";

import { MeshGradient } from "@paper-design/shaders-react";
import { ArrowDown } from "lucide-react";
import { motion, useScroll, useTransform } from "motion/react";
import { useRef, type CSSProperties, type ReactNode } from "react";

import { ShaderStage } from "@/components/brand/shader-stage";
import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { InboxPreview } from "./inbox-preview";
import { LandingCta } from "./landing-cta";
import { useLanding } from "./landing-context";

function RevealLine({
  children,
  delay,
  className,
}: {
  children: ReactNode;
  delay: number;
  className?: string;
}) {
  return (
    <span className="block overflow-hidden pb-[0.1em]">
      <span
        className={cn("landing-line", className ?? "block")}
        style={{ "--d": `${delay}s` } as CSSProperties}
      >
        {children}
      </span>
    </span>
  );
}

/**
 * First screen: the ink + signal mesh shader, the headline revealed line by
 * line (text is in the DOM from the first byte, so LCP is the heading, not
 * the canvas) and the inbox preview, which drifts up as the page scrolls.
 */
export function Hero() {
  const { t } = useLanding();
  const ref = useRef<HTMLElement>(null);
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start start", "end start"],
  });
  const previewY = useTransform(scrollYProgress, [0, 1], [0, -110]);
  const fade = useTransform(scrollYProgress, [0, 0.9], [1, 0.2]);

  return (
    <section
      ref={ref}
      className="relative isolate flex min-h-svh flex-col overflow-hidden pt-16"
    >
      <motion.div style={{ opacity: fade }} className="absolute inset-0 -z-10">
        <ShaderStage className="bg-[#0A0D0B]">
          {(active) => (
            <MeshGradient
              className="size-full"
              colors={[...brand.shader.mesh]}
              distortion={0.85}
              swirl={0.5}
              grainMixer={0.2}
              grainOverlay={0.08}
              speed={active ? 0.25 : 0}
              maxPixelCount={1600 * 900}
            />
          )}
        </ShaderStage>
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_25%_45%,transparent_0%,rgb(10_13_11/0.55)_55%,#0A0D0B_100%)]" />
        <div className="absolute inset-x-0 bottom-0 h-40 bg-linear-to-b from-transparent to-[#0A0D0B]" />
      </motion.div>

      <div className="mx-auto grid w-full max-w-7xl flex-1 items-center gap-14 px-4 pt-10 pb-24 sm:px-6 lg:grid-cols-[1.05fr_1fr] lg:gap-12 lg:pt-4">
        <div className="flex flex-col gap-7">
          <p
            style={{ "--d": "0.1s" } as CSSProperties}
            className="landing-fade font-accent flex items-center gap-3 text-[11px] font-semibold tracking-[0.26em] text-[#7DFF86] uppercase"
          >
            <span aria-hidden className="h-px w-8 bg-[#03FF13]" />
            {t("landing.hero.eyebrow")}
          </p>
          <h1 className="font-display text-[clamp(2.5rem,5.8vw,4.75rem)] leading-[0.95] font-semibold tracking-[-0.045em] text-balance">
            <RevealLine delay={0.15}>{t("landing.hero.title_1")}</RevealLine>
            <RevealLine delay={0.27}>{t("landing.hero.title_2")}</RevealLine>
            <RevealLine
              delay={0.39}
              className="block bg-linear-to-r from-[#03FF13] to-[#B8FFBE] bg-clip-text text-transparent"
            >
              {t("landing.hero.title_3")}
            </RevealLine>
          </h1>
          <p
            style={{ "--d": "0.3s" } as CSSProperties}
            className="landing-rise max-w-xl text-base leading-relaxed text-white/70 sm:text-lg"
          >
            {t("landing.hero.body")}
          </p>
          <div
            style={{ "--d": "0.4s" } as CSSProperties}
            className="landing-rise flex flex-col gap-5 sm:flex-row sm:items-start"
          >
            <LandingCta showHint />
            <a
              href="#product"
              className="group inline-flex h-12 items-center gap-2 self-start rounded-full border border-white/20 px-5 text-sm font-medium text-white/85 transition-colors hover:border-white/40 hover:text-white"
            >
              {t("landing.cta.see_how")}
              <ArrowDown
                aria-hidden
                className="size-4 transition-transform group-hover:translate-y-0.5"
              />
            </a>
          </div>
        </div>

        <motion.div style={{ y: previewY }}>
          <div
            style={{ "--d": "0.35s" } as CSSProperties}
            className="landing-rise"
          >
            <InboxPreview className="mx-auto w-full max-w-xl lg:max-w-none" />
          </div>
        </motion.div>
      </div>

      <div className="pointer-events-none absolute bottom-6 left-1/2 hidden -translate-x-1/2 flex-col items-center gap-2 text-white/40 lg:flex">
        <span className="font-mono text-[10px] tracking-[0.3em] uppercase">
          {t("landing.hero.scroll")}
        </span>
        <span className="relative h-10 w-px overflow-hidden bg-white/15">
          <span className="landing-scan absolute inset-x-0 h-1/2 bg-white/70" />
        </span>
      </div>
    </section>
  );
}
