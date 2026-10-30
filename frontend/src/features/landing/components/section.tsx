"use client";

import { motion } from "motion/react";
import type { PointerEvent, ReactNode } from "react";

import { BRAND_EASE } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Fades and lifts its children in once, when they scroll into view. */
export function Reveal({
  children,
  delay = 0,
  className,
}: {
  children: ReactNode;
  delay?: number;
  className?: string;
}) {
  return (
    <motion.div
      initial={false}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-12% 0px" }}
      transition={{ duration: 0.9, delay, ease: BRAND_EASE }}
      className={className}
    >
      {children}
    </motion.div>
  );
}

/** Eyebrow + title + lead used at the top of each section. */
export function SectionHeading({
  eyebrow,
  title,
  lead,
  align = "start",
  className,
}: {
  eyebrow: string;
  title: string;
  lead?: string;
  align?: "start" | "center";
  className?: string;
}) {
  return (
    <Reveal
      className={cn(
        "flex max-w-3xl flex-col gap-4",
        align === "center" && "mx-auto items-center text-center",
        className,
      )}
    >
      <p className="font-accent flex items-center gap-2.5 text-[11px] font-semibold tracking-[0.26em] text-[#7DFF86] uppercase">
        <span aria-hidden className="signal-dot size-1.5!" />
        {eyebrow}
      </p>
      <h2 className="font-display text-[clamp(2rem,4.6vw,3.5rem)] leading-[1.02] font-semibold tracking-[-0.04em] text-balance">
        {title}
      </h2>
      {lead ? (
        <p className="max-w-2xl text-base leading-relaxed text-pretty text-white/65 sm:text-lg">
          {lead}
        </p>
      ) : null}
    </Reveal>
  );
}

/** Moves the `.landing-spot` highlight of a card under the pointer. */
export function trackSpot(event: PointerEvent<HTMLElement>) {
  const rect = event.currentTarget.getBoundingClientRect();
  event.currentTarget.style.setProperty(
    "--x",
    `${event.clientX - rect.left}px`,
  );
  event.currentTarget.style.setProperty("--y", `${event.clientY - rect.top}px`);
}
