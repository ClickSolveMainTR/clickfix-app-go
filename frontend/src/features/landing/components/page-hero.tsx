"use client";

import { MeshGradient } from "@paper-design/shaders-react";
import Link from "next/link";
import type { CSSProperties, ReactNode } from "react";

import { ShaderStage } from "@/components/brand/shader-stage";
import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { useLanding } from "./landing-context";

/** CSS entrance (`landing.css`): runs before hydration, so LCP is not gated on JS. */
const rise = (delay: number) => ({
  className: "landing-rise",
  style: { "--d": `${delay}s` } as CSSProperties,
});

/**
 * Top of an inner marketing page: the auth / home mesh shader at low
 * intensity, a breadcrumb trail, the heading block and an optional visual on
 * the right. Text is server-rendered; only opacity and position animate, so
 * there is no layout shift.
 */
export function PageHero({
  crumbs,
  eyebrow,
  title,
  lead,
  actions,
  aside,
  compact = false,
}: {
  crumbs: { href?: string; label: string }[];
  eyebrow: ReactNode;
  title: string;
  lead: string;
  actions?: ReactNode;
  aside?: ReactNode;
  compact?: boolean;
}) {
  const { t } = useLanding();
  return (
    <section className="relative isolate overflow-hidden pt-16">
      <div aria-hidden className="absolute inset-0 -z-10">
        <ShaderStage className="bg-[#0A0D0B] opacity-70">
          {(active) => (
            <MeshGradient
              className="size-full"
              colors={[...brand.shader.mesh]}
              distortion={0.7}
              swirl={0.35}
              grainMixer={0.2}
              grainOverlay={0.08}
              speed={active ? 0.18 : 0}
              maxPixelCount={1280 * 720}
            />
          )}
        </ShaderStage>
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_20%_40%,transparent_0%,rgb(10_13_11/0.6)_55%,#0A0D0B_100%)]" />
        <div className="absolute inset-x-0 bottom-0 h-32 bg-linear-to-b from-transparent to-[#0A0D0B]" />
      </div>

      <div
        className={cn(
          "mx-auto grid w-full max-w-7xl items-center gap-12 px-4 sm:px-6",
          compact ? "pt-12 pb-14 sm:pt-16" : "pt-14 pb-20 sm:pt-20 sm:pb-24",
          aside && "lg:grid-cols-[1.1fr_1fr]",
        )}
      >
        <div className="flex flex-col gap-6">
          <nav aria-label={t("landing.nav.breadcrumb")}>
            <ol className="flex flex-wrap items-center gap-1.5 text-xs text-white/45">
              {crumbs.map((crumb, i) => (
                <li key={crumb.label} className="flex items-center gap-1.5">
                  {i > 0 ? <span aria-hidden>/</span> : null}
                  {crumb.href ? (
                    <Link
                      href={crumb.href}
                      className="transition-colors hover:text-white"
                    >
                      {crumb.label}
                    </Link>
                  ) : (
                    <span aria-current="page" className="text-white/70">
                      {crumb.label}
                    </span>
                  )}
                </li>
              ))}
            </ol>
          </nav>
          <p
            style={rise(0.05).style}
            className="landing-rise font-accent flex items-center gap-3 text-[11px] font-semibold tracking-[0.26em] text-[#7DFF86] uppercase"
          >
            <span aria-hidden className="h-px w-8 bg-[#03FF13]" />
            {eyebrow}
          </p>
          <h1
            style={rise(0.12).style}
            className="landing-rise font-display text-[clamp(2.25rem,5vw,4rem)] leading-[0.98] font-semibold tracking-[-0.045em] text-balance"
          >
            {title}
          </h1>
          <p
            style={rise(0.22).style}
            className="landing-rise max-w-2xl text-base leading-relaxed text-pretty text-white/70 sm:text-lg"
          >
            {lead}
          </p>
          {actions ? (
            <div
              style={rise(0.32).style}
              className="landing-rise flex flex-col gap-3 sm:flex-row sm:items-center"
            >
              {actions}
            </div>
          ) : null}
        </div>
        {aside ? <div {...rise(0.3)}>{aside}</div> : null}
      </div>
    </section>
  );
}

/** Secondary pill link next to the main CTA. */
export function GhostLink({
  href,
  children,
}: {
  href: string;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className="inline-flex h-12 items-center gap-2 self-start rounded-full border border-white/20 px-5 text-sm font-medium text-white/85 transition-colors hover:border-white/40 hover:text-white"
    >
      {children}
    </Link>
  );
}
