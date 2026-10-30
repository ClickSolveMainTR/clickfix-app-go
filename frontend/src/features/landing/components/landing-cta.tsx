"use client";

import { ArrowUpRight, CalendarClock } from "lucide-react";
import {
  CONTACT_PATH,
  contactEmail,
  SIGNUP_PATH,
} from "@/features/landing/constants";
import { cn } from "@/lib/utils";

import { useLanding } from "./landing-context";

/**
 * Main call to action, signal green with ink text: "start free" while self
 * sign-up is on, otherwise "book a demo" (mail to sales when configured,
 * else the sign-up page).
 */
export function LandingCta({
  className,
  size = "lg",
  showHint = false,
  href,
  label,
}: {
  className?: string;
  size?: "sm" | "lg";
  showHint?: boolean;
  /** Sign-up link override, e.g. with the chosen plan (`?plan=`). */
  href?: string;
  /** Label override for the sign-up variant. */
  label?: string;
}) {
  const { t, signupOpen } = useLanding();
  const demoHref = contactEmail
    ? `mailto:${contactEmail}?subject=${encodeURIComponent(t("landing.cta.demo_subject"))}`
    : CONTACT_PATH;
  const classes = cn(
    "group inline-flex items-center justify-center gap-2 rounded-full bg-[#03FF13] font-semibold text-black shadow-[0_0_0_1px_rgb(3_255_19/0.4),0_10px_40px_-10px_rgb(3_255_19/0.6)] transition-[transform,background-color,box-shadow] hover:bg-[#3DFF4B] active:scale-[0.98]",
    size === "lg" ? "h-12 px-6 text-base" : "h-9 px-4 text-sm",
    className,
  );

  // A full page load, not a client navigation: marketing tags must not keep
  // running in the app shell (CF-144).
  const button = signupOpen ? (
    <a href={href ?? SIGNUP_PATH} className={classes}>
      {label ?? t("landing.cta.signup")}
      <ArrowUpRight
        aria-hidden
        className="size-4 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5"
      />
    </a>
  ) : (
    <a href={demoHref} className={classes}>
      {t("landing.cta.demo")}
      <CalendarClock aria-hidden className="size-4" />
    </a>
  );

  if (!showHint) return button;
  return (
    <div className="flex flex-col items-start gap-2">
      {button}
      <p className="max-w-xs text-xs leading-relaxed text-white/55">
        {signupOpen ? t("landing.cta.signup_hint") : t("landing.cta.demo_hint")}
      </p>
    </div>
  );
}
