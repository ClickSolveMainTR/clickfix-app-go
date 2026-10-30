"use client";

import {
  ArrowRight,
  Cloud,
  GraduationCap,
  Landmark,
  Loader2,
  Palette,
  Server,
  ShoppingBag,
  Sparkles,
  Stethoscope,
  Wrench,
  type LucideIcon,
} from "lucide-react";
import { motion } from "motion/react";

import { Skeleton } from "@/components/ui/skeleton";
import { BRAND_EASE, usePrefersReducedMotion } from "@/lib/motion";
import { useLocale } from "@/providers/locale-provider";

import type { SetupTemplate } from "../../services/setup.service";

const ICONS: Record<string, LucideIcon> = {
  cloud: Cloud,
  "shopping-bag": ShoppingBag,
  palette: Palette,
  server: Server,
  landmark: Landmark,
  "graduation-cap": GraduationCap,
  stethoscope: Stethoscope,
  wrench: Wrench,
};

/** Summed counts shown on a card: what the template sets up. */
const COUNTED = ["team", "macro", "canned_reply", "kb_article"] as const;

/**
 * Use-case template gallery: one card per curated template, entering in a
 * stagger; picking one drafts a run with its plan for the preview.
 */
export function TemplateGallery({
  templates,
  loading,
  pendingKey,
  onPick,
}: {
  templates: SetupTemplate[] | undefined;
  loading: boolean;
  pendingKey: string | null;
  onPick: (template: SetupTemplate) => void;
}) {
  const { t, locale } = useLocale();
  const reduced = usePrefersReducedMotion();
  const lang = locale === "en" ? "en" : "tr";

  if (loading || !templates) {
    return (
      <div className="grid gap-3 sm:grid-cols-2">
        {Array.from({ length: 8 }, (_, i) => (
          <Skeleton key={i} className="h-40 rounded-2xl" />
        ))}
      </div>
    );
  }

  return (
    <ul className="grid gap-3 sm:grid-cols-2">
      {templates.map((tpl, i) => {
        const Icon = ICONS[tpl.icon] ?? Sparkles;
        const pending = pendingKey === tpl.key;
        return (
          <motion.li
            key={tpl.key}
            initial={reduced ? false : { opacity: 0, y: 14 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.45, ease: BRAND_EASE, delay: i * 0.05 }}
          >
            <button
              type="button"
              disabled={pendingKey !== null}
              onClick={() => onPick(tpl)}
              className="group bg-card focus-visible:ring-ring/50 relative flex h-full w-full flex-col gap-3 overflow-hidden rounded-2xl border p-5 text-left shadow-sm transition-all outline-none hover:-translate-y-0.5 hover:shadow-md focus-visible:ring-[3px] disabled:opacity-70 motion-reduce:transition-none motion-reduce:hover:translate-y-0"
            >
              <span
                aria-hidden
                className="absolute inset-x-0 top-0 h-1 opacity-70 transition-opacity group-hover:opacity-100"
                style={{ background: tpl.accent }}
              />
              <span className="flex items-center gap-3">
                <span
                  className="grid size-10 shrink-0 place-items-center rounded-xl text-white"
                  style={{ background: tpl.accent }}
                >
                  {pending ? (
                    <Loader2 className="size-5 animate-spin" aria-hidden />
                  ) : (
                    <Icon className="size-5" aria-hidden />
                  )}
                </span>
                <span className="font-display text-base leading-tight font-semibold">
                  {tpl.name[lang]}
                </span>
              </span>
              <span className="text-muted-foreground line-clamp-3 text-sm">
                {tpl.description[lang]}
              </span>
              <span className="mt-auto flex flex-wrap gap-1.5">
                {COUNTED.map((kind) =>
                  tpl.counts[kind] ? (
                    <span
                      key={kind}
                      className="bg-muted text-muted-foreground rounded-full px-2 py-0.5 text-xs"
                    >
                      {t(`onboarding.setup.count.${kind}`, {
                        count: tpl.counts[kind],
                      })}
                    </span>
                  ) : null,
                )}
              </span>
              <span className="text-foreground flex items-center gap-1 text-sm font-medium">
                {t("onboarding.setup.gallery.use")}
                <ArrowRight
                  className="size-4 transition-transform group-hover:translate-x-0.5"
                  aria-hidden
                />
              </span>
            </button>
          </motion.li>
        );
      })}
    </ul>
  );
}
