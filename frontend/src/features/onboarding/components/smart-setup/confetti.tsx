"use client";

import { motion } from "motion/react";
import { useMemo } from "react";

import { usePrefersReducedMotion } from "@/lib/motion";

const PIECES = 42;

/**
 * A short, subtle confetti burst (DOM + motion, no dependency). Renders
 * nothing for visitors who prefer reduced motion.
 */
export function Confetti({ colors }: { colors: string[] }) {
  const reduced = usePrefersReducedMotion();
  const pieces = useMemo(
    () =>
      Array.from({ length: PIECES }, (_, i) => {
        // Deterministic spread (no Math.random during render).
        const angle = (i / PIECES) * Math.PI * 2;
        const spread = 140 + ((i * 37) % 120);
        return {
          x: Math.cos(angle) * spread,
          y: Math.sin(angle) * spread * 0.6 - 120,
          rotate: ((i * 53) % 360) - 180,
          delay: (i % 7) * 0.02,
          color: colors[i % colors.length],
          round: i % 3 === 0,
        };
      }),
    [colors],
  );
  if (reduced) return null;
  return (
    <div
      aria-hidden
      className="pointer-events-none absolute inset-x-0 top-24 flex justify-center"
    >
      {pieces.map((p, i) => (
        <motion.span
          key={i}
          className={
            p.round
              ? "absolute size-2 rounded-full"
              : "absolute h-1.5 w-3 rounded-sm"
          }
          style={{ background: p.color }}
          initial={{ x: 0, y: 0, opacity: 1, rotate: 0, scale: 0.6 }}
          animate={{
            x: p.x,
            y: [0, p.y, p.y + 260],
            opacity: [1, 1, 0],
            rotate: p.rotate * 3,
            scale: 1,
          }}
          transition={{ duration: 1.9, delay: p.delay, ease: "easeOut" }}
        />
      ))}
    </div>
  );
}
