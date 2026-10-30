"use client";

import { useSyncExternalStore, type ReactNode } from "react";
import Link from "next/link";
import { motion } from "motion/react";
import { Bot, Inbox, Timer } from "lucide-react";

import { AppWordmark } from "@/components/brand";
import { BrandLogo } from "@/components/brand-logos";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { routes } from "@/config/routes";
import { BRAND_EASE, usePrefersReducedMotion } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { AuthBackdrop } from "./auth-backdrop";

const FEATURES = [
  { icon: Inbox, key: "auth.shell.feature_inbox" },
  { icon: Bot, key: "auth.shell.feature_ai" },
  { icon: Timer, key: "auth.shell.feature_sla" },
] as const;

const DESKTOP = "(min-width: 64rem)";

function subscribeDesktop(onChange: () => void) {
  const query = window.matchMedia(DESKTOP);
  query.addEventListener("change", onChange);
  return () => query.removeEventListener("change", onChange);
}

/** Matches Tailwind's `lg`; the server render assumes a phone. */
function useIsDesktop() {
  return useSyncExternalStore(
    subscribeDesktop,
    () => window.matchMedia(DESKTOP).matches,
    () => false,
  );
}

/** A glass "live conversation" card: what the product does, at a glance. */
function LivePreview() {
  const { t } = useLocale();
  const reducedMotion = usePrefersReducedMotion();
  return (
    <motion.div
      initial={reducedMotion ? false : { opacity: 0, y: 16 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.9, delay: 0.25, ease: BRAND_EASE }}
      className="hidden w-full max-w-md rounded-2xl border border-white/10 bg-white/[0.04] p-4 shadow-2xl shadow-black/40 backdrop-blur-xl [@media(min-height:52rem)]:block"
    >
      <div className="flex items-center justify-between text-[11px] text-white/55">
        <span className="font-accent flex items-center gap-2 tracking-[0.14em] uppercase">
          <span className="signal-dot size-1.5!" data-pulse="true" />
          {t("auth.shell.preview_live")}
        </span>
        <span className="flex items-center gap-1.5 rounded-full border border-white/10 px-2 py-0.5">
          <BrandLogo slug="whatsapp" className="size-3" decorative />
          WhatsApp
        </span>
      </div>
      <p className="mt-3 rounded-xl rounded-tl-sm bg-white/[0.07] px-3 py-2 text-sm text-white/85">
        {t("auth.shell.preview_customer")}
      </p>
      <div className="ms-8 mt-2 rounded-xl rounded-tr-sm border border-[#03FF13]/25 bg-[#03FF13]/[0.07] px-3 py-2 text-sm text-white/90">
        <span className="font-accent mb-1 flex items-center gap-1.5 text-[10px] tracking-[0.14em] text-[#7DFF86] uppercase">
          <Bot className="size-3" aria-hidden />
          {t("auth.shell.preview_ai")}
        </span>
        {t("auth.shell.preview_reply")}
      </div>
      <div className="mt-3 flex items-center gap-2 text-[11px] text-white/50">
        <Timer className="size-3.5" aria-hidden />
        {t("auth.shell.preview_sla")}
      </div>
    </motion.div>
  );
}

/**
 * Guest pages: on desktop a split layout, the animated ink mesh with the
 * product pitch on the left and the form on the page background on the
 * right. Below `lg` the form floats as a frosted card over the mesh, so those
 * screens always use the dark palette; the theme switch only shows on desktop.
 */
export function AuthShell({
  children,
  homeHref = routes.public.root,
  pitch = true,
  wide = false,
}: {
  children: ReactNode;
  /** Where the wordmark leads. */
  homeHref?: string;
  /** The product pitch beside the form; a node replaces the default one. */
  pitch?: boolean | ReactNode;
  /** A wider form column (the sign-up form). */
  wide?: boolean;
}) {
  const { t } = useLocale();
  const isDesktop = useIsDesktop();
  const reducedMotion = usePrefersReducedMotion();

  return (
    <div
      className={cn(
        "bg-background text-foreground grid min-h-svh lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]",
        !isDesktop && "dark",
      )}
    >
      <aside className="bg-ink relative hidden overflow-hidden text-white lg:flex lg:flex-col lg:justify-between lg:p-12 xl:p-14">
        <AuthBackdrop />
        <div className="from-ink/90 via-ink/30 to-ink/60 pointer-events-none absolute inset-0 bg-linear-to-t" />

        <Link
          href={homeHref}
          className="relative w-fit rounded-md focus-visible:ring-2 focus-visible:ring-[#03FF13] focus-visible:outline-none"
          aria-label={t("common.app_product")}
        >
          <AppWordmark tone="dark" className="text-2xl" />
        </Link>

        {typeof pitch !== "boolean" ? (
          <div className="relative flex max-w-lg flex-col gap-8 py-8">
            {pitch}
          </div>
        ) : pitch ? (
          <div className="relative flex max-w-lg flex-col gap-8 py-8">
            <motion.div
              initial={reducedMotion ? false : { opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.8, ease: BRAND_EASE }}
              className="space-y-4"
            >
              <p className="font-accent flex items-center gap-2 text-xs font-semibold tracking-[0.2em] text-white/60 uppercase">
                <span aria-hidden className="signal-dot" />
                {t("auth.shell.eyebrow")}
              </p>
              <h2 className="font-display text-4xl leading-[1.05] font-semibold tracking-[-0.03em] text-balance xl:text-5xl">
                {t("auth.shell.headline")}
              </h2>
              <p className="max-w-md text-base text-pretty text-white/70">
                {t("auth.shell.subline")}
              </p>
            </motion.div>
            <ul className="space-y-3">
              {FEATURES.map(({ icon: Icon, key }) => (
                <li key={key} className="flex items-center gap-3 text-white/85">
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-white/[0.06] ring-1 ring-white/12 backdrop-blur-sm">
                    <Icon className="size-4 text-[#7DFF86]" aria-hidden />
                  </span>
                  <span className="text-sm">{t(key)}</span>
                </li>
              ))}
            </ul>
            <LivePreview />
          </div>
        ) : (
          <span />
        )}

        <p className="relative text-xs text-white/45">
          {t("auth.shell.footer", { year: new Date().getFullYear() })}
        </p>
      </aside>

      <main className="relative flex min-h-svh flex-col items-center justify-center overflow-hidden p-5 sm:p-6 md:p-10">
        {isDesktop ? null : <AuthBackdrop />}
        <div className="absolute top-4 right-4 z-10 flex items-center gap-1 rounded-full bg-black/30 p-0.5 backdrop-blur lg:bg-transparent lg:backdrop-blur-none">
          <LocaleSwitch />
          {isDesktop ? <ThemeSwitch /> : null}
        </div>
        <div
          className={cn(
            "relative flex w-full flex-col gap-6",
            wide ? "max-w-md" : "max-w-sm",
          )}
        >
          <Link
            href={homeHref}
            className="self-center rounded-md lg:hidden"
            aria-label={t("common.app_product")}
          >
            <AppWordmark tone="dark" className="text-2xl" />
          </Link>
          <div className="auth-glass lg:[&_[data-slot=card]]:border-0 lg:[&_[data-slot=card]]:bg-transparent lg:[&_[data-slot=card]]:shadow-none">
            {children}
          </div>
        </div>
      </main>
    </div>
  );
}
