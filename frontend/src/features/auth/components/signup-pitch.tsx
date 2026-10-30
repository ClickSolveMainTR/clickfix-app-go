"use client";

import { motion } from "motion/react";
import { Building2, MailCheck, Sparkles, UserPlus } from "lucide-react";

import { BRAND_EASE, usePrefersReducedMotion } from "@/lib/motion";
import { useLocale } from "@/providers/locale-provider";

const STEPS = [
  { icon: UserPlus, key: "account" },
  { icon: MailCheck, key: "verify" },
  { icon: Building2, key: "organization" },
  { icon: Sparkles, key: "setup" },
] as const;

/** Left side of `/register` on desktop: what happens after the form (CF-143). */
export function SignupPitch({ open }: { open: boolean }) {
  const { t } = useLocale();
  const reducedMotion = usePrefersReducedMotion();
  return (
    <>
      <motion.div
        initial={reducedMotion ? false : { opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.8, ease: BRAND_EASE }}
        className="space-y-4"
      >
        <p className="font-accent flex items-center gap-2 text-xs font-semibold tracking-[0.2em] text-white/60 uppercase">
          <span aria-hidden className="signal-dot" />
          {t(open ? "signup.pitch.eyebrow" : "signup.pitch.eyebrow_invite")}
        </p>
        <h2 className="font-display text-4xl leading-[1.05] font-semibold tracking-[-0.03em] text-balance xl:text-5xl">
          {t("signup.pitch.headline")}
        </h2>
        <p className="max-w-md text-base text-pretty text-white/70">
          {t("signup.pitch.subline")}
        </p>
      </motion.div>
      {open ? (
        <ol className="space-y-4">
          {STEPS.map(({ icon: Icon, key }, index) => (
            <motion.li
              key={key}
              initial={reducedMotion ? false : { opacity: 0, x: -8 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{
                duration: 0.6,
                delay: 0.15 + index * 0.08,
                ease: BRAND_EASE,
              }}
              className="flex items-start gap-3 text-white/85"
            >
              <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-white/[0.06] ring-1 ring-white/12 backdrop-blur-sm">
                <Icon className="size-4 text-[#7DFF86]" aria-hidden />
              </span>
              <span className="space-y-0.5">
                <span className="block text-sm font-medium">
                  {t(`signup.pitch.step.${key}.title`)}
                </span>
                <span className="block text-sm text-white/60">
                  {t(`signup.pitch.step.${key}.body`)}
                </span>
              </span>
            </motion.li>
          ))}
        </ol>
      ) : null}
    </>
  );
}
