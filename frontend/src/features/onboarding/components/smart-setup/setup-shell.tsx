"use client";

import { Check, type LucideIcon } from "lucide-react";
import Link from "next/link";
import type { CSSProperties, ReactNode } from "react";

import { AppWordmark } from "@/components/brand";
import { brand as appBrand } from "@/config/brand";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { routes } from "@/config/routes";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type ShellStep = { key: string; icon: LucideIcon };

/** ClickFix green until the plan brings the customer's own colour. */
const defaultAccent = appBrand.colors.signalText;

/**
 * Two-column frame of the smart set-up (ported from otopoly-go
 * `onboarding-wizard.tsx` OnboardingAside): a dark aside with the steps
 * and perks, the active step on the right. `brand` (the plan's colour)
 * tints the aside glow and the accents as soon as it is known.
 */
export function SetupShell({
  steps,
  current,
  brand,
  children,
}: {
  steps: readonly ShellStep[];
  current: number;
  brand?: string | null;
  children: ReactNode;
}) {
  const { t } = useLocale();
  const accent = brand || defaultAccent;
  return (
    <div
      className="bg-background grid min-h-svh lg:grid-cols-[minmax(0,24rem)_1fr]"
      style={{ "--setup-accent": accent } as CSSProperties}
    >
      <aside className="relative isolate hidden overflow-hidden bg-[#0a0d0b] p-10 text-white lg:flex lg:flex-col">
        <div
          aria-hidden
          className="absolute inset-0 -z-10 transition-[background] duration-1000 motion-reduce:transition-none"
          style={{
            background: `radial-gradient(120% 70% at 100% 0%, color-mix(in oklab, ${accent} 60%, transparent), transparent 60%), radial-gradient(90% 60% at 0% 100%, color-mix(in oklab, ${accent} 22%, transparent), transparent 60%)`,
          }}
        />
        <Link href={routes.cms.home} aria-label={t("common.app_product")}>
          <AppWordmark tone="dark" className="text-2xl" />
        </Link>
        <div className="mt-14">
          <h2 className="font-display text-3xl leading-tight font-semibold tracking-tight text-balance">
            {t("onboarding.setup.aside_title")}
          </h2>
          <p className="mt-3 text-sm leading-relaxed text-white/70">
            {t("onboarding.setup.aside_body")}
          </p>
        </div>
        <ol className="mt-10 space-y-1">
          {steps.map((item, index) => {
            const done = index < current;
            const active = index === current;
            const Icon = item.icon;
            return (
              <li
                key={item.key}
                aria-current={active ? "step" : undefined}
                className={cn(
                  "flex items-center gap-3 rounded-xl px-3 py-2.5 transition-colors",
                  active && "bg-white/10",
                )}
              >
                <span
                  className={cn(
                    "grid size-9 shrink-0 place-items-center rounded-lg border transition-colors",
                    done
                      ? "border-transparent text-white"
                      : active
                        ? "border-white/40 text-white"
                        : "border-white/15 text-white/50",
                  )}
                  style={done ? { background: accent } : undefined}
                >
                  {done ? (
                    <Check className="size-4" aria-hidden />
                  ) : (
                    <Icon className="size-4" aria-hidden />
                  )}
                </span>
                <div className={cn(!active && !done && "text-white/55")}>
                  <p className="text-sm font-medium">
                    {t(`onboarding.setup.steps.${item.key}`)}
                  </p>
                  <p className="text-xs text-white/55">
                    {t(`onboarding.setup.steps.${item.key}_hint`)}
                  </p>
                </div>
              </li>
            );
          })}
        </ol>
        <ul className="mt-auto space-y-2 pt-10 text-sm text-white/70">
          {(["review", "undo", "ai"] as const).map((key) => (
            <li key={key} className="flex items-center gap-2">
              <Check className="size-4" style={{ color: accent }} aria-hidden />
              {t(`onboarding.setup.perks.${key}`)}
            </li>
          ))}
        </ul>
      </aside>

      <main className="relative flex min-w-0 flex-col">
        <div className="flex items-center justify-between gap-2 px-4 pt-4 sm:px-8">
          <Link
            href={routes.cms.home}
            className="lg:invisible"
            aria-label={t("common.app_product")}
          >
            <AppWordmark className="text-lg" />
          </Link>
          <div className="flex items-center gap-1">
            <LocaleSwitch />
            <ThemeSwitch />
          </div>
        </div>
        <div className="mx-auto mt-4 flex w-full max-w-3xl gap-1.5 px-4 sm:px-8 lg:hidden">
          {steps.map((item, index) => (
            <span
              key={item.key}
              aria-hidden
              className={cn(
                "h-1.5 flex-1 rounded-full transition-colors",
                index > current && "bg-muted",
              )}
              style={index <= current ? { background: accent } : undefined}
            />
          ))}
        </div>
        <div className="mx-auto flex w-full max-w-3xl flex-1 flex-col px-4 py-8 sm:px-8">
          {children}
        </div>
      </main>
    </div>
  );
}
